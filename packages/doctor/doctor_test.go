package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swayyaam/lasso/packages/binaries"
	"github.com/swayyaam/lasso/packages/core"
)

// newDoctor builds a Doctor with a manager pointed at empty directories. The
// binaries check will fail, which most of these tests do not care about — they
// look up the check they are about by id.
func newDoctor(t *testing.T, mutate func(*Config)) *Doctor {
	t.Helper()

	source := t.TempDir()
	manifest := `{"schemaVersion":1,"platform":"darwin-arm64","binaries":{
		"yt-dlp":{"version":"1","layout":"dir","entrypoint":"yt-dlp_macos"},
		"ffmpeg":{"version":"1","layout":"file","entrypoint":"ffmpeg"},
		"ffprobe":{"version":"1","layout":"file","entrypoint":"ffprobe"},
		"deno":{"version":"1","layout":"file","entrypoint":"deno"}}}`
	if err := os.WriteFile(filepath.Join(source, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	manager, err := binaries.New(binaries.Paths{Support: t.TempDir(), Bin: t.TempDir(), Source: source})
	if err != nil {
		t.Fatalf("binaries.New: %v", err)
	}

	cfg := Config{
		Manager:          manager,
		DownloadFolder:   t.TempDir(),
		MinimumFreeBytes: 500 << 20,
		FreeBytes:        func(string) (int64, error) { return 100 << 30, nil },
	}
	if mutate != nil {
		mutate(&cfg)
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

func find(t *testing.T, report Report, id string) Check {
	t.Helper()
	for _, c := range report.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no check with id %q", id)
	return Check{}
}

func TestDownloadFolderMissingIsFixable(t *testing.T) {
	// The common case: the folder was renamed or lived on a volume that is no
	// longer mounted. Lasso can make it again, so it offers to.
	gone := filepath.Join(t.TempDir(), "YT")
	d := newDoctor(t, func(c *Config) { c.DownloadFolder = gone })

	check := find(t, d.Run(context.Background()), CheckDownloadeFold)
	if check.Status != StatusFail {
		t.Errorf("Status = %q, want %q", check.Status, StatusFail)
	}
	if !check.Fixable {
		t.Fatal("a missing folder should be fixable")
	}

	if _, err := d.Fix(context.Background(), CheckDownloadeFold); err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if info, err := os.Stat(gone); err != nil || !info.IsDir() {
		t.Errorf("Fix did not create the folder: %v", err)
	}

	// And the re-run has to agree, or the button would appear to do nothing.
	if again := find(t, d.Run(context.Background()), CheckDownloadeFold); again.Status != StatusOK {
		t.Errorf("after fixing, Status = %q, want %q", again.Status, StatusOK)
	}
}

func TestDownloadFolderMustBeWritable(t *testing.T) {
	// Permission bits are not the question — whether a write succeeds is.
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })

	d := newDoctor(t, func(c *Config) { c.DownloadFolder = locked })
	check := find(t, d.Run(context.Background()), CheckDownloadeFold)

	if check.Status != StatusFail {
		t.Errorf("Status = %q, want a read-only folder to fail", check.Status)
	}
	if check.Remedy == "" {
		t.Error("a failure with no remedy tells the user nothing")
	}
}

func TestDiskSpaceGradesRatherThanFlips(t *testing.T) {
	cases := []struct {
		name string
		free int64
		want Status
	}{
		{"plenty", 100 << 30, StatusOK},
		{"getting tight", 1 << 30, StatusWarn},
		{"essentially full", 10 << 20, StatusFail},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDoctor(t, func(cfg *Config) {
				cfg.FreeBytes = func(string) (int64, error) { return c.free, nil }
			})
			if got := find(t, d.Run(context.Background()), CheckDiskSpace); got.Status != c.want {
				t.Errorf("Status = %q, want %q (%s free)", got.Status, c.want, humanBytes(c.free))
			}
		})
	}
}

func TestUnmeasurableDiskIsNotAFailure(t *testing.T) {
	// A volume that will not answer is the download's problem to report, and
	// it will do so with a better error than a guess here.
	d := newDoctor(t, func(c *Config) {
		c.FreeBytes = func(string) (int64, error) { return 0, errors.New("statfs: no such volume") }
	})
	if got := find(t, d.Run(context.Background()), CheckDiskSpace); got.Status != StatusOK {
		t.Errorf("Status = %q, want an unmeasurable volume not to be reported as full", got.Status)
	}
}

