package updater

import (
	"context"
	"os/exec"
)

// execRun is the default Runner: it actually executes the command.
//
// An argument slice, never a shell string. The paths here are the app's own
// rather than anything a site supplied, but the rule holds everywhere in this
// codebase and a bundle path can still contain a space or a quote.
func execRun(ctx context.Context, name string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(output), err
}
