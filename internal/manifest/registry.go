package manifest

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// EmbeddedManifests bundles the v0.1 manifests directly into the binary
// for offline use and faster startup. v0.2 will fall back to fetching
// per-theme manifests from each theme repo's slatewave.toml.
//
//go:embed all:embedded
var EmbeddedManifests embed.FS

// LocalDir is the override directory for development. Set via
// SLATEWAVE_MANIFESTS_DIR — when non-empty, the registry reads
// .toml files from there instead of the embedded set.
var LocalDir = os.Getenv("SLATEWAVE_MANIFESTS_DIR")

// currentGOOS is a test-overridable indirection for runtime.GOOS so the
// OS-support helpers can be exercised on any host. Production code should
// never write to it; tests use SetGOOSForTest.
var currentGOOS = runtime.GOOS

// SetGOOSForTest swaps the GOOS used by SupportsCurrentOS / DetectCommandFor
// / VerifyCommandFor / CurrentGOOS and returns a restorer. Call as
// `defer SetGOOSForTest("windows")()`.
func SetGOOSForTest(goos string) func() {
	prev := currentGOOS
	currentGOOS = goos
	return func() { currentGOOS = prev }
}

// CurrentGOOS returns the OS string the helpers in this package are
// using. Identical to runtime.GOOS in production; tests override it via
// SetGOOSForTest. Callsites that build user-facing "<theme> is not
// supported on <os>" messages should use this so the test override
// flows into the rendered string.
func CurrentGOOS() string { return currentGOOS }

// defaultSupportedOS is what an unset Meta.SupportedOS resolves to —
// the pre-Windows behavior every existing manifest had implicitly.
var defaultSupportedOS = []string{"darwin", "linux"}

// SupportsCurrentOS reports whether the theme's manifest opts in to the
// current runtime OS. Empty SupportedOS defaults to darwin + linux so
// existing manifests are unaffected; cross-platform manifests must list
// "windows" explicitly to be installable on Windows.
func SupportsCurrentOS(t Theme) bool {
	osList := t.Theme.SupportedOS
	if len(osList) == 0 {
		osList = defaultSupportedOS
	}
	return slices.Contains(osList, currentGOOS)
}

// DetectCommandFor returns the OS-appropriate detect command. On Windows
// it prefers Meta.DetectCommandWindows when set; otherwise it falls back
// to Meta.DetectCommand. Lets manifests share one detect across unix and
// override only when the syntax has to change for cmd.exe.
func DetectCommandFor(t Theme) string {
	if currentGOOS == "windows" && t.Theme.DetectCommandWindows != "" {
		return t.Theme.DetectCommandWindows
	}
	return t.Theme.DetectCommand
}

// VerifyCommandFor mirrors DetectCommandFor for the verify block.
func VerifyCommandFor(t Theme) string {
	if currentGOOS == "windows" && t.Verify.CommandWindows != "" {
		return t.Verify.CommandWindows
	}
	return t.Verify.Command
}

// LoadAll returns every theme manifest, sorted by slug.
//
// Resolution order:
//  1. SLATEWAVE_MANIFESTS_DIR if set (dev override)
//  2. embedded manifests bundled with the binary
func LoadAll() ([]Theme, error) {
	if LocalDir != "" {
		return loadFromDir(os.DirFS(LocalDir), ".")
	}
	return loadFromDir(EmbeddedManifests, "embedded")
}

// LoadSupported returns LoadAll filtered to themes that support the
// current OS. Used by the user-facing enumerators (list, browse, init)
// where unsupported themes should be invisible. Install/update guards
// use LoadOne + SupportsCurrentOS instead so they can produce a clean
// "<name> is not supported on <os>" error using the theme's display name.
func LoadSupported() ([]Theme, error) {
	all, err := LoadAll()
	if err != nil {
		return nil, err
	}
	out := make([]Theme, 0, len(all))
	for _, t := range all {
		if SupportsCurrentOS(t) {
			out = append(out, t)
		}
	}
	return out, nil
}

// LoadOne returns the manifest for a single slug, or os.ErrNotExist if
// no theme matches.
func LoadOne(slug string) (Theme, error) {
	all, err := LoadAll()
	if err != nil {
		return Theme{}, err
	}
	for _, t := range all {
		if t.Theme.Slug == slug {
			return t, nil
		}
	}
	return Theme{}, fmt.Errorf("%w: no manifest for theme %q", os.ErrNotExist, slug)
}

func loadFromDir(fsys fs.FS, root string) ([]Theme, error) {
	var out []Theme
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".toml") {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var t Theme
		if _, err := toml.Decode(string(data), &t); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if t.Theme.Slug == "" {
			return fmt.Errorf("manifest %s has empty theme.slug", filepath.Base(path))
		}
		out = append(out, t)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Theme.Slug < out[j].Theme.Slug
	})
	return out, nil
}
