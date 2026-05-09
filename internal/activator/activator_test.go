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

// Mode preservation: a user with chmod 0o600 on their config (e.g.
// .gitconfig with [user] secrets, or a paranoid btop.conf) must not
// have it silently downgraded to 0o644 just because slatewave activated
// a theme. The activator's WriteFile previously hardcoded 0o644;
// preservedMode now honors what the file already had. The same property
// must hold across the whole activate → backup round-trip: the .bak
// file matches original mode, so an eventual uninstall restore lands
// the user back at exactly the mode they started with.
func TestActivate_IniKey_PreservesOriginalFileMode(t *testing.T) {
	file := filepath.Join(t.TempDir(), "secrets.conf")
	original := "color_theme = \"Default\"\n"
	if err := os.WriteFile(file, []byte(original), 0o600); err != nil {
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

	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("activated file mode = %o, want 0o600 (mode silently widened)", got)
	}

	if len(rec.Backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(rec.Backups))
	}
	bakInfo, err := os.Stat(rec.Backups[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := bakInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("backup mode = %o, want 0o600 (uninstall would restore at wrong mode)", got)
	}
}

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

// indexOf returns the position of the first slice element equal to s,
// or -1 when absent. Local helper for the splice tests below.
func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return -1
}

// ----- shell-rc scaffold path -----

const wezScaffold = `local wezterm = require 'wezterm'
local config  = wezterm.config_builder()

config.color_scheme = 'Slatewave'

return config
`

func wezTheme(target string) manifest.Theme {
	return manifest.Theme{
		Theme: manifest.Meta{Slug: "wezterm"},
		Activate: manifest.Activate{
			Type:          "shell-rc",
			Files:         []string{target},
			Line:          "config.color_scheme = 'Slatewave'",
			Scaffold:      wezScaffold,
			InsertBefore:  "return config",
			CommentPrefix: "--",
		},
	}
}

// Missing file + scaffold set → scaffold becomes the full file contents
// and the path is recorded as a CreatedPath (so uninstall removes the
// file we created, rather than splicing one line out of it).
func TestActivate_ShellRC_ScaffoldWritesFullFileWhenMissing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "wezterm", "wezterm.lua")

	rec := state.Record{}
	if err := Activate(wezTheme(target), &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("scaffold not written: %v", err)
	}
	if string(got) != wezScaffold {
		t.Errorf("scaffold not written verbatim.\n--- want ---\n%s\n--- got ---\n%s", wezScaffold, got)
	}
	if rec.AppendedLine != nil {
		t.Errorf("scaffold path recorded AppendedLine = %+v, want nil (uninstall should delete the file, not splice a line)", rec.AppendedLine)
	}
	if len(rec.CreatedPaths) != 1 || rec.CreatedPaths[0] != target {
		t.Errorf("CreatedPaths = %v, want [%q]", rec.CreatedPaths, target)
	}
}

