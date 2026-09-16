package binaries

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSourceHonoursOverride(t *testing.T) {
	dir := newFakeSource(t, fakeVersions)
	t.Setenv(SourceEnv, dir)

	got, err := ResolveSource()
	if err != nil {
		t.Fatalf("ResolveSource: %v", err)
	}
	if got != dir {
		t.Errorf("ResolveSource = %q, want %q", got, dir)
	}
}

func TestResolveSourceRejectsBadOverride(t *testing.T) {
	t.Setenv(SourceEnv, t.TempDir())

	_, err := ResolveSource()
	if err == nil {
		t.Fatal("ResolveSource succeeded with an override that holds no manifest")
	}
	if !strings.Contains(err.Error(), SourceEnv) {
		t.Errorf("error = %q, want it to name the override variable", err)
	}
}

func TestSupportDirHonoursOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(SupportEnv, dir)

	got, err := SupportDir()
	if err != nil {
		t.Fatalf("SupportDir: %v", err)
	}
	if got != dir {
		t.Errorf("SupportDir = %q, want %q", got, dir)
	}
}

func TestSupportDirDefaultsToApplicationSupport(t *testing.T) {
	t.Setenv(SupportEnv, "")

	got, err := SupportDir()
	if err != nil {
		t.Fatalf("SupportDir: %v", err)
	}
	if want := filepath.Join("Library", "Application Support", AppName); !strings.HasSuffix(got, want) {
		t.Errorf("SupportDir = %q, want it to end in %q", got, want)
	}
}

func TestResolvePathsPutsBinUnderSupport(t *testing.T) {
	support := t.TempDir()
	t.Setenv(SupportEnv, support)
	t.Setenv(SourceEnv, newFakeSource(t, fakeVersions))

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	if paths.Bin != filepath.Join(support, "bin") {
		t.Errorf("Bin = %q", paths.Bin)
	}
}

func TestSourceMissingErrorIsActionable(t *testing.T) {
	err := &SourceMissingError{Tried: []string{"/Applications/Lasso.app/Contents/Resources/bin"}}
	if !strings.Contains(err.Error(), "make setup") {
		t.Errorf("Error() = %q, want it to suggest a fix", err)
	}
	if !strings.Contains(err.Error(), "/Applications/Lasso.app") {
		t.Errorf("Error() = %q, want it to list where it looked", err)
	}
	if msg := UserMessage(err); strings.Contains(msg, "\n") {
		t.Errorf("UserMessage = %q, want a single sentence for the UI", msg)
	}
}

func TestStartupInstallsAndVerifies(t *testing.T) {
	support := t.TempDir()
	t.Setenv(SupportEnv, support)
	t.Setenv(SourceEnv, newFakeSource(t, fakeVersions))

	m, status, err := Startup(context.Background())
	if err != nil {
		t.Fatalf("Startup: %v", err)
	}
	if !status.Ready {
		t.Fatalf("not ready, problems: %+v", status.Problems)
	}
	if !status.FirstRun {
		t.Error("FirstRun = false on a fresh install")
	}
	if status.BinDir != filepath.Join(support, "bin") {
		t.Errorf("BinDir = %q", status.BinDir)
	}
	for _, name := range requiredBinaries {
		if status.Versions[name] == "" {
			t.Errorf("no version reported for %s", name)
		}
	}

	// A second launch installs nothing and stays ready.
	_, status2, err := Startup(context.Background())
	if err != nil {
		t.Fatalf("second Startup: %v", err)
	}
	if status2.FirstRun {
		t.Error("FirstRun = true on a second launch")
	}
	if !status2.Ready {
		t.Errorf("not ready on second launch: %+v", status2.Problems)
	}
	_ = m
}

func TestStartupReportsProblemsWithoutFailing(t *testing.T) {
	support := t.TempDir()
	t.Setenv(SupportEnv, support)
	t.Setenv(SourceEnv, newFakeSource(t, fakeVersions))

	m, _, err := Startup(context.Background())
	if err != nil {
		t.Fatalf("Startup: %v", err)
	}
	// Break a binary the way a failed signature check would.
	broken := "#!/bin/sh\necho 'killed: 9' >&2\nexit 137\n"
	if err := os.WriteFile(m.Path(Deno), []byte(broken), 0o755); err != nil {
		t.Fatal(err)
	}

	_, status, err := Startup(context.Background())
	if err != nil {
		t.Fatalf("Startup returned a hard error for a single broken binary: %v", err)
	}
	if status.Ready {
		t.Fatal("Ready = true with a broken binary")
	}
	if len(status.Problems) != 1 || status.Problems[0].Name != Deno {
		t.Fatalf("problems = %+v, want one for deno", status.Problems)
	}
	if status.Problems[0].Message == "" || status.Problems[0].Detail == "" {
		t.Error("problem is missing its message or detail")
	}
}

func TestStartupFailsHardWhenSourceIsMissing(t *testing.T) {
	t.Setenv(SupportEnv, t.TempDir())
	t.Setenv(SourceEnv, t.TempDir())

	_, status, err := Startup(context.Background())
	if err == nil {
		t.Fatal("Startup succeeded with no bundled binaries")
	}
	if status.Ready {
		t.Error("Ready = true with no bundled binaries")
	}
	if len(status.Problems) == 0 {
		t.Error("no problem reported for a missing source")
	}
}
