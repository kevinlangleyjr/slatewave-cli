package activator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
)

// hasGit returns true when `git` is on PATH. The doGitconfigInclude
// activator shells out to `git config`; tests that exercise that path
// skip cleanly if git isn't available (unlikely on CI runners and dev
// machines, but the alternative is a panic in t.Fatal).
func hasGit() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// gitInTempHome redirects HOME so `git config --global` writes to a
// per-test ~/.gitconfig instead of the real user file. Mirrors the
// pattern used by state_test.go's stateInTempHome.
func gitInTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	// Some systems (CI) prefer XDG_CONFIG_HOME; force git to honor HOME.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	// GIT_CONFIG_GLOBAL is the most explicit override available — wins
	// over HOME / XDG. Pin to the path git would have used so the test
	// works whether git resolves --global via HOME or XDG.
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, ".gitconfig"))
	return dir
}

// ----- doGitconfigInclude -----

func TestActivate_GitconfigInclude_AddsAndIsIdempotent(t *testing.T) {
	if !hasGit() {
		t.Skip("git not on PATH")
	}
	gitInTempHome(t)

	includePath := filepath.Join(t.TempDir(), "delta-slatewave.gitconfig")
	if err := os.WriteFile(includePath, []byte("[delta]\n  features = slatewave\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "delta"},
		Activate: manifest.Activate{
			Type:        "gitconfig-include",
			IncludePath: includePath,
		},
	}
	rec := state.Record{}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate first run: %v", err)
	}

	// Verify git config now lists the include path.
	out, err := exec.Command("git", "config", "--global", "--get-all", "include.path").CombinedOutput()
	if err != nil {
		t.Fatalf("git config --get-all: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), includePath) {
		t.Errorf("include path not added to .gitconfig:\n%s", out)
	}

	// AppendedLine should record the include for uninstall.
	if rec.AppendedLine == nil || rec.AppendedLine.File != "git-config-include" || rec.AppendedLine.Line != includePath {
		t.Errorf("AppendedLine not recorded: %+v", rec.AppendedLine)
	}

	// Idempotency: second run is a no-op (same git config state, fresh rec).
	rec2 := state.Record{}
	if err := Activate(th, &rec2, Options{}); err != nil {
		t.Fatalf("Activate second run: %v", err)
	}
	out2, _ := exec.Command("git", "config", "--global", "--get-all", "include.path").CombinedOutput()
	count := strings.Count(string(out2), includePath)
	if count != 1 {
		t.Errorf("idempotency broken — include path appears %d times after second run, want 1:\n%s", count, out2)
	}
	// Second run shouldn't record AppendedLine since nothing changed.
	if rec2.AppendedLine != nil {
		t.Errorf("idempotent run still recorded AppendedLine: %+v", rec2.AppendedLine)
	}
}

func TestActivate_GitconfigInclude_MissingPathErrors(t *testing.T) {
	rec := state.Record{}
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "delta"},
		Activate: manifest.Activate{
			Type:        "gitconfig-include",
			IncludePath: "",
		},
	}
	err := Activate(th, &rec, Options{})
	if err == nil || !strings.Contains(err.Error(), "missing include_path") {
		t.Errorf("missing include_path: err = %v, want `missing include_path`", err)
	}
}

func TestActivate_GitconfigInclude_DryRunMakesNoChanges(t *testing.T) {
	if !hasGit() {
		t.Skip("git not on PATH")
	}
	gitInTempHome(t)

	includePath := filepath.Join(t.TempDir(), "delta.gitconfig")
	if err := os.WriteFile(includePath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "delta"},
		Activate: manifest.Activate{
			Type:        "gitconfig-include",
			IncludePath: includePath,
		},
	}
	rec := state.Record{}
	if err := Activate(th, &rec, Options{DryRun: true}); err != nil {
		t.Fatalf("dry-run Activate: %v", err)
	}

	out, _ := exec.Command("git", "config", "--global", "--get-all", "include.path").CombinedOutput()
	if strings.Contains(string(out), includePath) {
		t.Errorf("dry-run still wrote to gitconfig:\n%s", out)
	}
	if rec.AppendedLine != nil {
		t.Errorf("dry-run recorded AppendedLine: %+v", rec.AppendedLine)
	}
}

