package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/slhmy/identra/internal/serviceaccount"
)

func TestVersionCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"version"}, &stdout, &stderr); err != nil {
		t.Fatalf("version: %v", err)
	}
	if got := stdout.String(); got != "identra dev (commit unknown, built unknown)\n" {
		t.Fatalf("version output = %q", got)
	}
}

func TestReadSecretPrefersFileAndSupportsEnvironment(t *testing.T) {
	t.Setenv("IDENTRA_TEST_SECRET", "environment-secret")
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(" file-secret\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if got, err := readSecret(path, "IDENTRA_TEST_SECRET"); err != nil || got != "file-secret" {
		t.Fatalf("file secret = %q, error=%v", got, err)
	}
	if got, err := readSecret("", "IDENTRA_TEST_SECRET"); err != nil || got != "environment-secret" {
		t.Fatalf("environment secret = %q, error=%v", got, err)
	}
}

func TestTokenCommandRequiresSecretBeforeConnecting(t *testing.T) {
	t.Setenv("IDENTRA_CLIENT_SECRET", "")
	var stdout, stderr bytes.Buffer
	err := run([]string{"token", "service", "--client-id", "isa_test"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected missing secret error")
	}
}

func TestBootstrapServiceAccountCommandJSON(t *testing.T) {
	t.Setenv("PERSISTENCE_TYPE", "sqlite")
	t.Setenv("PERSISTENCE_SQLITE_PATH", filepath.Join(t.TempDir(), "identra.db"))
	var stdout, stderr bytes.Buffer
	err := run([]string{
		"bootstrap", "service-account",
		"--name", "platform-admin",
		"--scope", "identra.admin",
		"--output", "json",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("bootstrap command: %v\nstderr: %s", err, stderr.String())
	}
	var result serviceaccount.BootstrapResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	if !result.Created || result.ID == "" || result.ClientSecret == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
