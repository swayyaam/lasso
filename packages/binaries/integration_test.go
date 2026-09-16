package binaries

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestRealBinariesInstallAndRun exercises the whole startup path against the
// binaries `make setup` actually fetched — real Mach-O images, real quarantine
// and signature handling, real Gatekeeper warm-up.
//
// It is skipped unless LASSO_INTEGRATION is set, because it needs those
// binaries present and copies ~330 MB.
//
//	LASSO_INTEGRATION=1 go test ./... -run Real -v
func TestRealBinariesInstallAndRun(t *testing.T) {
	if os.Getenv("LASSO_INTEGRATION") == "" {
		t.Skip("set LASSO_INTEGRATION=1 to run against the fetched binaries")
	}

	source := repoBinDir(t)
	if _, err := os.Stat(filepath.Join(source, manifestName)); err != nil {
		t.Skipf("no fetched binaries at %s — run `make setup`", source)
	}

	t.Setenv(SourceEnv, source)
	t.Setenv(SupportEnv, t.TempDir())

	m, status, err := Startup(context.Background())
	if err != nil {
		t.Fatalf("Startup: %v", err)
	}
	if !status.Ready {
		for _, p := range status.Problems {
			t.Errorf("%s: %s\n%s", p.Name, p.Message, p.Detail)
		}
		t.Fatal("sidecar binaries are not ready")
	}

	for _, name := range requiredBinaries {
		t.Logf("%-8s %s", name, status.Versions[name])
	}

	// Second launch must skip the copy entirely and still verify clean.
	_, again, err := Startup(context.Background())
	if err != nil {
		t.Fatalf("second Startup: %v", err)
	}
	if again.FirstRun {
		t.Error("FirstRun = true on a second launch")
	}
	if !again.Ready {
		t.Errorf("not ready on second launch: %+v", again.Problems)
	}

	// The warm path is what every download pays.
	for _, r := range m.Verify(context.Background()) {
		t.Logf("warm %-8s %v", r.Name, r.Duration.Round(1e6))
	}
}

func repoBinDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	repo := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Clean(filepath.Join(repo, "apps", "desktop", "build", "bin", "darwin-"+runtime.GOARCH))
}
