package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIncomingLinkWaitsToBeCollected(t *testing.T) {
	// A lasso:// link that launches Lasso arrives before the interface is
	// listening; it has to still be there when the interface asks.
	a := &App{}
	a.openExternal("lasso://open?url=https%3A%2F%2Fvimeo.com%2F1")

	if got := a.TakeIncomingLink(); got != "https://vimeo.com/1" {
		t.Fatalf("TakeIncomingLink = %q, want the link", got)
	}
	if got := a.TakeIncomingLink(); got != "" {
		t.Errorf("TakeIncomingLink = %q the second time, want it opened once", got)
	}
}

func TestRefusedIncomingLinksAreNotHeld(t *testing.T) {
	a := &App{}
	a.openExternal("lasso://open?url=http%3A%2F%2F192.168.1.1%2F")
	if got := a.TakeIncomingLink(); got != "" {
		t.Errorf("TakeIncomingLink = %q, want a local-network link dropped", got)
	}
}

func TestShortcutFilesGiveUpTheirLink(t *testing.T) {
	dir := t.TempDir()

	url := filepath.Join(dir, "Clip.url")
	if err := os.WriteFile(url, []byte("[InternetShortcut]\r\nURL=https://vimeo.com/1\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := shortcutLink(url); err != nil || got != "https://vimeo.com/1" {
		t.Errorf(".url: shortcutLink = %q, %v", got, err)
	}

	// The Finder writes .webloc as a binary property list, which only a
	// plist reader can take apart.
	webloc := filepath.Join(dir, "Clip.webloc")
	for _, args := range [][]string{
		{"-create", "binary1", webloc},
		{"-insert", "URL", "-string", "https://www.youtube.com/watch?v=jNQXAC9IVRw", webloc},
	} {
		if out, err := exec.Command("/usr/bin/plutil", args...).CombinedOutput(); err != nil {
			t.Fatalf("plutil %v: %v: %s", args, err, out)
		}
	}
	if got, err := shortcutLink(webloc); err != nil || got != "https://www.youtube.com/watch?v=jNQXAC9IVRw" {
		t.Errorf(".webloc: shortcutLink = %q, %v", got, err)
	}

	other := filepath.Join(dir, "notes.txt")
	os.WriteFile(other, []byte("https://vimeo.com/1"), 0o644)
	if _, err := shortcutLink(other); err == nil {
		t.Error("a text file is not a shortcut")
	}
}