// Empty file (touched but never edited) is the same case as missing —
// we still own the contents and write the scaffold.
func TestActivate_ShellRC_ScaffoldWritesWhenFileWhitespaceOnly(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "wezterm.lua")
	if err := os.WriteFile(target, []byte("   \n\t\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := state.Record{}
	if err := Activate(wezTheme(target), &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got, _ := os.ReadFile(target)
	if !strings.Contains(string(got), "wezterm.config_builder()") {
		t.Errorf("scaffold did not replace whitespace-only file:\n%s", got)
	}
}

// Existing wezterm.lua with real content: scaffold must not overwrite,
// and the activation line must land ABOVE `return config` (Lua halts at
// return; lines below it never run). The append fallback is wrong here.
func TestActivate_ShellRC_InsertBeforeSplicesAboveAnchor(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "wezterm.lua")
	original := "local wezterm = require 'wezterm'\nlocal config = wezterm.config_builder()\n\nreturn config\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := state.Record{}
	if err := Activate(wezTheme(target), &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	got, _ := os.ReadFile(target)
	gotLines := strings.Split(string(got), "\n")
	colorIdx := indexOf(gotLines, "config.color_scheme = 'Slatewave'")
	returnIdx := indexOf(gotLines, "return config")
	if colorIdx == -1 {
		t.Fatalf("activation line missing from spliced file:\n%s", got)
	}
	if returnIdx == -1 {
		t.Fatalf("`return config` no longer in file:\n%s", got)
	}
	if colorIdx >= returnIdx {
		t.Errorf("activation line at idx %d, `return config` at idx %d — line should land ABOVE `return config`:\n%s", colorIdx, returnIdx, got)
	}
	// User's original lines must all still be present and in order.
	for _, want := range []string{"local wezterm = require 'wezterm'", "local config = wezterm.config_builder()", "return config"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("splice dropped user line %q; file now:\n%s", want, got)
		}
	}
	// The Lua-targeting wezTheme uses `--` for the marker so the inserted
	// block is a valid Lua comment, not a syntax error. The marker must
	// sit immediately above the activation line for the uninstaller's
	// "drop marker + next line" heuristic to round-trip.
	if !strings.Contains(string(got), "-- slatewave\nconfig.color_scheme = 'Slatewave'\n") {
		t.Errorf("splice didn't keep `-- slatewave` adjacent to the activation line:\n%s", got)
	}
	if strings.Contains(string(got), "# slatewave") {
		t.Errorf("default `#` marker leaked into a Lua-targeted splice:\n%s", got)
	}
	if rec.AppendedLine == nil || rec.AppendedLine.File != target {
		t.Errorf("splice should record AppendedLine for uninstall; got %+v", rec.AppendedLine)
	}
	if len(rec.CreatedPaths) != 0 {
		t.Errorf("splice should NOT record CreatedPaths; got %v", rec.CreatedPaths)
	}
}

// When InsertBefore doesn't match anything in the file, fall back to the
// append-with-marker behavior so manifests can opt into "splice if you
// can, append otherwise" without the activator failing closed.
func TestActivate_ShellRC_InsertBeforeFallsBackToAppend(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "wezterm.lua")
	// Custom config without `return config` — the user wrote their own
	// structure that the manifest's anchor doesn't cover.
	original := "local wezterm = require 'wezterm'\nlocal config = {}\n-- end\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := state.Record{}
	if err := Activate(wezTheme(target), &rec, Options{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got, _ := os.ReadFile(target)
	if !strings.HasPrefix(string(got), original) {
		t.Errorf("append fallback rewrote user content above the original; file now:\n%s", got)
	}
	if !strings.Contains(string(got), "-- slatewave\nconfig.color_scheme = 'Slatewave'\n") {
		t.Errorf("append fallback didn't add marker + activation line:\n%s", got)
	}
}

// markerComment unit tests — the helper that picks the comment style
// for the activation marker.
func TestMarkerComment_DefaultsToHash(t *testing.T) {
	if got := markerComment(""); got != "# slatewave" {
		t.Errorf("markerComment(\"\") = %q, want `# slatewave`", got)
	}
}

func TestMarkerComment_RespectsCustomPrefix(t *testing.T) {
	cases := map[string]string{
		"--":  "-- slatewave",
		"//":  "// slatewave",
		";":   "; slatewave",
		"#":   "# slatewave",
		"REM": "REM slatewave",
	}
	for prefix, want := range cases {
		if got := markerComment(prefix); got != want {
			t.Errorf("markerComment(%q) = %q, want %q", prefix, got, want)
		}
	}
}

// spliceBefore unit checks: substring matching + anchor-not-found
// returning ok=false so the caller can fall back to append.
func TestSpliceBefore_InsertsAboveFirstAnchorMatch(t *testing.T) {
	in := "a\nb\nreturn config\nc\n"
	out, ok := spliceBefore(in, "return config", "# slatewave", "MARKER")
	if !ok {
		t.Fatal("expected splice ok=true")
	}
	want := "a\nb\n# slatewave\nMARKER\n\nreturn config\nc\n"
	if out != want {
		t.Errorf("splice output mismatch.\n--- want ---\n%q\n--- got ---\n%q", want, out)
	}
}

func TestSpliceBefore_AnchorMissingReturnsFalse(t *testing.T) {
	in := "a\nb\nc\n"
	out, ok := spliceBefore(in, "return config", "# slatewave", "MARKER")
	if ok {
		t.Errorf("anchor missing should return ok=false; got out=%q", out)
	}
	if out != in {
		t.Errorf("output should equal input on miss; got %q, want %q", out, in)
	}
}

func TestSpliceBefore_PreservesNoTrailingNewline(t *testing.T) {
	// Input without a trailing newline (rare but possible) — output must
	// not gain or lose the EOF newline.
	in := "a\nreturn config"
	out, ok := spliceBefore(in, "return config", "# slatewave", "MARKER")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if strings.HasSuffix(out, "\n") {
		t.Errorf("trailing newline added where input had none: %q", out)
	}
}

// Marker arg is plumbed through verbatim — Lua targets pass `-- slatewave`
// so the inserted block is a valid Lua comment, not a syntax error.
func TestSpliceBefore_LuaCommentMarker(t *testing.T) {
	in := "local config = {}\nreturn config\n"
	out, ok := spliceBefore(in, "return config", "-- slatewave", "require('x')")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(out, "-- slatewave\nrequire('x')\n") {
		t.Errorf("Lua marker not threaded through; output:\n%s", out)
	}
	if strings.Contains(out, "# slatewave") {
		t.Errorf("# slatewave leaked into Lua-targeted splice:\n%s", out)
	}
}

// Scaffold + empty file with DryRun: must not touch the filesystem and
// must not record a CreatedPath.
func TestActivate_ShellRC_ScaffoldRespectsDryRun(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "wezterm.lua")

	rec := state.Record{}
	if err := Activate(wezTheme(target), &rec, Options{DryRun: true}); err != nil {
		t.Fatalf("Activate dry-run: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote the file: stat err = %v", err)
	}
	if len(rec.CreatedPaths) != 0 {
		t.Errorf("dry-run recorded CreatedPaths = %v, want none", rec.CreatedPaths)
	}
}

// Re-running install when the scaffold already wrote the activation line
// must be a no-op — the idempotency scanner sees the line and bails before
// the scaffold branch even runs.
func TestActivate_ShellRC_ScaffoldIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "wezterm.lua")

	th := wezTheme(target)
	rec := state.Record{}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("first Activate: %v", err)
	}
	snapshot, _ := os.ReadFile(target)

	rec2 := state.Record{}
	if err := Activate(th, &rec2, Options{}); err != nil {
		t.Fatalf("second Activate: %v", err)
	}
	after, _ := os.ReadFile(target)
	if string(snapshot) != string(after) {
		t.Errorf("second Activate mutated the scaffolded file:\nbefore:\n%s\nafter:\n%s", snapshot, after)
	}
	if rec2.AppendedLine != nil || len(rec2.CreatedPaths) > 0 {
		t.Errorf("idempotent Activate recorded reversal info: AppendedLine=%+v CreatedPaths=%v", rec2.AppendedLine, rec2.CreatedPaths)
	}
}

