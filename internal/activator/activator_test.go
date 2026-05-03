package activator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
)

// ----- quoteIfNeeded -----

func TestQuoteIfNeeded(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`slatewave`, `"slatewave"`},
		{`"slatewave"`, `"slatewave"`}, // already quoted, leave alone
		{`with space`, `"with space"`},
		{`"already quoted with space"`, `"already quoted with space"`},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := quoteIfNeeded(c.in); got != c.want {
				t.Errorf("quoteIfNeeded(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// ----- ini-key activator -----

func TestActivate_IniKey_ReplacesExistingValue(t *testing.T) {
	file := filepath.Join(t.TempDir(), "btop.conf")
	original := `# btop config
something_else = true
color_theme = "Default"
trailing = false
`
	if err := os.WriteFile(file, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := state.Record{Slug: "btop"}
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "btop"},
		Activate: manifest.Activate{
			Type:  "ini-key",
			File:  file,
			Key:   "color_theme",
			Value: "slatewave",
		},
	}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read after activate: %v", err)
	}
	if !strings.Contains(string(got), `color_theme = "slatewave"`) {
		t.Errorf("expected color_theme set to slatewave, got:\n%s", got)
	}
	if strings.Contains(string(got), `color_theme = "Default"`) {
		t.Errorf("old value still present after activate")
	}
	// Other keys must survive untouched.
	if !strings.Contains(string(got), "something_else = true") {
		t.Errorf("collateral damage: other keys removed")
	}

	// A backup must be recorded for uninstall to reverse.
	if len(rec.Backups) != 1 {
		t.Fatalf("expected 1 backup recorded, got %d", len(rec.Backups))
	}
	if rec.Backups[0].Original != file {
		t.Errorf("backup.Original mismatch: %q vs %q", rec.Backups[0].Original, file)
	}
	if _, err := os.Stat(rec.Backups[0].Path); err != nil {
		t.Errorf("backup file missing on disk: %v", err)
	}
}

func TestActivate_IniKey_IdempotentNoOpIfAlreadySet(t *testing.T) {
	file := filepath.Join(t.TempDir(), "btop.conf")
	original := `color_theme = "slatewave"
`
	if err := os.WriteFile(file, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := state.Record{Slug: "btop"}
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "btop"},
		Activate: manifest.Activate{
			Type: "ini-key", File: file, Key: "color_theme", Value: "slatewave",
		},
	}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	if len(rec.Backups) != 0 {
		t.Errorf("idempotent activate should not back up an unchanged file; got %d backups", len(rec.Backups))
	}
	// The file must be byte-for-byte identical.
	got, _ := os.ReadFile(file)
	if string(got) != original {
		t.Errorf("idempotent activate mutated the file:\nbefore:\n%s\nafter:\n%s", original, got)
	}
}

func TestActivate_IniKey_AppendsKeyWhenAbsent(t *testing.T) {
	// File exists but doesn't yet contain our target key — common shape
	// for a fresh config the user hasn't customized our option in yet.
	// Activator should append the line under a "# slatewave" marker
	// rather than erroring.
	file := filepath.Join(t.TempDir(), "btop.conf")
	original := "other_key = false\n"
	if err := os.WriteFile(file, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := state.Record{}
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "btop"},
		Activate: manifest.Activate{
			Type: "ini-key", File: file, Key: "color_theme", Value: "slatewave",
		},
	}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	got, _ := os.ReadFile(file)
	gotStr := string(got)
	if !strings.Contains(gotStr, "other_key = false") {
		t.Errorf("preexisting line was clobbered:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, `color_theme = "slatewave"`) {
		t.Errorf("color_theme line not appended:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "# slatewave") {
		t.Errorf("missing slatewave marker comment:\n%s", gotStr)
	}
	// Modified file → backup recorded.
	if len(rec.Backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(rec.Backups))
	}
}

