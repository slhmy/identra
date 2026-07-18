package identra

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func (s *Service) GetCurrentUser(
	ctx context.Context,
	_ *identra_v1_pb.GetCurrentUserRequest,
) (*identra_v1_pb.GetCurrentUserResponse, error) {
	accessToken := accessTokenFromMetadata(ctx)
	if accessToken == "" {
		return nil, status.Error(codes.Unauthenticated, "access token is required")
	}

	claims, err := security.ValidateAccessToken(accessToken, s.tokenCfg.PublicKey)
	if err != nil {
		slog.WarnContext(ctx, "invalid access token for get current user", "error", err)
		return nil, status.Error(codes.Unauthenticated, "invalid access token")
	}

	usr, err := s.userStore.GetByID(ctx, claims.UserID)
	switch {
	case err == nil:
	case errors.Is(err, ErrNotFound):
		return nil, status.Error(codes.NotFound, "user not found")
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user")
	}

	resp := &identra_v1_pb.GetCurrentUserResponse{
		User: &identra_v1_pb.User{
			Id:                   usr.ID,
			Email:                usr.Email,
			PasswordLoginEnabled: usr.HashedPassword != nil && strings.TrimSpace(*usr.HashedPassword) != "",
		},
	}

	identities, err := s.externalIdentityStore.GetByUserID(ctx, usr.ID)
	if err != nil {
		slog.WarnContext(ctx, "failed to fetch external identities", "error", err, "user_id", usr.ID)
	} else {
		for _, identity := range identities {
			resp.User.LinkedOauthAccounts = append(resp.User.LinkedOauthAccounts, &identra_v1_pb.LinkedOAuthAccount{
				Provider:       authProviderValue(identity.Provider),
				ProviderUserId: identity.ProviderUserID,
			})
		}
	}

	return resp, nil
}

func accessTokenFromMetadata(ctx context.Context) string {
	for _, header := range metadata.ValueFromIncomingContext(ctx, "authorization") {
		if token := bearerToken(header); token != "" {
			return token
		}
	}
	return ""
}

func bearerToken(header string) string {
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
