package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/tui"
)

// ----- noManifestError -----

func TestNoManifestError_AppendsHintWhenSlugIsClose(t *testing.T) {
	// "btap" is one char from "btop" (in the embedded set) — should hit
	// the SuggestSlug branch.
	err := noManifestError("btap")
	if err == nil {
		t.Fatal("noManifestError returned nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "btap") {
		t.Errorf("error missing the typo: %q", msg)
	}
	if !strings.Contains(msg, "btop") {
		t.Errorf("error missing did-you-mean suggestion `btop`: %q", msg)
	}
	if !strings.Contains(msg, "did you mean") {
		t.Errorf("error missing the `did you mean` framing: %q", msg)
	}
}

func TestNoManifestError_FallsBackWhenNoCloseMatch(t *testing.T) {
	err := noManifestError("xyzzy-thunderdome")
	if err == nil {
		t.Fatal("noManifestError returned nil")
	}
	msg := err.Error()
	if strings.Contains(msg, "did you mean") {
		t.Errorf("far-off slug shouldn't get a did-you-mean: %q", msg)
	}
	if !strings.Contains(msg, "slatewave list") {
		t.Errorf("fallback should point at `slatewave list`: %q", msg)
	}
}

// ----- resolveSlugs -----

func TestResolveSlugs_SingleArgPassesThrough(t *testing.T) {
	got, err := resolveSlugs([]string{"btop"}, false)
	if err != nil {
		t.Fatalf("resolveSlugs: %v", err)
	}
	if len(got) != 1 || got[0] != "btop" {
		t.Errorf("single-arg resolve = %v, want [btop]", got)
	}
}

func TestResolveSlugs_BulkAllReturnsEverySlug(t *testing.T) {
	prev := installCategory
	installCategory = ""
	defer func() { installCategory = prev }()

	got, err := resolveSlugs(nil, true)
	if err != nil {
		t.Fatalf("resolveSlugs bulk: %v", err)
	}
	// Embedded set has 24 slugs at time of writing — assert "more than a
	// handful" rather than a hard count so the test doesn't break every
	// time a manifest is added.
	if len(got) < 10 {
		t.Errorf("bulk resolve returned %d slugs, expected the full embedded set", len(got))
	}
}

func TestResolveSlugs_BulkWithCategoryFilters(t *testing.T) {
	prev := installCategory
	installCategory = "terminal"
	defer func() { installCategory = prev }()

	got, err := resolveSlugs(nil, true)
	if err != nil {
		t.Fatalf("resolveSlugs --category=terminal: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one terminal slug from embedded set")
	}
	// Spot-check: every returned slug must be in the terminal category.
	for _, slug := range got {
		th, err := manifest.LoadOne(slug)
		if err != nil {
			t.Errorf("manifest.LoadOne(%q): %v", slug, err)
			continue
		}
		if th.Theme.Category != "terminal" {
			t.Errorf("--category=terminal returned non-terminal slug %q (category=%q)", slug, th.Theme.Category)
		}
	}
}

func TestResolveSlugs_UnknownCategoryErrors(t *testing.T) {
	prev := installCategory
	installCategory = "no-such-category"
	defer func() { installCategory = prev }()

	_, err := resolveSlugs(nil, true)
	if err == nil || !strings.Contains(err.Error(), `no themes in category`) {
		t.Errorf("unknown category: err = %v, want `no themes in category`", err)
	}
}

// TestResolveSlugs_BulkFiltersUnsupportedOS asserts that --all on
// windows returns only the windows-supported slug set, never any unix-
// only manifest. Locks the filter behavior — the exact slug list isn't
// asserted (so adding a fifth windows manifest later doesn't break the
// test), but the absence of a known unix-only slug is.
func TestResolveSlugs_BulkFiltersUnsupportedOS(t *testing.T) {
	defer manifest.SetGOOSForTest("windows")()

	prev := installCategory
	installCategory = ""
	defer func() { installCategory = prev }()

	got, err := resolveSlugs(nil, true)
	if err != nil {
		t.Fatalf("resolveSlugs --all on windows: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least the four opted-in windows manifests")
	}
	for _, slug := range got {
		// alfred / iterm2 / xcode / etc. default to darwin+linux and
		// must never appear in a windows --all result.
		switch slug {
		case "alfred", "iterm2", "xcode", "raycast", "ghostty", "powerlevel10k":
			t.Errorf("unix-only slug %q leaked into windows --all result", slug)
		}
	}
}

// ----- installLabel + activateLabel + doneMessage -----

func TestInstallLabel(t *testing.T) {
	cases := []struct {
		install manifest.Install
		want    string
	}{
		{manifest.Install{Type: "curl"}, "Fetching theme file"},
		{manifest.Install{Type: "clone", Repo: "https://example.com/x"}, "Cloning https://example.com/x"},
		{manifest.Install{Type: "vscode-ext", Identifier: "x.y"}, "Installing code extension x.y"},
		{manifest.Install{Type: "vscode-ext", Identifier: "x.y", CLI: "cursor"}, "Installing cursor extension x.y"},
		{manifest.Install{Type: "marketplace"}, "Opening Marketplace in your browser"},
		{manifest.Install{Type: "gui-import"}, "Fetching theme file (GUI import follows)"},
		{manifest.Install{Type: "manual"}, "Manual install — see instructions below"},
		{manifest.Install{Type: "weirdo"}, "Running install step"}, // default branch
	}
	for _, c := range cases {
		got := installLabel(manifest.Theme{Install: c.install})
		if got != c.want {
			t.Errorf("installLabel(%q) = %q, want %q", c.install.Type, got, c.want)
		}
	}
}

func TestActivateLabel(t *testing.T) {
	cases := []struct {
		activate manifest.Activate
		want     string
	}{
		{manifest.Activate{Type: "ini-key", Key: "theme", Value: "Slatewave", File: "/x"}, `Setting theme = Slatewave in /x`},
		{manifest.Activate{Type: "gitconfig-include"}, "Adding include.path to ~/.gitconfig"},
		{manifest.Activate{Type: "shell-rc", Files: []string{"~/.zshrc"}}, "Appending to ~/.zshrc"},
		{manifest.Activate{Type: "shell-rc"}, "Appending activation line"},
		{manifest.Activate{Type: "toml-import", TOMLPath: "/y"}, "Importing into /y"},
		{manifest.Activate{Type: "yaml-set", YAMLPath: "/z/lsd.yaml"}, "Setting keys in /z/lsd.yaml"},
		{manifest.Activate{Type: "what"}, "Activating"}, // default branch
	}
	for _, c := range cases {
		got := activateLabel(manifest.Theme{Activate: c.activate})
		if got != c.want {
			t.Errorf("activateLabel(%q) = %q, want %q", c.activate.Type, got, c.want)
		}
	}
}

// ----- updateLabel (cmd/update.go) -----

func TestUpdateLabel(t *testing.T) {
	cases := []struct {
		install manifest.Install
		want    string
	}{
		{manifest.Install{Type: "curl"}, "Re-fetching theme file"},
		{manifest.Install{Type: "gui-import"}, "Re-fetching theme file"},
		{manifest.Install{Type: "clone", CloneDest: "~/.config/themes/x"}, "git pull --ff-only on ~/.config/themes/x"},
		{manifest.Install{Type: "vscode-ext", Identifier: "x.y"}, "Reinstalling code extension x.y"},
		{manifest.Install{Type: "vscode-ext", Identifier: "x.y", CLI: "cursor"}, "Reinstalling cursor extension x.y"},
		{manifest.Install{Type: "marketplace"}, "Marketplace install — manual update"},
		{manifest.Install{Type: "manual"}, "Manual install — manual update"},
		{manifest.Install{Type: "weirdo"}, "Updating"}, // default branch
	}
	for _, c := range cases {
		got := updateLabel(manifest.Theme{Install: c.install})
		if got != c.want {
			t.Errorf("updateLabel(%q) = %q, want %q", c.install.Type, got, c.want)
		}
	}
}

// ----- installDoneMessage / uninstallDoneMessage (cmd) -----
//
// Both helpers are now thin passthroughs: read manifest field, fall
// back to a generic default if empty. Per-theme strings live in the
// embedded *.toml manifests; the install/uninstall code shouldn't be
// the one keeping that data in sync.

func TestInstallDoneMessage_PassthroughAndDefault(t *testing.T) {
	custom := manifest.Theme{
		Install: manifest.Install{DoneMessage: "bat picks up the new theme on its next invocation."},
	}
	if got := installDoneMessage(custom); got != custom.Install.DoneMessage {
		t.Errorf("manifest done_message not echoed: got %q, want %q", got, custom.Install.DoneMessage)
	}

	bare := manifest.Theme{}
	if got := installDoneMessage(bare); got != "Slatewave is installed." {
		t.Errorf("default fallback = %q, want %q", got, "Slatewave is installed.")
	}
}

func TestUninstallDoneMessage_PassthroughAndDefault(t *testing.T) {
	custom := manifest.Theme{
		Uninstall: manifest.Uninstall{DoneMessage: "Reverted. VSCode picks up the change immediately."},
	}
	if got := uninstallDoneMessage(custom); got != custom.Uninstall.DoneMessage {
		t.Errorf("manifest uninstall.done_message not echoed: got %q, want %q", got, custom.Uninstall.DoneMessage)
	}

	bare := manifest.Theme{}
	if got := uninstallDoneMessage(bare); got != "Reverted." {
		t.Errorf("default fallback = %q, want %q", got, "Reverted.")
	}
}

// ----- summarize (cmd/init.go) -----

func TestSummarize_BucketsByPresentInstalledMissing(t *testing.T) {
	results := []tui.DetectResult{
		{Theme: manifest.Theme{Theme: manifest.Meta{Slug: "a"}}, Present: true, Installed: false},  // available
		{Theme: manifest.Theme{Theme: manifest.Meta{Slug: "b"}}, Present: true, Installed: true},   // already installed
		{Theme: manifest.Theme{Theme: manifest.Meta{Slug: "c"}}, Present: false, Installed: false}, // missing tool
		{Theme: manifest.Theme{Theme: manifest.Meta{Slug: "d"}}, Present: true, Installed: false},  // available
	}
	available, alreadyInstalled, missingTool := summarize(results)
	if available != 2 {
		t.Errorf("available = %d, want 2", available)
	}
	if alreadyInstalled != 1 {
		t.Errorf("alreadyInstalled = %d, want 1", alreadyInstalled)
	}
	if missingTool != 1 {
		t.Errorf("missingTool = %d, want 1", missingTool)
	}
}

// ----- installSuffix (cmd/list.go) -----

func TestInstallSuffix(t *testing.T) {
	cases := []struct {
		rec  state.Record
		want string
	}{
		{state.Record{ActivateType: "ini-key"}, ""},
		{state.Record{ActivateType: "shell-rc", AppendedLine: &state.Appended{File: "/Users/me/.zshrc"}}, " (via /Users/me/.zshrc)"},
		{state.Record{ActivateType: "shell-rc"}, ""}, // shell-rc but no AppendedLine = nothing useful to surface
		{state.Record{ActivateType: "gitconfig-include"}, " (gitconfig include)"},
		{state.Record{ActivateType: "weird"}, ""}, // unknown activate type
	}
	for _, c := range cases {
		got := installSuffix(c.rec)
		if got != c.want {
			t.Errorf("installSuffix(%q) = %q, want %q", c.rec.ActivateType, got, c.want)
		}
	}
}

// ----- printPostInstallInstructions (regression for browse → install
// where TUI dashboards swallowed the instructions block) -----

func TestPrintPostInstallInstructions_PrintsHeaderAndLines(t *testing.T) {
	env := setupCmdEnv(t)
	themes := []manifest.Theme{
		{
			Theme: manifest.Meta{Slug: "obsidian", Name: "Slatewave for Obsidian"},
			Install: manifest.Install{
				Instructions: []string{
					"Copy the theme into your vault:",
					"    cp -R ~/.local/share/obsidian-slatewave <vault>/.obsidian/themes/Slatewave",
					"In Obsidian, Settings → Appearance → Themes → Slatewave",
				},
			},
		},
	}
	printPostInstallInstructions(themes)
	out := env.out.String()
	if !strings.Contains(out, "Next steps for Slatewave for Obsidian") {
		t.Errorf("missing per-theme header: %q", out)
	}
	for _, want := range []string{"Copy the theme into your vault:", "<vault>/.obsidian/themes/Slatewave", "Settings → Appearance"} {
		if !strings.Contains(out, want) {
			t.Errorf("instructions output missing %q: %q", want, out)
		}
	}
}

func TestPrintPostInstallInstructions_NoOpWhenNoInstructions(t *testing.T) {
	env := setupCmdEnv(t)
	themes := []manifest.Theme{
		{Theme: manifest.Meta{Slug: "bat", Name: "Slatewave for bat"}},
	}
	printPostInstallInstructions(themes)
	if env.out.Len() != 0 {
		t.Errorf("themes with no instructions should produce no output, got %q", env.out.String())
	}
}

func TestPrintPostInstallInstructions_SkipsThemesWithoutInstructionsInMixedSet(t *testing.T) {
	env := setupCmdEnv(t)
	themes := []manifest.Theme{
		{Theme: manifest.Meta{Slug: "bat", Name: "Slatewave for bat"}},
		{
			Theme: manifest.Meta{Slug: "obsidian", Name: "Slatewave for Obsidian"},
			Install: manifest.Install{
				Instructions: []string{"Open Obsidian and pick Slatewave"},
			},
		},
		{Theme: manifest.Meta{Slug: "btop", Name: "Slatewave for btop"}},
	}
	printPostInstallInstructions(themes)
	out := env.out.String()
	if strings.Contains(out, "Slatewave for bat") || strings.Contains(out, "Slatewave for btop") {
		t.Errorf("themes without instructions shouldn't appear in output: %q", out)
	}
	if !strings.Contains(out, "Slatewave for Obsidian") {
		t.Errorf("theme with instructions should appear: %q", out)
	}
}

// ----- shell completion helpers (cmd/completion.go) -----

// validInstallArgs only completes when args is empty — once the user
// has typed a slug, cobra shouldn't suggest a second positional.
func TestValidInstallArgs_StopsAfterFirstArg(t *testing.T) {
	got, dir := validInstallArgs(nil, []string{"bat"}, "")
	if got != nil {
		t.Errorf("validInstallArgs after-first-arg = %v, want nil", got)
	}
	if dir != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", dir)
	}
}