func TestActivate_IniKey_CreatesFileWhenAbsent(t *testing.T) {
	// File doesn't exist (e.g., ~/.config/ghostty/config before the user
	// has ever launched ghostty). Activator should create the parent dir,
	// write a fresh file with just our line, and record the file as a
	// CreatedPath so uninstall removes it (rather than restoring a backup
	// of an empty file).
	dir := t.TempDir()
	file := filepath.Join(dir, "subdir", "config") // parent dir missing too

	rec := state.Record{}
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "ghostty"},
		Activate: manifest.Activate{
			Type: "ini-key", File: file, Key: "theme", Value: "Slatewave",
		},
	}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if !strings.Contains(string(got), `theme = "Slatewave"`) {
		t.Errorf("expected our line in fresh file:\n%s", got)
	}
	// Created from scratch → CreatedPath, NOT Backup. Uninstall should
	// remove the file rather than restore an empty one.
	if len(rec.CreatedPaths) != 1 || rec.CreatedPaths[0] != file {
		t.Errorf("expected CreatedPaths=[%s], got %+v", file, rec.CreatedPaths)
	}
	if len(rec.Backups) != 0 {
		t.Errorf("creating from scratch should not record a backup; got %d", len(rec.Backups))
	}
}

func TestActivate_IniKey_DryRunMakesNoChanges(t *testing.T) {
	file := filepath.Join(t.TempDir(), "btop.conf")
	original := `color_theme = "Default"
`
	if err := os.WriteFile(file, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := state.Record{}
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "btop"},
		Activate: manifest.Activate{
			Type: "ini-key", File: file, Key: "color_theme", Value: "slatewave",
		},
	}
	if err := Activate(th, &rec, Options{DryRun: true}); err != nil {
		t.Fatalf("Activate(dry-run): %v", err)
	}
	got, _ := os.ReadFile(file)
	if string(got) != original {
		t.Errorf("dry-run mutated the file:\n%s", got)
	}
	if len(rec.Backups) != 0 {
		t.Errorf("dry-run should not record backups")
	}
}

// ----- shell-rc activator -----

func TestActivate_ShellRC_AppendsAndIsIdempotent(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	if err := os.WriteFile(rc, []byte("# existing zshrc content\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "bat"},
		Activate: manifest.Activate{
			Type:  "shell-rc",
			Files: []string{rc},
			Line:  "export BAT_THEME=Slatewave",
		},
	}

	// First call appends.
	rec := state.Record{}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("first Activate: %v", err)
	}
	got, _ := os.ReadFile(rc)
	if !strings.Contains(string(got), "export BAT_THEME=Slatewave") {
		t.Errorf("expected line appended, got:\n%s", got)
	}
	if !strings.Contains(string(got), "# slatewave") {
		t.Errorf("expected slatewave marker comment; got:\n%s", got)
	}
	if rec.AppendedLine == nil || rec.AppendedLine.File != rc {
		t.Errorf("AppendedLine not recorded: %+v", rec.AppendedLine)
	}

	// Snapshot, run Activate again — must be a no-op (idempotent).
	snapshot, _ := os.ReadFile(rc)
	rec2 := state.Record{}
	if err := Activate(th, &rec2, Options{}); err != nil {
		t.Fatalf("second Activate: %v", err)
	}
	after, _ := os.ReadFile(rc)
	if string(snapshot) != string(after) {
		t.Errorf("idempotent Activate changed the file:\nbefore:\n%s\nafter:\n%s", snapshot, after)
	}
	if rec2.AppendedLine != nil {
		t.Errorf("idempotent Activate should not record AppendedLine; got %+v", rec2.AppendedLine)
	}
}

func TestActivate_ShellRC_CreatesFileWhenAbsent(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	// Note: rc does NOT exist on disk yet.
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "bat"},
		Activate: manifest.Activate{
			Type:  "shell-rc",
			Files: []string{rc},
			Line:  "export BAT_THEME=Slatewave",
		},
	}
	rec := state.Record{}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatalf("rc not created: %v", err)
	}
	if !strings.Contains(string(got), "export BAT_THEME=Slatewave") {
		t.Errorf("appended line missing in fresh rc:\n%s", got)
	}
}

// ----- pickShellRC -----

