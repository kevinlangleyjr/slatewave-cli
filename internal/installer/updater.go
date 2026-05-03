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
	files, err := curlFiles(t)
	if err != nil {
		return fmt.Errorf("update %q: %w", t.Theme.Slug, err)
	}
	if opts.DryRun {
		return nil
	}
	for _, f := range files {
		dest, err := expandPath(f.Dest)
		if err != nil {
			return err
		}
		if err := atomicRefetch(f.URL, dest); err != nil {
			return err
		}
	}
	return nil
}

// atomicRefetch downloads url to a temp file in dest's directory, then
// renames over dest. Atomicity matters here (more than in the install
// path) because a failed update mid-write would otherwise corrupt an
// existing, working theme.
func atomicRefetch(url, dest string) error {
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
	return os.Rename(tmp.Name(), dest)
}

func gitPull(t manifest.Theme, opts Options) error {
	dest, err := expandPath(pickCloneDest(t))
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
