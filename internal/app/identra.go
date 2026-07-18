package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/slhmy/identra/internal/buildinfo"
	"github.com/slhmy/identra/internal/config"
	"github.com/slhmy/identra/internal/identra"
	"github.com/slhmy/identra/internal/serviceaccount"
	"github.com/slhmy/identra/internal/store/sqlite"
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

	userStore, externalIdentityStore, serviceAccountStore, auditStore, cleanup, storeErr := buildStores(ctx, cfg.Persistence)
	if storeErr != nil {
		return identra.Dependencies{}, storeErr
	}
	if bootstrapErr := bootstrapServiceAccount(ctx, cfg.Bootstrap, serviceAccountStore); bootstrapErr != nil {
		_ = cleanup(ctx)
		return identra.Dependencies{}, bootstrapErr
	}

	keys, keyErr := buildTokenKeys(cfg.Auth)
	if keyErr != nil {
		_ = cleanup(ctx)
		return identra.Dependencies{}, keyErr
	}

	redisDeps, redisErr := buildRedisDependencies(cfg.Redis, cfg.Auth.OAuth)
	if redisErr != nil {
		_ = cleanup(ctx)
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
		ServiceTokenRateLimiter:  redisDeps.serviceTokenRateLimiter,
		AuditStore:               auditStore,
		ServerInfo: identra.ServerInfo{
			Version:       buildinfo.Version,
			Commit:        buildinfo.Commit,
			BuildDate:     buildinfo.Date,
			SchemaVersion: sqlite.CurrentSchemaVersion,
			Capabilities: []string{
				"auth.password", "auth.email_code", "auth.oauth.github",
				"service_accounts", "audit_events",
			},
		},
	}, nil
}

func bootstrapServiceAccount(ctx context.Context, cfg config.BootstrapConfig, store serviceaccount.Store) error {
	if !cfg.Enabled {
		return nil
	}
	result, err := serviceaccount.Bootstrap(ctx, store, serviceaccount.BootstrapRequest{
		Name:         cfg.Name,
		Scopes:       cfg.Scopes,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		IfNotExists:  true,
	})
	if err != nil {
		return fmt.Errorf("bootstrap configured service account: %w", err)
	}
	if !strings.EqualFold(result.ID, strings.TrimSpace(cfg.ClientID)) {
		return fmt.Errorf("bootstrap service account %q already exists with a different client ID", cfg.Name)
	}
	if result.Created {
		slog.InfoContext(ctx, "bootstrapped configured service account", "name", result.Name, "client_id", result.ID)
	}
	return nil
}
