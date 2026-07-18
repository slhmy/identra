package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRootWorkdirUsesNearestConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("grpc_port = 50051\n"), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	nested := filepath.Join(root, "cmd", "identra")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	if got := findRootWorkdir(nested); got != root {
		t.Fatalf("expected root workdir %q, got %q", root, got)
	}
}

func TestFindRootWorkdirFallsBackToStart(t *testing.T) {
	start := t.TempDir()

	if got := findRootWorkdir(start); got != start {
		t.Fatalf("expected fallback workdir %q, got %q", start, got)
	}
}

func TestInitWithConfigPathEReturnsConfigReadErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("grpc_port = \n"), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	if err := InitWithConfigPathE("test", root); err == nil {
		t.Fatal("expected malformed config to return an error")
	}
}
