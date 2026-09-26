package binaries

import "context"

// Problem is a binary that is not usable, in a form the UI can render directly:
// a plain-language sentence plus the raw detail for a "details" toggle.
type Problem struct {
	Name    Name   `json:"name"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
}

// Status is the result of bringing the sidecar binaries up at launch.
type Status struct {
	// Checked is false until the launch sequence has run. Before that, Ready
	// false means "not known yet", not "failed": the window opens before the
	// helpers have been checked, and reading the empty status as a failure
	// showed "did not start" on a launch that was about to succeed.
	Checked bool `json:"checked"`
	// Ready is true when every required binary ran successfully.
	Ready bool `json:"ready"`
	// BinDir is where the binaries are executed from.
	BinDir string `json:"binDir"`
	// FirstRun is true when this launch installed binaries for the first time
	// or refreshed them after a version bump.
	FirstRun bool `json:"firstRun"`
	// Versions maps each binary to the version string it reported.
	Versions map[Name]string `json:"versions"`
	// Problems lists what is wrong, empty when Ready.
	Problems []Problem `json:"problems"`
}

// Startup performs the whole launch sequence: find the bundled binaries,
// install them into Application Support if needed, and confirm each one runs.
//
// It returns a Status even when something is wrong, so the UI can show a
// specific, actionable message rather than a generic failure. A non-nil error
// means the sidecar set could not be located at all, which is unrecoverable
// from inside the app.
func Startup(ctx context.Context) (*Manager, Status, error) {
	m, err := Discover()
	if err != nil {
		return nil, Status{
			Checked:  true,
			Problems: []Problem{{Message: UserMessage(err), Detail: err.Error()}},
		}, err
	}

	status := Status{Checked: true, BinDir: m.paths.Bin, Versions: map[Name]string{}}

	report, err := m.Install(ctx)
	if err != nil {
		status.Problems = append(status.Problems, Problem{
			Message: UserMessage(err),
			Detail:  err.Error(),
		})
		return m, status, nil
	}
	status.FirstRun = report.Changed()

	for _, result := range m.Verify(ctx) {
		if result.OK {
			status.Versions[result.Name] = result.Version
			continue
		}
		status.Problems = append(status.Problems, Problem{
			Name:    result.Name,
			Message: UserMessage(result.Err),
			Detail:  result.Err.Error(),
		})
	}

	status.Ready = len(status.Problems) == 0
	return m, status, nil
}
