package identra

import (
	"time"

	"github.com/poly-workshop/identra/internal/infrastructure/cache/redis"
	"github.com/poly-workshop/identra/internal/infrastructure/notification/smtp"
	"github.com/poly-workshop/identra/internal/infrastructure/persistence/gorm"
	"github.com/poly-workshop/identra/internal/infrastructure/persistence/mongo"
)

// Config holds all settings required to run the identra service.
type Config struct {
	RSAPrivateKey                  string
	GithubClientID                 string
	GithubClientSecret             string
	OAuthFetchEmailIfMissing       bool
	OAuthStateExpirationDuration   time.Duration
	AccessTokenExpirationDuration  time.Duration
	RefreshTokenExpirationDuration time.Duration
	TokenIssuer                    string
	SmtpMailer                     smtp.Config
	DatabaseType                   string
	GORMClient                     *gorm.Config
	MongoClient                    *mongo.Config
	RedisClient                    *redis.Config
	PersistenceType                string
}

const (
	DefaultOAuthStateExpiration   = 10 * time.Minute
	DefaultAccessTokenExpiration  = 15 * time.Minute   // Short-lived access token
	DefaultRefreshTokenExpiration = 7 * 24 * time.Hour // 7 days refresh token
	DefaultTokenIssuer            = "identra"
)

type MongoConfig struct {
	URI        string
	Database   string
	Collection string
}
