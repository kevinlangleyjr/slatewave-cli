package installer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
)

// ----- Install dispatch -----

func TestInstall_UnknownTypeErrors(t *testing.T) {
	th := manifest.Theme{
		Theme:   manifest.Meta{Slug: "weirdo"},
		Install: manifest.Install{Type: "magicwand"},
	}
	_, err := Install(th, Options{})
	if err == nil || !strings.Contains(err.Error(), "unknown install type") {
		t.Errorf("Install with unknown type: err = %v, want `unknown install type`", err)
	}
}

func TestInstall_RecordCarriesSlugAndTypes(t *testing.T) {
	// Install populates Slug + InstallType + ActivateType on the record
	// regardless of dispatch outcome — even the unknown-type path returns
	// a populated record. The activator depends on this so it can write
	// state for activate-only re-runs.
	th := manifest.Theme{
		Theme:    manifest.Meta{Slug: "weirdo"},
		Install:  manifest.Install{Type: "magicwand"},
		Activate: manifest.Activate{Type: "ini-key"},
	}
	rec, _ := Install(th, Options{})
	if rec.Slug != "weirdo" {
		t.Errorf("rec.Slug = %q, want weirdo", rec.Slug)
	}
	if rec.InstallType != "magicwand" {
		t.Errorf("rec.InstallType = %q, want magicwand", rec.InstallType)
	}
	if rec.ActivateType != "ini-key" {
		t.Errorf("rec.ActivateType = %q, want ini-key", rec.ActivateType)
	}
}

// ----- curl install -----

// curlTheme builds a manifest.Theme that fetches `url` to `<dir>/<filename>`.
// dir is usually a t.TempDir() so writes don't escape the test sandbox.
func curlTheme(url, dir, filename string) manifest.Theme {
	return manifest.Theme{
		Theme: manifest.Meta{Slug: "bat", Name: "Slatewave for bat"},
		Install: manifest.Install{
			Type: "curl",
			URL:  url,
			Dest: filepath.Join(dir, filename),
		},
	}
}

func TestDoCurl_FetchesAndWritesPayload(t *testing.T) {
	body := "Slatewave theme payload — exactly this text\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	th := curlTheme(srv.URL, dir, "Slatewave.tmTheme")

	rec, err := Install(th, Options{})
	if err != nil {
		t.Fatalf("Install(curl) err = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "Slatewave.tmTheme"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != body {
		t.Errorf("file contents = %q, want %q", string(got), body)
	}
	// CreatedPaths must record the dest so uninstall can reverse cleanly.
	if len(rec.CreatedPaths) != 1 || rec.CreatedPaths[0] != filepath.Join(dir, "Slatewave.tmTheme") {
		t.Errorf("CreatedPaths = %v, want [%s]", rec.CreatedPaths, filepath.Join(dir, "Slatewave.tmTheme"))
	}
}

func TestDoCurl_CreatesIntermediateDirs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	// dest dir doesn't exist yet — installer must mkdir -p.
	dest := filepath.Join(dir, "nested", "deeply", "themes", "Slatewave.tmTheme")
	th := manifest.Theme{
		Theme:   manifest.Meta{Slug: "bat"},
		Install: manifest.Install{Type: "curl", URL: srv.URL, Dest: dest},
	}
	if _, err := Install(th, Options{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("expected file at %s: %v", dest, err)
	}
}

func TestDoCurl_DryRunMakesNoFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// If dry-run actually fetched, this would be hit. Fail the test if so.
		t.Error("dry-run should not have fetched")
	}))
	defer srv.Close()

	dir := t.TempDir()
	th := curlTheme(srv.URL, dir, "Slatewave.tmTheme")
	rec, err := Install(th, Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run Install err = %v", err)
	}
	if len(rec.CreatedPaths) != 0 {
		t.Errorf("dry-run recorded CreatedPaths = %v, want none", rec.CreatedPaths)
	}
	if _, err := os.Stat(filepath.Join(dir, "Slatewave.tmTheme")); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote a file: stat err = %v (want IsNotExist)", err)
	}
}

