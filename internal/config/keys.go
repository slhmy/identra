package config

// Configuration keys constants
const (
	// Server configuration keys
	GRPCPortKey = "grpc_port"

	// Auth configuration keys
	AuthRSAPrivateKeyKey            = "auth.rsa_private_key"
	AuthRSAPrivateKeyFileKey        = "auth.rsa_private_key_file"
	AuthOAuthStateExpirationKey     = "auth.oauth_state_expiration"
	AuthAccessTokenExpirationKey    = "auth.access_token_expiration"
	AuthRefreshTokenExpirationKey   = "auth.refresh_token_expiration"
	AuthServiceTokenExpirationKey   = "auth.service_token_expiration"
	AuthTokenIssuerKey              = "auth.token_issuer"
	AuthOAuthFetchEmailIfMissingKey = "auth.oauth.fetch_email_if_missing"
	AuthGithubClientIDKey           = "auth.github.client_id"
	AuthGithubClientSecretKey       = "auth.github.client_secret"

	BootstrapServiceAccountEnabledKey      = "bootstrap.service_account.enabled"
	BootstrapServiceAccountNameKey         = "bootstrap.service_account.name"
	BootstrapServiceAccountClientIDKey     = "bootstrap.service_account.client_id"
	BootstrapServiceAccountClientSecretKey = "bootstrap.service_account.client_secret"
	BootstrapServiceAccountScopesKey       = "bootstrap.service_account.scopes"

	// Persistence configuration keys
	PersistenceTypeKey       = "persistence.type"
	PersistenceSQLitePathKey = "persistence.sqlite.path"

	// Redis configuration keys
	RedisUrlsKey     = "redis.urls"
	RedisPasswordKey = "redis.password"

	// Mailer configuration keys
	SmtpMailerHostKey        = "smtp_mailer.host"
	SmtpMailerPortKey        = "smtp_mailer.port"
	SmtpMailerUsernameKey    = "smtp_mailer.username"
	SmtpMailerPasswordKey    = "smtp_mailer.password"
	SmtpMailerFromEmailKey   = "smtp_mailer.from_email"
	SmtpMailerFromNameKey    = "smtp_mailer.from_name"
	SmtpMailerStartTLSKey    = "smtp_mailer.start_tls"
	SmtpMailerAuthEnabledKey = "smtp_mailer.auth_enabled"
)