func TestUnreadableCookiesAreAFailureWithTheRealRemedy(t *testing.T) {
	// The exact situation that made a 4K video look like a 360p one: cookies
	// were asked for, and macOS refused them.
	refusal := core.ClassifyError(
		`ERROR: [Errno 1] Operation not permitted: '/Users/x/Library/Containers/com.apple.Safari/Data/Library/Cookies/Cookies.binarycookies'`,
		errors.New("exit status 1"),
	)

	d := newDoctor(t, func(c *Config) {
		c.Cookies = core.BrowserSafari
		c.CookieProbe = func(context.Context, core.Browser) error { return refusal }
	})

	check := find(t, d.Run(context.Background()), CheckCookies)
	if check.Status != StatusFail {
		t.Fatalf("Status = %q, want %q", check.Status, StatusFail)
	}
	// The remedy has to be the actionable one, not yt-dlp's errno.
	if !strings.Contains(check.Remedy, "Full Disk Access") {
		t.Errorf("Remedy = %q, want it to name the remedy", check.Remedy)
	}
	if check.Detail == "" {
		t.Error("the raw refusal should stay available behind a toggle")
	}
}

func TestNoCookiesWarnsRatherThanPasses(t *testing.T) {
	// Not an error — but it is the most common reason a site serves less than
	// it has, so a clean bill of health here would be misleading.
	d := newDoctor(t, func(c *Config) { c.Cookies = core.BrowserNone })

	check := find(t, d.Run(context.Background()), CheckCookies)
	if check.Status != StatusWarn {
		t.Errorf("Status = %q, want %q", check.Status, StatusWarn)
	}
	if check.Fixable {
		t.Error("which browser to use is the user's decision, not a fix")
	}
}

func TestWorkingCookiesPass(t *testing.T) {
	d := newDoctor(t, func(c *Config) {
		c.Cookies = core.BrowserChrome
		c.CookieProbe = func(context.Context, core.Browser) error { return nil }
	})
	if got := find(t, d.Run(context.Background()), CheckCookies); got.Status != StatusOK {
		t.Errorf("Status = %q, want %q", got.Status, StatusOK)
	}
}

func TestFailuresSortAboveWarnings(t *testing.T) {
	// The thing stopping a download must not sit below the thing that merely
	// might.
	d := newDoctor(t, func(c *Config) {
		c.Cookies = core.BrowserNone // warn
		c.DownloadFolder = ""        // fail
	})

	report := d.Run(context.Background())
	if len(report.Checks) < 2 {
		t.Fatal("expected several checks")
	}
	if report.Checks[0].Status != StatusFail {
		t.Errorf("first check is %q, want a failure to lead", report.Checks[0].Status)
	}
	if report.Healthy {
		t.Error("a report with a failure is not healthy")
	}
}

func TestWarningsAloneStayHealthy(t *testing.T) {
	// A warning means the app works. Reporting it as broken would make the
	// signal useless.
	d := newDoctor(t, nil)
	report := d.Run(context.Background())

	var sawWarn bool
	for _, c := range report.Checks {
		if c.Status == StatusWarn {
			sawWarn = true
		}
	}
	if !sawWarn {
		t.Skip("no warning in this environment")
	}
	for _, c := range report.Checks {
		if c.Status == StatusFail {
			t.Skip("a real failure in this environment; nothing to assert")
		}
	}
	if !report.Healthy {
		t.Error("warnings alone should not make a report unhealthy")
	}
}

func TestFixRefusesWhatItCannotDo(t *testing.T) {
	d := newDoctor(t, nil)
	if _, err := d.Fix(context.Background(), CheckDiskSpace); err == nil {
		t.Error("Fix claimed it could free disk space")
	}
	if _, err := d.Fix(context.Background(), "nonsense"); err == nil {
		t.Error("Fix accepted an unknown check")
	}
}