func TestValidInstallArgs_FiltersByPrefix(t *testing.T) {
	got, dir := validInstallArgs(nil, nil, "ba")
	if dir != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", dir)
	}
	// "bat" is in the embedded set; it must show up. Anything not
	// starting with "ba" must not.
	var foundBat bool
	for _, slug := range got {
		if !strings.HasPrefix(slug, "ba") {
			t.Errorf("slug %q leaked through prefix filter", slug)
		}
		if slug == "bat" {
			foundBat = true
		}
	}
	if !foundBat {
		t.Errorf("expected `bat` in completions, got %v", got)
	}
}

func TestValidInstallArgs_EmptyPrefixReturnsAll(t *testing.T) {
	got, _ := validInstallArgs(nil, nil, "")
	if len(got) < 10 {
		t.Errorf("empty-prefix completion returned %d slugs, expected the full embedded set", len(got))
	}
}

// validInstalledArgs completes from state, not the manifest set, so the
// test populates state and asserts the filter only surfaces installed
// slugs.
func TestValidInstalledArgs_OnlyInstalledSlugs(t *testing.T) {
	env := setupCmdEnv(t)
	env.putRecord(t, state.Record{Slug: "bat", InstallType: "manual"})
	env.putRecord(t, state.Record{Slug: "btop", InstallType: "manual"})

	got, dir := validInstalledArgs(nil, nil, "b")
	if dir != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", dir)
	}
	want := map[string]bool{"bat": true, "btop": true}
	if len(got) != 2 {
		t.Fatalf("got %d slugs, want 2 (bat, btop): %v", len(got), got)
	}
	for _, slug := range got {
		if !want[slug] {
			t.Errorf("unexpected slug in installed completions: %q", slug)
		}
	}
}

