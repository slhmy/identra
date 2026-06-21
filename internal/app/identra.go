package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/slhmy/identra/internal/cache"
	"github.com/slhmy/identra/internal/cache/redis"
	"github.com/slhmy/identra/internal/config"
	"github.com/slhmy/identra/internal/identra"
	"github.com/slhmy/identra/internal/mail/smtp"
	"github.com/slhmy/identra/internal/oauth"
	"github.com/slhmy/identra/internal/security"
	"github.com/slhmy/identra/internal/store"
	"github.com/slhmy/identra/internal/store/gorm"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

func NewService(ctx context.Context, cfg config.GRPCConfig) (*identra.Service, error) {
	deps, err := buildIdentraDependencies(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return identra.NewService(deps)
}

func buildIdentraDependencies(ctx context.Context, cfg config.GRPCConfig) (identra.Dependencies, error) {
	mailerCfg := cfg.SmtpMailer
	var mailer identra.EmailSender

	if strings.TrimSpace(mailerCfg.Host) != "" {
		if err := validateMailerConfig(mailerCfg); err != nil {
			return identra.Dependencies{}, fmt.Errorf("invalid mailer config: %w", err)
		}

		mailer = smtpMailerAdapter{sender: smtp.NewMailer(mailerCfg)}
	}

	km := security.GetKeyManager()
	if cfg.Auth.RSAPrivateKey != "" {
		if err := km.InitializeFromPEM(cfg.Auth.RSAPrivateKey); err != nil {
			return identra.Dependencies{}, fmt.Errorf("failed to load RSA private key: %w", err)
		}
	}
	if !km.IsInitialized() {
		if err := km.GenerateKeyPair(); err != nil {
			return identra.Dependencies{}, fmt.Errorf("failed to generate RSA key pair: %w", err)
		}
	}

	tokenCfg := security.TokenConfig{
		PrivateKey:             km.GetPrivateKey(),
		PublicKey:              km.GetPublicKey(),
		KeyID:                  km.GetKeyID(),
		Issuer:                 cfg.Auth.Token.Issuer,
		AccessTokenExpiration:  cfg.Auth.Token.AccessTokenExpiration,
		RefreshTokenExpiration: cfg.Auth.Token.RefreshTokenExpiration,
	}
	if tokenCfg.PrivateKey == nil || tokenCfg.PublicKey == nil {
		return identra.Dependencies{}, errors.New("token keys are not initialized")
	}

	stateTTL := cfg.Auth.OAuth.StateExpirationDuration
	if stateTTL <= 0 {
		stateTTL = identra.DefaultOAuthStateExpiration
	}

	userStore, externalIdentityStore, cleanup, storeErr := buildStores(ctx, cfg.Persistence)
	if storeErr != nil {
		return identra.Dependencies{}, storeErr
	}

	githubCfg := &oauth2.Config{
		ClientID:     cfg.Auth.OAuth.GithubClientID,
		ClientSecret: cfg.Auth.OAuth.GithubClientSecret,
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}

	rdb, storeErr := redis.NewRDB(cfg.Redis)
	if storeErr != nil {
		return identra.Dependencies{}, fmt.Errorf("failed to initialize redis client: %w", storeErr)
	}

	emailStore, storeErr := cache.NewRedisEmailCodeStore(10*time.Minute, rdb)
	if storeErr != nil {
		return identra.Dependencies{}, fmt.Errorf("failed to initialize email code store: %w", storeErr)
	}

	oauthStore, storeErr := cache.NewRedisOAuthStateStore(stateTTL, rdb)
	if storeErr != nil {
		return identra.Dependencies{}, fmt.Errorf("failed to initialize oauth state store: %w", storeErr)
	}

	loginMaxAttempts := identra.DefaultLoginMaxAttempts
	loginLockoutDuration := identra.DefaultLoginLockoutDuration

	loginLimiter, loginLimiterErr := cache.NewRedisRateLimiter(
		rdb,
		"identra:rl:login:",
		loginMaxAttempts,
		loginLockoutDuration,
	)
	if loginLimiterErr != nil {
		return identra.Dependencies{}, fmt.Errorf("failed to initialize login rate limiter: %w", loginLimiterErr)
	}

	sendCodeMaxAttempts := identra.DefaultSendCodeMaxAttempts
	sendCodeWindow := identra.DefaultSendCodeWindow

	sendCodeLimiter, sendCodeLimiterErr := cache.NewRedisRateLimiter(
		rdb,
		"identra:rl:send_code:",
		sendCodeMaxAttempts,
		sendCodeWindow,
	)
	if sendCodeLimiterErr != nil {
		return identra.Dependencies{}, fmt.Errorf("failed to initialize send-code rate limiter: %w", sendCodeLimiterErr)
	}

	refreshRevocations, refreshRevocationsErr := cache.NewRedisRefreshTokenRevocationStore(rdb)
	if refreshRevocationsErr != nil {
		return identra.Dependencies{}, fmt.Errorf("failed to initialize refresh token revocation store: %w", refreshRevocationsErr)
	}

	return identra.Dependencies{
		UserStore:                userStore,
		ExternalIdentityStore:    externalIdentityStore,
		KeyManager:               km,
		TokenConfig:              tokenCfg,
		OAuthStateStore:          oauthStateStoreAdapter{store: oauthStore},
		EmailCodeStore:           emailStore,
		GithubOAuthConfig:        githubCfg,
		OAuthFetchEmailIfMissing: cfg.Auth.OAuth.FetchEmailIfMissing,
		Mailer:                   mailer,
		UserStoreCleanup:         cleanup,
		LoginRateLimiter:         loginLimiter,
		SendCodeRateLimiter:      sendCodeLimiter,
		RefreshTokenRevocations:  refreshRevocations,
	}, nil
}

func validateMailerConfig(cfg smtp.Config) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil
	}

	switch {
	case cfg.Port == 0:
		return errors.New("smtp port is required")
	case strings.TrimSpace(cfg.Username) == "":
		return errors.New("smtp username is required")
	case strings.TrimSpace(cfg.Password) == "":
		return errors.New("smtp password is required")
	case strings.TrimSpace(cfg.FromEmail) == "":
		return errors.New("smtp from email is required")
	default:
		return nil
	}
}

