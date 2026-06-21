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

func (s *Service) GetCurrentUserLoginInfo(
	ctx context.Context,
	req *identra_v1_pb.GetCurrentUserLoginInfoRequest,
) (*identra_v1_pb.GetCurrentUserLoginInfoResponse, error) {
	accessToken := accessTokenFromRequest(ctx, req.GetAccessToken())
	if accessToken == "" {
		return nil, status.Error(codes.InvalidArgument, "access token is required")
	}

	claims, err := security.ValidateAccessToken(accessToken, s.tokenCfg.PublicKey)
	if err != nil {
		slog.WarnContext(ctx, "invalid access token for get login info", "error", err)
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

	resp := &identra_v1_pb.GetCurrentUserLoginInfoResponse{
		UserId:          usr.ID,
		Email:           usr.Email,
		PasswordEnabled: usr.HashedPassword != nil && strings.TrimSpace(*usr.HashedPassword) != "",
	}

	identities, err := s.externalIdentityStore.GetByUserID(ctx, usr.ID)
	if err != nil {
		slog.WarnContext(ctx, "failed to fetch external identities", "error", err, "user_id", usr.ID)
	} else {
		var githubID string
		for _, identity := range identities {
			resp.OauthConnections = append(resp.OauthConnections, &identra_v1_pb.OAuthConnection{
				Provider:       identity.Provider,
				ProviderUserId: identity.ProviderUserID,
			})
			if identity.Provider == "github" && (githubID == "" || identity.ProviderUserID < githubID) {
				githubID = identity.ProviderUserID
			}
		}
		if githubID != "" {
			resp.GithubId = &githubID
		}
	}

	return resp, nil
}

func accessTokenFromRequest(ctx context.Context, requestToken string) string {
	for _, header := range metadata.ValueFromIncomingContext(ctx, "authorization") {
		if token := bearerToken(header); token != "" {
			return token
		}
	}
	return strings.TrimSpace(requestToken)
}

func bearerToken(header string) string {
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
