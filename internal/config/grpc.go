package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/slhmy/identra/internal/bootstrap"
	"github.com/slhmy/identra/internal/cache/redis"
	"github.com/slhmy/identra/internal/mail/smtp"
	"github.com/slhmy/identra/internal/store/gorm"
	"github.com/slhmy/identra/internal/store/mongo"
)

type GRPCConfig struct {
	GRPCPort    uint
	Redis       redis.Config
	SmtpMailer  smtp.Config
	Persistence PersistenceConfig
	Auth        AuthConfig
}

type AuthConfig struct {
	RSAPrivateKey string
	OAuth         OAuthConfig
	Token         TokenConfig
}

type OAuthConfig struct {
	StateExpirationDuration time.Duration
	GithubClientID          string
	GithubClientSecret      string
	FetchEmailIfMissing     bool
}

type TokenConfig struct {
	Issuer                 string
	AccessTokenExpiration  time.Duration
	RefreshTokenExpiration time.Duration
}

type PersistenceConfig struct {
	Type  string
	GORM  *gorm.Config
	Mongo *mongo.Config
}

func (c GRPCConfig) Validate() error {
	if c.GRPCPort == 0 {
		return errors.New("grpc port is required")
	}
	if err := validateRedis(c.Redis); err != nil {
		return fmt.Errorf("redis config: %w", err)
	}
	if err := validateMailer(c.SmtpMailer); err != nil {
		return fmt.Errorf("smtp mailer config: %w", err)
	}
	if err := c.Persistence.Validate(); err != nil {
		return fmt.Errorf("persistence config: %w", err)
	}
	if err := c.Auth.Validate(); err != nil {
		return fmt.Errorf("auth config: %w", err)
	}
	return nil
}

func (c AuthConfig) Validate() error {
	if c.OAuth.StateExpirationDuration < 0 {
		return errors.New("oauth state expiration cannot be negative")
	}
	if strings.TrimSpace(c.Token.Issuer) == "" {
		return errors.New("token issuer is required")
	}
	if c.Token.AccessTokenExpiration <= 0 {
		return errors.New("access token expiration must be positive")
	}
	if c.Token.RefreshTokenExpiration <= 0 {
		return errors.New("refresh token expiration must be positive")
	}
	return nil
}

func (c PersistenceConfig) Validate() error {
	switch strings.ToLower(strings.TrimSpace(c.Type)) {
	case "", "gorm", "postgres", "mysql", "sqlite":
		if c.GORM == nil {
			return errors.New("gorm config is required")
		}
		if strings.TrimSpace(c.GORM.Driver) == "" {
			return errors.New("gorm driver is required")
		}
		if strings.TrimSpace(c.GORM.DbName) == "" {
			return errors.New("gorm dbname is required")
		}
		if err := c.GORM.Validate(); err != nil {
			return err
		}
	case "mongo", "mongodb":
		if c.Mongo == nil {
			return errors.New("mongo config is required")
		}
		if strings.TrimSpace(c.Mongo.URI) == "" {
			return errors.New("mongo uri is required")
		}
		if strings.TrimSpace(c.Mongo.Database) == "" {
			return errors.New("mongo database is required")
		}
	default:
		return fmt.Errorf("unsupported persistence type %q", c.Type)
	}
	return nil
}

func validateRedis(cfg redis.Config) error {
	if len(cfg.Urls) == 0 {
		return errors.New("at least one redis url is required")
	}
	for _, url := range cfg.Urls {
		if strings.TrimSpace(url) == "" {
			return errors.New("redis urls cannot contain empty values")
		}
	}
	return nil
}

func validateMailer(cfg smtp.Config) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil
	}
	switch {
	case cfg.Port == 0:
		return errors.New("smtp port is required")
	case cfg.Port < 0 || cfg.Port > 65535:
		return errors.New("smtp port must be between 1 and 65535")
	case cfg.AuthEnabled && strings.TrimSpace(cfg.Username) == "":
		return errors.New("smtp username is required")
	case cfg.AuthEnabled && strings.TrimSpace(cfg.Password) == "":
		return errors.New("smtp password is required")
	case strings.TrimSpace(cfg.FromEmail) == "":
		return errors.New("smtp from email is required")
	default:
		return nil
	}
}

func LoadGRPC() GRPCConfig {
	return GRPCConfig{
		GRPCPort: bootstrap.Config().GetUint(GRPCPortKey),
		SmtpMailer: smtp.Config{
			Host:        bootstrap.Config().GetString(SmtpMailerHostKey),
			Port:        bootstrap.Config().GetInt(SmtpMailerPortKey),
			Username:    bootstrap.Config().GetString(SmtpMailerUsernameKey),
			Password:    bootstrap.Config().GetString(SmtpMailerPasswordKey),
			FromEmail:   bootstrap.Config().GetString(SmtpMailerFromEmailKey),
			FromName:    bootstrap.Config().GetString(SmtpMailerFromNameKey),
			StartTLS:    bootstrap.Config().GetBool(SmtpMailerStartTLSKey),
			AuthEnabled: bootstrap.Config().GetBool(SmtpMailerAuthEnabledKey),
		},
		Persistence: PersistenceConfig{
			Type: bootstrap.Config().GetString(PersistenceTypeKey),
			GORM: &gorm.Config{
				Driver:   bootstrap.Config().GetString(PersistenceGORMDriverKey),
				Host:     bootstrap.Config().GetString(PersistenceGORMHostKey),
				Port:     bootstrap.Config().GetInt(PersistenceGORMPortKey),
				Username: bootstrap.Config().GetString(PersistenceGORMUsernameKey),
				Password: bootstrap.Config().GetString(PersistenceGORMPasswordKey),
				DbName:   bootstrap.Config().GetString(PersistenceGORMDBNameKey),
				SSLMode:  bootstrap.Config().GetString(PersistenceGORMSSLModeKey),
			},
			Mongo: &mongo.Config{
				URI:      bootstrap.Config().GetString(PersistenceMongoURIKey),
				Database: bootstrap.Config().GetString(PersistenceMongoDatabaseKey),
			},
		},
		Redis: redis.Config{
			Urls:     getStringSlice(RedisUrlsKey),
			Password: bootstrap.Config().GetString(RedisPasswordKey),
		},
		Auth: AuthConfig{
			RSAPrivateKey: bootstrap.Config().GetString(AuthRSAPrivateKeyKey),
			OAuth: OAuthConfig{
				StateExpirationDuration: bootstrap.Config().GetDuration(AuthOAuthStateExpirationKey),
				GithubClientID:          bootstrap.Config().GetString(AuthGithubClientIDKey),
				GithubClientSecret:      bootstrap.Config().GetString(AuthGithubClientSecretKey),
				FetchEmailIfMissing:     bootstrap.Config().GetBool(AuthOAuthFetchEmailIfMissingKey),
			},
			Token: TokenConfig{
				Issuer:                 bootstrap.Config().GetString(AuthTokenIssuerKey),
				AccessTokenExpiration:  bootstrap.Config().GetDuration(AuthAccessTokenExpirationKey),
				RefreshTokenExpiration: bootstrap.Config().GetDuration(AuthRefreshTokenExpirationKey),
			},
		},
	}
}
