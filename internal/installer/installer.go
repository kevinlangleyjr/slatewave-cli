// Package installer dispatches the install step of a theme manifest.
// Each [install].type in the manifest maps to a function below.
package installer

import (
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
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
)

// Options controls install behavior at the call site.
type Options struct {
	DryRun bool
}

// Install runs the install step for theme t. On success the returned
// Record is populated with reversal info (created paths, etc.) but
// its Activate fields are still empty — the activator fills those.
func Install(t manifest.Theme, opts Options) (state.Record, error) {
	rec := state.Record{
		Slug:         t.Theme.Slug,
		InstalledAt:  time.Now().UTC(),
		InstallType:  t.Install.Type,
		ActivateType: t.Activate.Type,
	}

	switch t.Install.Type {
	case "curl":
		return doCurl(t, rec, opts)
	case "clone":
		return doClone(t, rec, opts)
	case "vscode-ext":
		return doVSCodeExt(t, rec, opts)
	case "marketplace":
		return doMarketplace(t, rec, opts)
	case "gui-import":
		return doGUIImport(t, rec, opts)
	case "manual":
		return doManual(t, rec, opts)
	default:
		return rec, fmt.Errorf("unknown install type %q for theme %q", t.Install.Type, t.Theme.Slug)
	}
}

// Detect runs the manifest's detect_command; non-zero exit → tool not
// installed → CLI errors out (does NOT auto-install per design rule).
func Detect(t manifest.Theme) error {
	if t.Theme.DetectCommand == "" {
		return nil // no detect declared → assume present
	}
	cmd := exec.Command("sh", "-c", t.Theme.DetectCommand)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s not detected (run: %s)\n%s", t.Theme.Name, t.Theme.DetectCommand, strings.TrimSpace(string(out)))
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

func doCurl(t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
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
		if err := fetchToFile(f.URL, dest); err != nil {
			return rec, err
		}
		rec.CreatedPaths = append(rec.CreatedPaths, dest)
	}
	if t.Install.Post != nil {
		if err := exec.Command("sh", "-c", t.Install.Post.Command).Run(); err != nil {
			return rec, fmt.Errorf("post-hook %q: %w", t.Install.Post.Command, err)
		}
	}
	return rec, nil
}

// fetchToFile downloads url to dest, creating intermediate dirs and
// truncating any existing file at the destination. Returns wrapped
// errors keyed on the URL/path so multi-file installs surface which
// asset failed.
func fetchToFile(url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	return nil
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

func doClone(t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
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
	cmd := exec.Command("git", "clone", "--depth", "1", t.Install.Repo, dest)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return rec, fmt.Errorf("git clone %s: %w", t.Install.Repo, err)
	}
	rec.CreatedPaths = append(rec.CreatedPaths, dest)
	if t.Install.Post != nil {
		if err := exec.Command("sh", "-c", t.Install.Post.Command).Run(); err != nil {
			return rec, fmt.Errorf("post-hook %q: %w", t.Install.Post.Command, err)
		}
	}
	return rec, nil
}

// ----- type: vscode-ext -----

func doVSCodeExt(t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
	if t.Install.Identifier == "" {
		return rec, fmt.Errorf("vscode-ext install for %q has no install.identifier", t.Theme.Slug)
	}
	if opts.DryRun {
		return rec, nil
	}
	cmd := exec.Command("code", "--install-extension", t.Install.Identifier)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return rec, fmt.Errorf("code --install-extension %s: %w\n%s", t.Install.Identifier, err, out)
	}
	return rec, nil
}

// ----- type: marketplace (browser-open) -----

func doMarketplace(t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
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

func doGUIImport(t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
	rec, err := doCurl(t, rec, opts)
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

func doManual(t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
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
