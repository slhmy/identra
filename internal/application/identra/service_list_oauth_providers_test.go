package identra

import (
	"context"
	"testing"

	"golang.org/x/oauth2"
)

func TestListOAuthProviders_GitHubEnabled(t *testing.T) {
	s := &Service{
		githubOAuthConfig: &oauth2.Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		},
	}

	resp, err := s.ListOAuthProviders(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Providers) == 0 {
		t.Fatal("expected at least one provider")
	}

	var found bool
	for _, p := range resp.Providers {
		if p.Name == "github" {
			found = true
			if !p.Enabled {
				t.Error("expected github to be enabled")
			}
			if p.Reason != nil {
				t.Errorf("expected no reason, got %q", *p.Reason)
			}
		}
	}
	if !found {
		t.Error("github provider not found in response")
	}
}

func TestListOAuthProviders_MissingClientID(t *testing.T) {
	s := &Service{
		githubOAuthConfig: &oauth2.Config{
			ClientID:     "",
			ClientSecret: "test-client-secret",
		},
	}

	resp, err := s.ListOAuthProviders(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, p := range resp.Providers {
		if p.Name == "github" {
			if p.Enabled {
				t.Error("expected github to be disabled")
			}
			if p.Reason == nil || *p.Reason != "missing_client_id" {
				t.Errorf("expected reason missing_client_id, got %v", p.Reason)
			}
			return
		}
	}
	t.Error("github provider not found in response")
}

func TestListOAuthProviders_MissingClientSecret(t *testing.T) {
	s := &Service{
		githubOAuthConfig: &oauth2.Config{
			ClientID:     "test-client-id",
			ClientSecret: "",
		},
	}

	resp, err := s.ListOAuthProviders(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, p := range resp.Providers {
		if p.Name == "github" {
			if p.Enabled {
				t.Error("expected github to be disabled")
			}
			if p.Reason == nil || *p.Reason != "missing_client_secret" {
				t.Errorf("expected reason missing_client_secret, got %v", p.Reason)
			}
			return
		}
	}
	t.Error("github provider not found in response")
}

func TestListOAuthProviders_AllSupportedProvidersReturned(t *testing.T) {
	s := &Service{
		githubOAuthConfig: &oauth2.Config{},
	}

	resp, err := s.ListOAuthProviders(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Providers) != len(supportedProviders) {
		t.Errorf("expected %d providers, got %d", len(supportedProviders), len(resp.Providers))
	}

	providerNames := make(map[string]bool)
	for _, p := range resp.Providers {
		providerNames[p.Name] = true
	}
	for name := range supportedProviders {
		if !providerNames[name] {
			t.Errorf("expected provider %q to be in response", name)
		}
	}
}
