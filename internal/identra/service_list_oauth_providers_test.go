package identra

import (
	"context"
	"testing"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"golang.org/x/oauth2"
)

func TestListOAuthProviders_GitHubEnabled(t *testing.T) {
	s := &Service{githubOAuthConfig: &oauth2.Config{
		ClientID: "test-client-id", ClientSecret: "test-client-secret",
	}}

	provider := requireGitHubProvider(t, s)
	if !provider.Enabled {
		t.Error("expected github to be enabled")
	}
	if provider.UnavailableReason != identra_v1_pb.AuthProviderUnavailableReason_AUTH_PROVIDER_UNAVAILABLE_REASON_UNSPECIFIED {
		t.Errorf("expected no unavailable reason, got %s", provider.UnavailableReason)
	}
}

func TestListOAuthProviders_MissingClientID(t *testing.T) {
	s := &Service{githubOAuthConfig: &oauth2.Config{ClientSecret: "test-client-secret"}}
	provider := requireGitHubProvider(t, s)
	if provider.Enabled {
		t.Error("expected github to be disabled")
	}
	if provider.UnavailableReason != identra_v1_pb.AuthProviderUnavailableReason_AUTH_PROVIDER_UNAVAILABLE_REASON_MISSING_CLIENT_ID {
		t.Errorf("expected missing client ID reason, got %s", provider.UnavailableReason)
	}
}

func TestListOAuthProviders_MissingClientSecret(t *testing.T) {
	s := &Service{githubOAuthConfig: &oauth2.Config{ClientID: "test-client-id"}}
	provider := requireGitHubProvider(t, s)
	if provider.Enabled {
		t.Error("expected github to be disabled")
	}
	if provider.UnavailableReason != identra_v1_pb.AuthProviderUnavailableReason_AUTH_PROVIDER_UNAVAILABLE_REASON_MISSING_CLIENT_SECRET {
		t.Errorf("expected missing client secret reason, got %s", provider.UnavailableReason)
	}
}

func requireGitHubProvider(t *testing.T, s *Service) *identra_v1_pb.AuthProviderStatus {
	t.Helper()
	resp, err := s.ListOAuthProviders(context.Background(), &identra_v1_pb.ListOAuthProvidersRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Providers) != 1 {
		t.Fatalf("expected one supported provider, got %d", len(resp.Providers))
	}
	provider := resp.Providers[0]
	if provider.Provider != identra_v1_pb.AuthProvider_AUTH_PROVIDER_GITHUB {
		t.Fatalf("expected github provider, got %s", provider.Provider)
	}
	return provider
}
