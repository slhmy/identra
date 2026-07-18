package identra

import (
	"context"
	"sync"
	"testing"
	"time"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
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

func TestRefreshSessionRevokesUsedRefreshToken(t *testing.T) {
	tokenCfg := newTestTokenConfig(t)
	pair, err := security.NewTokenPair("uid-refresh", tokenCfg)
	if err != nil {
		t.Fatalf("failed to create token pair: %v", err)
	}

	svc := &Service{
		tokenCfg:                tokenCfg,
		refreshTokenRevocations: newMockRefreshTokenRevocationStore(),
	}
	req := &identra_v1_pb.RefreshSessionRequest{RefreshToken: pair.RefreshToken.Value}

	if _, err := svc.RefreshSession(context.Background(), req); err != nil {
		t.Fatalf("expected first refresh to succeed, got %v", err)
	}
	_, err = svc.RefreshSession(context.Background(), req)
	requireCode(t, err, codes.Unauthenticated)
}

func TestRevokeSessionBlocksRefresh(t *testing.T) {
	tokenCfg := newTestTokenConfig(t)
	pair, err := security.NewTokenPair("uid-revoke", tokenCfg)
	if err != nil {
		t.Fatalf("failed to create token pair: %v", err)
	}

	svc := &Service{
		tokenCfg:                tokenCfg,
		refreshTokenRevocations: newMockRefreshTokenRevocationStore(),
	}

	_, err = svc.RevokeSession(context.Background(), &identra_v1_pb.RevokeSessionRequest{
		RefreshToken: pair.RefreshToken.Value,
	})
	if err != nil {
		t.Fatalf("expected revoke to succeed, got %v", err)
	}

	_, err = svc.RefreshSession(context.Background(), &identra_v1_pb.RefreshSessionRequest{
		RefreshToken: pair.RefreshToken.Value,
	})
	requireCode(t, err, codes.Unauthenticated)
}
