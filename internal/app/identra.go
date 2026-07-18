package app

import (
	"context"
	"fmt"

	"github.com/slhmy/identra/internal/config"
	"github.com/slhmy/identra/internal/identra"
)

func NewService(ctx context.Context, cfg config.GRPCConfig) (*identra.Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid gRPC config: %w", err)
	}
	deps, err := buildIdentraDependencies(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return identra.NewService(deps)
}

func buildIdentraDependencies(ctx context.Context, cfg config.GRPCConfig) (identra.Dependencies, error) {
	mailer := buildMailer(cfg.SmtpMailer)

	userStore, externalIdentityStore, serviceAccountStore, cleanup, storeErr := buildStores(ctx, cfg.Persistence)
	if storeErr != nil {
		return identra.Dependencies{}, storeErr
	}

	keys, keyErr := buildTokenKeys(cfg.Auth)
	if keyErr != nil {
		return identra.Dependencies{}, keyErr
	}

	redisDeps, redisErr := buildRedisDependencies(cfg.Redis, cfg.Auth.OAuth)
	if redisErr != nil {
		return identra.Dependencies{}, redisErr
	}

	return identra.Dependencies{
		UserStore:                userStore,
		ExternalIdentityStore:    externalIdentityStore,
		ServiceAccountStore:      serviceAccountStore,
		KeyManager:               keys.manager,
		TokenConfig:              keys.tokenConfig,
		OAuthStateStore:          redisDeps.oauthStateStore,
		EmailCodeStore:           redisDeps.emailCodeStore,
		GithubOAuthConfig:        buildGithubOAuthConfig(cfg.Auth.OAuth),
		OAuthFetchEmailIfMissing: cfg.Auth.OAuth.FetchEmailIfMissing,
		Mailer:                   mailer,
		UserStoreCleanup:         cleanup,
		LoginRateLimiter:         redisDeps.loginRateLimiter,
		SendCodeRateLimiter:      redisDeps.sendCodeRateLimiter,
		RefreshTokenRevocations:  redisDeps.refreshTokenRevocations,
	}, nil
}