// ----- doTOMLImport (file I/O wrapping; the pure rewrite helper is
// already covered by tomlImportRewrite tests) -----

func TestActivate_TOMLImport_CreatesFileWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nested", "alacritty.toml") // parent missing

	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "alacritty"},
		Activate: manifest.Activate{
			Type:     "toml-import",
			TOMLPath: file,
			Import:   "/themes/slatewave.toml",
		},
	}
	rec := state.Record{}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if !strings.Contains(string(got), `"/themes/slatewave.toml"`) {
		t.Errorf("import entry missing in fresh file:\n%s", got)
	}
	// Fresh file should land in CreatedPaths (uninstall deletes), not Backups.
	if len(rec.CreatedPaths) != 1 || rec.CreatedPaths[0] != file {
		t.Errorf("CreatedPaths = %v, want [%s]", rec.CreatedPaths, file)
	}
	if len(rec.Backups) != 0 {
		t.Errorf("fresh-file create unexpectedly recorded a backup: %+v", rec.Backups)
	}
}

func TestActivate_TOMLImport_BacksUpExistingFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "alacritty.toml")
	original := "[font]\nsize = 14\n"
	if err := os.WriteFile(file, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "alacritty"},
		Activate: manifest.Activate{
			Type:     "toml-import",
			TOMLPath: file,
			Import:   "/themes/slatewave.toml",
		},
	}
	rec := state.Record{}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	if len(rec.Backups) != 1 {
		t.Fatalf("Backups = %v, want 1 entry", rec.Backups)
	}
	backup, err := os.ReadFile(rec.Backups[0].Path)
	if err != nil {
		t.Fatalf("backup not readable: %v", err)
	}
	if string(backup) != original {
		t.Errorf("backup contents drifted: got %q, want %q", backup, original)
	}
	// Existing-file path should NOT record CreatedPaths — uninstall must
	// restore the backup, not delete the file.
	if len(rec.CreatedPaths) != 0 {
		t.Errorf("existing-file activate recorded CreatedPaths: %v", rec.CreatedPaths)
	}
}

func TestActivate_TOMLImport_IdempotentIfAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "alacritty.toml")
	// File already imports our entry — Activate should be a no-op.
	body := "import = [\"/themes/slatewave.toml\"]\n[font]\nsize = 14\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "alacritty"},
		Activate: manifest.Activate{
			Type:     "toml-import",
			TOMLPath: file,
			Import:   "/themes/slatewave.toml",
		},
	}
	rec := state.Record{}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	got, _ := os.ReadFile(file)
	if string(got) != body {
		t.Errorf("idempotent run rewrote the file:\nbefore: %q\nafter:  %q", body, got)
	}
	if len(rec.Backups) != 0 || len(rec.CreatedPaths) != 0 {
		t.Errorf("idempotent run recorded reversal info: backups=%v, paths=%v", rec.Backups, rec.CreatedPaths)
	}
}

func TestActivate_TOMLImport_DryRunMakesNoChanges(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "alacritty.toml")
	original := "[font]\nsize = 14\n"
	if err := os.WriteFile(file, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "alacritty"},
		Activate: manifest.Activate{
			Type:     "toml-import",
			TOMLPath: file,
			Import:   "/themes/slatewave.toml",
		},
	}
	rec := state.Record{}
	if err := Activate(th, &rec, Options{DryRun: true}); err != nil {
		t.Fatalf("dry-run Activate: %v", err)
	}

	got, _ := os.ReadFile(file)
	if string(got) != original {
		t.Errorf("dry-run mutated file:\nbefore: %q\nafter:  %q", original, got)
	}
	if len(rec.Backups) != 0 || len(rec.CreatedPaths) != 0 {
		t.Errorf("dry-run recorded reversal info: backups=%v, paths=%v", rec.Backups, rec.CreatedPaths)
	}
}

