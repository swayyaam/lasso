package ghrelease

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveGitHubCostsNoAPIQuota checks the premise this package rests on,
// against the real GitHub: that reading a release through the latest-release
// redirect and the download CDN leaves the API's anonymous allowance where it
// was. If GitHub ever starts counting these, this is where it shows.
//
// Skipped unless LASSO_INTEGRATION is set, because it talks to github.com.
//
//	LASSO_INTEGRATION=1 go test ./packages/ghrelease -run Live -v
func TestLiveGitHubCostsNoAPIQuota(t *testing.T) {
	if os.Getenv("LASSO_INTEGRATION") == "" {
		t.Skip("set LASSO_INTEGRATION=1 to run against github.com")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	before := anonymousQuota(t, ctx)
	c := New(Config{UserAgent: "Lasso/test (+https://github.com/swayyaam/lasso)"})

	tag, err := c.LatestTag(ctx, "https://github.com/yt-dlp/yt-dlp/releases", 0)
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	sums, err := c.Get(ctx, "https://github.com/yt-dlp/yt-dlp/releases/download/"+tag+"/SHA2-256SUMS", 0)
	if err != nil {
		t.Fatalf("fetching the checksum list: %v", err)
	}
	if !strings.Contains(string(sums), "yt-dlp_macos.zip") {
		t.Errorf("the checksum list for %s does not list yt-dlp_macos.zip", tag)
	}

	after := anonymousQuota(t, ctx)
	t.Logf("latest yt-dlp is %s; anonymous API quota %d before, %d after", tag, before, after)
	if after < before {
		t.Errorf("reading a release spent %d of the API allowance; it should spend none", before-after)
	}
}

// anonymousQuota reads what is left of this address's allowance. Reading it
// is itself free.
func anonymousQuota(t *testing.T, ctx context.Context) int {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/rate_limit", nil)
	req.Header.Set("User-Agent", "Lasso/test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("cannot reach GitHub: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Resources struct {
			Core struct {
				Remaining int `json:"remaining"`
			} `json:"core"`
		} `json:"resources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("reading the rate limit: %v", err)
	}
	return body.Resources.Core.Remaining
}
