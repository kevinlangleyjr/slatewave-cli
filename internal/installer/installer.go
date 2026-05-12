// Package installer dispatches the install step of a theme manifest.
// Each [install].type in the manifest maps to a function below.
package installer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/shell"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/verbose"
)

// httpClient is the shared client every fetch in this package routes
// through. The default http.Get / http.DefaultClient has no timeout, so
// a hung server (or a TCP black hole on flaky Wi-Fi) would freeze
// `slatewave install` indefinitely with no way out except Ctrl-C. A
// 60-second cap covers slow networks for a few-MB theme file without
// being so generous it masks a real outage.
var httpClient = &http.Client{Timeout: 60 * time.Second}

// SetHTTPTimeoutForTest swaps httpClient.Timeout and returns a restorer.
// Tests use it to assert timeout behavior without sleeping for 60s.
// Safe to call from tests because the installer package's tests don't
// use t.Parallel().
func SetHTTPTimeoutForTest(d time.Duration) func() {
	prev := httpClient.Timeout
	httpClient.Timeout = d
	return func() { httpClient.Timeout = prev }
}

// detectTimeout caps each Detect call so a misbehaving detect_command
// (`command -v <tool>` against a hung mount, an infinite-loop shell
// alias, etc.) can't freeze `slatewave install` or `slatewave doctor`.
// Five seconds is generous — every detect command in the embedded
// manifest set runs in single-digit milliseconds.
//
// The TUI's parallel detect path (internal/tui/detect.go) has its own
// 3s timeout per row. They don't interact: that one is for the
// init/browse discovery sweep, this one for explicit single-theme
// operations.
var detectTimeout = 5 * time.Second

// SetDetectTimeoutForTest swaps detectTimeout and returns a restorer.
func SetDetectTimeoutForTest(d time.Duration) func() {
	prev := detectTimeout
	detectTimeout = d
	return func() { detectTimeout = prev }
}

// maxFetchBytes caps the response body any curl/refetch path will write.
// Theme files are universally tiny (the largest in the embedded set is a
// few hundred KB); 100 MiB is a paranoid ceiling against a compromised
// or misconfigured CDN streaming an unbounded response and filling the
// user's disk.
var maxFetchBytes int64 = 100 << 20

// SetMaxFetchBytesForTest swaps maxFetchBytes and returns a restorer.
func SetMaxFetchBytesForTest(n int64) func() {
	prev := maxFetchBytes
	maxFetchBytes = n
	return func() { maxFetchBytes = prev }
}

// rejectHTMLContentType returns an error when the response advertises
// text/html. No theme in the registry is HTML, so a text/html response
// is almost always a captive-portal landing page, a 200-status error
// page from a misconfigured server, or an unexpected redirect target —
// silently writing it to the user's config directory would just confuse
// their tool downstream. Other content types pass through (theme files
// vary widely: .tmTheme is XML, .toml is text/plain or octet-stream,
// .lua is whatever the host serves it as).
func rejectHTMLContentType(url, ct string) error {
	if strings.HasPrefix(strings.ToLower(ct), "text/html") {
		return fmt.Errorf("fetch %s: server returned text/html (Content-Type: %s) — expected theme content", url, ct)
	}
	return nil
}

// copyCapped wraps body in an io.LimitReader sized to maxFetchBytes+1
// before copying to dst. Returns an error if the response would exceed
// the cap (catches a runaway server before it fills the disk).
func copyCapped(dst io.Writer, body io.Reader, url string) error {
	limited := io.LimitReader(body, maxFetchBytes+1)
	n, err := io.Copy(dst, limited)
	if err != nil {
		return err
	}
	if n > maxFetchBytes {
		return fmt.Errorf("fetch %s: response exceeds %d-byte cap", url, maxFetchBytes)
	}
	return nil
}

// preservedMode returns the user's existing file mode so a rewrite
// doesn't silently downgrade it. Returns 0o644 when file doesn't
// exist (the caller is creating fresh — readable-by-others matches
// the convention for unix config files).
//
// Used at every site that overwrites a user-owned file or restores a
// backup. The activator has its own copy with the same shape; both
// could move into a shared internal/fsutil if a third caller appears.
func preservedMode(file string) os.FileMode {
	info, err := os.Stat(file)
	if err != nil {
		return 0o644
	}
	return info.Mode().Perm()
}

