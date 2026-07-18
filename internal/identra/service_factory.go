package identra

import (
	"context"
	"errors"

	"github.com/slhmy/identra/internal/security"
	"github.com/slhmy/identra/internal/serviceaccount"
	"golang.org/x/oauth2"
)

// Dependencies contains the collaborators required by Service.
type Dependencies struct {
	EmailCodeStore           EmailCodeStore
	OAuthStateStore          OAuthStateStore
	UserStore                UserStore
	ExternalIdentityStore    ExternalIdentityStore
	ServiceAccountStore      serviceaccount.Store
	UserStoreCleanup         func(context.Context) error
	KeyManager               *security.KeyManager
	TokenConfig              security.TokenConfig
	GithubOAuthConfig        *oauth2.Config
	OAuthFetchEmailIfMissing bool
	Mailer                   EmailSender
	LoginRateLimiter         RateLimiter
	SendCodeRateLimiter      RateLimiter
	RefreshTokenRevocations  RefreshTokenRevocationStore
	ServiceTokenRateLimiter  RateLimiter
	AuditStore               AuditStore
	ServerInfo               ServerInfo
}

func NewService(deps Dependencies) (*Service, error) {
	if deps.EmailCodeStore == nil {
		return nil, errors.New("email code store is required")
	}
	if deps.OAuthStateStore == nil {
		return nil, errors.New("oauth state store is required")
	}
	if deps.UserStore == nil {
		return nil, errors.New("user store is required")
	}
	if deps.ExternalIdentityStore == nil {
		return nil, errors.New("external identity store is required")
	}
	if deps.ServiceAccountStore == nil {
		return nil, errors.New("service account store is required")
	}
	if deps.AuditStore == nil {
		return nil, errors.New("audit store is required")
	}
	if deps.KeyManager == nil {
		return nil, errors.New("key manager is required")
	}
	if deps.TokenConfig.PrivateKey == nil || deps.TokenConfig.PublicKey == nil {
		return nil, errors.New("token keys are not initialized")
	}
	if deps.GithubOAuthConfig == nil {
		return nil, errors.New("github oauth config is required")
	}

	return &Service{
		userStore:                deps.UserStore,
		externalIdentityStore:    deps.ExternalIdentityStore,
		serviceAccountStore:      deps.ServiceAccountStore,
		keyManager:               deps.KeyManager,
		tokenCfg:                 deps.TokenConfig,
		oauthStateStore:          deps.OAuthStateStore,
		emailCodeStore:           deps.EmailCodeStore,
		githubOAuthConfig:        deps.GithubOAuthConfig,
		oauthFetchEmailIfMissing: deps.OAuthFetchEmailIfMissing,
		mailer:                   deps.Mailer,
		userStoreCleanup:         deps.UserStoreCleanup,
		loginRateLimiter:         deps.LoginRateLimiter,
		sendCodeRateLimiter:      deps.SendCodeRateLimiter,
		refreshTokenRevocations:  deps.RefreshTokenRevocations,
		serviceTokenRateLimiter:  deps.ServiceTokenRateLimiter,
		auditStore:               deps.AuditStore,
		serverInfo:               deps.ServerInfo,
	}, nil
}
