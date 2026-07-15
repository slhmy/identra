package config

// Configuration keys constants
const (
	// Server configuration keys
	GRPCPortKey     = "grpc_port"
	HTTPPortKey     = "http_port"
	GRPCEndpointKey = "grpc_endpoint"

	// CORS configuration keys
	CORSAllowedOriginsKey   = "cors.allowed_origins"
	CORSAllowCredentialsKey = "cors.allow_credentials"

	// Auth configuration keys
	AuthRSAPrivateKeyKey            = "auth.rsa_private_key"
	AuthOAuthStateExpirationKey     = "auth.oauth_state_expiration"
	AuthAccessTokenExpirationKey    = "auth.access_token_expiration"
	AuthRefreshTokenExpirationKey   = "auth.refresh_token_expiration"
	AuthTokenIssuerKey              = "auth.token_issuer"
	AuthOAuthFetchEmailIfMissingKey = "auth.oauth.fetch_email_if_missing"
	AuthGithubClientIDKey           = "auth.github.client_id"
	AuthGithubClientSecretKey       = "auth.github.client_secret"

	// Persistence configuration keys
	PersistenceTypeKey            = "persistence.type"
	PersistenceGORMDriverKey      = "persistence.gorm.driver"
	PersistenceGORMHostKey        = "persistence.gorm.host"
	PersistenceGORMPortKey        = "persistence.gorm.port"
	PersistenceGORMUsernameKey    = "persistence.gorm.username"
	PersistenceGORMPasswordKey    = "persistence.gorm.password"
	PersistenceGORMDBNameKey      = "persistence.gorm.dbname"
	PersistenceGORMSSLModeKey     = "persistence.gorm.sslmode"
	PersistenceMongoURIKey        = "persistence.mongo.uri"
	PersistenceMongoDatabaseKey   = "persistence.mongo.database"
	PersistenceMongoCollectionKey = "persistence.mongo.collection"

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
