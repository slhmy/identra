package bootstrap

import (
	"log/slog"
)

// logConfig logs the key configuration values when the server starts.
// Sensitive values like passwords and secrets are not logged.
func logConfig() {
	slog.Info("Configuration loaded",
		"log.level", config.GetString(configKeyLogLevel),
		"log.format", config.GetString(configKeyLogFormat),
	)

	if cmdName == "serve" {
		logGRPCConfig()
	}
}

func logGRPCConfig() {
	slog.Info("gRPC server configuration",
		"grpc_port", config.GetUint("grpc_port"),
		"persistence.type", config.GetString("persistence.type"),
		"persistence.sqlite.path", config.GetString("persistence.sqlite.path"),
		"redis.urls", config.GetStringSlice("redis.urls"),
		"smtp_mailer.host", config.GetString("smtp_mailer.host"),
		"smtp_mailer.port", config.GetInt("smtp_mailer.port"),
		"auth.oauth_state_expiration", config.GetString("auth.oauth_state_expiration"),
		"auth.access_token_expiration", config.GetString("auth.access_token_expiration"),
		"auth.refresh_token_expiration", config.GetString("auth.refresh_token_expiration"),
		"auth.token_issuer", config.GetString("auth.token_issuer"),
	)
}
