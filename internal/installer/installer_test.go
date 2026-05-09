package installer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
)

// hasGit reports whether `git` is on PATH. Tests that exercise the
// gitconfig reversal path skip cleanly when it isn't (highly unlikely
// on dev / CI but the alternative is a misleading panic in t.Fatal).
func hasGit() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// gitInTempHome redirects HOME / XDG / GIT_CONFIG_GLOBAL so `git config
// --global` writes to a per-test .gitconfig instead of the user's real
// one. Mirrors the activator package's helper of the same name; both
// could move into a shared testutil if a third caller appears.
func gitInTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, ".gitconfig"))
	return dir
}

// stubTheme is a minimal manifest.Theme for uninstall tests that don't
// exercise type-specific reversal (vscode-ext etc.).
func stubTheme(slug, installType string) manifest.Theme {
	return manifest.Theme{
		Theme:   manifest.Meta{Slug: slug},
		Install: manifest.Install{Type: installType},
	}
}

// ----- expandPath -----

func TestExpandPath_Tilde(t *testing.T) {
	t.Setenv("HOME", "/users/test")
	got, err := expandPath("~/.config/slatewave/foo.toml")
	if err != nil {
		t.Fatal(err)
	}
	want := "/users/test/.config/slatewave/foo.toml"
	if got != want {
		t.Errorf("expandPath ~ = %q, want %q", got, want)
	}
}

func TestExpandPath_EnvVar(t *testing.T) {
	t.Setenv("HOME", "/h")
	got, err := expandPath("$HOME/.bat/themes/Slatewave.tmTheme")
	if err != nil {
		t.Fatal(err)
	}
	want := "/h/.bat/themes/Slatewave.tmTheme"
	if got != want {
		t.Errorf("expandPath $HOME = %q, want %q", got, want)
	}
}

func TestExpandPath_Empty(t *testing.T) {
	if _, err := expandPath(""); err == nil {
		t.Error("expandPath should reject empty input")
	}
}

func TestExpandPath_AbsolutePathUnchanged(t *testing.T) {
	got, err := expandPath("/usr/local/bin/slatewave")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/usr/local/bin/slatewave" {
		t.Errorf("absolute path was rewritten: %q", got)
	}
}

// ----- removeShellRCLine -----
//
// removeShellRCLine is the most error-prone path in the uninstaller —
// any miss here means we either leave a slatewave line in the user's
// rc file forever, or we strip a line we shouldn't. Cover both shapes.

