package arch_test

import (
	"os/exec"
	"strings"
	"testing"
)

const corePackage = "github.com/slhmy/identra/internal/identra"

var forbiddenCoreImports = map[string]string{
	"github.com/slhmy/identra/internal/app":       "application assembly belongs outside the core service",
	"github.com/slhmy/identra/internal/bootstrap": "process bootstrap belongs outside the core service",
	"github.com/slhmy/identra/internal/cache":     "cache implementations belong outside the core service",
	"github.com/slhmy/identra/internal/config":    "startup configuration belongs outside the core service",
	"github.com/slhmy/identra/internal/mail":      "mailer implementations belong outside the core service",
	"github.com/slhmy/identra/internal/oauth":     "OAuth infrastructure belongs outside the core service",
	"github.com/slhmy/identra/internal/store":     "persistence implementations belong outside the core service",
}

func TestIdentraCoreDoesNotImportInfrastructure(t *testing.T) {
	imports := listPackageImports(t, corePackage)
	for _, imp := range imports {
		for forbidden, reason := range forbiddenCoreImports {
			if imp == forbidden || strings.HasPrefix(imp, forbidden+"/") {
				t.Fatalf("%s imports forbidden package %s: %s", corePackage, imp, reason)
			}
		}
	}
}

func listPackageImports(t *testing.T, pkg string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}{{\"\\n\"}}{{end}}", pkg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", pkg, err, out)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}
