package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/app"
	"github.com/slhmy/identra/internal/bootstrap"
	"github.com/slhmy/identra/internal/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func init() {
	bootstrap.Init("grpc")
}

// InterceptorLogger adapts slog logger to interceptor logger.
// This code is simple enough to be copied and not imported.
func InterceptorLogger(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(
		func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
			l.Log(ctx, slog.Level(lvl), msg, fields...)
		},
	)
}

func main() {
	ctx, stop := bootstrap.SignalContext(context.Background())
	defer stop()

	cfg := config.LoadGRPC()

	authService, err := app.NewService(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to create identra service: %v", err)
	}
	defer func() {
		if err := authService.Close(context.Background()); err != nil {
			slog.Warn("failed to cleanup service", "error", err)
		}
	}()

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			bootstrap.BuildRequestIDInterceptor(),
			logging.UnaryServerInterceptor(InterceptorLogger(slog.Default())),
		),
	)
	identra_v1_pb.RegisterIdentraServiceServer(grpcServer, authService)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		log.Fatalf("failed to listen on gRPC port: %v", err)
	}

	slog.Info("gRPC server started", "port", cfg.GRPCPort)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- grpcServer.Serve(lis)
	}()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Fatalf("failed to serve gRPC: %v", err)
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received, stopping gRPC server")
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)

		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			slog.Warn("gRPC graceful shutdown timed out, forcing stop")
			grpcServer.Stop()
		}

		if err := <-serveErr; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Fatalf("failed to serve gRPC: %v", err)
		}
		slog.Info("gRPC server stopped")
	}
}
