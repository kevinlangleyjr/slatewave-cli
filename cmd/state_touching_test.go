package cmd

import (
	"strings"
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
)

// Fixture manifest bodies used across the diagnose / reconcile tests.
// Detect/verify commands use `true` and `false` so tests don't depend
// on real tools being installed on the runner.
const (
	manifestHealthy = `[theme]
slug = "okayish"
name = "OK"
category = "editor"
detect_command = "true"

[install]
type = "manual"

[activate]
type = "none"

[verify]
command = "true"
`

	manifestVerifyFails = `[theme]
slug = "drifted"
name = "Drifted"
category = "editor"
detect_command = "true"

[install]
type = "manual"

[activate]
type = "none"

[verify]
command = "false"
`

	manifestDetectFails = `[theme]
slug = "gone"
name = "Gone"
category = "editor"
detect_command = "false"

[install]
type = "manual"

[activate]
type = "none"

[verify]
command = "true"
`
)

// ----- verifyInstalled -----

func TestVerifyInstalled_EmptyCommandIsTrue(t *testing.T) {
	if !verifyInstalled(manifest.Theme{Verify: manifest.Verify{}}) {
		t.Error("empty verify command should return true (we trust state)")
	}
}

func TestVerifyInstalled_ZeroExitIsTrue(t *testing.T) {
	if !verifyInstalled(manifest.Theme{Verify: manifest.Verify{Command: "true"}}) {
		t.Error("verify `true` should return true")
	}
}

func TestVerifyInstalled_NonZeroExitIsFalse(t *testing.T) {
	if verifyInstalled(manifest.Theme{Verify: manifest.Verify{Command: "false"}}) {
		t.Error("verify `false` should return false")
	}
}

func TestVerifyInstalled_ExpectMatchesSubstring(t *testing.T) {
	th := manifest.Theme{Verify: manifest.Verify{
		Command: "echo 'slatewave palette loaded'",
		Expect:  "slatewave",
	}}
	if !verifyInstalled(th) {
		t.Error("expect substring matched stdout but verifyInstalled returned false")
	}
}

func TestVerifyInstalled_ExpectAbsentIsFalse(t *testing.T) {
	th := manifest.Theme{Verify: manifest.Verify{
		Command: "echo 'something else'",
		Expect:  "slatewave",
	}}
	if verifyInstalled(th) {
		t.Error("expect substring missing from stdout but verifyInstalled returned true")
	}
}

// ----- diagnose -----

func TestDiagnose_EmptyStateReturnsEmpty(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})

	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	rows := diagnose(s)
	if len(rows) != 0 {
		t.Errorf("empty state diagnose = %d rows, want 0", len(rows))
	}
}

func TestDiagnose_HealthyRow(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})

	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	rows := diagnose(s)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].status != statusHealthy {
		t.Errorf("row status = %v, want statusHealthy", rows[0].status)
	}
	if rows[0].slug != "okayish" {
		t.Errorf("row slug = %q, want okayish", rows[0].slug)
	}
}

func TestDiagnose_StaleWhenVerifyFails(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"drifted": manifestVerifyFails})
	env.putRecord(t, state.Record{Slug: "drifted", InstallType: "manual"})

	s, _ := state.Load()
	rows := diagnose(s)
	if len(rows) != 1 || rows[0].status != statusStale {
		t.Errorf("got %+v, want one statusStale row", rows)
	}
	if !strings.Contains(rows[0].remedy, "slatewave update drifted") {
		t.Errorf("stale row should suggest `slatewave update`, got remedy %q", rows[0].remedy)
	}
}

func TestDiagnose_MissingToolWhenDetectFails(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"gone": manifestDetectFails})
	env.putRecord(t, state.Record{Slug: "gone", InstallType: "manual"})

	s, _ := state.Load()
	rows := diagnose(s)
	if len(rows) != 1 || rows[0].status != statusMissingTool {
		t.Errorf("got %+v, want one statusMissingTool row", rows)
	}
	if !strings.Contains(rows[0].remedy, "uninstall gone") {
		t.Errorf("missing-tool row should suggest uninstall, got %q", rows[0].remedy)
	}
}

func TestDiagnose_OrphanWhenManifestMissing(t *testing.T) {
	env := setupCmdEnv(t)
	// Manifest dir has no .toml at all — the recorded slug has no
	// matching manifest, so it's an orphan.
	env.useManifestDir(t, map[string]string{})
	env.putRecord(t, state.Record{Slug: "no-such", InstallType: "manual"})

	s, _ := state.Load()
	rows := diagnose(s)
	if len(rows) != 1 || rows[0].status != statusOrphan {
		t.Errorf("got %+v, want one statusOrphan row", rows)
	}
	if !strings.Contains(rows[0].remedy, "slatewave uninstall no-such") {
		t.Errorf("orphan row should suggest uninstall, got %q", rows[0].remedy)
	}
}

// ----- reconcileWithReality -----

