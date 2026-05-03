package manifest

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
