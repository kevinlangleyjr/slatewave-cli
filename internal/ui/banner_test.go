package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// goldenPath is the on-disk file the banner test compares against.
// Stored under testdata/ so go test handles the path automatically.
const goldenPath = "testdata/banner.golden"

// TestBanner_MatchesGolden guards against unintended changes to the
// slant-ASCII wordmark. The slant geometry is hand-laid (line by line
// in bannerLines) and one stray escape would silently break the visual
// without any test catching it. Diffing against a checked-in golden
// surfaces every byte-level change.
//
// Set SLATEWAVE_UPDATE_GOLDEN=1 to rewrite the golden when the change
// is intentional (e.g. updating the slant or the tagline). The CI
// workflow doesn't set this so a drift will fail there.
func TestBanner_MatchesGolden(t *testing.T) {
	got := Banner()

	if os.Getenv("SLATEWAVE_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote golden at %s (%d bytes)", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v\n(re-run with SLATEWAVE_UPDATE_GOLDEN=1 to seed)", err)
	}
	if got != string(want) {
		t.Errorf("Banner() differs from golden — set SLATEWAVE_UPDATE_GOLDEN=1 if intentional.\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
