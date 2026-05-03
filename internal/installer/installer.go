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

func doCurl(t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
	dest, err := expandPath(t.Install.Dest)
	if err != nil {
		return rec, err
	}
	if t.Install.URL == "" {
		return rec, fmt.Errorf("curl install for %q has no install.url", t.Theme.Slug)
	}
	if opts.DryRun {
		return rec, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return rec, fmt.Errorf("create dest dir: %w", err)
	}
	resp, err := http.Get(t.Install.URL)
	if err != nil {
		return rec, fmt.Errorf("fetch %s: %w", t.Install.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return rec, fmt.Errorf("fetch %s: %s", t.Install.URL, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return rec, fmt.Errorf("write %s: %w", dest, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return rec, fmt.Errorf("write %s: %w", dest, err)
	}
	rec.CreatedPaths = append(rec.CreatedPaths, dest)
	if t.Install.Post != nil {
		if err := exec.Command("sh", "-c", t.Install.Post.Command).Run(); err != nil {
			return rec, fmt.Errorf("post-hook %q: %w", t.Install.Post.Command, err)
		}
	}
	return rec, nil
}

// ----- type: clone -----

func doClone(t manifest.Theme, rec state.Record, opts Options) (state.Record, error) {
	dest, err := expandPath(t.Install.CloneDest)
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
