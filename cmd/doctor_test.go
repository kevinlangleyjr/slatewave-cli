package cmd

import (
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/tui"
)

// TestBuildFixes_MapsStatusToFixKind covers the closed-set mapping:
// statusStale → FixUpdate, statusMissingTool → FixUninstall, statusOrphan → FixDropOrphan.
// statusHealthy is dropped entirely (nothing to fix).
//
// Slugs come from the embedded manifests. "bat" resolves; "no-such-slug"
// triggers the orphan path because manifest.LoadOne errors and buildFixes
// falls through to the no-manifest branch.
func TestBuildFixes_MapsStatusToFixKind(t *testing.T) {
	rows := []doctorRow{
		{slug: "bat", name: "Slatewave for bat", status: statusHealthy},
		{slug: "btop", name: "Slatewave for btop", status: statusStale},
		{slug: "delta", name: "Slatewave for delta", status: statusMissingTool},
		{slug: "no-such-slug", name: "no-such-slug", status: statusOrphan},
	}

	got := buildFixes(rows)
	if len(got) != 3 {
		t.Fatalf("buildFixes returned %d fixes, want 3 (healthy is dropped)", len(got))
	}

	bySlug := map[string]tui.Fix{}
	for _, f := range got {
		bySlug[f.Slug] = f
	}

	if _, ok := bySlug["bat"]; ok {
		t.Error("buildFixes kept healthy bat row, expected drop")
	}

	if f, ok := bySlug["btop"]; !ok || f.Kind != tui.FixUpdate {
		t.Errorf("btop fix = %+v, want kind FixUpdate", f)
	}
	if f, ok := bySlug["delta"]; !ok || f.Kind != tui.FixUninstall {
		t.Errorf("delta fix = %+v, want kind FixUninstall", f)
	}
	if f, ok := bySlug["no-such-slug"]; !ok || f.Kind != tui.FixDropOrphan {
		t.Errorf("no-such-slug fix = %+v, want kind FixDropOrphan", f)
	}
}

// TestBuildFixes_StaleWithMissingManifestIsSkipped covers the edge where a
// row was diagnosed stale but the manifest has since been pulled — buildFixes
// drops it rather than queuing a fix it can't execute.
func TestBuildFixes_StaleWithMissingManifestIsSkipped(t *testing.T) {
	rows := []doctorRow{
		{slug: "no-such-slug", name: "ghost", status: statusStale},
	}
	got := buildFixes(rows)
	if len(got) != 0 {
		t.Errorf("buildFixes returned %d fixes for stale-with-no-manifest, want 0", len(got))
	}
}

// TestBuildFixes_MissingToolWithMissingManifestIsSkipped — same logic as the
// stale-with-no-manifest case. We can't reverse an install footprint without
// the manifest, so we drop it.
func TestBuildFixes_MissingToolWithMissingManifestIsSkipped(t *testing.T) {
	rows := []doctorRow{
		{slug: "no-such-slug", name: "ghost", status: statusMissingTool},
	}
	got := buildFixes(rows)
	if len(got) != 0 {
		t.Errorf("buildFixes returned %d fixes for missing-tool-with-no-manifest, want 0", len(got))
	}
}

func TestBuildFixes_OrphanCarriesNoTheme(t *testing.T) {
	// Orphan rows (no manifest) build a Fix with a zero-value Theme; the
	// runDropOrphanFix pipeline doesn't need it.
	rows := []doctorRow{
		{slug: "no-such-slug", name: "no-such-slug", status: statusOrphan},
	}
	got := buildFixes(rows)
	if len(got) != 1 {
		t.Fatalf("buildFixes returned %d fixes, want 1", len(got))
	}
	if got[0].Theme.Theme.Slug != "" {
		t.Errorf("orphan Fix carried a theme slug = %q, want empty", got[0].Theme.Theme.Slug)
	}
}

func TestBuildFixes_PreservesOrder(t *testing.T) {
	rows := []doctorRow{
		{slug: "btop", name: "btop", status: statusStale},
		{slug: "bat", name: "bat", status: statusHealthy}, // dropped
		{slug: "delta", name: "delta", status: statusMissingTool},
	}
	got := buildFixes(rows)
	if len(got) != 2 {
		t.Fatalf("buildFixes returned %d fixes, want 2", len(got))
	}
	if got[0].Slug != "btop" || got[1].Slug != "delta" {
		t.Errorf("order drift: got %q,%q, want btop,delta", got[0].Slug, got[1].Slug)
	}
}

func TestBuildFixes_EmptyInput(t *testing.T) {
	if got := buildFixes(nil); len(got) != 0 {
		t.Errorf("buildFixes(nil) = %d fixes, want 0", len(got))
	}
}
