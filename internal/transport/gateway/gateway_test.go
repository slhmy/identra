package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateCORSConfigRejectsWildcardWithCredentials(t *testing.T) {
	err := validateCORSConfig([]string{"https://app.example.com", "*"}, true)
	if err == nil {
		t.Fatal("expected wildcard origin with credentials to be rejected")
	}
}

func TestValidateCORSConfigAllowsWildcardWithoutCredentials(t *testing.T) {
	if err := validateCORSConfig([]string{"*"}, false); err != nil {
		t.Fatalf("expected wildcard without credentials to be valid: %v", err)
	}
}

func TestHandleHealthz(t *testing.T) {
	g := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	g.handleHealthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != `{"status":"ok"}` {
		t.Fatalf("expected ok body, got %q", got)
	}
}
