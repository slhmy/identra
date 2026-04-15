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
	PersistenceType                string
	GORMClient                     *gorm.Config
	MongoClient                    *mongo.Config
	RedisClient                    *redis.Config

	// LoginMaxAttempts is the maximum number of failed login attempts (password
	// or email-code) allowed within LoginLockoutDuration before the account is
	// temporarily locked. 0 means use DefaultLoginMaxAttempts.
	LoginMaxAttempts int
	// LoginLockoutDuration is the sliding window during which failed login
	// attempts are counted. 0 means use DefaultLoginLockoutDuration.
	LoginLockoutDuration time.Duration

	// SendCodeMaxAttempts is the maximum number of email verification codes
	// that can be requested per email address within SendCodeWindow. 0 means
	// use DefaultSendCodeMaxAttempts.
	SendCodeMaxAttempts int
	// SendCodeWindow is the sliding window for the send-code rate limit. 0
	// means use DefaultSendCodeWindow.
	SendCodeWindow time.Duration
}

const (
	DefaultOAuthStateExpiration   = 10 * time.Minute
	DefaultAccessTokenExpiration  = 15 * time.Minute   // Short-lived access token
	DefaultRefreshTokenExpiration = 7 * 24 * time.Hour // 7 days refresh token
	DefaultTokenIssuer            = "identra"

	// DefaultLoginMaxAttempts is the default maximum number of failed login
	// attempts before a temporary lockout is applied.
	DefaultLoginMaxAttempts = 5
	// DefaultLoginLockoutDuration is the default window over which failed login
	// attempts are counted.
	DefaultLoginLockoutDuration = 15 * time.Minute

	// DefaultSendCodeMaxAttempts is the default maximum number of email
	// verification codes that can be sent per address within DefaultSendCodeWindow.
	DefaultSendCodeMaxAttempts = 5
	// DefaultSendCodeWindow is the default rate-limit window for sending email
	// verification codes.
	DefaultSendCodeWindow = 1 * time.Hour
)

type MongoConfig struct {
	URI        string
	Database   string
	Collection string
}