func TestSuggestsDoctorOnlyForLocalProblems(t *testing.T) {
	// Offering diagnostics for a private video would teach the user that the
	// offer means nothing.
	local := []core.ErrorKind{core.ErrCookieAccess, core.ErrDiskFull, core.ErrPostProcess, core.ErrUnknown}
	remote := []core.ErrorKind{
		core.ErrPrivate, core.ErrUnavailable, core.ErrGeoBlocked,
		core.ErrRateLimited, core.ErrCancelled, core.ErrLoginRequired,
	}

	for _, kind := range local {
		if !SuggestsDoctor(kind) {
			t.Errorf("%q should offer the doctor", kind)
		}
	}
	for _, kind := range remote {
		if SuggestsDoctor(kind) {
			t.Errorf("%q is the site's answer, not a local problem", kind)
		}
	}
}

func TestReportSummaryCounts(t *testing.T) {
	cases := []struct {
		failed, warned int
		want           string
	}{
		{0, 0, "Everything checks out."},
		{1, 0, "1 problem stopping downloads."},
		{2, 0, "2 problems stopping downloads."},
		{0, 1, "1 warning worth looking at."},
		{1, 2, "1 problem and 2 warnings."},
	}
	for _, c := range cases {
		if got := summarise(c.failed, c.warned); got != c.want {
			t.Errorf("summarise(%d, %d) = %q, want %q", c.failed, c.warned, got, c.want)
		}
	}
}

func TestInstalledBrowserPointsAtPermissionsNotAbsence(t *testing.T) {
	// The finding that prompted this: macOS answers a process without
	// permission with "no such file", so an installed browser Lasso may not
	// read looks exactly like a browser that was never installed. Telling the
	// user it is not installed sends them hunting for something already there.
	missing := core.ClassifyError(
		`ERROR: could not find chrome cookies database in "/Users/x/Library/Application Support/Google/Chrome"`,
		errors.New("exit status 1"),
	)

	d := newDoctor(t, func(c *Config) {
		c.Cookies = core.BrowserChrome
		c.CookieProbe = func(context.Context, core.Browser) error { return missing }
		c.BrowserInstalled = func(core.Browser) bool { return true }
	})

	check := find(t, d.Run(context.Background()), CheckCookies)
	if check.Status != StatusFail {
		t.Fatalf("Status = %q, want %q", check.Status, StatusFail)
	}
	if !strings.Contains(check.Remedy, "Full Disk Access") {
		t.Errorf("Remedy = %q, want the permission remedy for an installed browser", check.Remedy)
	}
	if strings.Contains(check.Remedy, "not installed") {
		t.Errorf("Remedy = %q, claims a browser that is present is absent", check.Remedy)
	}
}

func TestAbsentBrowserSaysSo(t *testing.T) {
	missing := core.ClassifyError(
		`ERROR: could not find brave cookies database in "/Users/x/Library/Application Support/BraveSoftware"`,
		errors.New("exit status 1"),
	)

	d := newDoctor(t, func(c *Config) {
		c.Cookies = core.BrowserBrave
		c.CookieProbe = func(context.Context, core.Browser) error { return missing }
		c.BrowserInstalled = func(core.Browser) bool { return false }
	})

	check := find(t, d.Run(context.Background()), CheckCookies)
	if !strings.Contains(check.Remedy, "not installed") {
		t.Errorf("Remedy = %q, want it to say the browser is absent", check.Remedy)
	}
	if strings.Contains(check.Remedy, "Full Disk Access") {
		t.Errorf("Remedy = %q, sends the user to a permission that would not help", check.Remedy)
	}
}

func TestNonCookieFailuresKeepTheClassifiersAdvice(t *testing.T) {
	// The narrowing only applies to a cookie-access failure. Anything else
	// keeps whatever the classifier said.
	other := core.ClassifyError("ERROR: HTTP Error 429: Too Many Requests", errors.New("exit status 1"))

	d := newDoctor(t, func(c *Config) {
		c.Cookies = core.BrowserChrome
		c.CookieProbe = func(context.Context, core.Browser) error { return other }
		c.BrowserInstalled = func(core.Browser) bool { return true }
	})

	check := find(t, d.Run(context.Background()), CheckCookies)
	if !strings.Contains(check.Remedy, "slow down") {
		t.Errorf("Remedy = %q, want the classifier's own message", check.Remedy)
	}
}

