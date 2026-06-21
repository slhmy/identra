package identra

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func (s *Service) GetJWKS(ctx context.Context, _ *identra_v1_pb.GetJWKSRequest) (*identra_v1_pb.GetJWKSResponse, error) {
	response := s.keyManager.GetJWKS()

	// Generate ETag based on hash of key IDs in the response
	// This allows clients to efficiently check if keys have changed
	etag := generateJWKSETag(response)

	// Set HTTP cache headers via gRPC metadata
	// Cache-Control: public, max-age=3600 (1 hour)
	// This allows clients to cache the JWKS and reduces load on the server
	md := metadata.Pairs(
		"Cache-Control", "public, max-age=3600",
		"ETag", etag,
	)
	if err := grpc.SetHeader(ctx, md); err != nil {
		// Log error but don't fail the request
		slog.Warn("failed to set JWKS cache headers", "error", err)
	}

	return response, nil
}

// generateJWKSETag creates an ETag based on the key IDs in the JWKS response.
// This allows clients to efficiently check if the key set has changed.

func generateJWKSETag(jwks *identra_v1_pb.GetJWKSResponse) string {
	if jwks == nil || len(jwks.Keys) == 0 {
		return `"empty"`
	}

	// Join all key IDs with a delimiter to avoid ambiguous concatenations, then hash them
	keyIDs := make([]string, 0, len(jwks.Keys))
	for _, key := range jwks.Keys {
		keyIDs = append(keyIDs, key.Kid)
	}

	hash := sha256.Sum256([]byte(strings.Join(keyIDs, ",")))
	// Use the full 32 bytes (256 bits) of the SHA-256 hash to minimize collision risk
	// Quoted per HTTP ETag specification (RFC 7232)
	return fmt.Sprintf(`"%x"`, hash[:])
}