// writeAtomic streams the bytes the caller writes via the `write`
// callback into a temp file in dest's directory, then renames over
// dest. Either dest ends up with the full new content, or it stays
// untouched — partial files never appear at the destination path.
//
// The parent directory is created if missing. The temp file is removed
// on any error along the way (callback failure, close failure, rename
// failure) so a failed write doesn't leak `.slatewave-write-*` files
// into the user's config dirs.
//
// mode is chmod'd onto the temp file before rename. os.CreateTemp
// defaults to 0o600, which would silently downgrade theme files vs.
// the os.Create(dest) shape this helper replaced — callers pass an
// explicit mode (typically 0o644 for fresh writes; preservedMode of
// the existing file when overwriting user config).
func writeAtomic(dest string, mode os.FileMode, write func(io.Writer) error) error {
	verbose.Log("write: %s (mode %o)", dest, mode)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".slatewave-write-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp.Name())
		}
	}()
	if err := write(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return fmt.Errorf("rename temp to %s: %w", dest, err)
	}
	cleanup = false
	return nil
}

// fetchAtomic downloads url and writes the response to dest atomically
// (temp + rename). Replaces both the install-side fetchToFile (was
// non-atomic — could leave partial files on interrupt) and the
// update-side atomicRefetch (already had this shape). One implementation
// for both flows means the install path now gets the same corruption
// guarantee the update path always had.
//
// Status, content-type, and body-size guards run before any write hits
// the temp file. ctx aborts the request — Ctrl-C in the TUI / SIGINT in
// the CLI kills an in-flight download instead of blocking on the body
// read until the httpClient timeout fires.
func fetchAtomic(ctx context.Context, url, dest string) error {
	verbose.Log("fetch: %s → %s", url, dest)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: %s", url, resp.Status)
	}
	if err := rejectHTMLContentType(url, resp.Header.Get("Content-Type")); err != nil {
		return err
	}
	return writeAtomic(dest, 0o644, func(w io.Writer) error {
		return copyCapped(w, resp.Body, url)
	})
}

// Options controls install behavior at the call site.
type Options struct {
	DryRun bool
}

// installerImpl bundles the per-type handlers for one install type
// (curl, clone, etc.). The registry below maps each install.type
// string to its impl — Install, Update, and Uninstall all dispatch
// through that one map instead of three parallel switch statements.
//
// Adding a new install type means: write the install/update funcs
// (and an uninstall hook if the type needs one beyond the generic
// CreatedPaths + Backups reversal), then add one entry to installers.
// No changes to Install/Update/Uninstall themselves.
//
// install + update accept a context so a caller (the TUI on Ctrl-C,
// the CLI on SIGINT) can cancel an in-flight `git clone` / VS Code
// extension shell-out / post-hook. uninstallExtra is currently
// context-less — uninstall's blast radius is small enough that the
// hang risk wasn't worth the API churn; threading it through is a
// follow-up if a real cancellation case emerges.
type installerImpl struct {
	// install fetches assets and returns the populated record. Required.
	install func(ctx context.Context, t manifest.Theme, rec state.Record, opts Options) (state.Record, error)

	// update re-fetches assets without re-running activation. Optional —
	// nil means this install type has no automated update path
	// (marketplace and manual are the obvious cases) and Update returns
	// ErrNoAutomatedUpdate.
	update func(ctx context.Context, t manifest.Theme, opts Options) error

	// uninstallExtra runs after the generic reversal (deleting
	// CreatedPaths, restoring Backups, removing AppendedLine). Optional —
	// only types whose install didn't write to disk directly need this
	// (vscode-ext shells out to `code --uninstall-extension`).
	uninstallExtra func(rec state.Record, t manifest.Theme, opts Options) error
}

var installers = map[string]installerImpl{
	"curl": {
		install: doCurl,
		update:  refetch,
	},
	"clone": {
		install: doClone,
		update:  gitPull,
	},
	"vscode-ext": {
		install:        doVSCodeExt,
		update:         reinstallVSCodeExt,
		uninstallExtra: uninstallVSCodeExt,
	},
	"marketplace": {
		install: doMarketplace,
		// no update — opening the page is a manual action
	},
	"gui-import": {
		install: doGUIImport,
		update:  refetch,
	},
	"manual": {
		install: doManual,
		// no update — manual instructions are inherently manual
	},
}

