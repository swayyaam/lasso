package main

import (
	"strings"
	"testing"
)

func TestDocumentsAreFixedAndOnTheProjectsOwnPages(t *testing.T) {
	for name, url := range documents {
		if !strings.HasPrefix(url, "https://github.com/swayyaam/lasso/") {
			t.Errorf("%s points at %s, outside the project", name, url)
		}
	}
	for _, name := range []string{"privacy", "terms", "licence"} {
		if _, ok := documents[name]; !ok {
			t.Errorf("no document called %q", name)
		}
	}

	a := &App{}
	if err := a.OpenDocument("https://example.com"); err == nil {
		t.Error("an address was accepted in place of a document name")
	}
}
