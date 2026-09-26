package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/swayyaam/lasso/packages/core"
)

func TestAYtDlpFailureCrossesWithItsKind(t *testing.T) {
	failure := &core.DownloadError{
		Kind:    core.ErrBotCheck,
		Message: "YouTube wants to be sure you are not a bot.",
		Raw:     "ERROR: [youtube] aqz-KE-bpKQ: Sign in to confirm you’re not a bot.",
		Err:     errors.New("exit status 1"),
	}
	// Wrapped, as it is by the time it leaves a bound method.
	// Text, because Wails' runtime turns anything else into "[object Object]".
	text, ok := formatError(fmt.Errorf("resolving: %w", failure)).(string)
	if !ok {
		t.Fatal("a failure must cross as a string; the runtime flattens objects")
	}
	raw := []byte(text)
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("not JSON: %v: %s", err, raw)
	}
	if got["kind"] != string(core.ErrBotCheck) || got["message"] != failure.Message || got["raw"] != failure.Raw {
		t.Errorf("crossed as %s", raw)
	}
	if _, leaked := got["Err"]; leaked {
		t.Error("the underlying error crossed too; the page has no use for it")
	}
}

func TestOtherErrorsCrossAsTheirText(t *testing.T) {
	got := formatError(errors.New("Lasso is still starting up"))
	if got != "Lasso is still starting up" {
		t.Errorf("= %#v, want the plain text Wails would send", got)
	}
}