// Install runs the install step for theme t. On success the returned
// Record is populated with reversal info (created paths, etc.) but
// its Activate fields are still empty — the activator fills those.
//
// ctx is honored by every subprocess the install pipeline spawns —
// `git clone`, the VS Code extension CLI, and the optional post-hook —
// so a TUI Ctrl-C or a CLI SIGINT kills the in-flight child instead of
// orphaning it. Callers that don't need cancellation pass
// context.Background(); tests do the same.
//
// When the manifest declares Install.Variants, Install first detects
// the installed tool version (via Theme.DetectCommand + VersionRegex)
// and overlays the matching variant's URL/Dest/Files onto t.Install
// before dispatch. A version-detect failure or unmatched-but-required
// variant is propagated as a normal install error — never silently
// fall back, since that would re-ship the wrong file to exactly the
// hosts the variant exists to handle.
func Install(ctx context.Context, t manifest.Theme, opts Options) (state.Record, error) {
	rec := state.Record{
		Slug:         t.Theme.Slug,
		InstalledAt:  time.Now().UTC(),
		InstallType:  t.Install.Type,
		ActivateType: t.Activate.Type,
	}
	if len(t.Install.Variants) > 0 {
		ver, err := detectVersion(t)
		if err != nil {
			return rec, fmt.Errorf("detect version for %q: %w", t.Theme.Slug, err)
		}
		if ver != "" {
			v, err := resolveVariant(t.Install.Variants, ver)
			if err != nil {
				return rec, fmt.Errorf("resolve variant for %q: %w", t.Theme.Slug, err)
			}
			t.Install = applyVariant(t.Install, v)
		}
	}
	impl, ok := installers[t.Install.Type]
	if !ok {
		return rec, fmt.Errorf("unknown install type %q for theme %q", t.Install.Type, t.Theme.Slug)
	}
	return impl.install(ctx, t, rec, opts)
}

// Detect runs the manifest's detect_command; non-zero exit → tool not
// installed → CLI errors out (does NOT auto-install per design rule).
//
// Bounded by detectTimeout so a hung command can't freeze install /
// doctor flows. A timeout surfaces as a normal "not detected" error
// since from the user's perspective the outcome is the same: the CLI
// can't confirm the tool is there.
func Detect(t manifest.Theme) error {
	cmd := manifest.DetectCommandFor(t)
	if cmd == "" {
		return nil // no detect declared → assume present
	}
	ctx, cancel := context.WithTimeout(context.Background(), detectTimeout)
	defer cancel()
	out, err := shell.Run(ctx, cmd)
	if err != nil {
		return fmt.Errorf("%s not detected (run: %s)\n%s", t.Theme.Name, cmd, strings.TrimSpace(string(out)))
	}
	return nil
}

// ----- type: curl -----

// curlFiles returns the (url, dest) pairs to fetch. When Install.Files
// is set, that's the source of truth and URL/Dest must be empty (catches
// manifest authoring mistakes early). Otherwise we synthesize a single-
// entry slice from URL/Dest so the rest of doCurl can iterate uniformly.
func curlFiles(t manifest.Theme) ([]manifest.InstallFile, error) {
	if len(t.Install.Files) > 0 {
		if t.Install.URL != "" || t.Install.Dest != "" {
			return nil, fmt.Errorf("curl install for %q sets both files and url/dest — pick one", t.Theme.Slug)
		}
		for i, f := range t.Install.Files {
			if f.URL == "" || f.Dest == "" {
				return nil, fmt.Errorf("curl install for %q: files[%d] missing url or dest", t.Theme.Slug, i)
			}
		}
		return t.Install.Files, nil
	}
	if t.Install.URL == "" {
		return nil, fmt.Errorf("curl install for %q has no install.url", t.Theme.Slug)
	}
	return []manifest.InstallFile{{URL: t.Install.URL, Dest: t.Install.Dest}}, nil
}

func doCurl(ctx context.Context, t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
	files, err := curlFiles(t)
	if err != nil {
		return rec, err
	}
	if opts.DryRun {
		return rec, nil
	}
	for _, f := range files {
		dest, err := expandPath(f.Dest)
		if err != nil {
			return rec, err
		}
		if err := fetchAtomic(ctx, f.URL, dest); err != nil {
			return rec, err
		}
		rec.CreatedPaths = append(rec.CreatedPaths, dest)
	}
	if t.Install.Post != nil {
		if err := shell.RunInherit(ctx, t.Install.Post.Command); err != nil {
			return rec, fmt.Errorf("post-hook %q: %w", t.Install.Post.Command, err)
		}
	}
	return rec, nil
}

// ----- type: clone -----

