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

func TestVerifyInstalled_TrustStateOverridesFailingCommand(t *testing.T) {
	// trust_state opts out of verification entirely — even when Command
	// is set and would fail, it's not run. Keeps doctor from flagging
	// installs whose post-install location we can't peek at.
	th := manifest.Theme{Verify: manifest.Verify{
		Command:    "false",
		TrustState: true,
	}}
	if !verifyInstalled(th) {
		t.Error("trust_state=true should bypass Command and return true")
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

// ----- installOne / installBulk -----

// installOne with a manual+none manifest persists a state.Record. We use
// `manual` to avoid network/binary dependencies in tests — the install
// pipeline is a no-op so we exercise the wiring without side-effects.
func TestInstallOne_PersistsRecordOnSuccess(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})

	if err := installOne("okayish", false); err != nil {
		t.Fatalf("installOne: %v", err)
	}

	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	rec, ok := s.Get("okayish")
	if !ok {
		t.Fatal("install didn't persist a record")
	}
	if rec.InstallType != "manual" {
		t.Errorf("rec.InstallType = %q, want manual", rec.InstallType)
	}
	out := env.out.String()
	if !strings.Contains(out, "Done.") {
		t.Errorf("output missing `Done.` marker: %q", out)
	}
}

func TestInstallOne_SuppressFinalSkipsDoneLine(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})

	if err := installOne("okayish", true); err != nil {
		t.Fatalf("installOne suppressFinal: %v", err)
	}
	if strings.Contains(env.out.String(), "Done.") {
		t.Errorf("suppressFinal=true leaked the `Done.` line: %q", env.out.String())
	}
}

func TestInstallOne_DryRunSkipsState(t *testing.T) {
	prev := installDryRun
	installDryRun = true
	defer func() { installDryRun = prev }()

	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})

	if err := installOne("okayish", false); err != nil {
		t.Fatalf("installOne dry-run: %v", err)
	}
	s, _ := state.Load()
	if _, ok := s.Get("okayish"); ok {
		t.Error("dry-run install wrote a state record — should be a no-op")
	}
	if !strings.Contains(env.out.String(), "Dry run") {
		t.Errorf("dry-run output missing `Dry run` marker: %q", env.out.String())
	}
}

func TestInstallOne_UnknownSlugReturnsNoManifestError(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{})

	err := installOne("ghost", false)
	if err == nil {
		t.Fatal("installOne for missing slug returned nil err")
	}
	if !strings.Contains(err.Error(), "no manifest") {
		t.Errorf("err = %v, want `no manifest`", err)
	}
}

// installBulk skips slugs already in state (re-running --all after a
// theme is added is a common workflow) and emits a summary line at the end.
func TestInstallBulk_SkipsAlreadyInstalledAndCounts(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{
		"okayish": manifestHealthy,
		"second":  strings.Replace(manifestHealthy, "okayish", "second", 1),
	})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})

	if err := installBulk([]string{"okayish", "second"}); err != nil {
		t.Fatalf("installBulk: %v", err)
	}

	out := env.out.String()
	if !strings.Contains(out, "Skipping okayish") {
		t.Errorf("expected skip line for already-installed slug: %q", out)
	}
	if !strings.Contains(out, "1 installed, 1 skipped.") {
		t.Errorf("expected `1 installed, 1 skipped.` summary: %q", out)
	}
}

func TestInstallBulk_AllAlreadyInstalledReportsNothingToInstall(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})

	if err := installBulk([]string{"okayish"}); err != nil {
		t.Fatalf("installBulk: %v", err)
	}
	if !strings.Contains(env.out.String(), "Nothing to install") {
		t.Errorf("expected `Nothing to install` summary: %q", env.out.String())
	}
}

// ----- uninstallOne / uninstallBulk -----

// uninstallOne with a manual install record cleanly reverses (no created
// paths, no appended line) and removes the record from state.
func TestUninstallOne_RemovesRecord(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})

	if err := uninstallOne("okayish"); err != nil {
		t.Fatalf("uninstallOne: %v", err)
	}
	s, _ := state.Load()
	if _, ok := s.Get("okayish"); ok {
		t.Error("uninstall didn't remove the record from state")
	}
	if !strings.Contains(env.out.String(), "Reverted.") {
		t.Errorf("expected `Reverted.` in output: %q", env.out.String())
	}
}

func TestUninstallOne_DryRunKeepsRecord(t *testing.T) {
	prev := uninstallDryRun
	uninstallDryRun = true
	defer func() { uninstallDryRun = prev }()

	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})

	if err := uninstallOne("okayish"); err != nil {
		t.Fatalf("uninstallOne dry-run: %v", err)
	}
	s, _ := state.Load()
	if _, ok := s.Get("okayish"); !ok {
		t.Error("dry-run uninstall removed the record — should be a no-op")
	}
	if !strings.Contains(env.out.String(), "Dry run") {
		t.Errorf("dry-run output missing `Dry run` marker: %q", env.out.String())
	}
}

func TestUninstallOne_NotInstalledErrors(t *testing.T) {
	setupCmdEnv(t)
	err := uninstallOne("ghost")
	if err == nil || !strings.Contains(err.Error(), "is not installed") {
		t.Errorf("uninstallOne(ghost): err = %v, want `is not installed`", err)
	}
}

func TestUninstallOne_MissingManifestErrors(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{})
	env.putRecord(t, state.Record{Slug: "ghost", InstallType: "manual"})

	err := uninstallOne("ghost")
	if err == nil || !strings.Contains(err.Error(), "no manifest") {
		t.Errorf("uninstallOne with missing manifest: err = %v, want `no manifest`", err)
	}
}

