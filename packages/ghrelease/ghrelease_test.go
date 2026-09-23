package ghrelease

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeGitHub counts what actually arrives, which is the only honest measure
// of whether a client is being polite.
type fakeGitHub struct {
	hits    atomic.Int32
	handler http.HandlerFunc
	server  *httptest.Server
}

func newFake(t *testing.T, h http.HandlerFunc) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{handler: h}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.handler(w, r)
	}))
	t.Cleanup(f.server.Close)
	return f
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newClient(t *testing.T, statePath string) (*Client, *clock) {
	t.Helper()
	clk := &clock{t: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)}
	return New(Config{UserAgent: "Lasso/test", StatePath: statePath, Now: clk.now}), clk
}

func TestAFreshCopyIsServedWithoutAsking(t *testing.T) {
	gh := newFake(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"version":"1.0.0"}`) })
	c, clk := newClient(t, "")

	for i := 0; i < 5; i++ {
		body, err := c.Get(context.Background(), gh.server.URL+"/latest.json", time.Hour)
		if err != nil || !strings.Contains(string(body), "1.0.0") {
			t.Fatalf("Get = %q, %v", body, err)
		}
		clk.advance(5 * time.Minute)
	}
	if got := gh.hits.Load(); got != 1 {
		t.Errorf("%d requests for five reads inside an hour; want 1", got)
	}
}

func TestTheCacheSurvivesARelaunch(t *testing.T) {
	// The case that matters while developing: every relaunch used to be a
	// fresh process with nothing remembered, so every relaunch asked again.
	state := filepath.Join(t.TempDir(), "github.json")
	gh := newFake(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `manifest`) })

	first, _ := newClient(t, state)
	if _, err := first.Get(context.Background(), gh.server.URL+"/latest.json", time.Hour); err != nil {
		t.Fatal(err)
	}
	relaunched, clk := newClient(t, state)
	clk.advance(10 * time.Minute)
	body, err := relaunched.Get(context.Background(), gh.server.URL+"/latest.json", time.Hour)
	if err != nil || string(body) != "manifest" {
		t.Fatalf("after relaunch Get = %q, %v", body, err)
	}
	if got := gh.hits.Load(); got != 1 {
		t.Errorf("%d requests across a relaunch; want 1", got)
	}
}

func TestAnExpiredCopyIsRevalidatedNotRedownloaded(t *testing.T) {
	var conditional atomic.Bool
	gh := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			conditional.Store(true)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		fmt.Fprint(w, "body-v1")
	})
	c, clk := newClient(t, "")

	if _, err := c.Get(context.Background(), gh.server.URL+"/f", time.Minute); err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * time.Minute)
	body, err := c.Get(context.Background(), gh.server.URL+"/f", time.Minute)
	if err != nil || string(body) != "body-v1" {
		t.Fatalf("revalidated Get = %q, %v", body, err)
	}
	if !conditional.Load() {
		t.Error("the second request did not ask conditionally")
	}
}

func TestARefusalIsHonouredEvenAcrossARelaunch(t *testing.T) {
	// Being told to stop and asking again straight away is what turns a
	// throttle into a block. Quitting and reopening must not be a way round it.
	state := filepath.Join(t.TempDir(), "github.json")
	gh := newFake(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	c, clk := newClient(t, state)
	_, err := c.Get(context.Background(), gh.server.URL+"/latest.json", 0)
	if err == nil || !strings.Contains(err.Error(), "slow down") {
		t.Fatalf("err = %v, want the slow-down explanation", err)
	}
	if !strings.Contains(err.Error(), "10 minutes") {
		t.Errorf("err = %v, want it to say when, in words", err)
	}

	for i := 0; i < 3; i++ {
		clk.advance(time.Minute)
		if _, err := c.Get(context.Background(), gh.server.URL+"/latest.json", 0); err == nil {
			t.Fatal("a request inside the refusal window succeeded")
		}
	}
	relaunched, clk2 := newClient(t, state)
	clk2.advance(4 * time.Minute)
	if _, err := relaunched.Get(context.Background(), gh.server.URL+"/latest.json", 0); err == nil {
		t.Fatal("a relaunch forgot the refusal")
	}
	if err := relaunched.Download(context.Background(), gh.server.URL+"/a.zip", filepath.Join(t.TempDir(), "a"), 1<<20); err == nil {
		t.Fatal("a download went out inside the refusal window")
	}
	if got := gh.hits.Load(); got != 1 {
		t.Errorf("%d requests reached GitHub; want 1, the rest refused locally", got)
	}

	// And it lifts when it said it would.
	gh.handler = func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") }
	clk2.advance(7 * time.Minute)
	if _, err := relaunched.Get(context.Background(), gh.server.URL+"/latest.json", 0); err != nil {
		t.Errorf("still refused after the window passed: %v", err)
	}
}

func TestARefusalWithoutRetryAfterStillWaits(t *testing.T) {
	gh := newFake(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) })
	c, clk := newClient(t, "")

	_, _ = c.Get(context.Background(), gh.server.URL+"/f", 0)
	clk.advance(30 * time.Second)
	_, _ = c.Get(context.Background(), gh.server.URL+"/f", 0)
	if got := gh.hits.Load(); got != 1 {
		t.Errorf("%d requests; a bare 429 should hold for at least a minute", got)
	}
}

func TestPressingRepeatedlyServesTheLastAnswer(t *testing.T) {
	gh := newFake(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "answer") })
	c, clk := newClient(t, "")

	for i := 0; i < 6; i++ {
		body, err := c.Get(context.Background(), gh.server.URL+"/f", 0)
		if err != nil || string(body) != "answer" {
			t.Fatalf("press %d: %q, %v", i, body, err)
		}
		clk.advance(time.Second)
	}
	if got := gh.hits.Load(); got != 1 {
		t.Errorf("%d requests for six presses in six seconds; want 1", got)
	}
	clk.advance(MinInterval)
	_, _ = c.Get(context.Background(), gh.server.URL+"/f", 0)
	if got := gh.hits.Load(); got != 2 {
		t.Errorf("%d requests; a press after the interval should ask again", got)
	}
}

func TestFailuresBackOff(t *testing.T) {
	c, clk := newClient(t, "")
	dead := "http://127.0.0.1:1/latest.json"

	if _, err := c.Get(context.Background(), dead, 0); err == nil {
		t.Fatal("an unreachable server reported success")
	}
	clk.advance(MinInterval + time.Second)
	_, err := c.Get(context.Background(), dead, 0)
	if err == nil || !strings.Contains(err.Error(), "could not reach GitHub") {
		t.Errorf("err = %v, want the original failure repeated from memory", err)
	}
	if c.retryAt.Sub(clk.t) <= 0 {
		t.Error("no backoff was recorded")
	}
}

func TestNotFoundIsDistinguishable(t *testing.T) {
	gh := newFake(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	c, _ := newClient(t, "")

	_, err := c.Get(context.Background(), gh.server.URL+"/latest.json", 0)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound so the updater can explain a missing manifest", err)
	}
}

func TestLatestTagReadsTheRedirectAndNothingElse(t *testing.T) {
	var method string
	gh := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		if r.URL.Path == "/yt-dlp/yt-dlp/releases/latest" {
			http.Redirect(w, r, "/yt-dlp/yt-dlp/releases/tag/2026.08.19", http.StatusFound)
			return
		}
		t.Errorf("the redirect was followed to %s", r.URL.Path)
	})
	c, _ := newClient(t, "")

	tag, err := c.LatestTag(context.Background(), gh.server.URL+"/yt-dlp/yt-dlp/releases", 0)
	if err != nil || tag != "2026.08.19" {
		t.Fatalf("LatestTag = %q, %v", tag, err)
	}
	if method != http.MethodHead {
		t.Errorf("asked with %s; HEAD reads the redirect without the page", method)
	}
}

func TestLatestTagRefusesATagThatIsNotOne(t *testing.T) {
	for _, location := range []string{
		"/r/releases/tag/..%2F..%2Fsecrets",
		"/r/releases/tag/a b",
		"/r/somewhere-else/2026.08.19",
		"",
	} {
		gh := newFake(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", location)
			w.WriteHeader(http.StatusFound)
		})
		c, _ := newClient(t, "")
		if tag, err := c.LatestTag(context.Background(), gh.server.URL+"/r/releases", 0); err == nil {
			t.Errorf("Location %q gave tag %q; want a refusal", location, tag)
		}
	}
}

func TestLatestTagWithNoReleases(t *testing.T) {
	gh := newFake(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	c, _ := newClient(t, "")
	if _, err := c.LatestTag(context.Background(), gh.server.URL+"/r/releases", 0); err == nil || !strings.Contains(err.Error(), "no published release") {
		t.Errorf("err = %v", err)
	}
}

func TestDownloadRefusesAnOversizeFile(t *testing.T) {
	gh := newFake(t, func(w http.ResponseWriter, _ *http.Request) { w.Write(make([]byte, 2048)) })
	c, _ := newClient(t, "")
	dest := filepath.Join(t.TempDir(), "a.zip")
	if err := c.Download(context.Background(), gh.server.URL+"/a.zip", dest, 1024); err == nil {
		t.Fatal("an oversize download was accepted")
	}
	if err := c.Download(context.Background(), gh.server.URL+"/a.zip", dest, 4096); err != nil {
		t.Fatalf("a download within the limit failed: %v", err)
	}
	if info, _ := os.Stat(dest); info == nil || info.Size() != 2048 {
		t.Error("the downloaded file is not the served bytes")
	}
}

func TestEveryRequestIdentifiesItself(t *testing.T) {
	var agents []string
	var mu sync.Mutex
	gh := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		agents = append(agents, r.Header.Get("User-Agent"))
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/latest") {
			http.Redirect(w, r, "/r/releases/tag/v1.0.0", http.StatusFound)
			return
		}
		fmt.Fprint(w, "x")
	})
	c, _ := newClient(t, "")
	_, _ = c.Get(context.Background(), gh.server.URL+"/f", 0)
	_, _ = c.LatestTag(context.Background(), gh.server.URL+"/r/releases", 0)
	_ = c.Download(context.Background(), gh.server.URL+"/d", filepath.Join(t.TempDir(), "d"), 10)
	for _, a := range agents {
		if a != "Lasso/test" {
			t.Errorf("User-Agent = %q, want Lasso/test", a)
		}
	}
	if len(agents) != 3 {
		t.Errorf("%d requests, want 3", len(agents))
	}
}

func TestRequestsAreSerial(t *testing.T) {
	var inFlight, overlapped int32
	gh := newFake(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&inFlight, 1) > 1 {
			atomic.StoreInt32(&overlapped, 1)
		}
		time.Sleep(15 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		fmt.Fprint(w, "x")
	})
	c := New(Config{UserAgent: "Lasso/test"})

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = c.Get(context.Background(), fmt.Sprintf("%s/f%d", gh.server.URL, i), 0)
		}(i)
	}
	wg.Wait()
	if atomic.LoadInt32(&overlapped) == 1 {
		t.Error("two requests were in flight at once")
	}
}

func TestHumanWaitReadsAsASentence(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
		ok   bool
	}{
		{20 * time.Second, "", false},
		{90 * time.Second, "2 minutes", true},
		{time.Minute, "a minute", true},
		{23 * time.Minute, "23 minutes", true},
		{time.Hour, "an hour", true},
		{9 * time.Hour, "", false}, // a wrong clock, not a real wait
		{-5 * time.Minute, "", false},
	}
	for _, c := range cases {
		if got, ok := HumanWait(c.in); got != c.want || ok != c.ok {
			t.Errorf("HumanWait(%v) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
