package identra

import (
	"context"
	"testing"
	"time"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestTokenConfig generates a throwaway RSA key pair for unit tests.
func newTestTokenConfig(t *testing.T) security.TokenConfig {
	t.Helper()
	km := &security.KeyManager{}
	if err := km.GenerateKeyPair(); err != nil {
		t.Fatalf("failed to generate key pair for tests: %v", err)
	}
	return security.TokenConfig{
		PrivateKey:             km.GetPrivateKey(),
		PublicKey:              km.GetPublicKey(),
		KeyID:                  km.GetKeyID(),
		Issuer:                 "test",
		AccessTokenExpiration:  15 * time.Minute,
		RefreshTokenExpiration: 7 * 24 * time.Hour,
		ServiceTokenExpiration: 15 * time.Minute,
	}
}

// requireCode asserts the gRPC status code of an error.
func requireCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", want)
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	if st.Code() != want {
		t.Errorf("expected status code %s, got %s: %s", want, st.Code(), st.Message())
	}
}

// ---- RegisterWithPassword tests ----

func TestRegisterWithPassword_MissingFields(t *testing.T) {
	svc := &Service{userStore: newMockUserStore()}

	cases := []struct {
		name  string
		email string
		pwd   string
	}{
		{"empty email", "", "secret"},
		{"empty password", "user@example.com", ""},
		{"both empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.RegisterWithPassword(context.Background(), &identra_v1_pb.RegisterWithPasswordRequest{
				Email:    tc.email,
				Password: tc.pwd,
			})
			requireCode(t, err, codes.InvalidArgument)
		})
	}
}

func TestRegisterWithPassword_Success(t *testing.T) {
	svc := &Service{
		userStore: newMockUserStore(),
		tokenCfg:  newTestTokenConfig(t),
	}

	resp, err := svc.RegisterWithPassword(context.Background(), &identra_v1_pb.RegisterWithPasswordRequest{
		Email:    "new@example.com",
		Password: "s3cr3t",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Tokens == nil || resp.Tokens.AccessToken == nil || resp.Tokens.AccessToken.Value == "" {
		t.Error("expected a non-empty access token in the response")
	}
}

func TestRegisterWithPassword_ExistingEmail(t *testing.T) {
	store := newMockUserStore()
	// Pre-populate a user with the same email.
	_ = store.Create(context.Background(), &UserModel{
		ID:    "existing-id",
		Email: "taken@example.com",
	})

	svc := &Service{userStore: store}

	_, err := svc.RegisterWithPassword(context.Background(), &identra_v1_pb.RegisterWithPasswordRequest{
		Email:    "taken@example.com",
		Password: "s3cr3t",
	})
	requireCode(t, err, codes.AlreadyExists)
}

func TestRegisterWithPassword_DuplicateRace(t *testing.T) {
	// Simulate a race where GetByEmail returns ErrNotFound (pre-check passes)
	// but Create fails with ErrAlreadyExists (another goroutine just
	// created the same user).
	store := newMockUserStore()
	store.forceCreateErr = ErrAlreadyExists

	svc := &Service{userStore: store}

	_, err := svc.RegisterWithPassword(context.Background(), &identra_v1_pb.RegisterWithPasswordRequest{
		Email:    "race@example.com",
		Password: "s3cr3t",
	})
	requireCode(t, err, codes.AlreadyExists)
}

// ---- LoginWithPassword tests ----

func TestLoginWithPassword_MissingFields(t *testing.T) {
	svc := &Service{userStore: newMockUserStore()}

	cases := []struct {
		name  string
		email string
		pwd   string
	}{
		{"empty email", "", "secret"},
		{"empty password", "user@example.com", ""},
		{"both empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.LoginWithPassword(context.Background(), &identra_v1_pb.LoginWithPasswordRequest{
				Email:    tc.email,
				Password: tc.pwd,
			})
			requireCode(t, err, codes.InvalidArgument)
		})
	}
}

func TestLoginWithPassword_NotFound(t *testing.T) {
	svc := &Service{userStore: newMockUserStore()}

	_, err := svc.LoginWithPassword(context.Background(), &identra_v1_pb.LoginWithPasswordRequest{
		Email:    "nobody@example.com",
		Password: "s3cr3t",
	})
	requireCode(t, err, codes.NotFound)
}

func TestLoginWithPassword_NoPassword(t *testing.T) {
	store := newMockUserStore()
	_ = store.Create(context.Background(), &UserModel{
		ID:             "uid1",
		Email:          "oauth@example.com",
		HashedPassword: nil, // OAuth-only account
	})
	svc := &Service{userStore: store}

	_, err := svc.LoginWithPassword(context.Background(), &identra_v1_pb.LoginWithPasswordRequest{
		Email:    "oauth@example.com",
		Password: "s3cr3t",
	})
	requireCode(t, err, codes.FailedPrecondition)
}

func TestLoginWithPassword_WrongPassword(t *testing.T) {
	hash, err := security.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	store := newMockUserStore()
	_ = store.Create(context.Background(), &UserModel{
		ID:             "uid2",
		Email:          "user@example.com",
		HashedPassword: &hash,
	})
	svc := &Service{userStore: store}

	_, loginErr := svc.LoginWithPassword(context.Background(), &identra_v1_pb.LoginWithPasswordRequest{
		Email:    "user@example.com",
		Password: "wrong-password",
	})
	requireCode(t, loginErr, codes.Unauthenticated)
}

func TestLoginWithPassword_Success(t *testing.T) {
	hash, err := security.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	store := newMockUserStore()
	_ = store.Create(context.Background(), &UserModel{
		ID:             "uid3",
		Email:          "user@example.com",
		HashedPassword: &hash,
	})
	svc := &Service{
		userStore: store,
		tokenCfg:  newTestTokenConfig(t),
	}

	resp, loginErr := svc.LoginWithPassword(context.Background(), &identra_v1_pb.LoginWithPasswordRequest{
		Email:    "user@example.com",
		Password: "correct-password",
	})
	if loginErr != nil {
		t.Fatalf("expected no error, got %v", loginErr)
	}
	if resp.Tokens == nil || resp.Tokens.AccessToken == nil || resp.Tokens.AccessToken.Value == "" {
		t.Error("expected a non-empty access token in the response")
	}
}