func TestNotificationStates(t *testing.T) {
	cases := []struct {
		name  string
		state NotificationState
		want  Status
		says  string
	}{
		{"allowed", NotificationsAllowed, StatusOK, "as Lasso"},
		{"not yet asked", NotificationsNotAsked, StatusOK, "ask for permission"},
		{"refused", NotificationsRefused, StatusWarn, "System Settings"},
		// Not a failure: the notification still arrives, and downloading is
		// unaffected. But a banner from Lasso that says Script Editor is
		// otherwise inexplicable.
		{"unavailable", NotificationsUnavailable, StatusWarn, "Script Editor"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDoctor(t, func(cfg *Config) {
				cfg.Notifications = func() NotificationState { return c.state }
			})
			check := find(t, d.Run(context.Background()), CheckNotifications)

			if check.Status != c.want {
				t.Errorf("Status = %q, want %q", check.Status, c.want)
			}
			if !strings.Contains(check.Summary+check.Remedy, c.says) {
				t.Errorf("check says %q / %q, want it to mention %q", check.Summary, check.Remedy, c.says)
			}
			if check.Fixable {
				t.Error("offered to fix something only code signing or System Settings can")
			}
		})
	}
}

func TestNotificationsNeverFailTheReport(t *testing.T) {
	// Downloading works regardless, so nothing here may make the report
	// unhealthy — that signal is reserved for what actually stops a download.
	for _, state := range []NotificationState{
		NotificationsUnavailable, NotificationsNotAsked,
		NotificationsRefused, NotificationsAllowed,
	} {
		d := newDoctor(t, func(cfg *Config) {
			cfg.Notifications = func() NotificationState { return state }
			cfg.Cookies = core.BrowserChrome
			cfg.CookieProbe = func(context.Context, core.Browser) error { return nil }
		})
		if check := find(t, d.Run(context.Background()), CheckNotifications); check.Status == StatusFail {
			t.Errorf("state %v reported as a failure", state)
		}
	}
}

func TestNoBotCheckMeansNoYouTubeLine(t *testing.T) {
	// The doctor does not ask YouTube, so without a refusal to report it has
	// nothing to say — and a standing "fine" would be a claim it cannot back.
	d := newDoctor(t, nil)
	for _, c := range d.Run(context.Background()).Checks {
		if c.ID == CheckYouTube {
			t.Fatalf("unexpected YouTube check: %+v", c)
		}
	}
}

func TestBotCheckWithoutCookiesNamesTheFix(t *testing.T) {
	d := newDoctor(t, func(c *Config) {
		c.Cookies = core.BrowserNone
		c.BotCheckAt = time.Now().Add(-12 * time.Minute)
	})

	check := find(t, d.Run(context.Background()), CheckYouTube)
	if check.Status != StatusFail {
		t.Errorf("Status = %q, want %q", check.Status, StatusFail)
	}
	if !strings.Contains(check.Summary, "12 minutes ago") {
		t.Errorf("Summary = %q, want it to say when", check.Summary)
	}
	if !strings.Contains(check.Remedy, "Settings") {
		t.Errorf("Remedy = %q, want it to point at the cookie setting", check.Remedy)
	}
}

func TestBotCheckWithBrokenCookiesPointsAtThem(t *testing.T) {
	d := newDoctor(t, func(c *Config) {
		c.Cookies = core.BrowserChrome
		c.CookieProbe = func(context.Context, core.Browser) error { return errors.New("locked") }
		c.BotCheckAt = time.Now()
	})

	check := find(t, d.Run(context.Background()), CheckYouTube)
	if check.Status != StatusFail {
		t.Errorf("Status = %q, want %q", check.Status, StatusFail)
	}
	if !strings.Contains(check.Remedy, "cookie problem above") {
		t.Errorf("Remedy = %q, want it to send the user to the cookie check", check.Remedy)
	}
}

func TestBotCheckDespiteCookiesIsTheNetwork(t *testing.T) {
	// Cookies were sent and YouTube refused anyway. Nothing in Lasso is broken,
	// so this warns rather than fails, and says whose block it is.
	d := newDoctor(t, func(c *Config) {
		c.Cookies = core.BrowserSafari
		c.CookieProbe = func(context.Context, core.Browser) error { return nil }
		c.BotCheckAt = time.Now().Add(-3 * time.Hour)
	})

	check := find(t, d.Run(context.Background()), CheckYouTube)
	if check.Status != StatusWarn {
		t.Errorf("Status = %q, want %q", check.Status, StatusWarn)
	}
	if !strings.Contains(check.Summary, "3 hours ago") || !strings.Contains(check.Summary, "Safari") {
		t.Errorf("Summary = %q", check.Summary)
	}
	if !strings.Contains(check.Remedy, "connection") {
		t.Errorf("Remedy = %q, want it to place the block on the connection", check.Remedy)
	}
}

