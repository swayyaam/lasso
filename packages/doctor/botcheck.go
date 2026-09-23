package doctor

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/swayyaam/lasso/packages/core"
)

// BotChecks remembers whether YouTube has lately refused this connection.
//
// The doctor looks at Lasso's own setup, and nothing in that setup changes when
// YouTube starts distrusting an address — so without a memory of the refusal
// there would be nothing for a check to find. It is held by the app for the
// life of the process and fed every resolve and download outcome.
type BotChecks struct {
	mu   sync.Mutex
	last time.Time
}

// Observe records one outcome. A bot check from anywhere is remembered; only a
// YouTube link that worked forgets it, because success on another site says
// nothing about whether YouTube has relented.
func (b *BotChecks) Observe(link string, kind core.ErrorKind, succeeded bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch {
	case kind == core.ErrBotCheck:
		b.last = time.Now()
	case succeeded && IsYouTube(link):
		b.last = time.Time{}
	}
}

// Last is when YouTube last asked for proof, zero if it has not since the
// last time it worked.
func (b *BotChecks) Last() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.last
}

// IsYouTube reports whether a link is on one of YouTube's own hosts.
func IsYouTube(link string) bool {
	link = strings.TrimSpace(link)
	if !strings.Contains(link, "://") {
		link = "https://" + link
	}
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range []string{"youtube.com", "youtu.be", "youtube-nocookie.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// checkBotCheck turns a remembered refusal into advice, and reports false when
// there is nothing to say.
//
// It is only present after a refusal. A standing "YouTube: fine" line would be
// a claim the doctor cannot back — it does not ask YouTube, because asking
// from a distrusted address is exactly what makes things worse.
//
// The advice depends on the cookie check, which has already run: without
// cookies there is a clear fix, with working cookies the block is the
// network's, and with broken cookies the cookie problem comes first.
func (d *Doctor) checkBotCheck(cookies Check) (Check, bool) {
	at := d.cfg.BotCheckAt
	if at.IsZero() {
		return Check{}, false
	}

	check := Check{ID: CheckYouTube, Title: "YouTube"}
	when := ago(time.Since(at))
	browser := d.cfg.Cookies.Name()

	switch {
	case d.cfg.Cookies == core.BrowserNone:
		check.Status = StatusFail
		check.Summary = "YouTube asked Lasso to prove it is not a bot " + when + "."
		check.Remedy = "YouTube asks this of connections it sees a lot of automated traffic from — a VPN or a shared office network is the usual cause. Choose a browser you are signed in to YouTube with in Settings: its cookies show YouTube a person."

	case !cookies.Healthy():
		check.Status = StatusFail
		check.Summary = "YouTube asked Lasso to prove it is not a bot " + when + "."
		check.Remedy = "Lasso is set to use " + browser + "'s cookies but cannot read them, so YouTube saw a signed-out visitor. Fix the cookie problem above first."

	default:
		// Not a failure of anything Lasso controls: the cookies were sent and
		// YouTube refused anyway.
		check.Status = StatusWarn
		check.Summary = "YouTube asked for proof " + when + ", even with " + browser + "'s cookies."
		check.Remedy = "Check you are signed in to YouTube in " + browser + ". If you are, the block is on this connection rather than on Lasso: it usually lifts within the hour, and turning off a VPN often lifts it at once."
	}
	return check, true
}

// ago renders a past duration the way a person would say it.
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < 2*time.Minute:
		return "a minute ago"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d/time.Minute))
	case d < 2*time.Hour:
		return "an hour ago"
	default:
		return fmt.Sprintf("%d hours ago", int(d/time.Hour))
	}
}
