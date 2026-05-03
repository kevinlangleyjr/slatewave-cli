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