func TestDoCurl_MissingURLErrors(t *testing.T) {
	dir := t.TempDir()
	th := manifest.Theme{
		Theme:   manifest.Meta{Slug: "bat"},
		Install: manifest.Install{Type: "curl", URL: "", Dest: filepath.Join(dir, "x")},
	}
	_, err := Install(th, Options{})
	if err == nil || !strings.Contains(err.Error(), "no install.url") {
		t.Errorf("missing URL: err = %v, want `no install.url`", err)
	}
}

// A server that hangs forever shouldn't hang the CLI. The package-level
// httpClient sets a 60s ceiling; the test shortens it to a few hundred
// ms so the assertion is fast. A failure here means a flaky-network user
// would see `slatewave install` freeze with no feedback.
//
// The handler blocks on the request context — when the client times out
// and drops the connection, the server cancels the context, so srv.Close()
// can return without deadlocking on a still-running handler.
func TestDoCurl_HungServerHitsHTTPTimeout(t *testing.T) {
	defer SetHTTPTimeoutForTest(200 * time.Millisecond)()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	dir := t.TempDir()
	th := curlTheme(srv.URL, dir, "x.tmTheme")

	start := time.Now()
	_, err := Install(th, Options{})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("hung server: want timeout error, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("hung server: timeout fired after %v, want ~200ms", elapsed)
	}
}

// Atomicity check: when the fetch fails (here, oversized response), an
// existing file at dest must be left untouched. Pre-Phase-2 the install
// path wrote directly to dest, so a failed mid-write would have left a
// truncated file. With writeAtomic + temp-rename, dest is only touched
// after a successful copy.
func TestDoCurl_FetchFailureLeavesExistingDestUntouched(t *testing.T) {
	defer SetMaxFetchBytesForTest(16)()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 64)))
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "Slatewave.tmTheme")
	original := []byte("original theme content — must survive a failed update\n")
	if err := os.WriteFile(dest, original, 0o644); err != nil {
		t.Fatal(err)
	}

	th := curlTheme(srv.URL, dir, "Slatewave.tmTheme")
	if _, err := Install(th, Options{}); err == nil {
		t.Fatal("oversized response: want error, got nil")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("dest file disappeared after failed fetch: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("dest file was overwritten by failed fetch:\ngot  %q\nwant %q", got, original)
	}
}

// A response that exceeds the per-fetch byte cap must error out before
// it fills the disk. Test shrinks the cap to 16 bytes and serves 64 — if
// the cap fires, the dest file is removed too (no partial theme file
// left behind for callers to treat as installed).
func TestDoCurl_RejectsOversizedResponse(t *testing.T) {
	defer SetMaxFetchBytesForTest(16)()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 64)))
	}))
	defer srv.Close()

	dir := t.TempDir()
	th := curlTheme(srv.URL, dir, "x.tmTheme")

	_, err := Install(th, Options{})
	if err == nil {
		t.Fatal("oversized response: want error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("oversized response: err = %v, want `exceeds` in message", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.tmTheme")); !os.IsNotExist(err) {
		t.Errorf("oversized response: partial file left at dest, stat err = %v", err)
	}
}

// A server returning text/html is almost never serving a theme — it's a
// captive portal, error page, or redirect target. Reject before writing
// to the user's config dir.
func TestDoCurl_RejectsHTMLContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>not a theme</html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	th := curlTheme(srv.URL, dir, "x.tmTheme")

	_, err := Install(th, Options{})
	if err == nil {
		t.Fatal("text/html response: want error, got nil")
	}
	if !strings.Contains(err.Error(), "text/html") {
		t.Errorf("text/html response: err = %v, want `text/html` in message", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.tmTheme")); !os.IsNotExist(err) {
		t.Errorf("text/html response: file written despite rejection, stat err = %v", err)
	}
}