// pickCloneDest returns the platform-specific clone_dest if the manifest sets one for this runtime.GOOS, otherwise the generic CloneDest. Lets one manifest target tools whose config dirs differ between OSes (sublime-text is the canonical case).
func pickCloneDest(t manifest.Theme) string {
	switch runtime.GOOS {
	case "darwin":
		if t.Install.CloneDestDarwin != "" {
			return t.Install.CloneDestDarwin
		}
	case "linux":
		if t.Install.CloneDestLinux != "" {
			return t.Install.CloneDestLinux
		}
	case "windows":
		if t.Install.CloneDestWindows != "" {
			return t.Install.CloneDestWindows
		}
	}
	return t.Install.CloneDest
}

func doClone(ctx context.Context, t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
	dest, err := expandPath(pickCloneDest(t))
	if err != nil {
		return rec, err
	}
	if t.Install.Repo == "" {
		return rec, fmt.Errorf("clone install for %q has no install.repo", t.Theme.Slug)
	}
	if opts.DryRun {
		return rec, nil
	}
	if _, err := os.Stat(dest); err == nil {
		return rec, fmt.Errorf("%s already exists; remove it or run uninstall first", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return rec, fmt.Errorf("create parent dir: %w", err)
	}
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", t.Install.Repo, dest)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return rec, fmt.Errorf("git clone %s: %w", t.Install.Repo, err)
	}
	rec.CreatedPaths = append(rec.CreatedPaths, dest)
	if t.Install.Post != nil {
		if err := shell.RunInherit(ctx, t.Install.Post.Command); err != nil {
			return rec, fmt.Errorf("post-hook %q: %w", t.Install.Post.Command, err)
		}
	}
	return rec, nil
}

// ----- type: vscode-ext -----

// VSCodeExtCLI returns the binary the vscode-ext handlers should shell
// out to — whatever the manifest declared in `install.cli`, or "code"
// when unset. Cursor manifests set this to "cursor", VSCodium to
// "codium", etc. All three accept the same --install-extension /
// --list-extensions / --uninstall-extension flags, so the binary name
// is the only thing that varies.
func VSCodeExtCLI(t manifest.Theme) string {
	if t.Install.CLI != "" {
		return t.Install.CLI
	}
	return "code"
}

func doVSCodeExt(ctx context.Context, t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
	if t.Install.Identifier == "" {
		return rec, fmt.Errorf("vscode-ext install for %q has no install.identifier", t.Theme.Slug)
	}
	if opts.DryRun {
		return rec, nil
	}
	cli := VSCodeExtCLI(t)
	cmd := exec.CommandContext(ctx, cli, "--install-extension", t.Install.Identifier)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return rec, fmt.Errorf("%s --install-extension %s: %w\n%s", cli, t.Install.Identifier, err, out)
	}
	return rec, nil
}

// ----- type: marketplace (browser-open) -----

func doMarketplace(_ context.Context, t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
	if t.Install.URL == "" {
		return rec, fmt.Errorf("marketplace install for %q has no install.url", t.Theme.Slug)
	}
	if opts.DryRun {
		return rec, nil
	}
	if err := openURL(t.Install.URL); err != nil {
		return rec, fmt.Errorf("open %s: %w", t.Install.URL, err)
	}
	return rec, nil
}

// ----- type: gui-import -----

func doGUIImport(ctx context.Context, t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
	rec, err := doCurl(ctx, t, rec, opts)
	if err != nil {
		return rec, err
	}
	if opts.DryRun {
		return rec, nil
	}
	dest, _ := expandPath(t.Install.Dest)
	if err := openURL(dest); err != nil {
		return rec, fmt.Errorf("open %s: %w", dest, err)
	}
	return rec, nil
}

// ----- type: manual -----

func doManual(_ context.Context, _ manifest.Theme, rec state.Record, _ Options) (state.Record, error) {
	// Manual is "print instructions and exit" — no filesystem effects.
	// The cobra command is responsible for printing the instructions
	// from t.Install.Instructions.
	return rec, nil
}

// ----- helpers -----

// expandPath resolves ~ and $ENV in a path.
func expandPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	p = os.ExpandEnv(p)
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p, nil
}

// openURL opens a URL or file path with the OS default handler.
func openURL(target string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target).Run()
	case "linux":
		return exec.Command("xdg-open", target).Run()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Run()
	default:
		return fmt.Errorf("don't know how to open %q on %s", target, runtime.GOOS)
	}
}
