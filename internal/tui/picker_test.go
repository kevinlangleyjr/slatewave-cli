package tui

import (
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
)

// stubTheme builds a minimal manifest.Theme with just enough fields for the picker / detect tests. detectCmd is wired into Theme.DetectCommand so the same helper drives both packages.
func stubTheme(slug, category, detectCmd string) manifest.Theme {
	return manifest.Theme{
		Theme: manifest.Meta{
			Slug:          slug,
			Name:          "Slatewave for " + slug,
			Category:      category,
			DetectCommand: detectCmd,
		},
	}
}

func TestOfferable_FiltersInstalledAndMissingTool(t *testing.T) {
	results := []DetectResult{
		{Theme: stubTheme("bat", "editor", "true"), Present: true, Installed: false},
		{Theme: stubTheme("btop", "terminal", "true"), Present: true, Installed: true},  // already installed → drop
		{Theme: stubTheme("delta", "editor", "true"), Present: false, Installed: false}, // tool missing → drop
		{Theme: stubTheme("alacritty", "terminal", "true"), Present: true, Installed: false},
	}

	got := offerable(results)
	if len(got) != 2 {
		t.Fatalf("offerable returned %d, want 2 (installed + missing tool dropped)", len(got))
	}

	gotSlugs := map[string]bool{}
	for _, d := range got {
		gotSlugs[d.Theme.Theme.Slug] = true
	}
	if !gotSlugs["bat"] || !gotSlugs["alacritty"] {
		t.Errorf("expected bat + alacritty in offerable, got %+v", gotSlugs)
	}
	if gotSlugs["btop"] {
		t.Error("offerable kept already-installed btop")
	}
	if gotSlugs["delta"] {
		t.Error("offerable kept tool-missing delta")
	}
}

func TestOfferable_SortsByCategoryThenSlug(t *testing.T) {
	// Insert in deliberately-shuffled order so the sort has work to do.
	results := []DetectResult{
		{Theme: stubTheme("zellij", "terminal", "true"), Present: true},
		{Theme: stubTheme("bat", "editor", "true"), Present: true},
		{Theme: stubTheme("alacritty", "terminal", "true"), Present: true},
		{Theme: stubTheme("delta", "editor", "true"), Present: true},
	}

	got := offerable(results)
	want := []string{"bat", "delta", "alacritty", "zellij"} // editor block, then terminal block, alpha within
	if len(got) != len(want) {
		t.Fatalf("offerable len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Theme.Theme.Slug != w {
			t.Errorf("offerable[%d] = %q, want %q", i, got[i].Theme.Theme.Slug, w)
		}
	}
}

func TestOfferable_EmptyInputReturnsEmpty(t *testing.T) {
	if got := offerable(nil); len(got) != 0 {
		t.Errorf("offerable(nil) returned %d entries, want 0", len(got))
	}
}

func TestOfferable_AllFilteredReturnsEmpty(t *testing.T) {
	results := []DetectResult{
		{Theme: stubTheme("bat", "editor", "true"), Present: true, Installed: true},
		{Theme: stubTheme("btop", "terminal", "true"), Present: false},
	}
	if got := offerable(results); len(got) != 0 {
		t.Errorf("offerable returned %d, want 0 — every input should be filtered", len(got))
	}
}