func TestDoCurl_404SurfacesAsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	dir := t.TempDir()
	th := curlTheme(srv.URL, dir, "x.tmTheme")
	_, err := Install(th, Options{})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("404 response: err = %v, want a 404", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.tmTheme")); !os.IsNotExist(err) {
		t.Errorf("404 still wrote file: stat err = %v", err)
	}
}

// ----- multi-file curl install -----

// Multi-file curl fetches every entry in install.files in order, records
// each dest as a CreatedPath, and triggers the post-hook once at the end.
// Wezterm's slatewave-full + slatewave.lua dependency is the canonical
// case.
func TestDoCurl_MultiFile_FetchesAllAndRecordsCreatedPaths(t *testing.T) {
	bodies := map[string]string{
		"/full.lua": "-- slatewave-full payload\n",
		"/lib.lua":  "-- slatewave lib payload\n",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	fullDest := filepath.Join(dir, "slatewave-full.lua")
	libDest := filepath.Join(dir, "slatewave.lua")
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "wezterm"},
		Install: manifest.Install{
			Type: "curl",
			Files: []manifest.InstallFile{
				{URL: srv.URL + "/full.lua", Dest: fullDest},
				{URL: srv.URL + "/lib.lua", Dest: libDest},
			},
		},
	}

	rec, err := Install(th, Options{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if got, _ := os.ReadFile(fullDest); string(got) != bodies["/full.lua"] {
		t.Errorf("full.lua contents = %q, want %q", got, bodies["/full.lua"])
	}
	if got, _ := os.ReadFile(libDest); string(got) != bodies["/lib.lua"] {
		t.Errorf("slatewave.lua contents = %q, want %q", got, bodies["/lib.lua"])
	}
	// Both dests must be in CreatedPaths so uninstall removes them.
	if len(rec.CreatedPaths) != 2 {
		t.Fatalf("CreatedPaths = %v, want 2 entries", rec.CreatedPaths)
	}
	if rec.CreatedPaths[0] != fullDest || rec.CreatedPaths[1] != libDest {
		t.Errorf("CreatedPaths order/contents wrong: %v", rec.CreatedPaths)
	}
}

// Mixing url/dest with files is a manifest-authoring mistake — surface
// it as an error rather than silently picking one branch.
func TestDoCurl_MultiFile_RejectsMixedShape(t *testing.T) {
	dir := t.TempDir()
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "wezterm"},
		Install: manifest.Install{
			Type:  "curl",
			URL:   "https://example.com/x",
			Dest:  filepath.Join(dir, "x"),
			Files: []manifest.InstallFile{{URL: "https://example.com/y", Dest: filepath.Join(dir, "y")}},
		},
	}
	_, err := Install(th, Options{})
	if err == nil || !strings.Contains(err.Error(), "pick one") {
		t.Errorf("mixed shape: err = %v, want `pick one`", err)
	}
}

// A files entry missing url or dest must error before any fetch happens
// — no half-installed state.
func TestDoCurl_MultiFile_RejectsIncompleteEntry(t *testing.T) {
	dir := t.TempDir()
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "wezterm"},
		Install: manifest.Install{
			Type: "curl",
			Files: []manifest.InstallFile{
				{URL: "https://example.com/x", Dest: filepath.Join(dir, "x")},
				{URL: "", Dest: filepath.Join(dir, "y")}, // bad entry
			},
		},
	}
	_, err := Install(th, Options{})
	if err == nil || !strings.Contains(err.Error(), "files[1]") {
		t.Errorf("incomplete entry: err = %v, want `files[1]`", err)
	}
}

// Dry-run on a multi-file install must not contact the network or write
// any files (the test server fails the run if it gets a request).
func TestDoCurl_MultiFile_DryRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("dry-run should not have fetched")
	}))
	defer srv.Close()

	dir := t.TempDir()
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "wezterm"},
		Install: manifest.Install{
			Type: "curl",
			Files: []manifest.InstallFile{
				{URL: srv.URL + "/x", Dest: filepath.Join(dir, "x")},
				{URL: srv.URL + "/y", Dest: filepath.Join(dir, "y")},
			},
		},
	}
	rec, err := Install(th, Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run Install: %v", err)
	}
	if len(rec.CreatedPaths) != 0 {
		t.Errorf("dry-run recorded CreatedPaths = %v, want none", rec.CreatedPaths)
	}
}

