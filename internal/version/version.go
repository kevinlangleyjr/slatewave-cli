// Package version checks the GitHub Releases API for a newer slatewave
// build, caches the result for 24h, and surfaces a one-line nag when
// the user is on an out-of-date binary. Best-effort: any error path
// (no network, unparseable response, write-protected home dir) silently
// returns no result so a hermetic environment never breaks the CLI.
//
// SLATEWAVE_NO_UPDATE_CHECK=1 disables the check entirely — set this
// in CI / Docker / hermetic build pipelines that shouldn't reach out
// to api.github.com on every CLI invocation.
package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Result carries the latest published release. URL points at the
// human-readable releases page (not the API endpoint) so the nag is
// click-friendly.
type Result struct {
	Latest string
	URL    string
}

// cacheTTL is how long we trust a previously-fetched release tag
// before going back to the API. 24h hits the right balance: the user
// sees an upgrade prompt within a day of release, and we don't rate-
// limit ourselves on every short-lived `slatewave list` invocation.
const cacheTTL = 24 * time.Hour

// apiURL is the GitHub Releases API endpoint we hit. Held as a var
// (not const) so tests can swap in an httptest server URL without
// reaching production.
var apiURL = "https://api.github.com/repos/kevinlangleyjr/slatewave-cli/releases/latest"

const releasesURL = "https://github.com/kevinlangleyjr/slatewave-cli/releases/latest"

// httpClient bounds the API call. The whole check runs async on a
// goroutine the cmd layer waits on for ≤200ms; a 5s ceiling here
// means a slow API response just times out cleanly without blocking
// the user's actual command past the 200ms wait.
var httpClient = &http.Client{Timeout: 5 * time.Second}

// Check kicks off an async check and returns a channel that emits
// exactly one *Result (or nil if there's no newer version, the user
// is on a dev build, the check is disabled, or the network failed)
// and then closes.
//
// Callers should wait on the channel with a short timeout so a slow
// API response doesn't delay the user's command. If the wait expires
// before the goroutine returns, the cache may still get written in
// the background — next run picks it up.
func Check(current string) <-chan *Result {
	ch := make(chan *Result, 1)
	if os.Getenv("SLATEWAVE_NO_UPDATE_CHECK") == "1" {
		close(ch)
		return ch
	}
	if !parseable(current) {
		// "dev" / dirty / non-vX.Y.Z builds — there's no meaningful
		// comparison to make. Skip silently.
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)
		latest, url := loadCache()
		fresh := !cacheStale()
		if !fresh || latest == "" {
			fetched, fetchedURL, err := fetchLatest(context.Background())
			if err != nil {
				return
			}
			latest, url = fetched, fetchedURL
			_ = saveCache(latest, url)
		}
		if isNewer(latest, current) {
			ch <- &Result{Latest: latest, URL: url}
		}
	}()
	return ch
}

type cacheFile struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
	URL       string    `json:"url"`
}

func cachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "slatewave", "version-check.json"), nil
}

func loadCache() (latest, url string) {
	path, err := cachePath()
	if err != nil {
		return "", ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	var c cacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		return "", ""
	}
	return c.Latest, c.URL
}

func cacheStale() bool {
	path, err := cachePath()
	if err != nil {
		return true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	var c cacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		return true
	}
	return time.Since(c.CheckedAt) > cacheTTL
}

func saveCache(latest, url string) error {
	path, err := cachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cacheFile{
		CheckedAt: time.Now().UTC(),
		Latest:    latest,
		URL:       url,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// fetchLatest hits the GitHub Releases API for the repo's latest
// release. Only the tag_name field is consumed — everything else
// (assets, body, etc.) is irrelevant to the version comparison.
func fetchLatest(ctx context.Context) (latest, url string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", err
	}
	// Send Accept so GitHub uses the v3 schema explicitly. Without it
	// they may default to whatever is current at request time.
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("github api: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return "", "", err
	}
	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", err
	}
	if payload.TagName == "" {
		return "", "", fmt.Errorf("github api: empty tag_name")
	}
	if payload.HTMLURL == "" {
		payload.HTMLURL = releasesURL
	}
	return payload.TagName, payload.HTMLURL, nil
}

// parseable reports whether s looks like a vX.Y.Z release tag we can
// compare. "dev" / dirty / non-conforming strings return false.
func parseable(s string) bool {
	_, _, _, ok := parseVersion(s)
	return ok
}

// parseVersion extracts (major, minor, patch) from a release tag.
// Supports the canonical vX.Y.Z shape and the dev-build form
// "vX.Y.Z-N-gHASH-dirty" (split on the first dash).
func parseVersion(s string) (major, minor, patch int, ok bool) {
	s = strings.TrimPrefix(s, "v")
	if i := strings.Index(s, "-"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], true
}

// isNewer reports whether `latest` semver-numerically beats `current`.
// Equal or unparseable versions return false so a parse failure can't
// produce a false-positive nag.
func isNewer(latest, current string) bool {
	la, lb, lc, ok1 := parseVersion(latest)
	ca, cb, cc, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	if la != ca {
		return la > ca
	}
	if lb != cb {
		return lb > cb
	}
	return lc > cc
}
