package main

import (
	"bytes"
	"encoding/json"
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