func TestDoCurl_PostHookRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix `touch` command; cmd.exe equivalent involves redirect and would need a separate test")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	marker := filepath.Join(dir, "post-ran.marker")
	th := curlTheme(srv.URL, dir, "theme.tmTheme")
	th.Install.Post = &manifest.PostHook{
		Description: "Touch marker",
		Command:     "touch " + marker,
	}

	if _, err := Install(th, Options{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("post-hook marker not created: %v", err)
	}
}

func TestDoCurl_PostHookFailureSurfaces(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("`exit 1` semantics differ in cmd.exe — covered on linux/macos")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	th := curlTheme(srv.URL, dir, "theme.tmTheme")
	th.Install.Post = &manifest.PostHook{
		Description: "Always fail",
		Command:     "exit 1",
	}

	_, err := Install(th, Options{})
	if err == nil || !strings.Contains(err.Error(), "post-hook") {
		t.Errorf("post-hook failure: err = %v, want `post-hook`", err)
	}
	// Note: file is intentionally still on disk — current behavior. If we
	// add rollback later, this assertion will need to flip.
	if _, statErr := os.Stat(filepath.Join(dir, "theme.tmTheme")); statErr != nil {
		t.Errorf("payload missing after post-hook failure (current behavior keeps it): %v", statErr)
	}
}

// ----- validation: dispatchers that error before doing anything -----

func TestDoMarketplace_MissingURLErrors(t *testing.T) {
	th := manifest.Theme{
		Theme:   manifest.Meta{Slug: "jetbrains"},
		Install: manifest.Install{Type: "marketplace", URL: ""},
	}
	_, err := Install(th, Options{})
	if err == nil || !strings.Contains(err.Error(), "no install.url") {
		t.Errorf("marketplace missing URL: err = %v", err)
	}
}

func TestDoMarketplace_DryRunSkipsBrowserOpen(t *testing.T) {
	// We can't unit-test the browser open, but we *can* verify dry-run
	// short-circuits before openURL — otherwise the test would actually
	// open a browser tab on every run.
	th := manifest.Theme{
		Theme:   manifest.Meta{Slug: "jetbrains"},
		Install: manifest.Install{Type: "marketplace", URL: "https://example.invalid"},
	}
	_, err := Install(th, Options{DryRun: true})
	if err != nil {
		t.Errorf("marketplace dry-run err = %v, want nil", err)
	}
}

func TestDoVSCodeExt_MissingIdentifierErrors(t *testing.T) {
	th := manifest.Theme{
		Theme:   manifest.Meta{Slug: "vscode"},
		Install: manifest.Install{Type: "vscode-ext", Identifier: ""},
	}
	_, err := Install(th, Options{})
	if err == nil || !strings.Contains(err.Error(), "no install.identifier") {
		t.Errorf("vscode-ext missing identifier: err = %v", err)
	}
}

func TestDoVSCodeExt_DryRunSkipsCodeBinary(t *testing.T) {
	// Without dry-run this would try to exec `code --install-extension`
	// which probably isn't on a CI runner's PATH.
	th := manifest.Theme{
		Theme:   manifest.Meta{Slug: "vscode"},
		Install: manifest.Install{Type: "vscode-ext", Identifier: "kevinlangleyjr.slatewave"},
	}
	if _, err := Install(th, Options{DryRun: true}); err != nil {
		t.Errorf("vscode-ext dry-run err = %v", err)
	}
}

func TestVSCodeExtCLI_DefaultsToCode(t *testing.T) {
	// Empty install.cli → "code". Locks the default so existing
	// vscode manifests keep shelling out to `code` unchanged.
	th := manifest.Theme{Install: manifest.Install{Type: "vscode-ext"}}
	if got := VSCodeExtCLI(th); got != "code" {
		t.Errorf("default VSCodeExtCLI = %q, want %q", got, "code")
	}
}

