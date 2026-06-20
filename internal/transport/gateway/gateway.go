package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	identra_v1_pb "github.com/poly-workshop/identra/gen/go/identra/v1"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Gateway wraps the grpc-gateway mux and provides HTTP endpoints for gRPC services.
type Gateway struct {
	mux                  *runtime.ServeMux
	grpcConn             *grpc.ClientConn
	staticDir            string
	apiPrefix            string
	corsAllowedOrigins   []string
	corsAllowCredentials bool
}

// New creates a gateway instance.
func New(
	grpcEndpoint,
	staticDir,
	apiPrefix string,
	corsAllowedOrigins []string,
	corsAllowCredentials bool,
) (*Gateway, error) {
	if err := validateCORSConfig(corsAllowedOrigins, corsAllowCredentials); err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(grpcEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
			switch strings.ToLower(key) {
			case "authorization":
				return key, true
			case "x-client-id":
				return key, true
			case "x-client-secret":
				return key, true
			case "x-forwarded-for":
				return key, true
			case "x-real-ip":
				return key, true
			default:
				return "", false
			}
		}),
		runtime.WithOutgoingHeaderMatcher(func(key string) (string, bool) {
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

func (g *Gateway) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (g *Gateway) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	client := healthpb.NewHealthClient(g.grpcConn)
	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not_ready"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

// Handler returns an HTTP handler with CORS support and static file serving.
func (g *Gateway) Handler() http.Handler {
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

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", g.handleHealthz)
	mux.HandleFunc("/readyz", g.handleReadyz)

	mux.Handle(g.apiPrefix, http.StripPrefix(strings.TrimSuffix(g.apiPrefix, "/"), g.mux))

	if g.staticDir != "" {
		if _, err := os.Stat(g.staticDir); err == nil {
			fileServer := http.FileServer(http.Dir(g.staticDir))
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				path := filepath.Join(g.staticDir, r.URL.Path)
				if _, err := os.Stat(path); os.IsNotExist(err) {
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
			mux.Handle("/", g.mux)
		}
	} else {
		mux.Handle("/", g.mux)
	}

	return c.Handler(mux)
}

// Close closes the gRPC connection.
func (g *Gateway) Close() error {
	if g.grpcConn != nil {
		return g.grpcConn.Close()
	}
	return nil
}
