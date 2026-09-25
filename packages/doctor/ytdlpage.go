package doctor

import (
	"fmt"
	"time"

	"github.com/swayyaam/lasso/packages/binaries"
)

// ytDlpStaleAfter is when yt-dlp's age becomes worth a warning. Sites change
// faster than this, and yt-dlp usually releases within weeks of one breaking.
const ytDlpStaleAfter = 60 * 24 * time.Hour

// checkYtDlpAge says how old yt-dlp is, and reports false when that cannot be
// told — yt-dlp not running, which the helper programs check already says,
// or a version that is not a date.
//
// Always shown when it can be: "is yt-dlp up to date" is the first question
// anyone troubleshooting a download asks.
func (d *Doctor) checkYtDlpAge(results []binaries.VerifyResult) (Check, bool) {
	var version string
	for _, r := range results {
		if r.Name == binaries.YtDlp && r.OK {
			version = r.Version
		}
	}
	// yt-dlp versions are release dates, 2026.08.19, with a time appended on
	// nightly builds.
	if len(version) < len("2006.01.02") {
		return Check{}, false
	}
	released, err := time.Parse("2006.01.02", version[:len("2006.01.02")])
	if err != nil {
		return Check{}, false
	}

	now := time.Now()
	if d.cfg.Now != nil {
		now = d.cfg.Now()
	}
	age := now.Sub(released)

	check := Check{ID: CheckYtDlpAge, Title: "yt-dlp", Detail: "yt-dlp " + version}
	check.Summary = fmt.Sprintf("Version %s, released %s.", version, daysAgo(age))
	if age < ytDlpStaleAfter {
		check.Status = StatusOK
		return check, true
	}

	check.Status = StatusWarn
	check.Remedy = "Sites change often, and yt-dlp keeps up with them. A download that stopped working usually works again with a newer yt-dlp."
	if d.cfg.UpdateYtDlp != nil {
		check.Fixable = true
		check.FixLabel = "Update yt-dlp"
	} else {
		check.Remedy += " Update it in Settings › About & diagnostics."
	}
	return check, true
}

// daysAgo says how long ago in the unit a person would use.
func daysAgo(d time.Duration) string {
	days := int(d / (24 * time.Hour))
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 60:
		return fmt.Sprintf("%d days ago", days)
	default:
		return fmt.Sprintf("about %d months ago", days/30)
	}
}