func TestValidInstalledArgs_StopsAfterFirstArg(t *testing.T) {
	setupCmdEnv(t)
	got, _ := validInstalledArgs(nil, []string{"bat"}, "")
	if got != nil {
		t.Errorf("validInstalledArgs after-first-arg = %v, want nil", got)
	}
}

// validCategories deduplicates — the embedded set has multiple themes
// per category but the completion must surface each category once.
func TestValidCategories_DedupesAndFilters(t *testing.T) {
	got, dir := validCategories(nil, nil, "")
	if dir != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", dir)
	}
	seen := map[string]int{}
	for _, c := range got {
		seen[c]++
	}
	for c, n := range seen {
		if n > 1 {
			t.Errorf("category %q duplicated %d times in completions", c, n)
		}
	}
	// Embedded set ships at least these — a regression here means
	// validCategories started filtering / sorting differently.
	for _, want := range []string{"editor", "terminal"} {
		if seen[want] == 0 {
			t.Errorf("expected category %q in completions, got %v", want, got)
		}
	}
}

func TestValidCategories_PrefixFilter(t *testing.T) {
	got, _ := validCategories(nil, nil, "term")
	for _, c := range got {
		if !strings.HasPrefix(c, "term") {
			t.Errorf("category %q leaked through `term` prefix filter", c)
		}
	}
	if len(got) == 0 {
		t.Error("expected at least `terminal` for prefix `term`")
	}
}

// ----- fallback (cmd/status.go) -----

func TestFallback(t *testing.T) {
	if got := fallback("present", "default"); got != "present" {
		t.Errorf("fallback non-empty = %q, want present", got)
	}
	if got := fallback("", "default"); got != "default" {
		t.Errorf("fallback empty = %q, want default", got)
	}
}