func TestVSCodeExtCLI_HonorsManifestOverride(t *testing.T) {
	// Cursor / VSCodium manifests set install.cli explicitly. The
	// helper must return the manifest value verbatim so the install,
	// update, and uninstall handlers all shell out to the same binary.
	th := manifest.Theme{Install: manifest.Install{Type: "vscode-ext", CLI: "cursor"}}
	if got := VSCodeExtCLI(th); got != "cursor" {
		t.Errorf("cursor override VSCodeExtCLI = %q, want %q", got, "cursor")
	}
}

func TestDoClone_MissingRepoErrors(t *testing.T) {
	th := manifest.Theme{
		Theme:   manifest.Meta{Slug: "btop"},
		Install: manifest.Install{Type: "clone", Repo: "", CloneDest: t.TempDir() + "/btop-slatewave"},
	}
	_, err := Install(th, Options{})
	if err == nil || !strings.Contains(err.Error(), "no install.repo") {
		t.Errorf("clone missing repo: err = %v", err)
	}
}

func TestDoClone_DestAlreadyExistsErrors(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "btop-slatewave")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	th := manifest.Theme{
		Theme:   manifest.Meta{Slug: "btop"},
		Install: manifest.Install{Type: "clone", Repo: "https://example.invalid/btop", CloneDest: dest},
	}
	_, err := Install(th, Options{})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("clone existing dest: err = %v, want `already exists`", err)
	}
}

func TestDoClone_DryRunSkipsGit(t *testing.T) {
	th := manifest.Theme{
		Theme:   manifest.Meta{Slug: "btop"},
		Install: manifest.Install{Type: "clone", Repo: "https://example.invalid/btop", CloneDest: t.TempDir() + "/btop"},
	}
	if _, err := Install(th, Options{DryRun: true}); err != nil {
		t.Errorf("clone dry-run err = %v", err)
	}
}

// ----- type: manual -----

func TestDoManual_IsAlwaysNoOp(t *testing.T) {
	th := manifest.Theme{
		Theme:   manifest.Meta{Slug: "slack"},
		Install: manifest.Install{Type: "manual"},
	}
	rec, err := Install(th, Options{})
	if err != nil {
		t.Errorf("manual Install err = %v", err)
	}
	if len(rec.CreatedPaths) != 0 {
		t.Errorf("manual install recorded paths: %v", rec.CreatedPaths)
	}
}

// ----- pickCloneDest -----

func TestPickCloneDest_FallsBackToCloneDestWhenNoOverride(t *testing.T) {
	th := manifest.Theme{Install: manifest.Install{CloneDest: "/fallback"}}
	if got := pickCloneDest(th); got != "/fallback" {
		t.Errorf("no overrides → expected fallback, got %q", got)
	}
}

func TestPickCloneDest_PicksOverrideForCurrentOS(t *testing.T) {
	// Set the override matching the current GOOS and assert it wins over
	// CloneDest. We can't change runtime.GOOS at runtime so this test
	// only covers the runtime's actual OS — but cross-platform CI (now
	// matrixed across linux + macos) covers both branches in aggregate.
	overrides := manifest.Install{
		CloneDest:        "/fallback",
		CloneDestDarwin:  "/mac-only",
		CloneDestLinux:   "/linux-only",
		CloneDestWindows: "/win-only",
	}
	got := pickCloneDest(manifest.Theme{Install: overrides})
	switch got {
	case "/mac-only", "/linux-only", "/win-only":
		// expected — one of the per-OS branches fired
	case "/fallback":
		t.Errorf("per-OS override didn't win over CloneDest: got %q", got)
	default:
		t.Errorf("unexpected pick: %q", got)
	}
}

