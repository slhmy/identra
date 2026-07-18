package identra

import (
	"context"
	"log/slog"
	"strings"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) RefreshSession(
	ctx context.Context,
	req *identra_v1_pb.RefreshSessionRequest,
) (*identra_v1_pb.RefreshSessionResponse, error) {
	refreshToken := strings.TrimSpace(req.GetRefreshToken())
	if refreshToken == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh token is required")
	}

	claims, err := security.ValidateRefreshToken(refreshToken, s.tokenCfg.PublicKey)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid refresh token")
	}
	if err := s.ensureRefreshTokenActive(ctx, claims); err != nil {
		return nil, err
	}

	tokenPair, err := security.NewTokenPair(claims.UserID, s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to refresh token pair", "error", err)
		return nil, status.Error(codes.Internal, "failed to refresh token")
	}
	if err := s.revokeRefreshClaims(ctx, claims); err != nil {
		return nil, err
	}

	return &identra_v1_pb.RefreshSessionResponse{Tokens: tokenPair}, nil
}

func (s *Service) RevokeSession(
	ctx context.Context,
	req *identra_v1_pb.RevokeSessionRequest,
) (*identra_v1_pb.RevokeSessionResponse, error) {
	refreshToken := strings.TrimSpace(req.GetRefreshToken())
	if refreshToken == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh token is required")
	}

	claims, err := security.ValidateRefreshToken(refreshToken, s.tokenCfg.PublicKey)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid refresh token")
	}
	if err := s.ensureRefreshTokenActive(ctx, claims); err != nil {
		return nil, err
	}
	if err := s.revokeRefreshClaims(ctx, claims); err != nil {
		return nil, err
	}

	return &identra_v1_pb.RevokeSessionResponse{}, nil
}

func (s *Service) ensureRefreshTokenActive(ctx context.Context, claims *security.StandardClaims) error {
	if s.refreshTokenRevocations == nil {
		return nil
	}
	revoked, err := s.refreshTokenRevocations.IsRevoked(ctx, claims.TokenID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to check refresh token revocation", "error", err)
		return status.Error(codes.Internal, "failed to validate refresh token")
	}
	if revoked {
		return status.Error(codes.Unauthenticated, "refresh token has been revoked")
	}
	return nil
}

func (s *Service) revokeRefreshClaims(ctx context.Context, claims *security.StandardClaims) error {
	if s.refreshTokenRevocations == nil || claims == nil || claims.ExpiresAt == nil {
		return nil
	}
	if err := s.refreshTokenRevocations.Revoke(ctx, claims.TokenID, claims.ExpiresAt.Time); err != nil {
		slog.ErrorContext(ctx, "failed to revoke refresh token", "error", err)
		return status.Error(codes.Internal, "failed to revoke refresh token")
	}
	return nil
}
