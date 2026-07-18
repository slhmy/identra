package identra

import (
	"context"
	"testing"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

func TestAccessTokenFromMetadataAcceptsBearerHeader(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer header-token"))

	got := accessTokenFromMetadata(ctx)
	if got != "header-token" {
		t.Fatalf("expected bearer token from header, got %q", got)
	}
}

func TestGetCurrentUserAcceptsBearerHeader(t *testing.T) {
	tokenCfg := newTestTokenConfig(t)
	pair, err := security.NewTokenPair("uid-bearer", tokenCfg)
	if err != nil {
		t.Fatalf("failed to create token pair: %v", err)
	}

	store := newMockUserStore()
	if err := store.Create(context.Background(), &UserModel{
		ID:    "uid-bearer",
		Email: "bearer@example.com",
	}); err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	svc := &Service{
		userStore:             store,
		externalIdentityStore: newMockExternalIdentityStore(),
		tokenCfg:              tokenCfg,
	}
	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("authorization", "Bearer "+pair.AccessToken.Value),
	)

	resp, err := svc.GetCurrentUser(ctx, &identra_v1_pb.GetCurrentUserRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.GetUser().GetId() != "uid-bearer" {
		t.Fatalf("expected uid-bearer, got %q", resp.GetUser().GetId())
	}
}

func TestGetCurrentUserRejectsMissingMetadata(t *testing.T) {
	svc := &Service{}
	_, err := svc.GetCurrentUser(context.Background(), &identra_v1_pb.GetCurrentUserRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestLinkOAuthAccountRejectsMissingMetadata(t *testing.T) {
	svc := &Service{}
	_, err := svc.LinkOAuthAccount(context.Background(), &identra_v1_pb.LinkOAuthAccountRequest{})
	requireCode(t, err, codes.Unauthenticated)
}
