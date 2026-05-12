package jsonout

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// updateGoldens regenerates every testdata/*.golden.json from the
// fixtures below. Run with `go test ./internal/jsonout -update`
// whenever a schema change is intentional; commit the new golden file
// alongside the struct edit so reviewers see the wire shape change.
var updateGoldens = flag.Bool("update", false, "regenerate golden files from in-test fixtures")

// The fixtures cover each Output struct's wire shape with a realistic
// mix of populated and zero-value fields — populated fields prove the
// field name + JSON tag round-trip, zero-value fields prove omitempty
// strips the rest. A change to a struct tag, type, or field name
// surfaces here as a byte-diff and the test surface points the author
// at the `-update` flag.

func TestListOutput_GoldenSchema(t *testing.T) {
	// One installed theme + one available-but-not-installed theme is the
	// canonical `slatewave list` shape. The not-installed row exercises
	// the omitempty path: installed_at / install_type / activate_type
	// must disappear when zero so consumers don't see "0001-01-01T00:00:00Z".
	installedAt := time.Date(2026, 5, 9, 14, 32, 1, 0, time.UTC)
	payload := ListOutput{
		Themes: []ThemeRow{
			{
				Slug:         "bat",
				Name:         "Slatewave for bat",
				Category:     "terminal",
				Installed:    true,
				InstalledAt:  &installedAt,
				InstallType:  "curl",
				ActivateType: "ini-key",
			},
			{
				Slug:      "ghostty",
				Name:      "Slatewave for ghostty",
				Category:  "terminal",
				Installed: false,
			},
		},
		Counts: ListCounts{Total: 2, Installed: 1},
	}
	assertGolden(t, "list.golden.json", payload)
}

func TestStatusOutput_GoldenSchema(t *testing.T) {
	// Status carries the full install footprint, so the fixture has every
	// optional sub-shape populated: CreatedPaths, AppendedLine (with the
	// inner two-field object), and Backups (with original/path pairs).
	// A second entry that activated cleanly without any backups exercises
	// the omitempty paths for backups + appended_line so consumers don't
	// see empty arrays/nulls there.
	installedAt := time.Date(2026, 5, 9, 14, 32, 1, 0, time.UTC)
	payload := StatusOutput{
		Themes: []StatusEntry{
			{
				Slug:         "bat",
				Name:         "Slatewave for bat",
				InstalledAt:  installedAt,
				InstallType:  "curl",
				ActivateType: "ini-key",
				CreatedPaths: []string{"/home/user/.config/bat/themes/slatewave.tmTheme"},
				AppendedLine: &AppendedLine{
					File: "/home/user/.config/bat/config",
					Line: "--theme=Slatewave",
				},
				Backups: []Backup{
					{
						Original: "/home/user/.config/bat/config",
						Path:     "/home/user/.config/bat/config.slatewave.1715266321.bak",
					},
				},
			},
			{
				Slug:         "btop",
				Name:         "Slatewave for btop",
				InstalledAt:  installedAt,
				InstallType:  "curl",
				ActivateType: "ini-key",
				CreatedPaths: []string{"/home/user/.config/btop/themes/slatewave.theme"},
			},
		},
	}
	assertGolden(t, "status.golden.json", payload)
}

func TestDoctorOutput_GoldenSchema(t *testing.T) {
	// One row per status bucket so the wire shape covers every variant.
	// Detail / remedy must omit for healthy rows and populate for the
	// other three — both halves of the omitempty contract.
	payload := DoctorOutput{
		Summary: DoctorSummary{Healthy: 1, Stale: 1, MissingTool: 1, Orphan: 1},
		Themes: []DoctorRow{
			{Slug: "bat", Name: "Slatewave for bat", Status: "healthy"},
			{
				Slug:   "btop",
				Name:   "Slatewave for btop",
				Status: "stale",
				Detail: "verify command exited non-zero",
				Remedy: "slatewave update btop",
			},
			{
				Slug:   "lsd",
				Name:   "Slatewave for lsd",
				Status: "missing-tool",
				Detail: "lsd is not installed",
				Remedy: "slatewave uninstall lsd",
			},
			{
				Slug:   "ghost",
				Name:   "ghost (orphan)",
				Status: "orphan",
				Detail: "no manifest ships for this slug",
				Remedy: "slatewave doctor --fix",
			},
		},
	}
	assertGolden(t, "doctor.golden.json", payload)
}

func TestInfoOutput_GoldenSchema(t *testing.T) {
	// Info covers four nested objects (install + activate + verify on top
	// of the theme metadata). A curl-type install with an ini-key activator
	// is a representative manifest — populates most curl/install fields
	// while leaving repo/clone_dest/identifier zero so omitempty trims them.
	payload := InfoOutput{
		Slug:        "bat",
		Name:        "Slatewave for bat",
		Category:    "terminal",
		SupportedOS: []string{"darwin", "linux", "windows"},
		SourceURL:   "https://getslatewave.com/themes/bat",
		Install: InfoInstall{
			Type:        "curl",
			URL:         "https://example.test/themes/bat/slatewave.tmTheme",
			Dest:        "~/.config/bat/themes/slatewave.tmTheme",
			DoneMessage: "Run `bat cache --build` then `bat --theme=Slatewave` to verify.",
		},
		Activate: InfoActivate{
			Type:  "ini-key",
			File:  "~/.config/bat/config",
			Key:   "--theme",
			Value: "Slatewave",
		},
		Verify: InfoVerify{
			Command: "bat --list-themes",
			Expect:  "Slatewave",
		},
	}
	assertGolden(t, "info.golden.json", payload)
}

// assertGolden marshals payload with json.MarshalIndent (matching the
// production formatting used by the cmd layer), then either rewrites
// or diffs against testdata/<name>. A diff fails the test and points
// the author at -update for intentional schema changes.
func assertGolden(t *testing.T, name string, payload any) {
	t.Helper()
	got, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	got = append(got, '\n') // editor convention: trailing newline

	path := filepath.Join("testdata", name)
	if *updateGoldens {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run `go test ./internal/jsonout -update` to create): %v", path, err)
	}
	// Normalize CRLF→LF before comparing. .gitattributes pins golden
	// files to LF on checkout, but defense-in-depth: a Windows
	// contributor cloning without honoring gitattributes (or a future
	// editor "fixing" line endings) shouldn't see a confusing diff that
	// looks byte-identical. Schema drift produces a visible content
	// change; line-ending drift would otherwise mask as the same.
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		t.Errorf("schema drift in %s — wire shape changed. If intentional, run:\n\n  go test ./internal/jsonout -update\n\nand commit the updated golden. Diff:\n--- want\n%s\n+++ got\n%s", name, want, got)
	}
}
