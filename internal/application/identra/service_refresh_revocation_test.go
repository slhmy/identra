package identra

import (
	"context"
	"sync"
	"testing"
	"time"

	identra_v1_pb "github.com/poly-workshop/identra/gen/go/identra/v1"
	"github.com/poly-workshop/identra/internal/infrastructure/security"
	"google.golang.org/grpc/codes"
)

type mockRefreshTokenRevocationStore struct {
	mu      sync.Mutex
	revoked map[string]time.Time
}

func newMockRefreshTokenRevocationStore() *mockRefreshTokenRevocationStore {
	return &mockRefreshTokenRevocationStore{revoked: make(map[string]time.Time)}
}

func (s *mockRefreshTokenRevocationStore) Revoke(_ context.Context, tokenID string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked[tokenID] = expiresAt
	return nil
}

func (s *mockRefreshTokenRevocationStore) IsRevoked(_ context.Context, tokenID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.revoked[tokenID]
	return ok, nil
}

func TestRefreshTokenRevokesUsedRefreshToken(t *testing.T) {
	tokenCfg := newTestTokenConfig(t)
	pair, err := security.NewTokenPair("uid-refresh", tokenCfg)
	if err != nil {
		t.Fatalf("failed to create token pair: %v", err)
	}

	svc := &Service{
		tokenCfg:                tokenCfg,
		refreshTokenRevocations: newMockRefreshTokenRevocationStore(),
	}
	req := &identra_v1_pb.RefreshTokenRequest{RefreshToken: pair.RefreshToken.Token}

	if _, err := svc.RefreshToken(context.Background(), req); err != nil {
		t.Fatalf("expected first refresh to succeed, got %v", err)
	}
	_, err = svc.RefreshToken(context.Background(), req)
	requireCode(t, err, codes.Unauthenticated)
}

func TestRevokeRefreshTokenBlocksRefresh(t *testing.T) {
	tokenCfg := newTestTokenConfig(t)
	pair, err := security.NewTokenPair("uid-revoke", tokenCfg)
	if err != nil {
		t.Fatalf("failed to create token pair: %v", err)
	}

	svc := &Service{
		tokenCfg:                tokenCfg,
		refreshTokenRevocations: newMockRefreshTokenRevocationStore(),
	}

	_, err = svc.RevokeRefreshToken(context.Background(), &identra_v1_pb.RevokeRefreshTokenRequest{
		RefreshToken: pair.RefreshToken.Token,
	})
	if err != nil {
		t.Fatalf("expected revoke to succeed, got %v", err)
	}

	_, err = svc.RefreshToken(context.Background(), &identra_v1_pb.RefreshTokenRequest{
		RefreshToken: pair.RefreshToken.Token,
	})
	requireCode(t, err, codes.Unauthenticated)
}