func buildStores(ctx context.Context, cfg config.PersistenceConfig) (identra.UserStore, identra.ExternalIdentityStore, func(context.Context) error, error) {
	repoType := strings.ToLower(strings.TrimSpace(cfg.Type))
	switch repoType {
	case "mongo", "mongodb":
		mongoCfg := cfg.Mongo
		if strings.TrimSpace(mongoCfg.URI) == "" {
			return nil, nil, nil, fmt.Errorf("mongo uri is required when using mongo user repository")
		}
		if strings.TrimSpace(mongoCfg.Database) == "" {
			return nil, nil, nil, fmt.Errorf("mongo database is required when using mongo user repository")
		}

		client, err := mongo.Connect(options.Client().ApplyURI(mongoCfg.URI))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to connect to mongo: %w", err)
		}

		userStore, repoErr := store.NewMongoUserStore(ctx, client, mongoCfg.Database, "users")
		if repoErr != nil {
			_ = client.Disconnect(ctx)
			return nil, nil, nil, repoErr
		}

		extStore, extErr := store.NewMongoExternalIdentityStore(ctx, client, mongoCfg.Database, "external_identities")
		if extErr != nil {
			_ = client.Disconnect(ctx)
			return nil, nil, nil, extErr
		}

		cleanup := func(cleanupCtx context.Context) error {
			return client.Disconnect(cleanupCtx)
		}
		return userStore, extStore, cleanup, nil
	case "", "gorm", "postgres", "mysql", "sqlite":
		db, dbErr := gorm.NewDB(*cfg.GORM)
		if dbErr != nil {
			return nil, nil, nil, fmt.Errorf("failed to initialize gorm database: %w", dbErr)
		}
		if err := store.AutoMigrateGorm(db); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to migrate database: %w", err)
		}
		userStore := store.NewGormUserStore(db)
		extStore := store.NewGormExternalIdentityStore(db)
		return userStore, extStore, func(context.Context) error { return nil }, nil
	default:
		return nil, nil, nil, fmt.Errorf("unsupported user repository type: %s", cfg.Type)
	}
}

type smtpMailerAdapter struct {
	sender *smtp.Mailer
}

func (a smtpMailerAdapter) SendEmail(message identra.EmailMessage) error {
	return a.sender.SendEmail(smtp.Message{
		ToEmails: message.ToEmails,
		Subject:  message.Subject,
		Body:     message.Body,
		IsHTML:   message.IsHTML,
	})
}

type oauthStateStoreAdapter struct {
	store oauth.StateStore
}

func (a oauthStateStoreAdapter) Add(ctx context.Context, state, provider, redirectURL string) error {
	return a.store.Add(ctx, state, provider, redirectURL)
}

func (a oauthStateStoreAdapter) Consume(ctx context.Context, state string) (identra.OAuthState, bool, error) {
	data, ok, err := a.store.Consume(ctx, state)
	return identra.OAuthState{Provider: data.Provider, RedirectURL: data.RedirectURL}, ok, err
}