func TestUninstallBulk_NothingInstalled(t *testing.T) {
	prev := uninstallCategory
	uninstallCategory = ""
	defer func() { uninstallCategory = prev }()

	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{})

	if err := uninstallBulk(); err != nil {
		t.Fatalf("uninstallBulk: %v", err)
	}
	if !strings.Contains(env.out.String(), "Nothing to uninstall") {
		t.Errorf("expected `Nothing to uninstall` for empty state: %q", env.out.String())
	}
}

func TestUninstallBulk_CategoryFilterMissesAllInstalled(t *testing.T) {
	prev := uninstallCategory
	uninstallCategory = "terminal" // okayish is editor in manifestHealthy
	defer func() { uninstallCategory = prev }()

	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})

	err := uninstallBulk()
	if err == nil || !strings.Contains(err.Error(), `no installed themes in category "terminal"`) {
		t.Errorf("category filter with no matches: err = %v, want `no installed themes in category`", err)
	}
}

func TestUninstallBulk_RemovesAllRecords(t *testing.T) {
	prev := uninstallCategory
	uninstallCategory = ""
	defer func() { uninstallCategory = prev }()

	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{
		"okayish": manifestHealthy,
		"second":  strings.Replace(manifestHealthy, "okayish", "second", 1),
	})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})
	env.putRecord(t, state.Record{Slug: "second", InstallType: "manual"})

	if err := uninstallBulk(); err != nil {
		t.Fatalf("uninstallBulk: %v", err)
	}
	s, _ := state.Load()
	if len(s.Records) != 0 {
		t.Errorf("expected empty state after bulk uninstall, got %v", s.Records)
	}
	if !strings.Contains(env.out.String(), "2 uninstalled.") {
		t.Errorf("expected `2 uninstalled.` summary: %q", env.out.String())
	}
}

// ----- updateOne / updateBulk -----

// `manual` install type has no automated update — updater.Update returns
// ErrNoAutomatedUpdate, which updateOne propagates after printing a hint.
func TestUpdateOne_ManualReturnsNoAutomatedUpdate(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})

	err := updateOne("okayish", false)
	if err == nil {
		t.Fatal("updateOne for manual install returned nil err")
	}
	if !strings.Contains(env.out.String(), "No automated update for install type") {
		t.Errorf("expected hint about no automated update: %q", env.out.String())
	}
}

func TestUpdateOne_NotInstalledErrors(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})

	err := updateOne("okayish", false)
	if err == nil || !strings.Contains(err.Error(), "is not installed") {
		t.Errorf("updateOne for not-installed slug: err = %v, want `is not installed`", err)
	}
}

func TestUpdateOne_UnknownSlugErrors(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{})

	err := updateOne("ghost", false)
	if err == nil || !strings.Contains(err.Error(), "no manifest") {
		t.Errorf("updateOne for unknown slug: err = %v, want `no manifest`", err)
	}
}

// updateBulk treats ErrNoAutomatedUpdate as a "skipped" — the run keeps
// going and the summary shows no failures.
func TestUpdateBulk_ManualThemeIsSkippedNotFailed(t *testing.T) {
	prev := updateCategory
	updateCategory = ""
	defer func() { updateCategory = prev }()

	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})

	if err := updateBulk(); err != nil {
		t.Fatalf("updateBulk: %v", err)
	}
	if !strings.Contains(env.out.String(), "0 updated, 1 skipped.") {
		t.Errorf("expected `0 updated, 1 skipped.` summary: %q", env.out.String())
	}
}

func TestUpdateBulk_NothingInstalled(t *testing.T) {
	prev := updateCategory
	updateCategory = ""
	defer func() { updateCategory = prev }()

	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{})

	if err := updateBulk(); err != nil {
		t.Fatalf("updateBulk: %v", err)
	}
	if !strings.Contains(env.out.String(), "Nothing to update") {
		t.Errorf("expected `Nothing to update` for empty state: %q", env.out.String())
	}
}

func TestUpdateBulk_CategoryFilterMatchesNothing(t *testing.T) {
	prev := updateCategory
	updateCategory = "terminal" // okayish is editor
	defer func() { updateCategory = prev }()

	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{"okayish": manifestHealthy})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})

	err := updateBulk()
	if err == nil || !strings.Contains(err.Error(), `no installed themes in category "terminal"`) {
		t.Errorf("category filter with no matches: err = %v, want `no installed themes in category`", err)
	}
}

// ----- statusAll -----

func TestStatusAll_EmptyStatePrintsHint(t *testing.T) {
	env := setupCmdEnv(t)

	s, _ := state.Load()
	if err := statusAll(s); err != nil {
		t.Fatalf("statusAll: %v", err)
	}
	if !strings.Contains(env.out.String(), "Nothing installed yet") {
		t.Errorf("expected `Nothing installed yet` for empty state: %q", env.out.String())
	}
}

func TestStatusAll_PrintsEveryInstalledSlug(t *testing.T) {
	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{
		"okayish": manifestHealthy,
		"drifted": manifestVerifyFails,
	})
	env.putRecord(t, state.Record{Slug: "okayish", InstallType: "manual"})
	env.putRecord(t, state.Record{Slug: "drifted", InstallType: "manual"})

	s, _ := state.Load()
	if err := statusAll(s); err != nil {
		t.Fatalf("statusAll: %v", err)
	}
	out := env.out.String()
	// Both manifests are loaded so each record gets its theme name printed.
	for _, want := range []string{"OK", "Drifted"} {
		if !strings.Contains(out, want) {
			t.Errorf("statusAll output missing %q: %q", want, out)
		}
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
