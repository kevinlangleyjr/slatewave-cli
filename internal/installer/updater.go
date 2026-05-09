package installer

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

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
// Dispatch goes through the same installers registry as Install. Types
// whose impl has nil update (marketplace, manual) get ErrNoAutomatedUpdate.
//
// Like Install, Update applies version-aware variant overrides before
// dispatch so a re-fetch picks the file matching the *currently*
// installed tool version (the user may have upgraded since the last
// install — pulling the wrong variant after that would re-introduce
// the version-mismatch bug variants exist to fix).
func Update(t manifest.Theme, opts Options) error {
	if len(t.Install.Variants) > 0 {
		ver, err := detectVersion(t)
		if err != nil {
			return fmt.Errorf("detect version for %q: %w", t.Theme.Slug, err)
		}
		if ver != "" {
			v, err := resolveVariant(t.Install.Variants, ver)
			if err != nil {
				return fmt.Errorf("resolve variant for %q: %w", t.Theme.Slug, err)
			}
			t.Install = applyVariant(t.Install, v)
		}
	}
	impl, ok := installers[t.Install.Type]
	if !ok {
		return fmt.Errorf("unknown install type %q for theme %q", t.Install.Type, t.Theme.Slug)
	}
	if impl.update == nil {
		return ErrNoAutomatedUpdate
	}
	return impl.update(t, opts)
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
		if err := fetchAtomic(f.URL, dest); err != nil {
			return err
		}
	}
	return nil
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
	cli := VSCodeExtCLI(t)
	cmd := exec.Command(cli, "--install-extension", t.Install.Identifier, "--force")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s --install-extension --force: %w\n%s", cli, err, out)
	}
	return nil
}
