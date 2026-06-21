package config

import (
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

func LoadGRPC() GRPCConfig {
	return GRPCConfig{
		GRPCPort: bootstrap.Config().GetUint(GRPCPortKey),
		SmtpMailer: smtp.Config{
			Host:      bootstrap.Config().GetString(SmtpMailerHostKey),
			Port:      bootstrap.Config().GetInt(SmtpMailerPortKey),
			Username:  bootstrap.Config().GetString(SmtpMailerUsernameKey),
			Password:  bootstrap.Config().GetString(SmtpMailerPasswordKey),
			FromEmail: bootstrap.Config().GetString(SmtpMailerFromEmailKey),
			FromName:  bootstrap.Config().GetString(SmtpMailerFromNameKey),
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
			Urls:     bootstrap.Config().GetStringSlice(RedisUrlsKey),
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
