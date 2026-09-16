package binaries

import (
	"context"
	"os"
	"strings"
	"testing"
)

// selfUpdatingYtDlp mimics `yt-dlp -U`: it rewrites its own file to report a
// new version, the way the real updater swaps itself out on disk.
const selfUpdatingYtDlp = `#!/bin/sh
if [ "$1" = "-U" ]; then
  printf '#!/bin/sh\necho "2026.09.01"\n' > "$0"
  chmod +x "$0"
  echo "Updated yt-dlp to stable@2026.09.01"
  exit 0
fi
echo "2026.08.19"
`

func installedManager(t *testing.T) *Manager {
	t.Helper()
	m, _ := newFakeManager(t)
	if _, err := m.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}
	return m
}

func TestUpdateYtDlpReportsNewVersion(t *testing.T) {
	m := installedManager(t)
	if err := os.WriteFile(m.Path(YtDlp), []byte(selfUpdatingYtDlp), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := m.UpdateYtDlp(context.Background())
	if err != nil {
		t.Fatalf("UpdateYtDlp: %v", err)
	}
	if result.VersionBefore != "2026.08.19" {
		t.Errorf("VersionBefore = %q", result.VersionBefore)
	}
	if result.VersionAfter != "2026.09.01" {
		t.Errorf("VersionAfter = %q", result.VersionAfter)
	}
	if !result.Updated {
		t.Error("Updated = false after the version changed")
	}
	if !strings.Contains(result.Output, "Updated yt-dlp") {
		t.Errorf("Output = %q, want yt-dlp's own text kept for the details toggle", result.Output)
	}
	if msg := result.UserMessage(); !strings.Contains(msg, "2026.09.01") {
		t.Errorf("UserMessage = %q", msg)
	}
}

func TestUpdateYtDlpReappliesFixups(t *testing.T) {
	m := installedManager(t)
	// An updater that drops the executable bit, as an unlucky write can.
	dropsExecBit := `#!/bin/sh
if [ "$1" = "-U" ]; then
  printf '#!/bin/sh\necho "2026.09.01"\n' > "$0"
  chmod 644 "$0"
  echo "updated"
  exit 0
fi
echo "2026.08.19"
`
	if err := os.WriteFile(m.Path(YtDlp), []byte(dropsExecBit), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := m.UpdateYtDlp(context.Background()); err != nil {
		t.Fatalf("UpdateYtDlp: %v", err)
	}
	// Without the post-update chmod, the next launch would fail to start yt-dlp.
	mustBeExecutable(t, m.Path(YtDlp))
}

func TestUpdateYtDlpWhenAlreadyCurrent(t *testing.T) {
	m := installedManager(t)
	noop := "#!/bin/sh\nif [ \"$1\" = \"-U\" ]; then echo 'yt-dlp is up to date'; exit 0; fi\necho '2026.08.19'\n"
	if err := os.WriteFile(m.Path(YtDlp), []byte(noop), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := m.UpdateYtDlp(context.Background())
	if err != nil {
		t.Fatalf("UpdateYtDlp: %v", err)
	}
	if result.Updated {
		t.Error("Updated = true when the version did not change")
	}
	if msg := result.UserMessage(); !strings.Contains(msg, "already up to date") {
		t.Errorf("UserMessage = %q", msg)
	}
}

func TestUpdateYtDlpSurfacesFailure(t *testing.T) {
	m := installedManager(t)
	failing := "#!/bin/sh\nif [ \"$1\" = \"-U\" ]; then echo 'network unreachable' >&2; exit 1; fi\necho '2026.08.19'\n"
	if err := os.WriteFile(m.Path(YtDlp), []byte(failing), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := m.UpdateYtDlp(context.Background())
	if err == nil {
		t.Fatal("UpdateYtDlp succeeded despite a failing update")
	}
	if !strings.Contains(result.Output, "network unreachable") {
		t.Errorf("Output = %q, want the raw failure kept", result.Output)
	}
	if UserMessage(err) == "" {
		t.Error("no user-facing message for a failed update")
	}
}

func TestUpdateYtDlpFailsWhenNotInstalled(t *testing.T) {
	m, _ := newFakeManager(t)
	// Never installed.
	if _, err := m.UpdateYtDlp(context.Background()); err == nil {
		t.Fatal("UpdateYtDlp succeeded with yt-dlp missing")
	}
}
