package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/poly-workshop/identra/internal/infrastructure/bootstrap"
	"github.com/poly-workshop/identra/internal/infrastructure/configs"
	"github.com/poly-workshop/identra/internal/transport/gateway"
)

func init() {
	bootstrap.Init("gateway")
}

func main() {
	cfg := configs.LoadGateway()

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

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("failed to serve HTTP: %v", err)
	}
}