func TestBotChecksRememberUntilYouTubeWorks(t *testing.T) {
	var b BotChecks
	if !b.Last().IsZero() {
		t.Fatal("a fresh memory should be empty")
	}

	b.Observe("https://www.youtube.com/watch?v=x", core.ErrBotCheck, false)
	if b.Last().IsZero() {
		t.Fatal("a bot check should be remembered")
	}

	// Success elsewhere proves nothing about YouTube.
	b.Observe("https://vimeo.com/1", "", true)
	if b.Last().IsZero() {
		t.Fatal("a Vimeo download should not clear a YouTube refusal")
	}
	// Nor does a YouTube failure of another kind.
	b.Observe("https://youtu.be/x", core.ErrPrivate, false)
	if b.Last().IsZero() {
		t.Fatal("an unrelated YouTube failure should not clear it")
	}

	b.Observe("https://music.youtube.com/watch?v=x", "", true)
	if !b.Last().IsZero() {
		t.Fatal("YouTube working again should clear it")
	}
}

func TestIsYouTube(t *testing.T) {
	cases := map[string]bool{
		"https://www.youtube.com/watch?v=x": true,
		"https://m.youtube.com/watch?v=x":   true,
		"https://youtu.be/x":                true,
		"youtube.com/shorts/x":              true,
		"https://www.youtube-nocookie.com/": true,
		"https://notyoutube.com/":           false,
		"https://youtube.com.evil.example/": false,
		"https://vimeo.com/1":               false,
		"":                                  false,
	}
	for link, want := range cases {
		if got := IsYouTube(link); got != want {
			t.Errorf("IsYouTube(%q) = %v, want %v", link, got, want)
		}
	}
}

func TestYtDlpAgeIsReportedAndWarnsWhenStale(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	results := func(version string) []binaries.VerifyResult {
		return []binaries.VerifyResult{{Name: binaries.YtDlp, OK: true, Version: version}}
	}

	d := newDoctor(t, func(c *Config) { c.Now = func() time.Time { return now } })
	fresh, ok := d.checkYtDlpAge(results("2026.08.19"))
	if !ok || fresh.Status != StatusOK || !strings.Contains(fresh.Summary, "38 days ago") {
		t.Errorf("fresh = %+v, %v; want OK and its age", fresh, ok)
	}

	stale, _ := d.checkYtDlpAge(results("2026.05.01"))
	if stale.Status != StatusWarn || stale.Fixable {
		t.Errorf("stale without an updater = %+v; want a warning that points at Settings", stale)
	}

	// Nightly builds append a time.
	if nightly, ok := d.checkYtDlpAge(results("2026.09.20.232345")); !ok || nightly.Status != StatusOK {
		t.Errorf("nightly = %+v, %v", nightly, ok)
	}

	// Not running, or not a date: nothing to say, and the helper programs
	// check already covers a yt-dlp that will not start.
	if _, ok := d.checkYtDlpAge([]binaries.VerifyResult{{Name: binaries.YtDlp, OK: false}}); ok {
		t.Error("a yt-dlp that does not run has no age to report")
	}
	if _, ok := d.checkYtDlpAge(results("unknown")); ok {
		t.Error("a version that is not a date has no age to report")
	}
}

func TestAStaleYtDlpCanBeUpdatedFromTheDoctor(t *testing.T) {
	updated := false
	d := newDoctor(t, func(c *Config) {
		c.Now = func() time.Time { return time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC) }
		c.UpdateYtDlp = func(context.Context) (string, error) {
			updated = true
			return "Updated to 2026.09.25.", nil
		}
	})
	stale, _ := d.checkYtDlpAge([]binaries.VerifyResult{{Name: binaries.YtDlp, OK: true, Version: "2026.05.01"}})
	if !stale.Fixable || stale.FixLabel != "Update yt-dlp" {
		t.Fatalf("stale = %+v; want it fixable", stale)
	}
	out, err := d.Fix(context.Background(), CheckYtDlpAge)
	if err != nil || !updated || out != "Updated to 2026.09.25." {
		t.Errorf("Fix = %q, %v (updated %v)", out, err, updated)
	}
}
