package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestOnlyOneCopyHoldsTheSupportFolder(t *testing.T) {
	support := t.TempDir()

	first, _, err := acquireInstance(support)
	if err != nil {
		t.Fatalf("the first copy could not take the lock: %v", err)
	}

	// flock conflicts between separate opens even within one process, so a
	// second acquire here stands in for a second copy of the app.
	_, holder, err := acquireInstance(support)
	if !errors.Is(err, errInstanceHeld) {
		t.Fatalf("a second copy was allowed to run: %v", err)
	}
	if holder != os.Getpid() {
		t.Errorf("the second copy was told the holder is %d, want %d, so it could bring it up", holder, os.Getpid())
	}

	// The updater's restart: the old copy lets go before the new one starts.
	first.Release()
	second, _, err := acquireInstance(support)
	if err != nil {
		t.Fatalf("after a release the new copy still could not start: %v", err)
	}
	second.Release()
	second.Release() // safe twice
}

func TestSeparateSupportFoldersDoNotConflict(t *testing.T) {
	// LASSO_SUPPORT_DIR points a copy somewhere else entirely; it shares
	// nothing, so it may run beside the normal one.
	a, _, err := acquireInstance(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	b, _, err := acquireInstance(t.TempDir())
	if err != nil {
		t.Errorf("a copy with its own support folder was refused: %v", err)
	}
	b.Release()
}

func TestTheLockRecordsItsHolder(t *testing.T) {
	support := t.TempDir()
	lock, _, err := acquireInstance(support)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	raw, _ := os.ReadFile(filepath.Join(support, instanceLockFile))
	if strings.TrimSpace(string(raw)) != strconv.Itoa(os.Getpid()) {
		t.Errorf("lock file holds %q, want this process's PID", raw)
	}
}
