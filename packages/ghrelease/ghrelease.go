// Package ghrelease reads GitHub release files without touching GitHub's API.
//
// Both of Lasso's updaters need two things from GitHub: which release is the
// newest, and that release's files. The obvious way to ask — the REST API —
// allows 60 unauthenticated requests an hour per address, shared by every
// program on the network, and an update check is exactly the kind of small,
// repeated request that runs that allowance down.
//
// None of it is needed. GitHub serves release files from its download CDN at
// documented addresses:
//
//	https://github.com/<owner>/<repo>/releases/latest/download/<file>
//	https://github.com/<owner>/<repo>/releases/download/<tag>/<file>
//
// and the latest-release page redirects to /releases/tag/<tag>, which names
// the newest version without a body at all. Neither counts against the API
// allowance. Lasso publishes a small latest.json with every release for the
// first, and reads the redirect for yt-dlp, which publishes no such file.
//
// Being off the API is not a licence to be careless, so everything here still
// behaves as a good client: requests are serial and identify themselves,
// answers are cached on disk so a relaunch does not repeat them, a refusal
// from GitHub is honoured until it expires — across relaunches too — and
// failures back off instead of retrying on every screen.
package ghrelease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// MinInterval is the closest two requests for the same file may be. The
	// answer cannot change in ten seconds, and someone pressing a button
	// repeatedly should not become a burst of traffic, so inside the window
	// the last answer is served instead.
	MinInterval = 10 * time.Second

	// refusalFloor is how long a refusal holds when GitHub does not say.
	// GitHub asks callers to wait at least a minute after a secondary limit
	// that carries no Retry-After.
	refusalFloor = time.Minute
	refusalMax   = 30 * time.Minute

	// backoffBase and backoffMax space out retries after failures that were
	// not refusals — no network, a timeout — so a dead connection is not
	// retried every time a settings screen opens.
	backoffBase = 30 * time.Second
	backoffMax  = 30 * time.Minute

	// maxSmallFile bounds anything read into memory: a manifest or a
	// checksum list is a few kilobytes.
	maxSmallFile = 1 << 20
)

// ErrNotFound means the address answered, and there is no such file.
var ErrNotFound = errors.New("not found")

// Config configures a Client.
type Config struct {
	// HTTP defaults to a plain client. Timeouts come from the caller's
	// context, because a manifest and a 40 MB download need different ones.
	HTTP *http.Client
	// UserAgent identifies the caller. GitHub asks every client to send one.
	UserAgent string
	// StatePath is where the cache and any refusal are kept between
	// launches. Empty keeps them in memory only.
	StatePath string
	// Now defaults to time.Now; tests move the clock instead of sleeping.
	Now func() time.Time
}

// Client reads release files politely. Keep one for the life of the process
// and share it between updaters: a refusal from GitHub applies to all of
// them, and a Client rebuilt per request would remember none.
type Client struct {
	cfg Config

	// busy serialises requests. GitHub asks for requests one at a time, and
	// two racing checks would each miss the other's refusal.
	busy sync.Mutex

	// mu guards everything below.
	mu       sync.Mutex
	st       state
	next     map[string]time.Time
	retryAt  time.Time
	lastErr  error
	failures int
	refusals int
}

type state struct {
	Entries      map[string]entry `json:"entries"`
	BlockedUntil time.Time        `json:"blockedUntil"`
	BlockReason  string           `json:"blockReason,omitempty"`
}

type entry struct {
	Body      string    `json:"body"`
	ETag      string    `json:"etag,omitempty"`
	FetchedAt time.Time `json:"fetchedAt"`
}

// New builds a Client, reading any state a previous launch left.
func New(cfg Config) *Client {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{}
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Lasso"
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	c := &Client{cfg: cfg, next: map[string]time.Time{}}
	c.st = c.load()
	return c
}

// Get returns a small release file: a manifest, a checksum list.
//
// A copy fetched within maxAge is returned without a request; zero maxAge
// asks every time, still subject to MinInterval and to any refusal.
func (c *Client) Get(ctx context.Context, rawURL string, maxAge time.Duration) ([]byte, error) {
	c.busy.Lock()
	defer c.busy.Unlock()

	cached, ok, err := c.gate(rawURL, maxAge)
	if ok || err != nil {
		return cached, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	c.identify(req)
	if e, have := c.entry(rawURL); have && e.ETag != "" {
		// Carried through GitHub's redirects to the CDN, which answers 304
		// when nothing has changed: no body, and nothing re-downloaded.
		req.Header.Set("If-None-Match", e.ETag)
	}

	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return nil, c.failed(fmt.Errorf("could not reach GitHub: %w", err))
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified:
		if e, have := c.entry(rawURL); have {
			c.store(rawURL, e.Body, e.ETag)
			return []byte(e.Body), nil
		}
		return nil, c.failed(fmt.Errorf("GitHub said nothing had changed, but there was nothing to compare with"))
	case resp.StatusCode == http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxSmallFile+1))
		if err != nil {
			return nil, c.failed(fmt.Errorf("could not read %s: %w", path.Base(req.URL.Path), err))
		}
		if len(body) > maxSmallFile {
			return nil, c.failed(fmt.Errorf("%s is larger than any release manifest should be", path.Base(req.URL.Path)))
		}
		c.store(rawURL, string(body), resp.Header.Get("ETag"))
		return body, nil
	default:
		return nil, c.refusedOr(resp)
	}
}

