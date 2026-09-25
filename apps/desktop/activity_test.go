package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/swayyaam/lasso/packages/core"
)

func TestActivityCountsUnfinishedAndBusySeparately(t *testing.T) {
	var a activity
	check := func(gotU, gotB, wantU, wantB int, when string) {
		t.Helper()
		if gotU != wantU || gotB != wantB {
			t.Errorf("%s: unfinished %d, busy %d; want %d, %d", when, gotU, gotB, wantU, wantB)
		}
	}

	u, b := a.observe("1", core.StateDownloading)
	check(u, b, 1, 1, "one downloading")
	u, b = a.observe("2", core.StateQueued)
	check(u, b, 2, 2, "one waiting its turn")
	// Paused keeps its badge but is no reason to keep the Mac awake.
	u, b = a.observe("1", core.StatePaused)
	check(u, b, 2, 1, "one paused")
	u, b = a.observe("2", core.StateDone)
	check(u, b, 1, 0, "the other finished")
	u, b = a.forget([]string{"1"})
	check(u, b, 0, 0, "the paused one removed")
}

func TestKeepAwakeHoldsAnAssertionAndLetsItGo(t *testing.T) {
	// The real thing, checked the way someone asking "why won't my Mac sleep"
	// would: pmset lists every assertion by name.
	assertions := func() string {
		out, err := exec.Command("/usr/bin/pmset", "-g", "assertions").Output()
		if err != nil {
			t.Skipf("pmset: %v", err)
		}
		return string(out)
	}

	keepAwake(true)
	keepAwake(true) // a second hold is not a second assertion
	held := assertions()
	keepAwake(false)
	released := assertions()

	if n := strings.Count(held, "Lasso is downloading"); n != 1 {
		t.Errorf("while held, pmset lists %d Lasso assertions, want 1", n)
	}
	if strings.Contains(released, "Lasso is downloading") {
		t.Error("the assertion was still listed after it was released")
	}
}