// TestActivate_ShellRC_WindowsUsesWindowsFields covers the load-bearing
// Phase 5 promise: on Windows the activator picks files_windows /
// line_windows and ignores the unix variants. Mirrors the unix append
// test (a fresh "PowerShell profile" file lands the line + marker) but
// with the OS swapped.
func TestActivate_ShellRC_WindowsUsesWindowsFields(t *testing.T) {
	defer manifest.SetGOOSForTest("windows")()

	profile := filepath.Join(t.TempDir(), "Microsoft.PowerShell_profile.ps1")

	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "starship"},
		Activate: manifest.Activate{
			Type:         "shell-rc",
			Files:        []string{filepath.Join(t.TempDir(), ".zshrc")}, // unix variant — must be ignored
			Line:         `eval "$(starship init zsh)"`,
			FilesWindows: []string{profile},
			LineWindows:  `Invoke-Expression (&starship init powershell)`,
		},
	}
	rec := state.Record{}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate on windows: %v", err)
	}
	got, err := os.ReadFile(profile)
	if err != nil {
		t.Fatalf("profile not written: %v", err)
	}
	if !strings.Contains(string(got), "Invoke-Expression (&starship init powershell)") {
		t.Errorf("windows variant not appended:\n%s", got)
	}
	if strings.Contains(string(got), "starship init zsh") {
		t.Errorf("unix variant leaked into the windows profile:\n%s", got)
	}
	if rec.AppendedLine == nil || rec.AppendedLine.File != profile {
		t.Errorf("AppendedLine should record the windows profile path, got %+v", rec.AppendedLine)
	}
}

func TestActivate_ShellRC_WindowsErrorsWhenWindowsFieldsMissing(t *testing.T) {
	defer manifest.SetGOOSForTest("windows")()

	rc := filepath.Join(t.TempDir(), ".zshrc")
	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "starship"},
		Activate: manifest.Activate{
			Type:  "shell-rc",
			Files: []string{rc},
			Line:  `eval "$(starship init zsh)"`,
			// FilesWindows / LineWindows deliberately empty — this is the
			// configuration mistake we want to catch loudly rather than
			// silently writing bash to a phantom unix path.
		},
	}
	rec := state.Record{}
	err := Activate(th, &rec, Options{})
	if err == nil {
		t.Fatal("Activate on windows without files_windows/line_windows should error")
	}
	if !strings.Contains(err.Error(), "windows requires files_windows and line_windows") {
		t.Errorf("err = %v, want mention of files_windows + line_windows", err)
	}
	// The unix file must NOT have been touched as a fallback.
	if _, statErr := os.Stat(rc); statErr == nil {
		t.Errorf("activator wrote to unix path %q on windows fallback — guard failed", rc)
	}
}

// TestActivate_ShellRC_DarwinIgnoresWindowsFields locks the inverse: a
// manifest that ships both unix and windows variants must use the unix
// variant on darwin. Without this test, a regression in the OS branch
// could silently route every shell-rc activation through the Windows
// path on every platform.
func TestActivate_ShellRC_DarwinIgnoresWindowsFields(t *testing.T) {
	defer manifest.SetGOOSForTest("darwin")()

	rc := filepath.Join(t.TempDir(), ".zshrc")
	if err := os.WriteFile(rc, []byte("# existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	th := manifest.Theme{
		Theme: manifest.Meta{Slug: "starship"},
		Activate: manifest.Activate{
			Type:         "shell-rc",
			Files:        []string{rc},
			Line:         `eval "$(starship init zsh)"`,
			FilesWindows: []string{filepath.Join(t.TempDir(), "should-not-be-written.ps1")},
			LineWindows:  `Invoke-Expression (&starship init powershell)`,
		},
	}
	rec := state.Record{}
	if err := Activate(th, &rec, Options{}); err != nil {
		t.Fatalf("Activate on darwin: %v", err)
	}
	got, _ := os.ReadFile(rc)
	if !strings.Contains(string(got), "starship init zsh") {
		t.Errorf("unix line missing on darwin:\n%s", got)
	}
	if strings.Contains(string(got), "Invoke-Expression") {
		t.Errorf("windows line leaked into unix rc on darwin:\n%s", got)
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
