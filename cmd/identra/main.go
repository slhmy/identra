package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/app"
	"github.com/slhmy/identra/internal/bootstrap"
	"github.com/slhmy/identra/internal/buildinfo"
	"github.com/slhmy/identra/internal/config"
	"github.com/slhmy/identra/internal/serviceaccount"
	"github.com/slhmy/identra/internal/store/sqlite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "identra:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		if err := printUsage(stderr); err != nil {
			return err
		}
		return errors.New("a command is required")
	}
	switch args[0] {
	case "serve":
		if len(args) != 1 {
			return errors.New("serve does not accept arguments")
		}
		return serve()
	case "bootstrap":
		return runBootstrap(args[1:], stdout, stderr)
	case "token":
		return runTokenCommand(args[1:], stdout, stderr)
	case "service-account":
		return runServiceAccountCommand(args[1:], stdout, stderr)
	case "audit":
		return runAuditCommand(args[1:], stdout, stderr)
	case "server-info":
		return runServerInfoCommand(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		_, err := fmt.Fprintf(stdout, "identra %s (commit %s, built %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
		return err
	case "help", "--help", "-h":
		return printUsage(stdout)
	default:
		if err := printUsage(stderr); err != nil {
			return err
		}
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) error {
	_, err := fmt.Fprintln(w, `Usage:
  identra serve
  identra bootstrap service-account --name NAME --scope SCOPE [--scope SCOPE...]
  identra token service --client-id ID [--client-secret-file FILE]
  identra service-account create|list|disable|rotate [options]
  identra audit list [options]
  identra server-info [options]
  identra version`)
	return err
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func runBootstrap(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "service-account" {
		return errors.New("usage: identra bootstrap service-account --name NAME --scope SCOPE")
	}
	flags := flag.NewFlagSet("bootstrap service-account", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var scopes stringList
	name := flags.String("name", "", "service-account display name")
	ifNotExists := flags.Bool("if-not-exists", false, "succeed without returning a secret when the name already exists")
	force := flags.Bool("force", false, "allow creation after initial bootstrap has completed")
	output := flags.String("output", "human", "output format: human or json")
	flags.Var(&scopes, "scope", "granted scope; may be repeated")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if *output != "human" && *output != "json" {
		return errors.New("output must be human or json")
	}

	if err := bootstrap.InitE("bootstrap"); err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	persistence := config.LoadPersistence()
	if err := persistence.Validate(); err != nil {
		return fmt.Errorf("invalid persistence config: %w", err)
	}
	db, err := sqlite.Open(persistence.SQLite)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Warn("failed to close bootstrap database", "error", err)
		}
	}()

	result, err := serviceaccount.Bootstrap(context.Background(), sqlite.NewServiceAccountStore(db), serviceaccount.BootstrapRequest{
		Name:        *name,
		Scopes:      scopes,
		IfNotExists: *ifNotExists,
		Force:       *force,
	})
	if err != nil {
		return err
	}
	if *output == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	if !result.Created {
		_, err := fmt.Fprintf(stdout, "Service account %q already exists (client_id: %s); no new secret was created.\n", result.Name, result.ID)
		return err
	}
	_, err = fmt.Fprintf(stdout, `Service account created. Store the client secret now; it will not be shown again.

client_id:     %s
client_secret: %s
scopes:        %s
`, result.ID, result.ClientSecret, strings.Join(result.Scopes, ", "))
	return err
}

// interceptorLogger adapts slog to grpc-middleware logging.
func interceptorLogger(logger *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
		logger.Log(ctx, slog.Level(level), msg, fields...)
	})
}

func serve() error {
	if err := bootstrap.InitE("serve"); err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	ctx, stop := bootstrap.SignalContext(context.Background())
	defer stop()

	cfg := config.LoadGRPC()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid gRPC config: %w", err)
	}
	authService, err := app.NewService(ctx, cfg)
	if err != nil {
		return fmt.Errorf("create identra service: %w", err)
	}
	defer func() {
		if err := authService.Close(context.Background()); err != nil {
			slog.Warn("failed to cleanup service", "error", err)
		}
	}()

	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(
		bootstrap.BuildRequestIDInterceptor(),
		logging.UnaryServerInterceptor(interceptorLogger(slog.Default())),
	))
	identra_v1_pb.RegisterAuthServiceServer(grpcServer, authService)
	identra_v1_pb.RegisterSessionServiceServer(grpcServer, authService)
	identra_v1_pb.RegisterUserServiceServer(grpcServer, authService)
	identra_v1_pb.RegisterKeyServiceServer(grpcServer, authService)
	identra_v1_pb.RegisterServiceAccountServiceServer(grpcServer, authService)
	identra_v1_pb.RegisterAuditServiceServer(grpcServer, authService)
	identra_v1_pb.RegisterSystemServiceServer(grpcServer, authService)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	reflection.Register(grpcServer)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("listen on gRPC port: %w", err)
	}
	slog.Info("gRPC server started", "port", cfg.GRPCPort)
	serveErr := make(chan error, 1)
	go func() { serveErr <- grpcServer.Serve(listener) }()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return fmt.Errorf("serve gRPC: %w", err)
		}
		return nil
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
			return fmt.Errorf("serve gRPC: %w", err)
		}
		slog.Info("gRPC server stopped")
		return nil
	}
}
