package installer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

func TestDoCurl_PostHookRuns(t *testing.T) {
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

// ----- expandPath round-trip with $HOME swap -----

func TestExpandPath_TildeAndHomeMatchAfterSetenv(t *testing.T) {
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