func TestPickCloneDest_FallsBackWhenOnlyOtherOSesHaveOverrides(t *testing.T) {
	// Set overrides for OSes other than the current one — should fall
	// through to CloneDest. Same caveat as above: only exercises the
	// non-matching branch for the runtime's current OS.
	var overrides manifest.Install
	overrides.CloneDest = "/fallback"
	switch runtime.GOOS {
	case "darwin":
		overrides.CloneDestLinux = "/linux-only"
		overrides.CloneDestWindows = "/win-only"
	case "linux":
		overrides.CloneDestDarwin = "/mac-only"
		overrides.CloneDestWindows = "/win-only"
	case "windows":
		overrides.CloneDestDarwin = "/mac-only"
		overrides.CloneDestLinux = "/linux-only"
	default:
		t.Skipf("unrecognized GOOS=%s", runtime.GOOS)
	}
	if got := pickCloneDest(manifest.Theme{Install: overrides}); got != "/fallback" {
		t.Errorf("expected fallback (no override for current OS), got %q", got)
	}
}

// ----- Detect -----

func TestDetect_EmptyCommandIsNil(t *testing.T) {
	th := manifest.Theme{Theme: manifest.Meta{Slug: "x", Name: "X", DetectCommand: ""}}
	if err := Detect(th); err != nil {
		t.Errorf("Detect with empty command err = %v, want nil", err)
	}
}

func TestDetect_ZeroExitIsNil(t *testing.T) {
	th := manifest.Theme{Theme: manifest.Meta{Slug: "x", Name: "X", DetectCommand: "true"}}
	if err := Detect(th); err != nil {
		t.Errorf("Detect with `true` err = %v, want nil", err)
	}
}

func TestDetect_NonZeroExitErrors(t *testing.T) {
	th := manifest.Theme{Theme: manifest.Meta{Slug: "x", Name: "X", DetectCommand: "false"}}
	err := Detect(th)
	if err == nil {
		t.Fatal("Detect with `false` returned nil, want error")
	}
	// Error must surface the manifest's tool name + the command — that's
	// what the user sees in their terminal when detect fails.
	if !strings.Contains(err.Error(), "X not detected") {
		t.Errorf("Detect error didn't mention theme name: %v", err)
	}
	if !strings.Contains(err.Error(), "false") {
		t.Errorf("Detect error didn't mention the command: %v", err)
	}
}

// A hung detect_command must not freeze Detect — the surrounding install
// or doctor pipeline relies on Detect returning quickly. The test shrinks
// the timeout to 200ms and points at a sleep that would otherwise run for
// 60s, then asserts Detect returns inside ~1s with a "not detected" error
// (a timed-out detect is observationally identical to a failing detect).
func TestDetect_HangingCommandHitsTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires `sleep` on PATH; cmd.exe equivalent (timeout) has different semantics — covered on linux/macos")
	}
	defer SetDetectTimeoutForTest(200 * time.Millisecond)()

	th := manifest.Theme{Theme: manifest.Meta{Slug: "x", Name: "X", DetectCommand: "sleep 60"}}

	start := time.Now()
	err := Detect(th)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("hanging detect_command: want error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("hanging detect_command: returned after %v, want ~200ms", elapsed)
	}
	if !strings.Contains(err.Error(), "X not detected") {
		t.Errorf("Detect error didn't mention theme name: %v", err)
	}
}

// ----- expandPath round-trip with $HOME swap -----

func TestExpandPath_TildeAndHomeMatchAfterSetenv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses unix $HOME semantics; tilde resolution on Windows reads USERPROFILE and isn't symmetric with $HOME expansion via os.ExpandEnv")
	}
	// Sanity check that ~ and $HOME resolve to the same place — uninstall
	// pathing relies on this, and the env var path isn't covered above.
	t.Setenv("HOME", "/tmp/fakehome")
	tilde, err := expandPath("~/foo")
	if err != nil {
		t.Fatal(err)
	}
	envvar, err := expandPath("$HOME/foo")
	if err != nil {
		t.Fatal(err)
	}
	if tilde != envvar {
		t.Errorf("~/foo (%q) != $HOME/foo (%q)", tilde, envvar)
	}
}
