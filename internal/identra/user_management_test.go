package identra

import (
	"context"
	"testing"
	"time"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"github.com/slhmy/identra/internal/serviceaccount"
	"google.golang.org/grpc/codes"
)

func TestUserManagementFlow(t *testing.T) {
	ctx := context.Background()
	accounts := newMemoryServiceAccountStore()
	admin, err := serviceaccount.Bootstrap(ctx, accounts, serviceaccount.BootstrapRequest{
		Name: "user-admin", Scopes: []string{ScopeUsersManage},
	})
	if err != nil {
		t.Fatalf("bootstrap user admin: %v", err)
	}
	tokenConfig := newTestTokenConfig(t)
	token, err := security.NewServiceToken(admin.ID, admin.Scopes, tokenConfig)
	if err != nil {
		t.Fatalf("issue service token: %v", err)
	}
	store := newMockUserStore()
	audits := &memoryAuditStore{}
	svc := &Service{userStore: store, serviceAccountStore: accounts, tokenCfg: tokenConfig, auditStore: audits}
	adminCtx := serviceTokenContext(token.Value)

	created, err := svc.CreatePasswordUser(adminCtx, &identra_v1_pb.CreatePasswordUserRequest{
		Email: "managed@example.com", Password: "initial-password",
	})
	if err != nil {
		t.Fatalf("create password user: %v", err)
	}
	if created.GetUser().GetId() == "" || !created.GetUser().GetPasswordLoginEnabled() {
		t.Fatalf("unexpected created user: %+v", created.GetUser())
	}
	stored, err := store.GetByID(ctx, created.GetUser().GetId())
	if err != nil || stored.HashedPassword == nil {
		t.Fatalf("stored user=%+v error=%v", stored, err)
	}
	if valid, verifyErr := security.VerifyPassword("initial-password", *stored.HashedPassword); verifyErr != nil || !valid {
		t.Fatalf("created password valid=%v error=%v", valid, verifyErr)
	}

	oauthUser := &UserModel{ID: "oauth-only", Email: "oauth@example.com", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	store.users[oauthUser.ID] = oauthUser
	reset, err := svc.ResetUserPassword(adminCtx, &identra_v1_pb.ResetUserPasswordRequest{
		UserId: oauthUser.ID, Password: "new-password",
	})
	if err != nil {
		t.Fatalf("reset OAuth-only user password: %v", err)
	}
	if !reset.GetUser().GetPasswordLoginEnabled() {
		t.Fatal("reset did not enable password login")
	}

	if _, err := svc.DeleteUser(adminCtx, &identra_v1_pb.DeleteUserRequest{UserId: created.GetUser().GetId()}); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if len(audits.events) != 3 || audits.events[0].Action != "user.create" || audits.events[1].Action != "user.reset_password" || audits.events[2].Action != "user.delete" {
		t.Fatalf("unexpected audit events: %+v", audits.events)
	}
}

func TestListUsersRequiresScopeAndPaginates(t *testing.T) {
	ctx := context.Background()
	accounts := newMemoryServiceAccountStore()
	reader, err := serviceaccount.Bootstrap(ctx, accounts, serviceaccount.BootstrapRequest{
		Name: "user-reader", Scopes: []string{ScopeUsersRead},
	})
	if err != nil {
		t.Fatalf("bootstrap user reader: %v", err)
	}
	tokenConfig := newTestTokenConfig(t)
	token, err := security.NewServiceToken(reader.ID, reader.Scopes, tokenConfig)
	if err != nil {
		t.Fatalf("issue service token: %v", err)
	}
	store := newMockUserStore()
	store.users["one"] = &UserModel{ID: "one", Email: "one@example.com"}
	store.users["two"] = &UserModel{ID: "two", Email: "two@example.com"}
	svc := &Service{userStore: store, serviceAccountStore: accounts, tokenCfg: tokenConfig}

	_, err = svc.ListUsers(ctx, &identra_v1_pb.ListUsersRequest{})
	requireCode(t, err, codes.Unauthenticated)
	response, err := svc.ListUsers(serviceTokenContext(token.Value), &identra_v1_pb.ListUsersRequest{PageSize: 1})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(response.GetUsers()) != 1 || response.GetNextPageToken() != "1" {
		t.Fatalf("unexpected first page: %+v", response)
	}
	_, err = svc.CreatePasswordUser(serviceTokenContext(token.Value), &identra_v1_pb.CreatePasswordUserRequest{Email: "no@example.com", Password: "password"})
	requireCode(t, err, codes.PermissionDenied)
}