// tagPattern is what a release tag may look like before it goes into a URL.
// Tags come from a redirect GitHub sends, but they end up as a path segment,
// and nothing that could climb out of one belongs there.
var tagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)

// LatestTag names a repository's newest release without the API.
//
// releases is the repository's releases address, like
// https://github.com/yt-dlp/yt-dlp/releases. Its /latest page redirects to
// /releases/tag/<tag>; only the redirect is read, never the page.
func (c *Client) LatestTag(ctx context.Context, releases string, maxAge time.Duration) (string, error) {
	c.busy.Lock()
	defer c.busy.Unlock()

	key := strings.TrimSuffix(releases, "/") + "/latest"
	cached, ok, err := c.gate(key, maxAge)
	if ok || err != nil {
		return string(cached), err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, key, nil)
	if err != nil {
		return "", err
	}
	c.identify(req)

	noFollow := *c.cfg.HTTP
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noFollow.Do(req)
	if err != nil {
		return "", c.failed(fmt.Errorf("could not reach GitHub: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusOK {
			// No redirect at all: the repository has no published release.
			return "", c.failed(fmt.Errorf("%s has no published release", releases))
		}
		return "", c.refusedOr(resp)
	}

	tag, err := tagFromLocation(resp.Header.Get("Location"))
	if err != nil {
		return "", c.failed(err)
	}
	c.store(key, tag, "")
	return tag, nil
}

func tagFromLocation(location string) (string, error) {
	u, err := url.Parse(location)
	if err != nil || location == "" {
		return "", fmt.Errorf("GitHub's latest-release redirect had no usable address")
	}
	dir, tag := path.Split(u.Path)
	if !strings.HasSuffix(dir, "/releases/tag/") {
		return "", fmt.Errorf("GitHub's latest-release redirect did not name a release")
	}
	tag, err = url.PathUnescape(tag)
	if err != nil || !tagPattern.MatchString(tag) {
		return "", fmt.Errorf("GitHub named a release tag Lasso will not use: %q", tag)
	}
	return tag, nil
}

// Download streams a release file to dest, refusing anything over limit.
//
// Downloads are not cached or spaced — each one follows a deliberate action,
// and caching 40 MB archives would be its own problem — but they identify
// themselves and respect a standing refusal like everything else.
func (c *Client) Download(ctx context.Context, rawURL, dest string, limit int64) error {
	c.busy.Lock()
	defer c.busy.Unlock()

	if err := c.blocked(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	c.identify(req)

	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return c.failed(fmt.Errorf("could not download %s: %w", path.Base(req.URL.Path), err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.refusedOr(resp)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, io.LimitReader(resp.Body, limit+1))
	closeErr := out.Close()
	switch {
	case copyErr != nil:
		return c.failed(fmt.Errorf("the download of %s was interrupted: %w", path.Base(req.URL.Path), copyErr))
	case n > limit:
		return fmt.Errorf("%s is larger than expected, so it was discarded", path.Base(req.URL.Path))
	case closeErr != nil:
		return closeErr
	}
	c.succeeded()
	return nil
}

func (c *Client) identify(req *http.Request) {
	req.Header.Set("User-Agent", c.cfg.UserAgent)
}

// gate decides whether a request may go out. It returns the answer to serve
// instead (ok), an error to report instead, or neither — meaning go ahead.
func (c *Client) gate(key string, maxAge time.Duration) ([]byte, bool, error) {
	now := c.cfg.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	e, have := c.st.Entries[key]
	if have && maxAge > 0 && now.Sub(e.FetchedAt) < maxAge {
		return []byte(e.Body), true, nil
	}
	if now.Before(c.st.BlockedUntil) {
		return nil, false, c.refusalErrorLocked(now)
	}
	if now.Before(c.retryAt) && c.lastErr != nil {
		return nil, false, c.lastErr
	}
	if now.Before(c.next[key]) {
		if have {
			return []byte(e.Body), true, nil
		}
		return nil, false, fmt.Errorf("Lasso just asked GitHub; try again in a moment")
	}
	c.next[key] = now.Add(MinInterval)
	return nil, false, nil
}

func (c *Client) blocked() error {
	now := c.cfg.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if now.Before(c.st.BlockedUntil) {
		return c.refusalErrorLocked(now)
	}
	return nil
}

func (c *Client) entry(key string) (entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.st.Entries[key]
	return e, ok
}

func (c *Client) store(key, body, etag string) {
	c.mu.Lock()
	if c.st.Entries == nil {
		c.st.Entries = map[string]entry{}
	}
	c.st.Entries[key] = entry{Body: body, ETag: etag, FetchedAt: c.cfg.Now()}
	c.failures, c.refusals, c.retryAt, c.lastErr = 0, 0, time.Time{}, nil
	c.mu.Unlock()
	c.save()
}

func (c *Client) succeeded() {
	c.mu.Lock()
	c.failures, c.refusals, c.retryAt, c.lastErr = 0, 0, time.Time{}, nil
	c.mu.Unlock()
}

// failed records a request that did not complete, and backs off.
func (c *Client) failed(err error) error {
	now := c.cfg.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures++
	wait := backoffBase << min(c.failures-1, 16)
	if wait <= 0 || wait > backoffMax {
		wait = backoffMax
	}
	c.retryAt, c.lastErr = now.Add(wait), err
	return err
}

// refusedOr turns a non-success status into an error, and when it is GitHub
// asking Lasso to stop, holds every later request until it may start again.
func (c *Client) refusedOr(resp *http.Response) error {
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%s: %w", path.Base(resp.Request.URL.Path), ErrNotFound)
	}
	after := resp.Header.Get("Retry-After")
	if resp.StatusCode != http.StatusTooManyRequests && after == "" {
		return c.failed(fmt.Errorf("GitHub answered %s", resp.Status))
	}

	now := c.cfg.Now()
	c.mu.Lock()
	c.refusals++
	wait := refusalFloor << min(c.refusals-1, 16)
	if wait <= 0 || wait > refusalMax {
		wait = refusalMax
	}
	if secs, err := strconv.Atoi(after); err == nil && secs > 0 {
		wait = time.Duration(secs) * time.Second
	} else if when, err := http.ParseTime(after); err == nil && when.After(now) {
		wait = when.Sub(now)
	}
	c.st.BlockedUntil = now.Add(wait)
	c.st.BlockReason = "GitHub asked Lasso to slow down"
	err := c.refusalErrorLocked(now)
	c.mu.Unlock()
	c.save()
	return err
}

func (c *Client) refusalErrorLocked(now time.Time) error {
	when := "shortly"
	if wait, ok := HumanWait(c.st.BlockedUntil.Sub(now)); ok {
		when = "in about " + wait
	}
	reason := c.st.BlockReason
	if reason == "" {
		reason = "GitHub asked Lasso to slow down"
	}
	return fmt.Errorf("%s. Lasso will ask again %s — or download it from the releases page.", reason, when)
}

// HumanWait renders a wait the way a sentence needs it.
//
// Duration.String() gives "24m0s", which belongs in a log rather than in
// something a person reads. Refusals last minutes to half an hour; a wait far
// outside that means the clock is wrong, and a time quoted against a wrong
// clock is worse than none, so that case declines to give one.
func HumanWait(d time.Duration) (string, bool) {
	minutes := int(d.Round(time.Minute).Minutes())
	switch {
	case minutes < 1 || minutes > 120:
		return "", false
	case minutes == 1:
		return "a minute", true
	case minutes < 60:
		return fmt.Sprintf("%d minutes", minutes), true
	default:
		return "an hour", true
	}
}

// load reads what a previous launch knew. Anything unreadable is dropped:
// the cost of forgetting is one request, which is not worth failing over.
func (c *Client) load() state {
	st := state{Entries: map[string]entry{}}
	if c.cfg.StatePath == "" {
		return st
	}
	raw, err := os.ReadFile(c.cfg.StatePath)
	if err != nil {
		return st
	}
	if json.Unmarshal(raw, &st) != nil {
		return state{Entries: map[string]entry{}}
	}
	if st.Entries == nil {
		st.Entries = map[string]entry{}
	}
	return st
}

// save persists the cache and any refusal via a temporary file and a rename,
// like every other store Lasso keeps, so an interrupted write cannot leave a
// truncated file.
func (c *Client) save() {
	if c.cfg.StatePath == "" {
		return
	}
	c.mu.Lock()
	raw, err := json.MarshalIndent(c.st, "", "  ")
	c.mu.Unlock()
	if err != nil {
		return
	}
	dir := filepath.Dir(c.cfg.StatePath)
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".ghrelease-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return
	}
	if tmp.Close() != nil {
		return
	}
	_ = os.Rename(tmp.Name(), c.cfg.StatePath)
}
