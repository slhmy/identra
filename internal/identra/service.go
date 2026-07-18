package identra

import (
	"context"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"golang.org/x/oauth2"
)

// Service implements the identra.v1 domain services.
type Service struct {
	identra_v1_pb.UnimplementedAuthServiceServer
	identra_v1_pb.UnimplementedSessionServiceServer
	identra_v1_pb.UnimplementedUserServiceServer
	identra_v1_pb.UnimplementedKeyServiceServer

	emailCodeStore           EmailCodeStore
	oauthStateStore          OAuthStateStore
	userStore                UserStore
	externalIdentityStore    ExternalIdentityStore
	userStoreCleanup         func(context.Context) error
	keyManager               *security.KeyManager
	tokenCfg                 security.TokenConfig
	githubOAuthConfig        *oauth2.Config
	oauthFetchEmailIfMissing bool
	mailer                   EmailSender

	// loginRateLimiter counts failed login attempts per email address and
	// blocks further attempts after the configured threshold.
	loginRateLimiter RateLimiter
	// sendCodeRateLimiter limits how many email verification codes can be sent
	// to a single address within the configured window.
	sendCodeRateLimiter RateLimiter
	// refreshTokenRevocations blocks reuse of refresh tokens after logout,
	// explicit revocation, or successful refresh-token rotation.
	refreshTokenRevocations RefreshTokenRevocationStore
}

func (s *Service) Close(ctx context.Context) error {
	if s.userStoreCleanup != nil {
		return s.userStoreCleanup(ctx)
	}
	return nil
}