func TestReconcileWithReality_DropsStaleAndOrphan(t *testing.T) {
	env := setupCmdEnv(t)
	// Healthy + stale + orphan, all installed in state.
	env.useManifestDir(t, map[string]string{
		"okayish": manifestHealthy,
		"drifted": manifestVerifyFails,
	})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})
	env.putRecord(t, state.Record{Slug: "drifted", InstallType: "manual"})
	env.putRecord(t, state.Record{Slug: "no-manifest", InstallType: "manual"})

	s, _ := state.Load()
	removed := reconcileWithReality(s)
	if removed != 2 {
		t.Errorf("removed = %d, want 2 (drifted stale + no-manifest orphan)", removed)
	}
	if _, ok := s.Get("okayish"); !ok {
		t.Error("healthy record was removed by reconcile")
	}
	if _, ok := s.Get("drifted"); ok {
		t.Error("stale record survived reconcile")
	}
	if _, ok := s.Get("no-manifest"); ok {
		t.Error("orphan record survived reconcile")
	}
}

// ----- summary (cmd/list.go) -----

func TestSummary_CountsInstalled(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{
		"okayish": manifestHealthy,
		"drifted": manifestVerifyFails,
	})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})

	all, _ := manifest.LoadAll()
	s, _ := state.Load()
	out := summary(all, s)
	if !strings.Contains(out, "1 of 2 installed") {
		t.Errorf("summary doesn't show `1 of 2 installed`: %q", out)
	}
}

// ----- statusOne via void contract -----

func TestStatusOne_PrintsRecordFields(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})
	env.putRecord(t, state.Record{
		Slug:         "okayish",
		InstallType:  "manual",
		ActivateType: "none",
		CreatedPaths: []string{"/tmp/example"},
	})

	s, _ := state.Load()
	statusOne(s, "okayish")
	out := env.out.String()
	if !strings.Contains(out, "OK") {
		t.Errorf("status output missing theme name `OK`: %q", out)
	}
	if !strings.Contains(out, "/tmp/example") {
		t.Errorf("status output missing CreatedPaths entry: %q", out)
	}
}

func TestStatusOne_UnknownSlugErrors(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{})

	s, _ := state.Load()
	statusOne(s, "ghost")
	out := env.out.String()
	if !strings.Contains(out, "ghost is not installed") {
		t.Errorf("expected `ghost is not installed` in output: %q", out)
	}
}

// ----- renderRow + renderDoctorRow (smoke) -----

func TestRenderRow_NotInstalledShowsCircle(t *testing.T) {
	th := manifest.Theme{Theme: manifest.Meta{Slug: "bat", Category: "terminal"}}
	s := &state.Store{Records: map[string]state.Record{}}
	out := renderRow(th, s)
	if !strings.Contains(out, "○") {
		t.Errorf("not-installed row should contain ○: %q", out)
	}
	if !strings.Contains(out, "bat") {
		t.Errorf("not-installed row missing slug: %q", out)
	}
}

func TestRenderRow_InstalledShowsFilledCircle(t *testing.T) {
	th := manifest.Theme{Theme: manifest.Meta{Slug: "bat", Category: "terminal"}}
	s := &state.Store{Records: map[string]state.Record{
		"bat": {Slug: "bat", InstallType: "curl", ActivateType: "ini-key"},
	}}
	out := renderRow(th, s)
	if !strings.Contains(out, "●") {
		t.Errorf("installed row should contain ●: %q", out)
	}
	if !strings.Contains(out, "installed") {
		t.Errorf("installed row should say `installed`: %q", out)
	}
}

func TestRenderDoctorRow_HealthyShowsCheck(t *testing.T) {
	out := renderDoctorRow(doctorRow{slug: "bat", status: statusHealthy})
	if !strings.Contains(out, "✓") || !strings.Contains(out, "healthy") {
		t.Errorf("healthy row should show ✓ and `healthy`: %q", out)
	}
}

func TestRenderDoctorRow_StaleShowsRemedy(t *testing.T) {
	r := doctorRow{slug: "bat", status: statusStale, detail: "verify failed", remedy: "slatewave update bat"}
	out := renderDoctorRow(r)
	if !strings.Contains(out, "⚠") || !strings.Contains(out, "stale") {
		t.Errorf("stale row missing ⚠ + `stale`: %q", out)
	}
	if !strings.Contains(out, "slatewave update bat") {
		t.Errorf("stale row missing remedy: %q", out)
	}
}

func TestRenderDoctorRow_OrphanShowsDanger(t *testing.T) {
	r := doctorRow{slug: "ghost", status: statusOrphan, detail: "no manifest", remedy: "slatewave uninstall ghost"}
	out := renderDoctorRow(r)
	if !strings.Contains(out, "✗") || !strings.Contains(out, "orphan") {
		t.Errorf("orphan row missing ✗ + `orphan`: %q", out)
	}
}

func TestDoctorSummary(t *testing.T) {
	out := doctorSummary(3, 1, 0, 2)
	for _, want := range []string{"3 healthy", "1 stale", "2 orphan"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor summary missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "missing tool") {
		t.Errorf("doctor summary should omit zero-count buckets: %q", out)
	}
}
