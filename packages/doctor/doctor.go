// Package doctor diagnoses why Lasso cannot download, and repairs what it can.
//
// It exists because the failures that stop a download are rarely about the
// link. A quarantined binary, a download folder that was renamed, a full disk
// and a cookie jar macOS will not let Lasso read all surface as the same thing
// — a download that failed — and yt-dlp's own output explains none of them in
// terms a person can act on.
//
// Every check answers three questions in order: what was found, what it means,
// and what to do about it. A check that Lasso can fix itself says so, and the
// fix is a method here rather than a paragraph of instructions.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swayyaam/lasso/packages/binaries"
	"github.com/swayyaam/lasso/packages/core"
)

// Status is how a check came out.
type Status string

const (
	// StatusOK means nothing needs doing.
	StatusOK Status = "ok"
	// StatusWarn means the app works but something will bite later, or is
	// already limiting what can be downloaded.
	StatusWarn Status = "warn"
	// StatusFail means downloads cannot succeed until it is dealt with.
	StatusFail Status = "fail"
)

// Check IDs. They are stable strings because the frontend asks for a fix by id
// and a notice can point at one.
const (
	CheckBinaries      = "binaries"
	CheckDownloadeFold = "download-folder"
	CheckDiskSpace     = "disk-space"
	CheckCookies       = "cookies"
	CheckNotifications = "notifications"
)

// Check is one diagnosis.
type Check struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status Status `json:"status"`
	// Summary is the finding in one line, in plain language.
	Summary string `json:"summary"`
	// Remedy is what to do about it, empty when there is nothing to do.
	Remedy string `json:"remedy"`
	// Detail is the raw evidence, shown behind a toggle.
	Detail string `json:"detail"`
	// Fixable says whether Fix can act on this check without the user leaving
	// the app.
	Fixable bool `json:"fixable"`
	// FixLabel names the button, e.g. "Repair binaries".
	FixLabel string `json:"fixLabel"`
}

// Healthy reports whether nothing is wrong.
func (c Check) Healthy() bool { return c.Status == StatusOK }

// Report is a full run.
type Report struct {
	Checks []Check `json:"checks"`
	// Healthy is true when no check failed. A warning does not make the report
	// unhealthy: the app still works.
	Healthy bool `json:"healthy"`
	// Summary is the one-line verdict for a banner.
	Summary string `json:"summary"`
}

// Config is what the doctor needs to look at. Everything is injected so the
// package stays testable without an installed app.
type Config struct {
	// Manager is the sidecar binaries. Required.
	Manager *binaries.Manager
	// DownloadFolder is where files are meant to land.
	DownloadFolder string
	// Cookies is the browser configured in settings, empty for none.
	Cookies core.Browser
	// FreeBytes reports free space on the volume holding a directory. Injected
	// because it is a syscall, and a test cannot fill a disk.
	FreeBytes func(dir string) (int64, error)
	// MinimumFreeBytes is the floor below which space is a failure.
	MinimumFreeBytes int64
	// CookieProbe reports whether the configured browser's cookies can
	// actually be read. Injected because the only honest test is asking yt-dlp
	// to try, which needs a subprocess.
	CookieProbe func(ctx context.Context, browser core.Browser) error
	// Notifications reports whether the app may post notifications under its
	// own name. Injected because it is a framework call, and because a build
	// that cannot is a fact about the build rather than about the machine.
	Notifications func() NotificationState
	// BrowserInstalled reports whether a browser is on this Mac at all.
	//
	// This is what separates the two causes of "could not find that browser's
	// cookies". A protected location reports "no such file" to a process
	// without permission rather than "permission denied", so an installed
	// browser Lasso may not read is indistinguishable from one that was never
	// installed — unless something looks for the application itself, which is
	// not protected.
	BrowserInstalled func(browser core.Browser) bool
}

// NotificationState is whether finished-download notifications arrive as Lasso.
type NotificationState int

const (
	// NotificationsUnavailable means macOS refused this build. Notifications
	// still appear, posted through AppleScript, but under Script Editor's name.
	NotificationsUnavailable NotificationState = iota
	// NotificationsNotAsked means the permission prompt has not been shown yet.
	NotificationsNotAsked
	// NotificationsRefused means the user declined.
	NotificationsRefused
	// NotificationsAllowed means they arrive as Lasso.
	NotificationsAllowed
)