func TestActivate_TOMLImport_MissingTomlPathErrors(t *testing.T) {
	rec := state.Record{}
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "alacritty"},
		Activate: manifest.Activate{
			Type:     "toml-import",
			TOMLPath: "",
			Import:   "/themes/slatewave.toml",
		},
	}
	err := Activate(th, &rec, Options{})
	if err == nil || !strings.Contains(err.Error(), "missing toml_path") {
		t.Errorf("missing toml_path: err = %v", err)
	}
}

func TestActivate_TOMLImport_MissingImportErrors(t *testing.T) {
	rec := state.Record{}
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "alacritty"},
		Activate: manifest.Activate{
			Type:     "toml-import",
			TOMLPath: filepath.Join(t.TempDir(), "x.toml"),
			Import:   "",
		},
	}
	err := Activate(th, &rec, Options{})
	if err == nil || !strings.Contains(err.Error(), "missing toml_path or import") {
		t.Errorf("missing import: err = %v", err)
	}
}

// ----- yaml-set activator -----

// lsdPairs is the lsd manifest's exact set: two color subkeys.
var lsdPairs = []manifest.YAMLPair{
	{Path: "color.when", Value: "auto"},
	{Path: "color.theme", Value: "custom"},
}

func TestYAMLSetRewrite_EmptyFile_WritesFresh(t *testing.T) {
	got, changed, err := yamlSetRewrite("", lsdPairs)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true on empty file")
	}
	want := "# slatewave\ncolor:\n  when: auto\n  theme: custom\n"
	if got != want {
		t.Errorf("empty-file output mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestYAMLSetRewrite_NoParent_AppendsBlock(t *testing.T) {
	in := "# user config\ndate:\n  date: relative\n"
	got, changed, err := yamlSetRewrite(in, lsdPairs)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when parent absent")
	}
	// Original content preserved.
	if !strings.Contains(got, "date:\n  date: relative") {
		t.Errorf("preexisting keys lost:\n%s", got)
	}
	// New block appended with both children.
	if !strings.Contains(got, "color:\n  when: auto\n  theme: custom") {
		t.Errorf("color block not appended:\n%s", got)
	}
	// Slatewave marker is present so uninstall (and a human reader) can
	// identify the block.
	if !strings.Contains(got, "# slatewave") {
		t.Errorf("missing slatewave marker:\n%s", got)
	}
}

func TestYAMLSetRewrite_ParentExistsMissingChildren_InsertsBoth(t *testing.T) {
	in := "color:\n  unrelated: thing\n"
	got, changed, err := yamlSetRewrite(in, lsdPairs)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when children absent under existing parent")
	}
	// Both new children are inserted under `color:` (after `unrelated`).
	for _, want := range []string{"  when: auto", "  theme: custom", "  unrelated: thing"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// `color:` should appear exactly once — we didn't accidentally append a
	// second top-level block.
	if strings.Count(got, "\ncolor:") != 0 || strings.Count(got, "color:\n") != 1 {
		t.Errorf("color: should appear exactly once at top:\n%s", got)
	}
}

func TestYAMLSetRewrite_ChildAtWrongValue_Replaces(t *testing.T) {
	in := "color:\n  when: never\n  theme: nord\n"
	got, changed, err := yamlSetRewrite(in, lsdPairs)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when child value differs")
	}
	if !strings.Contains(got, "  when: auto") || strings.Contains(got, "  when: never") {
		t.Errorf("when not replaced:\n%s", got)
	}
	if !strings.Contains(got, "  theme: custom") || strings.Contains(got, "  theme: nord") {
		t.Errorf("theme not replaced:\n%s", got)
	}
}