func TestPickShellRC_PrefersExistingFile(t *testing.T) {
	dir := t.TempDir()
	zshrc := filepath.Join(dir, ".zshrc")
	bashrc := filepath.Join(dir, ".bashrc")
	if err := os.WriteFile(zshrc, []byte("z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// .bashrc deliberately not created — we want pickShellRC to choose
	// .zshrc because it's the only one that exists.
	got, err := pickShellRC([]string{bashrc, zshrc})
	if err != nil {
		t.Fatal(err)
	}
	if got != zshrc {
		t.Errorf("pickShellRC = %q, want %q", got, zshrc)
	}
}

func TestPickShellRC_FallsBackToShellEnvWhenNoneExist(t *testing.T) {
	dir := t.TempDir()
	zshrc := filepath.Join(dir, ".zshrc")
	bashrc := filepath.Join(dir, ".bashrc")
	// Neither file created.

	t.Setenv("SHELL", "/bin/zsh")
	got, err := pickShellRC([]string{bashrc, zshrc})
	if err != nil {
		t.Fatal(err)
	}
	if got != zshrc {
		t.Errorf("with SHELL=zsh, pickShellRC = %q, want %q", got, zshrc)
	}
}

func TestPickShellRC_FallsBackToFirstCandidateWhenNoMatch(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, ".zshrc")
	second := filepath.Join(dir, ".bashrc")
	t.Setenv("SHELL", "/usr/bin/exotic-shell") // nothing matches
	got, err := pickShellRC([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Errorf("ultimate fallback = %q, want first candidate %q", got, first)
	}
}

func TestPickShellRC_EmptyCandidatesErrors(t *testing.T) {
	if _, err := pickShellRC(nil); err == nil {
		t.Error("expected error on empty candidates")
	}
}

// ----- toml-import activator -----

func TestTOMLImportRewrite_AddsToExistingArray(t *testing.T) {
	in := `[general]
import = ["~/.config/alacritty/themes/gruvbox.toml"]

[font]
size = 14
`
	got, changed, err := tomlImportRewrite(in, "/users/test/.config/alacritty/themes/slatewave.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected change")
	}
	if !strings.Contains(got, `"~/.config/alacritty/themes/gruvbox.toml"`) {
		t.Errorf("existing entry was clobbered:\n%s", got)
	}
	if !strings.Contains(got, `"/users/test/.config/alacritty/themes/slatewave.toml"`) {
		t.Errorf("our entry not added:\n%s", got)
	}
	// Other sections must survive.
	if !strings.Contains(got, "size = 14") {
		t.Errorf("unrelated section damaged:\n%s", got)
	}
}

func TestTOMLImportRewrite_IdempotentIfAlreadyImported(t *testing.T) {
	entry := "/users/test/.config/alacritty/themes/slatewave.toml"
	in := fmt.Sprintf(`[general]
import = [%q]
`, entry)
	got, changed, err := tomlImportRewrite(in, entry)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("expected no change when entry already imported")
	}
	if got != in {
		t.Errorf("idempotent rewrite mutated content:\nbefore:\n%s\nafter:\n%s", in, got)
	}
}

func TestTOMLImportRewrite_AddsArrayWhenMissing(t *testing.T) {
	in := `[general]

[font]
size = 14
`
	got, changed, err := tomlImportRewrite(in, "/users/test/themes/slatewave.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected change")
	}
	if !strings.Contains(got, `import = ["/users/test/themes/slatewave.toml"]`) {
		t.Errorf("expected new import line, got:\n%s", got)
	}
	if !strings.Contains(got, "# slatewave") {
		t.Errorf("expected slatewave marker comment, got:\n%s", got)
	}
}

func TestTOMLImportRewrite_EmptyArrayGetsFilled(t *testing.T) {
	in := `import = []
`
	got, changed, err := tomlImportRewrite(in, "/path/slatewave.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected change")
	}
	if !strings.Contains(got, `import = ["/path/slatewave.toml"]`) {
		t.Errorf("empty array not filled cleanly: %q", got)
	}
}

// ----- Activate dispatch -----

func TestActivate_NoneIsNoOp(t *testing.T) {
	rec := state.Record{}
	th := manifest.Theme{
		Theme:    manifest.Meta{Slug: "vscode"},
		Activate: manifest.Activate{Type: "none"},
	}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate(none): %v", err)
	}
}

func TestActivate_UnknownTypeErrors(t *testing.T) {
	rec := state.Record{}
	th := manifest.Theme{
		Theme:    manifest.Meta{Slug: "x"},
		Activate: manifest.Activate{Type: "telepathy"},
	}
	if err := Activate(th, &rec, Options{}); err == nil {
		t.Error("Activate with unknown type should error")
	}
}