func TestRemoveShellRCLine_RemovesLineAndAdjacentMarker(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	body := `# user content
alias gs='git status'

# slatewave
export BAT_THEME=Slatewave

# unrelated tail
`
	if err := os.WriteFile(rc, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := removeShellRCLine(rc, "export BAT_THEME=Slatewave", "# slatewave", Options{}); err != nil {
		t.Fatalf("removeShellRCLine: %v", err)
	}
	got, _ := os.ReadFile(rc)
	gotStr := string(got)

	if strings.Contains(gotStr, "BAT_THEME") {
		t.Errorf("BAT_THEME line still present:\n%s", gotStr)
	}
	if strings.Contains(gotStr, "# slatewave") {
		t.Errorf("# slatewave marker still present (should drop the marker that precedes our line):\n%s", gotStr)
	}
	// User content must survive.
	if !strings.Contains(gotStr, "alias gs='git status'") {
		t.Errorf("user content was stripped:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "# unrelated tail") {
		t.Errorf("user trailing content was stripped:\n%s", gotStr)
	}
}

func TestRemoveShellRCLine_PreservesUnrelatedSlatewaveCommentAbove(t *testing.T) {
	// If a "# slatewave" comment appears in the file but is NOT followed
	// by our exact line, we must NOT silently strip it — it's the user's
	// own annotation. The current implementation re-inserts the marker
	// in that case.
	rc := filepath.Join(t.TempDir(), ".zshrc")
	body := `# slatewave
echo not-our-line

# slatewave
export BAT_THEME=Slatewave
`
	if err := os.WriteFile(rc, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeShellRCLine(rc, "export BAT_THEME=Slatewave", "# slatewave", Options{}); err != nil {
		t.Fatalf("removeShellRCLine: %v", err)
	}
	got, _ := os.ReadFile(rc)
	gotStr := string(got)

	if !strings.Contains(gotStr, "echo not-our-line") {
		t.Errorf("user line was incorrectly removed:\n%s", gotStr)
	}
	if strings.Contains(gotStr, "BAT_THEME") {
		t.Errorf("our line was not removed:\n%s", gotStr)
	}
	// The first "# slatewave" should be preserved (it's the user's),
	// the second was the marker for our line and should be gone.
	if !strings.Contains(gotStr, "# slatewave\necho not-our-line") {
		t.Errorf("user's # slatewave comment was eaten by uninstaller:\n%s", gotStr)
	}
}

func TestRemoveShellRCLine_NoOpIfLineAbsent(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	body := "# nothing related\n"
	if err := os.WriteFile(rc, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeShellRCLine(rc, "export BAT_THEME=Slatewave", "# slatewave", Options{}); err != nil {
		t.Fatalf("removeShellRCLine on absent line: %v", err)
	}
	got, _ := os.ReadFile(rc)
	if string(got) != body {
		t.Errorf("file mutated when line was absent:\nbefore:\n%sAfter:\n%s", body, got)
	}
}

func TestRemoveShellRCLine_NoOpIfFileMissing(t *testing.T) {
	rc := filepath.Join(t.TempDir(), "does-not-exist")
	// Should NOT error — the rc file might have been deleted by the
	// user between install and uninstall.
	if err := removeShellRCLine(rc, "export BAT_THEME=Slatewave", "# slatewave", Options{}); err != nil {
		t.Errorf("expected no error on missing file: %v", err)
	}
}

// Lua targets like wezterm.lua use `-- slatewave` as the marker. The
// uninstaller must drop that adjacent marker just like it drops `# slatewave`.
func TestRemoveShellRCLine_RemovesLuaMarker(t *testing.T) {
	rc := filepath.Join(t.TempDir(), "wezterm.lua")
	body := `local wezterm = require 'wezterm'
local config = wezterm.config_builder()

-- slatewave
require('slatewave-full').apply_to_config(config)

return config
`
	if err := os.WriteFile(rc, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeShellRCLine(rc, "require('slatewave-full').apply_to_config(config)", "-- slatewave", Options{}); err != nil {
		t.Fatalf("removeShellRCLine: %v", err)
	}
	got, _ := os.ReadFile(rc)
	gotStr := string(got)
	if strings.Contains(gotStr, "slatewave-full") {
		t.Errorf("activation line still present:\n%s", gotStr)
	}
	if strings.Contains(gotStr, "-- slatewave") {
		t.Errorf("`-- slatewave` marker still present (must drop with the line):\n%s", gotStr)
	}
	// User content must survive intact.
	for _, want := range []string{"local wezterm = require 'wezterm'", "wezterm.config_builder()", "return config"} {
		if !strings.Contains(gotStr, want) {
			t.Errorf("user line %q stripped:\n%s", want, gotStr)
		}
	}
}

// Mixed-style file (a `# slatewave` block AND a `-- slatewave` block,
// hypothetically left by sequential installs of two different themes
// against the same file): each uninstall removes only its own pair.
func TestRemoveShellRCLine_PreservesUnrelatedLuaMarker(t *testing.T) {
	rc := filepath.Join(t.TempDir(), "mixed.conf")
	body := `# slatewave
export BAT_THEME=Slatewave

-- slatewave
local x = 1
`
	if err := os.WriteFile(rc, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// Only remove the BAT_THEME line — the Lua block must survive.
	if err := removeShellRCLine(rc, "export BAT_THEME=Slatewave", "# slatewave", Options{}); err != nil {
		t.Fatalf("removeShellRCLine: %v", err)
	}
	got, _ := os.ReadFile(rc)
	gotStr := string(got)
	if strings.Contains(gotStr, "BAT_THEME") {
		t.Errorf("target line not removed:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "-- slatewave\nlocal x = 1\n") {
		t.Errorf("unrelated `-- slatewave` block was eaten:\n%s", gotStr)
	}
}

func TestRemoveShellRCLine_DryRunMakesNoChanges(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	body := "# slatewave\nexport BAT_THEME=Slatewave\n"
	if err := os.WriteFile(rc, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeShellRCLine(rc, "export BAT_THEME=Slatewave", "# slatewave", Options{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(rc)
	if string(got) != body {
		t.Errorf("dry-run mutated the file:\n%s", got)
	}
}

// ----- Uninstall happy path -----

func TestUninstall_DeletesCreatedPaths(t *testing.T) {
	dir := t.TempDir()
	created := filepath.Join(dir, "Slatewave.tmTheme")
	if err := os.WriteFile(created, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := state.Record{
		Slug:         "bat",
		InstallType:  "curl",
		CreatedPaths: []string{created},
	}
	if err := Uninstall(rec, stubTheme("bat", "curl"), Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("CreatedPaths entry %s still on disk after uninstall", created)
	}
}

func TestUninstall_RestoresBackup(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "btop.conf")
	backup := filepath.Join(dir, "btop.conf.bak")
	if err := os.WriteFile(original, []byte("color_theme = \"slatewave\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("color_theme = \"Default\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := state.Record{
		Slug:         "btop",
		InstallType:  "curl",
		ActivateType: "ini-key",
		Backups:      []state.Backup{{Original: original, Path: backup}},
	}
	if err := Uninstall(rec, stubTheme("btop", "curl"), Options{}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(original)
	if !strings.Contains(string(got), "Default") {
		t.Errorf("backup was not restored: %s", got)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Errorf("backup file %s should be removed after restore", backup)
	}
}

// Restore round-trip: the .bak captured the user's pre-install mode,
// so writing the original back must use that mode rather than the
// activated file's (or a hardcoded 0o644). Pairs with the activator's
// preservedMode test on the install side.
func TestUninstall_RestoreUsesBackupFileMode(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "secrets.conf")
	backup := filepath.Join(dir, "secrets.conf.bak")
	// Activated file (currently on disk) is whatever mode — say 0o644.
	if err := os.WriteFile(original, []byte("activated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Backup carries the user's pre-install mode (0o600 — paranoid setup).
	if err := os.WriteFile(backup, []byte("pre-install\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := state.Record{
		Slug: "x", InstallType: "curl", ActivateType: "ini-key",
		Backups: []state.Backup{{Original: original, Path: backup}},
	}
	if err := Uninstall(rec, stubTheme("x", "curl"), Options{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(original)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("restored file mode = %o, want 0o600 (restore widened user's mode)", got)
	}
}

// If the same activation line ended up in the rc file twice (a bug in
// an older slatewave version, or a manual paste from the docs while
// our state record was already there), uninstall must remove every
// occurrence — not just the first. A leftover duplicate keeps the
// theme sourced and survives `slatewave list` reconcile.
func TestRemoveShellRCLine_RemovesAllOccurrences(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	body := `# user prelude

# slatewave
export BAT_THEME=Slatewave

alias gs='git status'

# slatewave
export BAT_THEME=Slatewave

# user epilogue
`
	if err := os.WriteFile(rc, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeShellRCLine(rc, "export BAT_THEME=Slatewave", "# slatewave", Options{}); err != nil {
		t.Fatalf("removeShellRCLine: %v", err)
	}
	got, _ := os.ReadFile(rc)
	gotStr := string(got)

	if strings.Contains(gotStr, "BAT_THEME") {
		t.Errorf("BAT_THEME line still present (only first occurrence removed):\n%s", gotStr)
	}
	if strings.Contains(gotStr, "# slatewave") {
		t.Errorf("# slatewave marker still present after removal:\n%s", gotStr)
	}
	for _, want := range []string{"# user prelude", "alias gs='git status'", "# user epilogue"} {
		if !strings.Contains(gotStr, want) {
			t.Errorf("user content %q stripped:\n%s", want, gotStr)
		}
	}
}

// `git config --unset-all <name> <value-pattern>` treats the value as
// a regex. Paths contain `.`, which would match any character — without
// the regexp.QuoteMeta + ^…$ anchors we'd risk removing an unrelated
// include whose path coincidentally matches.
//
// The test seeds two includes whose paths differ only in a character
// position where one has `.` and the other has `x`. An unescaped
// pattern based on the first path matches both; an escaped+anchored
// pattern only matches the first.
func TestUninstall_GitconfigInclude_RegexEscapesPath(t *testing.T) {
	if !hasGit() {
		t.Skip("git not on PATH")
	}
	dir := gitInTempHome(t)
	target := filepath.Join(dir, "a.gitconfig")  // the one we want unset
	collide := filepath.Join(dir, "axgitconfig") // unrelated; matches if `.` is unescaped
	for _, p := range []string{target, collide} {
		if err := os.WriteFile(p, []byte("# noop\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command("git", "config", "--global", "--add", "include.path", p).CombinedOutput()
		if err != nil {
			t.Fatalf("seed include.path %s: %v\n%s", p, err, out)
		}
	}

	rec := state.Record{
		Slug:         "delta",
		InstallType:  "curl",
		ActivateType: "gitconfig-include",
		AppendedLine: &state.Appended{File: "git-config-include", Line: target},
	}
	if err := Uninstall(rec, stubTheme("delta", "curl"), Options{}); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	out, err := exec.Command("git", "config", "--global", "--get-all", "include.path").CombinedOutput()
	if err != nil && !strings.Contains(err.Error(), "exit status 1") {
		// exit 1 just means "no values found" — fine if all entries were
		// (incorrectly) removed; the assertions below catch that.
		t.Fatalf("git config --get-all: %v\n%s", err, out)
	}
	got := string(out)
	if strings.Contains(got, target) {
		t.Errorf("target include %q still present after uninstall:\n%s", target, got)
	}
	if !strings.Contains(got, collide) {
		t.Errorf("unrelated include %q was removed (regex collision):\n%s", collide, got)
	}
}

func TestUninstall_DryRunMakesNoChanges(t *testing.T) {
	dir := t.TempDir()
	created := filepath.Join(dir, "Slatewave.tmTheme")
	if err := os.WriteFile(created, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := state.Record{Slug: "bat", InstallType: "curl", CreatedPaths: []string{created}}
	if err := Uninstall(rec, stubTheme("bat", "curl"), Options{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(created); err != nil {
		t.Errorf("dry-run uninstall removed the file: %v", err)
	}
}

