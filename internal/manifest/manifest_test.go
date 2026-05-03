package manifest

import (
	"os"
	"strings"
	"testing"
)

// TestEmbeddedManifests_AllParse is a structural canary: every .toml
// bundled into the binary has to decode and have the minimum fields
// populated. Catches schema drift at build/test time before a release.
func TestEmbeddedManifests_AllParse(t *testing.T) {
	all, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("LoadAll returned no manifests — embed missing?")
	}

	seenSlugs := map[string]bool{}
	for _, th := range all {
		if th.Theme.Slug == "" {
			t.Errorf("manifest with empty slug: %+v", th)
			continue
		}
		if th.Theme.Name == "" {
			t.Errorf("%s: empty theme.name", th.Theme.Slug)
		}
		if th.Theme.Category == "" {
			t.Errorf("%s: empty theme.category", th.Theme.Slug)
		}
		if th.Install.Type == "" {
			t.Errorf("%s: empty install.type", th.Theme.Slug)
		}
		if seenSlugs[th.Theme.Slug] {
			t.Errorf("duplicate slug across manifests: %q", th.Theme.Slug)
		}
		seenSlugs[th.Theme.Slug] = true
	}
}

// TestEmbeddedManifests_KnownSlugs locks the v0.1 manifest set so we
// notice if one is silently dropped from the embed.
func TestEmbeddedManifests_KnownSlugs(t *testing.T) {
	all, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	want := []string{"bat", "btop", "delta", "oh-my-posh", "vscode"}
	got := map[string]bool{}
	for _, th := range all {
		got[th.Theme.Slug] = true
	}
	for _, slug := range want {
		if !got[slug] {
			t.Errorf("expected manifest for %q in embedded set", slug)
		}
	}
}

// TestEmbeddedManifests_InstallTypesAreKnown ensures every manifest
// declares an install.type that the installer knows how to dispatch.
// Updating the installer with a new type without adding it here means
// this test starts failing — exactly the feedback loop we want.
func TestEmbeddedManifests_InstallTypesAreKnown(t *testing.T) {
	known := map[string]bool{
		"curl":        true,
		"clone":       true,
		"vscode-ext":  true,
		"marketplace": true,
		"gui-import":  true,
		"manual":      true,
	}
	all, _ := LoadAll()
	for _, th := range all {
		if !known[th.Install.Type] {
			t.Errorf("%s declares unknown install.type %q", th.Theme.Slug, th.Install.Type)
		}
	}
}

// TestEmbeddedManifests_ActivateTypesAreKnown — same shape for activate.
func TestEmbeddedManifests_ActivateTypesAreKnown(t *testing.T) {
	known := map[string]bool{
		"":                  true, // empty = none
		"none":              true,
		"ini-key":           true,
		"gitconfig-include": true,
		"shell-rc":          true,
	}
	all, _ := LoadAll()
	for _, th := range all {
		if !known[th.Activate.Type] {
			t.Errorf("%s declares unknown activate.type %q", th.Theme.Slug, th.Activate.Type)
		}
	}
}

// TestLoadOne_RoundtripAndMissing — happy path + the not-found error
// shape (other code calls errors.Is(err, os.ErrNotExist) on it).
func TestLoadOne_RoundtripAndMissing(t *testing.T) {
	got, err := LoadOne("bat")
	if err != nil {
		t.Fatalf("LoadOne(bat): %v", err)
	}
	if got.Theme.Slug != "bat" {
		t.Errorf("got slug %q want bat", got.Theme.Slug)
	}

	_, err = LoadOne("does-not-exist")
	if err == nil {
		t.Fatal("LoadOne on missing slug should error")
	}
	// We don't lock the exact error string but it must mention the slug.
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("not-found error should name the slug: %v", err)
	}
}

// TestLoadAll_LocalDirOverride confirms SLATEWAVE_MANIFESTS_DIR works
// for development. Critical because the website's authors will likely
// use it to test new manifests against the CLI without rebuilding.
func TestLoadAll_LocalDirOverride(t *testing.T) {
	dir := t.TempDir()
	manifestPath := dir + "/test.toml"
	body := `
[theme]
slug = "test-theme"
name = "Test"
category = "editor"

[install]
type = "manual"
`
	if err := writeFile(t, manifestPath, body); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}

	prev := LocalDir
	LocalDir = dir
	t.Cleanup(func() { LocalDir = prev })

	all, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll(local): %v", err)
	}
	if len(all) != 1 || all[0].Theme.Slug != "test-theme" {
		t.Errorf("local override didn't take precedence: %+v", all)
	}
}

func writeFile(t *testing.T, path, body string) error {
	t.Helper()
	return os.WriteFile(path, []byte(body), 0o644)
}
