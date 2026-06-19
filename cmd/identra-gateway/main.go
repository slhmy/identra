package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	identra_v1_pb "github.com/poly-workshop/identra/gen/go/identra/v1"
	"github.com/poly-workshop/identra/internal/infrastructure/bootstrap"
	"github.com/poly-workshop/identra/internal/infrastructure/configs"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func init() {
	bootstrap.Init("gateway")
}

// Gateway wraps the grpc-gateway mux and provides HTTP endpoints for gRPC services
type Gateway struct {
	mux                  *runtime.ServeMux
	grpcConn             *grpc.ClientConn
	staticDir            string
	apiPrefix            string
	corsAllowedOrigins   []string
	corsAllowCredentials bool
}

// NewGateway creates a new gateway instance
func NewGateway(
	grpcEndpoint,
	staticDir,
	apiPrefix string,
	corsAllowedOrigins []string,
	corsAllowCredentials bool,
) (*Gateway, error) {
	if err := validateCORSConfig(corsAllowedOrigins, corsAllowCredentials); err != nil {
		return nil, err
	}

	// Create gRPC connection
	conn, err := grpc.NewClient(grpcEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	// Create gateway mux with custom options
	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
			switch strings.ToLower(key) {
			case "authorization":
				return key, true
			case "x-client-id":
				return key, true
			case "x-client-secret":
				return key, true
			default:
				return "", false
			}
		}),
		runtime.WithOutgoingHeaderMatcher(func(key string) (string, bool) {
			// Allow cache-related headers to pass through to HTTP response
			switch strings.ToLower(key) {
			case "cache-control":
				return "Cache-Control", true
			case "etag":
				return "ETag", true
			default:
				return "", false
			}
		}),
	)

	// Register services
	ctx := context.Background()
	if err := identra_v1_pb.RegisterIdentraServiceHandler(ctx, mux, conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to register identra service handler: %w", err)
	}

	return &Gateway{
		mux:                  mux,
		grpcConn:             conn,
		staticDir:            staticDir,
		apiPrefix:            apiPrefix,
		corsAllowedOrigins:   corsAllowedOrigins,
		corsAllowCredentials: corsAllowCredentials,
	}, nil
}

func validateCORSConfig(allowedOrigins []string, allowCredentials bool) error {
	if !allowCredentials {
		return nil
	}
	for _, origin := range allowedOrigins {
		if strings.TrimSpace(origin) == "*" {
			return fmt.Errorf("cors.allowed_origins cannot contain * when cors.allow_credentials is true")
		}
	}
	return nil
}

// Handler returns an HTTP handler with CORS support and static file serving
func (g *Gateway) Handler() http.Handler {
	// Setup CORS
	c := cors.New(cors.Options{
		AllowedOrigins: g.corsAllowedOrigins,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Accept-Language",
			"Content-Language",
			"Content-Type",
			"Authorization",
			"X-Client-Id",
			"X-Client-Secret",
		},
		ExposedHeaders:   []string{},
		AllowCredentials: g.corsAllowCredentials,
	})

	// Create a multiplexer that handles both API and static files
	mux := http.NewServeMux()

	// Handle API routes with the gRPC gateway
	mux.Handle(g.apiPrefix, http.StripPrefix(strings.TrimSuffix(g.apiPrefix, "/"), g.mux))

	// Handle static files for the frontend
	if g.staticDir != "" {
		// Check if static directory exists
		if _, err := os.Stat(g.staticDir); err == nil {
			// Serve static files, with index.html as fallback for SPA
			fileServer := http.FileServer(http.Dir(g.staticDir))
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				// Check if the requested file exists
				path := filepath.Join(g.staticDir, r.URL.Path)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					// If file doesn't exist and it's not an API request, serve index.html for SPA routing
					if !strings.HasPrefix(r.URL.Path, strings.TrimSuffix(g.apiPrefix, "/")) {
						http.ServeFile(w, r, filepath.Join(g.staticDir, "index.html"))
						return
					}
				}
				fileServer.ServeHTTP(w, r)
			})
			slog.Info("Static file serving enabled", "directory", g.staticDir)
		} else {
			slog.Warn("Static directory not found, serving API only", "directory", g.staticDir)
			// If static directory doesn't exist, just serve the API
			mux.Handle("/", g.mux)
		}
	} else {
		// If no static directory specified, just serve the API
		mux.Handle("/", g.mux)
	}

	return c.Handler(mux)
}

// Close closes the gRPC connection
func (g *Gateway) Close() error {
	if g.grpcConn != nil {
		return g.grpcConn.Close()
	}
	return nil
}

func main() {
	cfg := configs.LoadGateway()

	// Get current working directory to locate frontend dist
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get current working directory: %v", err)
	}

	// Static directory path (relative to project root)
	staticDir := filepath.Join(cwd, "frontend", "dist")
	apiPrefix := "/api/"

	// Create gateway instance
	gateway, err := NewGateway(
		cfg.GRPCEndpoint,
		staticDir,
		apiPrefix,
		cfg.CORS.AllowedOrigins,
		cfg.CORS.AllowCredentials,
	)
	if err != nil {
		log.Fatalf("failed to create gateway: %v", err)
	}
	defer func() { _ = gateway.Close() }()

	// Create HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: gateway.Handler(),
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
