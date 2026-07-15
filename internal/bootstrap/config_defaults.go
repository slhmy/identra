package bootstrap

import "github.com/spf13/viper"

func applyConfigDefaults(v *viper.Viper) {
	v.SetDefault("grpc_port", 50051)
	v.SetDefault("http_port", 8080)
	v.SetDefault("grpc_endpoint", "localhost:50051")
	v.SetDefault("cors.allowed_origins", []string{
		"http://localhost:3000",
		"http://localhost:5173",
		"http://localhost:8080",
	})
	v.SetDefault("cors.allow_credentials", true)

	v.SetDefault(configKeyLogLevel, "info")
	v.SetDefault(configKeyLogFormat, logFormatTint)

	v.SetDefault("auth.oauth_state_expiration", "10m")
	v.SetDefault("auth.access_token_expiration", "15m")
	v.SetDefault("auth.refresh_token_expiration", "168h")
	v.SetDefault("auth.token_issuer", "identra")
	v.SetDefault("auth.oauth.fetch_email_if_missing", false)

	v.SetDefault("redis.urls", []string{"localhost:6379"})
	v.SetDefault("smtp_mailer.start_tls", true)
	v.SetDefault("smtp_mailer.auth_enabled", true)

	v.SetDefault("persistence.type", "sqlite")
	v.SetDefault("persistence.sqlite.path", "data/users.db")
}
