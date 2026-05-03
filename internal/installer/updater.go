package installer

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
)

// ErrNoAutomatedUpdate is returned when a theme's install type has no
// automated update path. The cobra command catches it and prints a
// human-friendly hint pointing the user at the manifest's instructions.
var ErrNoAutomatedUpdate = errors.New("no automated update for this install type")

// Update re-fetches a theme's assets without re-running activation
// (the user's config already references the theme; only the source
// files / extension binaries change).
//
//	curl, gui-import → re-download URL to Dest, overwriting in place
//	clone            → `git pull --ff-only` on the existing clone
//	vscode-ext       → `code --install-extension <id> --force`
//	marketplace      → ErrNoAutomatedUpdate
//	manual           → ErrNoAutomatedUpdate
func Update(t manifest.Theme, opts Options) error {
	switch t.Install.Type {
	case "curl", "gui-import":
		return refetch(t, opts)
	case "clone":
		return gitPull(t, opts)
	case "vscode-ext":
		return reinstallVSCodeExt(t, opts)
	case "marketplace", "manual":
		return ErrNoAutomatedUpdate
	default:
		return fmt.Errorf("unknown install type %q for theme %q", t.Install.Type, t.Theme.Slug)
	}
}

func refetch(t manifest.Theme, opts Options) error {
	if t.Install.URL == "" || t.Install.Dest == "" {
		return fmt.Errorf("update %q: install.url or install.dest missing", t.Theme.Slug)
	}
	dest, err := expandPath(t.Install.Dest)
	if err != nil {
		return err
	}
	if opts.DryRun {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	resp, err := http.Get(t.Install.URL)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", t.Install.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: %s", t.Install.URL, resp.Status)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".slatewave-fetch-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Atomic replace — if the rename fails, the original is still in place.
	return os.Rename(tmp.Name(), dest)
}

func gitPull(t manifest.Theme, opts Options) error {
	dest, err := expandPath(t.Install.CloneDest)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dest); err != nil {
		return fmt.Errorf("clone dest %s missing — reinstall instead of update", dest)
	}
	if opts.DryRun {
		return nil
	}
	cmd := exec.Command("git", "-C", dest, "pull", "--ff-only")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull %s: %w\n%s", dest, err, out)
	}
	return nil
}

func reinstallVSCodeExt(t manifest.Theme, opts Options) error {
	if t.Install.Identifier == "" {
		return fmt.Errorf("update %q: install.identifier missing", t.Theme.Slug)
	}
	if opts.DryRun {
		return nil
	}
	cmd := exec.Command("code", "--install-extension", t.Install.Identifier, "--force")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("code --install-extension --force: %w\n%s", err, out)
	}
	return nil
}