func TestYAMLSetRewrite_AlreadyAtDesiredValue_NoOp(t *testing.T) {
	in := "color:\n  when: auto\n  theme: custom\n"
	got, changed, err := yamlSetRewrite(in, lsdPairs)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if changed {
		t.Errorf("expected changed=false when both pairs already at desired value, got changed=true; output:\n%s", got)
	}
	if got != in {
		t.Errorf("content modified despite no-op:\n got: %q\nwant: %q", got, in)
	}
}

func TestYAMLSetRewrite_FourSpaceIndent_PreservesIndent(t *testing.T) {
	// User has chosen 4-space indent for their YAML — the rewriter must
	// respect it, not silently force 2-space children.
	in := "color:\n    when: never\n"
	got, _, err := yamlSetRewrite(in, lsdPairs)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if !strings.Contains(got, "    when: auto") {
		t.Errorf("when: replacement didn't preserve 4-space indent:\n%s", got)
	}
	if !strings.Contains(got, "    theme: custom") {
		t.Errorf("theme: insertion didn't use existing 4-space indent:\n%s", got)
	}
}

func TestYAMLSetRewrite_RejectsDeepPath(t *testing.T) {
	_, _, err := yamlSetRewrite("", []manifest.YAMLPair{{Path: "a.b.c", Value: "x"}})
	if err == nil || !strings.Contains(err.Error(), "depth 2") {
		t.Errorf("expected depth-2 error for path a.b.c, got %v", err)
	}
}

// End-to-end test through Activate(): file is created, backup recorded
// when overwriting, idempotent on repeat run.
func TestActivate_YAMLSet_CreatesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")

	rec := state.Record{Slug: "lsd"}
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "lsd"},
		Activate: manifest.Activate{
			Type:     "yaml-set",
			YAMLPath: file,
			YAMLSet:  lsdPairs,
		},
	}

	// First run: file absent → created, recorded as CreatedPath.
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("first activate: %v", err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(got), "  when: auto") || !strings.Contains(string(got), "  theme: custom") {
		t.Errorf("file content missing keys:\n%s", got)
	}
	if len(rec.CreatedPaths) != 1 || rec.CreatedPaths[0] != file {
		t.Errorf("expected CreatedPath %s, got %v", file, rec.CreatedPaths)
	}
	if len(rec.Backups) != 0 {
		t.Errorf("expected no backups when file was created fresh, got %d", len(rec.Backups))
	}

	// Second run: file already at desired state → no-op (no new backup).
	rec2 := state.Record{Slug: "lsd"}
	if err := Activate(th, &rec2, Options{}); err != nil {
		t.Fatalf("idempotent activate: %v", err)
	}
	if len(rec2.Backups) != 0 {
		t.Errorf("expected no-op on idempotent rerun, got %d backups", len(rec2.Backups))
	}
}

func TestActivate_YAMLSet_BacksUpExistingFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	original := "# user config\ndate:\n  date: relative\n"
	if err := os.WriteFile(file, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := state.Record{Slug: "lsd"}
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "lsd"},
		Activate: manifest.Activate{
			Type:     "yaml-set",
			YAMLPath: file,
			YAMLSet:  lsdPairs,
		},
	}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("activate: %v", err)
	}

	if len(rec.Backups) != 1 {
		t.Fatalf("expected 1 backup recorded, got %d", len(rec.Backups))
	}
	backupContent, err := os.ReadFile(rec.Backups[0].Path)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backupContent) != original {
		t.Errorf("backup contents diverged from original")
	}

	// Live file got our keys; original keys preserved.
	live, _ := os.ReadFile(file)
	if !strings.Contains(string(live), "date: relative") {
		t.Errorf("collateral damage: lost user's date config:\n%s", live)
	}
	if !strings.Contains(string(live), "  when: auto") {
		t.Errorf("color.when not set:\n%s", live)
	}
}
