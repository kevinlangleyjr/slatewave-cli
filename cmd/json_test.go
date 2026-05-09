package cmd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kevinlangleyjr/slatewave-cli/internal/jsonout"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
)

// `slatewave status --json` (no slug) emits an array of every installed
// theme. The test seeds a record with a populated install footprint
// (CreatedPaths, AppendedLine, Backups) and asserts every field
// round-trips through the wire format.
func TestStatusCmd_JSONOutputShape(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})

	installedAt := time.Date(2026, 5, 9, 10, 30, 0, 0, time.UTC)
	env.putRecord(t, state.Record{
		Slug:         "okayish",
		InstalledAt:  installedAt,
		InstallType:  "curl",
		ActivateType: "ini-key",
		CreatedPaths: []string{"/tmp/slatewave-test/Slatewave.tmTheme"},
		AppendedLine: &state.Appended{File: "/tmp/.zshrc", Line: "export FOO=bar"},
		Backups:      []state.Backup{{Original: "/tmp/.zshrc", Path: "/tmp/.zshrc.bak"}},
	})

	resetFlags := setFlags(t, statusCmd, map[string]string{"json": "true"})
	defer resetFlags()
	if err := statusCmd.RunE(statusCmd, nil); err != nil {
		t.Fatalf("statusCmd.RunE: %v", err)
	}

	var got jsonout.StatusOutput
	if err := json.Unmarshal(env.out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\nraw: %s", err, env.out.String())
	}
	if len(got.Themes) != 1 {
		t.Fatalf("themes count = %d, want 1", len(got.Themes))
	}
	row := got.Themes[0]
	if row.Slug != "okayish" {
		t.Errorf("slug = %q, want okayish", row.Slug)
	}
	if row.Name != "OK" {
		t.Errorf("name = %q, want OK (manifest's name)", row.Name)
	}
	if !row.InstalledAt.Equal(installedAt) {
		t.Errorf("installed_at = %v, want %v", row.InstalledAt, installedAt)
	}
	if row.InstallType != "curl" || row.ActivateType != "ini-key" {
		t.Errorf("install/activate = %q/%q, want curl/ini-key", row.InstallType, row.ActivateType)
	}
	if len(row.CreatedPaths) != 1 || row.CreatedPaths[0] != "/tmp/slatewave-test/Slatewave.tmTheme" {
		t.Errorf("created_paths = %v", row.CreatedPaths)
	}
	if row.AppendedLine == nil || row.AppendedLine.Line != "export FOO=bar" {
		t.Errorf("appended_line = %+v", row.AppendedLine)
	}
	if len(row.Backups) != 1 || row.Backups[0].Path != "/tmp/.zshrc.bak" {
		t.Errorf("backups = %+v", row.Backups)
	}
}

// `slatewave status <slug> --json` errors when the slug isn't installed
// instead of printing an error line. JSON consumers need a non-zero exit
// to short-circuit their script (the human path's ui.Errorf wouldn't
// produce one).
func TestStatusCmd_JSONUnknownSlugErrors(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})
	// No state record — slug isn't installed.

	resetFlags := setFlags(t, statusCmd, map[string]string{"json": "true"})
	defer resetFlags()
	err := statusCmd.RunE(statusCmd, []string{"okayish"})
	if err == nil {
		t.Fatal("expected error for uninstalled slug, got nil")
	}
}

// `slatewave doctor --json` emits per-theme rows + a summary count.
// Seed: one healthy + one stale + one orphan, assert all three buckets
// in summary plus the per-row status strings round-trip.
func TestDoctorCmd_JSONOutputShape(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{
		"okayish": manifestHealthy,
		"drifted": manifestVerifyFails,
	})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})
	env.putRecord(t, state.Record{Slug: "drifted", InstallType: "manual"})
	env.putRecord(t, state.Record{Slug: "orphaned", InstallType: "manual"}) // no manifest → orphan

	resetFlags := setFlags(t, doctorCmd, map[string]string{"json": "true"})
	defer resetFlags()
	if err := doctorCmd.RunE(doctorCmd, nil); err != nil {
		t.Fatalf("doctorCmd.RunE: %v", err)
	}

	var got jsonout.DoctorOutput
	if err := json.Unmarshal(env.out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\nraw: %s", err, env.out.String())
	}

	if got.Summary.Healthy != 1 {
		t.Errorf("summary.healthy = %d, want 1", got.Summary.Healthy)
	}
	if got.Summary.Stale != 1 {
		t.Errorf("summary.stale = %d, want 1", got.Summary.Stale)
	}
	if got.Summary.Orphan != 1 {
		t.Errorf("summary.orphan = %d, want 1", got.Summary.Orphan)
	}

	bySlug := map[string]string{}
	for _, r := range got.Themes {
		bySlug[r.Slug] = r.Status
	}
	if bySlug["okayish"] != "healthy" {
		t.Errorf("okayish status = %q, want healthy", bySlug["okayish"])
	}
	if bySlug["drifted"] != "stale" {
		t.Errorf("drifted status = %q, want stale", bySlug["drifted"])
	}
	if bySlug["orphaned"] != "orphan" {
		t.Errorf("orphaned status = %q, want orphan", bySlug["orphaned"])
	}
}

// Empty-state doctor must still produce a well-formed JSON object so a
// consumer can tell "no installs" apart from a parse / exec failure.
func TestDoctorCmd_JSONEmptyStateIsWellFormed(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{}) // no manifests, no records

	resetFlags := setFlags(t, doctorCmd, map[string]string{"json": "true"})
	defer resetFlags()
	if err := doctorCmd.RunE(doctorCmd, nil); err != nil {
		t.Fatalf("doctorCmd.RunE: %v", err)
	}

	var got jsonout.DoctorOutput
	if err := json.Unmarshal(env.out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\nraw: %s", err, env.out.String())
	}
	if got.Themes == nil {
		t.Error("themes is nil; want empty array (consumers shouldn't have to nil-check)")
	}
	if got.Summary.Healthy+got.Summary.Stale+got.Summary.MissingTool+got.Summary.Orphan != 0 {
		t.Errorf("summary not all-zero: %+v", got.Summary)
	}
}
