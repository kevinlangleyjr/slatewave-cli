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

// TestEmbeddedManifests_KnownSlugs locks the full shipping manifest set
// so we notice if one is silently dropped from the embed. Update both
// this list AND the website's getslatewave/src/content/themes/ in lockstep
// when a new theme ships, so the two sources can't drift out of agreement.
func TestEmbeddedManifests_KnownSlugs(t *testing.T) {
	all, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	want := []string{
		// editors
		"vscode", "jetbrains", "sublime-text", "zed", "neovim", "helix", "xcode",
		// prompts
		"oh-my-posh", "starship",
		// terminal emulators
		"ghostty", "alacritty", "iterm2", "wezterm", "windows-terminal",
		// terminal CLI tools
		"bat", "delta", "lsd",
		// multiplexer
		"tmux",
		// dashboard
		"btop",
		// notes
		"obsidian", "logseq", "markedit",
		// launchers
		"alfred", "raycast",
		// chat
		"slack",
	}
	got := map[string]bool{}
	for _, th := range all {
		got[th.Theme.Slug] = true
	}
	for _, slug := range want {
		if !got[slug] {
			t.Errorf("expected manifest for %q in embedded set", slug)
		}
	}
	// Count check — catches an extra manifest landing without an entry
	// in `want`, which means a new theme shipped without updating the lock.
	if len(all) != len(want) {
		t.Errorf("manifest count drift: got %d manifests, want %d (extra/missing in embed?)", len(all), len(want))
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
		"toml-import":       true,
		"yaml-set":          true,
	}
	all, _ := LoadAll()
	for _, th := range all {
		if !known[th.Activate.Type] {
			t.Errorf("%s declares unknown activate.type %q", th.Theme.Slug, th.Activate.Type)
		}
	}
}

// TestEmbeddedManifests_FieldsByInstallType validates that each manifest
// has the fields its install type actually requires. Catches a drifted
// manifest like a `type = "curl"` block with empty `url` before the
// installer would surface it as a runtime error mid-install.
func TestEmbeddedManifests_FieldsByInstallType(t *testing.T) {
	all, _ := LoadAll()
	for _, th := range all {
		slug := th.Theme.Slug
		switch th.Install.Type {
		case "curl", "gui-import":
			// curl supports two shapes: single-file via url+dest, or
			// multi-file via [[install.files]]. Exactly one must be set,
			// and a Files entry must have both fields.
			hasSingle := th.Install.URL != "" || th.Install.Dest != ""
			hasMulti := len(th.Install.Files) > 0
			switch {
			case hasSingle && hasMulti:
				t.Errorf("%s: %q install sets both url/dest and files — pick one", slug, th.Install.Type)
			case !hasSingle && !hasMulti:
				t.Errorf("%s: %q install missing install.url/dest or install.files", slug, th.Install.Type)
			case hasSingle:
				if th.Install.URL == "" {
					t.Errorf("%s: %q install missing install.url", slug, th.Install.Type)
				}
				if th.Install.Dest == "" {
					t.Errorf("%s: %q install missing install.dest", slug, th.Install.Type)
				}
			default:
				for i, f := range th.Install.Files {
					if f.URL == "" || f.Dest == "" {
						t.Errorf("%s: %q install files[%d] missing url or dest", slug, th.Install.Type, i)
					}
				}
			}
		case "clone":
			if th.Install.Repo == "" {
				t.Errorf("%s: clone install missing install.repo", slug)
			}
			if th.Install.CloneDest == "" {
				t.Errorf("%s: clone install missing install.clone_dest", slug)
			}
		case "vscode-ext":
			if th.Install.Identifier == "" {
				t.Errorf("%s: vscode-ext install missing install.identifier", slug)
			}
		case "marketplace":
			if th.Install.URL == "" {
				t.Errorf("%s: marketplace install missing install.url", slug)
			}
		case "manual":
			if len(th.Install.Instructions) == 0 {
				t.Errorf("%s: manual install missing install.instructions (the whole point of manual is to print them)", slug)
			}
		}
	}
}

// TestEmbeddedManifests_FieldsByActivateType — same shape for the
// activate block. An activate type without its required fields would
// crash the activator at runtime; this catches it at build/test time.
func TestEmbeddedManifests_FieldsByActivateType(t *testing.T) {
	all, _ := LoadAll()
	for _, th := range all {
		slug := th.Theme.Slug
		switch th.Activate.Type {
		case "ini-key":
			if th.Activate.File == "" || th.Activate.Key == "" || th.Activate.Value == "" {
				t.Errorf("%s: ini-key activate missing one of file/key/value", slug)
			}
		case "shell-rc":
			if len(th.Activate.Files) == 0 || th.Activate.Line == "" {
				t.Errorf("%s: shell-rc activate missing files/line", slug)
			}
		case "gitconfig-include":
			if th.Activate.IncludePath == "" {
				t.Errorf("%s: gitconfig-include activate missing include_path", slug)
			}
		case "toml-import":
			if th.Activate.TOMLPath == "" || th.Activate.Import == "" {
				t.Errorf("%s: toml-import activate missing toml_path/import", slug)
			}
		case "yaml-set":
			if th.Activate.YAMLPath == "" {
				t.Errorf("%s: yaml-set activate missing yaml_path", slug)
			}
			if len(th.Activate.YAMLSet) == 0 {
				t.Errorf("%s: yaml-set activate missing yaml_set entries", slug)
			}
			for i, p := range th.Activate.YAMLSet {
				if p.Path == "" || p.Value == "" {
					t.Errorf("%s: yaml-set entry %d missing path or value", slug, i)
				}
			}
		}
	}
}

// TestEmbeddedManifests_HaveDetectCommand requires every manifest to
// declare a detect_command. Detection is the safety net that makes
// `slatewave install bat` fail cleanly with "bat not detected" when
// the underlying tool isn't installed — without it, the curl + activate
// runs and leaves the user with a half-installed theme for a tool
// they don't have.
func TestEmbeddedManifests_HaveDetectCommand(t *testing.T) {
	all, _ := LoadAll()
	for _, th := range all {
		if th.Theme.DetectCommand == "" {
			t.Errorf("%s: missing detect_command", th.Theme.Slug)
		}
	}
}

// TestEmbeddedManifests_SlugMatchesFilename keeps the manifest filename
// in sync with theme.slug so `LoadOne(slug)` reliably finds it and the
// embed walker doesn't surprise us with mismatched IDs.
func TestEmbeddedManifests_SlugMatchesFilename(t *testing.T) {
	all, _ := LoadAll()
	// Build an inverse map filename → slug by walking the embed FS.
	entries, err := EmbeddedManifests.ReadDir("embedded")
	if err != nil {
		t.Fatal(err)
	}
	slugByFile := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".toml")
		slugByFile[base] = ""
	}
	for _, th := range all {
		if _, ok := slugByFile[th.Theme.Slug]; !ok {
			t.Errorf("%s: no embedded file matches slug (expected embedded/%s.toml)", th.Theme.Slug, th.Theme.Slug)
		} else {
			slugByFile[th.Theme.Slug] = th.Theme.Slug
		}
	}
	// Catch the reverse case — a file with a slug that doesn't map back.
	for file, slug := range slugByFile {
		if slug == "" {
			t.Errorf("embedded/%s.toml has no manifest claiming slug %q", file, file)
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