// Doctor runs checks and applies fixes.
type Doctor struct {
	cfg Config
}

// New builds a Doctor.
func New(cfg Config) (*Doctor, error) {
	if cfg.Manager == nil {
		return nil, fmt.Errorf("doctor needs a binaries manager")
	}
	return &Doctor{cfg: cfg}, nil
}

// Run performs every check.
//
// Checks are independent and none of them modify anything, so a report is safe
// to produce at any time — including from a failed download, which is where it
// is most useful.
func (d *Doctor) Run(ctx context.Context) Report {
	checks := []Check{
		d.checkBinaries(ctx),
		d.checkDownloadFolder(),
		d.checkDiskSpace(),
		d.checkCookies(ctx),
		d.checkNotifications(),
	}

	// Worst first: the thing stopping a download should not be below the thing
	// that merely might.
	sort.SliceStable(checks, func(i, j int) bool {
		return severity(checks[i].Status) > severity(checks[j].Status)
	})

	report := Report{Checks: checks, Healthy: true}
	var failed, warned int
	for _, c := range checks {
		switch c.Status {
		case StatusFail:
			failed++
			report.Healthy = false
		case StatusWarn:
			warned++
		}
	}
	report.Summary = summarise(failed, warned)
	return report
}

func severity(s Status) int {
	switch s {
	case StatusFail:
		return 2
	case StatusWarn:
		return 1
	default:
		return 0
	}
}

