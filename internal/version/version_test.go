package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in    string
		major int
		minor int
		patch int
		ok    bool
	}{
		{"v0.0.14", 0, 0, 14, true},
		{"v1.2.3", 1, 2, 3, true},
		{"0.0.14", 0, 0, 14, true},                    // missing leading "v" still works
		{"v0.0.14-19-ge934821-dirty", 0, 0, 14, true}, // dev suffix stripped
		{"dev", 0, 0, 0, false},
		{"v0.1", 0, 0, 0, false}, // wrong arity
		{"vfoo", 0, 0, 0, false}, // non-numeric
		{"", 0, 0, 0, false},
	}
	for _, c := range cases {
		ma, mi, pa, ok := parseVersion(c.in)
		if ma != c.major || mi != c.minor || pa != c.patch || ok != c.ok {
			t.Errorf("parseVersion(%q) = %d.%d.%d ok=%v, want %d.%d.%d ok=%v",
				c.in, ma, mi, pa, ok, c.major, c.minor, c.patch, c.ok)
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.0.15", "v0.0.14", true},
		{"v0.1.0", "v0.0.99", true},
		{"v1.0.0", "v0.99.99", true},
		{"v0.0.14", "v0.0.14", false}, // equal
		{"v0.0.13", "v0.0.14", false}, // older
		{"dev", "v0.0.14", false},     // unparseable latest → false
		{"v0.0.15", "dev", false},     // unparseable current → false
	}
	for _, c := range cases {
		got := isNewer(c.latest, c.current)
		if got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

// SLATEWAVE_NO_UPDATE_CHECK=1 must short-circuit before any cache read
// or network call. Channel closes immediately with no value sent.
func TestCheck_DisabledByEnvVar(t *testing.T) {
	t.Setenv("SLATEWAVE_NO_UPDATE_CHECK", "1")

	ch := Check("v0.0.14")
	select {
	case res, ok := <-ch:
		if ok && res != nil {
			t.Errorf("disabled check returned %+v, want closed channel with no value", res)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("disabled check didn't close channel within 500ms")
	}
}

// A "dev" build can't be compared meaningfully — the check must
// short-circuit (no API call, no nag).
func TestCheck_DevBuildShortCircuits(t *testing.T) {
	t.Setenv("SLATEWAVE_NO_UPDATE_CHECK", "0")

	ch := Check("dev")
	select {
	case res, ok := <-ch:
		if ok && res != nil {
			t.Errorf("dev build returned %+v, want closed channel", res)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("dev-build check didn't close channel within 500ms")
	}
}

// Cache hit on a fresh entry skips the API entirely. Seed the cache,
// run Check, assert no API hit (we don't mock the API, so just confirm
// the result matches the cache).
func TestCheck_FreshCacheReturnsCachedResult(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("SLATEWAVE_NO_UPDATE_CHECK", "0")

	cachePath, _ := cachePath()
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(cacheFile{
		CheckedAt: time.Now().UTC(),
		Latest:    "v9.9.9",
		URL:       "https://example.com/release",
	})
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	ch := Check("v0.0.14")
	select {
	case res, ok := <-ch:
		if !ok || res == nil {
			t.Fatal("expected newer-version result from cache, got closed channel")
		}
		if res.Latest != "v9.9.9" {
			t.Errorf("Latest = %q, want v9.9.9 (from cache)", res.Latest)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Check didn't return within 500ms (should be near-instant from cache)")
	}
}

// Stale cache → fetch from API. Test points the apiURL var at a local
// httptest server that serves a canned release tag, then asserts the
// cache gets refreshed and the result matches.
func TestCheck_StaleCacheFetchesAndRefreshes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("SLATEWAVE_NO_UPDATE_CHECK", "0")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","html_url":"https://example.com/release"}`))
	}))
	defer srv.Close()

	prevAPI := apiURL
	apiURL = srv.URL
	defer func() { apiURL = prevAPI }()

	// Seed a stale cache (2 days old).
	cachePath, _ := cachePath()
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	stale, _ := json.Marshal(cacheFile{
		CheckedAt: time.Now().UTC().Add(-48 * time.Hour),
		Latest:    "v0.0.1",
	})
	if err := os.WriteFile(cachePath, stale, 0o644); err != nil {
		t.Fatal(err)
	}

	ch := Check("v0.0.14")
	select {
	case res, ok := <-ch:
		if !ok || res == nil {
			t.Fatal("expected newer-version result, got closed channel")
		}
		if res.Latest != "v1.2.3" {
			t.Errorf("Latest = %q, want v1.2.3 (from API)", res.Latest)
		}
		if res.URL != "https://example.com/release" {
			t.Errorf("URL = %q, want https://example.com/release", res.URL)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Check didn't return within 2s (API roundtrip should be fast against httptest)")
	}

	// Cache should now reflect the fresh fetch.
	data, _ := os.ReadFile(cachePath)
	var c cacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if c.Latest != "v1.2.3" {
		t.Errorf("post-fetch cache latest = %q, want v1.2.3", c.Latest)
	}
}
