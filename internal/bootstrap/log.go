package bootstrap

import (
	"context"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
)

type contextKey string

const (
	configKeyLogLevel  = "log.level"
	configKeyLogFormat = "log.format"

	logFormatJSON      = "json"
	logFormatPlainText = "plain-text"
	logFormatTint      = "tint"

	logAttrsKey contextKey = "log_attrs"
)

func stringToSlogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type logHandler struct {
	slog.Handler
}

// WithLogAttrs adds additional log attributes to the context.
func WithLogAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	existingAttrs, ok := ctx.Value(logAttrsKey).([]slog.Attr)
	if !ok {
		existingAttrs = []slog.Attr{}
	}
	return context.WithValue(ctx, logAttrsKey, append(existingAttrs, attrs...))
}

func (h *logHandler) Handle(ctx context.Context, r slog.Record) error {
	if cmdName != "" {
		r.AddAttrs(slog.String("cmd", cmdName))
	}
	if hostname != "" {
		r.AddAttrs(slog.String("hostname", hostname))
	}

	// Add log fields from context
	if attrs, ok := ctx.Value(logAttrsKey).([]slog.Attr); ok {
		for _, v := range attrs {
			r.AddAttrs(v)
		}
	}
	return h.Handler.Handle(ctx, r)
}

func initLog() {
	logLevel := stringToSlogLevel(config.GetString(configKeyLogLevel))

	var handler slog.Handler
	switch config.GetString(configKeyLogFormat) {
	case logFormatJSON:
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	case logFormatPlainText:
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	case logFormatTint:
		fallthrough
	default:
		handler = tint.NewHandler(os.Stderr, &tint.Options{Level: logLevel})
	}
	handler = &logHandler{handler}
	slog.SetDefault(slog.New(handler))
}