func summarise(failed, warned int) string {
	switch {
	case failed > 0 && warned > 0:
		return fmt.Sprintf("%s and %s.", count(failed, "problem"), count(warned, "warning"))
	case failed > 0:
		return fmt.Sprintf("%s stopping downloads.", count(failed, "problem"))
	case warned > 0:
		return fmt.Sprintf("%s worth looking at.", count(warned, "warning"))
	default:
		return "Everything checks out."
	}
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// Fix applies the repair for a check, and returns what changed.
//
// Only checks reporting Fixable can be fixed; anything else needs a decision
// the app has no business making, such as which browser to take cookies from.
func (d *Doctor) Fix(ctx context.Context, id string) (string, error) {
	switch id {
	case CheckBinaries:
		report, err := d.cfg.Manager.Repair(ctx)
		if err != nil {
			return "", err
		}
		return describeFixups(report), nil

	case CheckDownloadeFold:
		if d.cfg.DownloadFolder == "" {
			return "", fmt.Errorf("no download folder is set")
		}
		if err := os.MkdirAll(d.cfg.DownloadFolder, 0o755); err != nil {
			return "", fmt.Errorf("could not create %s: %w", d.cfg.DownloadFolder, err)
		}
		return "Created " + d.cfg.DownloadFolder + ".", nil
	}
	return "", fmt.Errorf("there is nothing Lasso can fix automatically for that")
}

func describeFixups(report binaries.InstallReport) string {
	if len(report.Fixups) == 0 {
		return "Checked every helper program; nothing needed repairing."
	}

	names := make([]string, 0, len(report.Fixups))
	for name := range report.Fixups {
		names = append(names, string(name))
	}
	sort.Strings(names)

	var parts []string
	for _, name := range names {
		parts = append(parts, name+": "+strings.Join(report.Fixups[binaries.Name(name)], ", "))
	}
	return strings.Join(parts, "\n")
}

// ---- individual checks ------------------------------------------------

func (d *Doctor) checkBinaries(ctx context.Context) Check {
	check := Check{
		ID:       CheckBinaries,
		Title:    "Helper programs",
		Fixable:  true,
		FixLabel: "Repair",
	}

	results := d.cfg.Manager.Verify(ctx)
	failures := binaries.Failures(results)

	if len(failures) == 0 {
		var versions []string
		for _, r := range results {
			versions = append(versions, fmt.Sprintf("%s %s", r.Name, r.Version))
		}
		sort.Strings(versions)
		check.Status = StatusOK
		check.Summary = "yt-dlp, ffmpeg, ffprobe and deno all run."
		check.Detail = strings.Join(versions, "\n")
		// Nothing to repair, so do not offer a button that would do nothing.
		check.Fixable = false
		return check
	}

	var names, details []string
	for _, r := range failures {
		names = append(names, string(r.Name))
		details = append(details, fmt.Sprintf("%s: %s", r.Name, r.Err))
	}
	check.Status = StatusFail
	check.Summary = fmt.Sprintf("%s will not run.", strings.Join(names, ", "))
	check.Remedy = "Lasso can re-apply the permissions, quarantine and signature fixes these need."
	check.Detail = strings.Join(details, "\n")
	return check
}

func (d *Doctor) checkDownloadFolder() Check {
	check := Check{ID: CheckDownloadeFold, Title: "Download folder"}

	folder := d.cfg.DownloadFolder
	if folder == "" {
		check.Status = StatusFail
		check.Summary = "No download folder is set."
		check.Remedy = "Choose one in Settings."
		return check
	}

	info, err := os.Stat(folder)
	if os.IsNotExist(err) {
		check.Status = StatusFail
		check.Summary = folder + " does not exist."
		check.Remedy = "It was probably moved or renamed. Lasso can create it again, or pick another in Settings."
		check.Fixable = true
		check.FixLabel = "Create it"
		return check
	}
	if err != nil {
		check.Status = StatusFail
		check.Summary = "Cannot read " + folder + "."
		check.Detail = err.Error()
		check.Remedy = "Choose another folder in Settings."
		return check
	}
	if !info.IsDir() {
		check.Status = StatusFail
		check.Summary = folder + " is a file, not a folder."
		check.Remedy = "Choose a folder in Settings."
		return check
	}
	if err := writable(folder); err != nil {
		check.Status = StatusFail
		check.Summary = "Lasso cannot write to " + folder + "."
		check.Detail = err.Error()
		check.Remedy = "macOS protects some folders until an app is granted access. Choose another folder, or grant Lasso access in System Settings."
		return check
	}

	check.Status = StatusOK
	check.Summary = "Writable: " + folder
	return check
}

// writable proves the folder can be written to by writing to it. Checking the
// permission bits is not the same question: macOS can refuse a folder whose
// mode says otherwise.
func writable(dir string) error {
	f, err := os.CreateTemp(dir, ".lasso-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func (d *Doctor) checkDiskSpace() Check {
	check := Check{ID: CheckDiskSpace, Title: "Disk space"}

	if d.cfg.FreeBytes == nil || d.cfg.DownloadFolder == "" {
		check.Status = StatusOK
		check.Summary = "Not checked."
		return check
	}

	free, err := d.cfg.FreeBytes(d.cfg.DownloadFolder)
	if err != nil {
		// Unmeasurable is not the same as full, and the download itself will
		// report a genuine write failure better than a guess here would.
		check.Status = StatusOK
		check.Summary = "Could not measure free space."
		check.Detail = err.Error()
		return check
	}

	check.Summary = humanBytes(free) + " free"
	switch {
	case free < d.cfg.MinimumFreeBytes:
		check.Status = StatusFail
		check.Summary = "Only " + humanBytes(free) + " free."
		check.Remedy = "Free up space, or choose a folder on another volume in Settings."
	case free < d.cfg.MinimumFreeBytes*8:
		check.Status = StatusWarn
		check.Summary = humanBytes(free) + " free — enough for a few downloads."
		check.Remedy = "A long video at 4K can be several gigabytes."
	default:
		check.Status = StatusOK
	}
	return check
}

func (d *Doctor) checkCookies(ctx context.Context) Check {
	check := Check{ID: CheckCookies, Title: "Browser cookies"}

	if d.cfg.Cookies == core.BrowserNone {
		// Not an error. It is, however, the single most common reason a site
		// serves less than it has, so it is worth saying out loud rather than
		// reporting a clean bill of health.
		check.Status = StatusWarn
		check.Summary = "Not using cookies from any browser."
		check.Remedy = "Sites hold back their higher qualities — and sometimes the whole video — for signed-out visitors. Pick a browser in Settings that you are signed in to."
		return check
	}

	if d.cfg.CookieProbe == nil {
		check.Status = StatusOK
		check.Summary = "Using cookies from " + string(d.cfg.Cookies) + "."
		return check
	}

	if err := d.cfg.CookieProbe(ctx, d.cfg.Cookies); err != nil {
		check.Status = StatusFail
		check.Summary = "Cannot read " + string(d.cfg.Cookies) + "'s cookies."
		check.Detail = err.Error()
		check.Remedy = d.cookieRemedy(err)
		return check
	}

	check.Status = StatusOK
	check.Summary = "Using cookies from " + string(d.cfg.Cookies) + "."
	return check
}

func (d *Doctor) checkNotifications() Check {
	check := Check{ID: CheckNotifications, Title: "Notifications"}

	if d.cfg.Notifications == nil {
		check.Status = StatusOK
		check.Summary = "Not checked."
		return check
	}

	switch d.cfg.Notifications() {
	case NotificationsAllowed:
		check.Status = StatusOK
		check.Summary = "Finished downloads notify you, as Lasso."

	case NotificationsNotAsked:
		check.Status = StatusOK
		check.Summary = "macOS will ask for permission the first time a download finishes."

	case NotificationsRefused:
		check.Status = StatusWarn
		check.Summary = "Notifications are turned off for Lasso."
		check.Remedy = "Turn them back on in System Settings › Notifications › Lasso. Downloads still work; you just will not be told when they finish."

	default:
		// Not a failure: the notification still arrives, and nothing about
		// downloading is affected. But a user seeing Script Editor on a banner
		// from Lasso has no way to work out why, so it is worth saying.
		check.Status = StatusWarn
		check.Summary = "Notifications arrive from “Script Editor”, not from Lasso."
		check.Remedy = "macOS only lets a properly code-signed app post notifications under its own name, and this build is ad-hoc signed. Lasso falls back to AppleScript so you still get told when a download finishes. Signing and notarising the app fixes the name."
	}
	return check
}

// cookieRemedy narrows the classifier's advice using something only the doctor
// can know: whether the browser is actually on this Mac.
//
// The classifier has to offer both causes, because from yt-dlp's output they
// look the same. Here the application itself can be looked for, and finding it
// settles the question — an installed browser whose cookies cannot be found is
// macOS hiding them, not a missing browser.
func (d *Doctor) cookieRemedy(err error) string {
	message := core.UserMessage(err)

	var downloadErr *core.DownloadError
	if !errors.As(err, &downloadErr) || downloadErr.Kind != core.ErrCookieAccess {
		return message
	}
	if d.cfg.BrowserInstalled == nil {
		return message
	}

	if d.cfg.BrowserInstalled(d.cfg.Cookies) {
		return string(d.cfg.Cookies) + " is installed, so this is a permissions problem rather than " +
			"a missing browser: macOS reports a protected folder as absent to an app that may not " +
			"read it. Give Lasso Full Disk Access in System Settings › Privacy & Security."
	}
	return string(d.cfg.Cookies) + " is not installed on this Mac. Choose a browser you actually " +
		"use in Settings, or turn cookies off."
}

// humanBytes renders a size the way the interface does.
func humanBytes(bytes int64) string {
	const unit = 1000
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value, exp := float64(bytes), 0
	for value >= unit && exp < 4 {
		value /= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", value, "KMGT"[exp-1])
}

// SuggestsDoctor reports whether a failed download looks like a broken install
// rather than a bad link.
//
// This is what decides whether the queue offers the doctor from a failure. It
// is deliberately narrow: a private video and a geo-block are the site's
// answer, not Lasso's problem, and offering to run diagnostics on them would
// teach the user that the offer means nothing.
func SuggestsDoctor(kind core.ErrorKind) bool {
	switch kind {
	// A bot check is the network, but its remedy is local: cookies from a
	// browser that is signed in, which is what the doctor checks.
	case core.ErrCookieAccess, core.ErrDiskFull, core.ErrPostProcess, core.ErrUnknown, core.ErrBotCheck:
		return true
	default:
		return false
	}
}

// DefaultDownloadFolder is where a fresh install points.
func DefaultDownloadFolder() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Downloads")
}
