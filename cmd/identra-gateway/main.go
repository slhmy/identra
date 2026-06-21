package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/slhmy/identra/internal/bootstrap"
	"github.com/slhmy/identra/internal/config"
	"github.com/slhmy/identra/internal/gateway"
)

func init() {
	bootstrap.Init("gateway")
}

func main() {
	ctx, stop := bootstrap.SignalContext(context.Background())
	defer stop()

	cfg := config.LoadGateway()

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get current working directory: %v", err)
	}

	staticDir := filepath.Join(cwd, "frontend", "dist")
	apiPrefix := "/api/"

	gw, err := gateway.New(
		cfg.GRPCEndpoint,
		staticDir,
		apiPrefix,
		cfg.CORS.AllowedOrigins,
		cfg.CORS.AllowCredentials,
	)
	if err != nil {
		log.Fatalf("failed to create gateway: %v", err)
	}
	defer func() { _ = gw.Close() }()

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: gw.Handler(),
	}

	slog.Info("HTTP gateway server started",
		"port", cfg.HTTPPort,
		"grpc_endpoint", cfg.GRPCEndpoint,
		"static_dir", staticDir,
		"api_prefix", apiPrefix)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("failed to serve HTTP: %v", err)
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received, stopping HTTP gateway")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("failed to shutdown HTTP gateway: %v", err)
		}

		if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("failed to serve HTTP: %v", err)
		}
		slog.Info("HTTP gateway stopped")
	}
}
