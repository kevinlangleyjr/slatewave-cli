package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/activator"
	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/tui"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// emitInteractiveDeprecationWarning writes a one-line stderr nudge
// when --interactive is set on install / update. The flag still works
// today — the dashboard is now the default for bulk runs on a TTY, so
// --interactive's only remaining effect is forcing the dashboard for
// the single-theme case. Visible deprecation now, removal in a future
// major release.
//
// Always stderr (never the cmd writer or stdout) — same contract as
// the upgrade nag (see cmd/root.go's emitUpgradeNag): a `slatewave
// install --json --interactive` pipeline mustn't see the warning in
// stdout. Tested in install_test.go's deprecation test.
func emitInteractiveDeprecationWarning() {
	fmt.Fprintln(os.Stderr, ui.Muted.Render("➜ --interactive is deprecated and will be removed in a future release; use --no-interactive to opt out of the dashboard (it's now the default for bulk runs on a TTY)"))
}

// installFlags bundles install's parsed flag values. Constructed once
// per RunE invocation from cmd.Flags() and threaded through every
// helper, so flag values are scoped to the call instead of living in
// package-level vars across the package.
type installFlags struct {
	DryRun        bool
	All           bool
	Category      string
	Interactive   bool
	NoInteractive bool
}

func parseInstallFlags(cmd *cobra.Command) installFlags {
	f := cmd.Flags()
	return installFlags{
		DryRun:        flagBool(f, "dry-run"),
		All:           flagBool(f, "all"),
		Category:      flagString(f, "category"),
		Interactive:   flagBool(f, "interactive"),
		NoInteractive: flagBool(f, "no-interactive"),
	}
}

var installCmd = &cobra.Command{
	Use:   "install [theme]",
	Short: "Install one Slatewave theme, an entire category, or every shipping theme",
	Long: `Install a Slatewave theme.

  slatewave install bat                  # one theme
  slatewave install --category=editor    # every theme in a category
  slatewave install --all                # every shipping theme

In bulk mode, themes already recorded in state are skipped (so re-running
` + "`--all`" + ` after adding a few new themes only installs the new ones).
Individual failures don't bail the rest — bulk install reports a summary
at the end.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: validInstallArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := ui.Writer(cmd)
		f := parseInstallFlags(cmd)
		if f.All && f.Category != "" {
			return fmt.Errorf("--all and --category are mutually exclusive")
		}
		if f.Interactive && f.NoInteractive {
			return fmt.Errorf("--interactive and --no-interactive are mutually exclusive")
		}
		bulk := f.All || f.Category != ""
		if bulk && len(args) > 0 {
			return fmt.Errorf("don't pass a theme name with --all or --category")
		}
		if !bulk && len(args) == 0 {
			return fmt.Errorf("specify a theme name, --all, or --category=<name>")
		}

		slugs, err := resolveSlugs(args, bulk, f)
		if err != nil {
			return err
		}

		// Dispatch:
		//   --interactive          → TUI (handles a TUI-wrapped runner the
		//                             stdout-stat check might miss).
		//   --no-interactive       → streaming (forces summary mode even
		//                             on a TTY; also the right default for
		//                             CI).
		//   bulk + TTY (default)   → TUI dashboard.
		//   bulk + no TTY          → streaming summary.
		//   single (no bulk flag)  → streaming, single-theme pipeline.
		ctx := cmd.Context()
		if f.Interactive {
			emitInteractiveDeprecationWarning()
		}
		switch {
		case f.Interactive:
			return installInteractiveTUI(ctx, slugs, f, out)
		case f.NoInteractive:
			if !bulk {
				return installOne(ctx, slugs[0], false, f, out)
			}
			return installBulk(ctx, slugs, f, out)
		case !bulk:
			return installOne(ctx, slugs[0], false, f, out)
		case isTerminal():
			return installInteractiveTUI(ctx, slugs, f, out)
		default:
			return installBulk(ctx, slugs, f, out)
		}
	},
}

func init() {
	installCmd.Flags().Bool("dry-run", false, "Print what would happen without writing files")
	installCmd.Flags().Bool("all", false, "Install every shipping theme")
	installCmd.Flags().String("category", "", "Install every theme in this category (editor / terminal / notes / productivity / chat)")
	// --interactive used to be required to opt into the TUI dashboard.
	// Since the dashboard became the default for bulk + TTY runs in
	// v0.0.10, the flag's only remaining effect is to force the
	// dashboard for the single-theme case. Marking it (deprecated) in
	// help and printing a one-line nudge to stderr when used — see the
	// emitInteractiveDeprecationWarning helper invoked from RunE.
	//
	// Cobra's pflag.MarkDeprecated would be the idiomatic path but
	// cobra reroutes pflag's output to an internal flagErrorBuf that
	// only flushes on parse errors, so the deprecation print never
	// reaches stderr in practice. DIY-ing the warning in RunE works
	// reliably across cobra versions and lets us keep the flag visible
	// in --help (with a (deprecated) marker) instead of hidden, which
	// is friendlier for users wondering why their existing wrappers
	// still work.
	installCmd.Flags().Bool("interactive", false, "(deprecated) Force the live progress dashboard — now the default for bulk installs on a TTY")
	installCmd.Flags().Bool("no-interactive", false, "Force streaming output instead of the dashboard (useful for CI / log capture)")
	_ = installCmd.RegisterFlagCompletionFunc("category", validCategories)
}

// resolveSlugs returns the slugs to install. For single-theme mode it's
// just args[0]; for bulk it filters by category if set.
func resolveSlugs(args []string, bulk bool, f installFlags) ([]string, error) {
	if !bulk {
		return []string{args[0]}, nil
	}
	// LoadSupported drops themes that don't claim the current OS so
	// --all / --category bulk runs never surface a theme the user was
	// never offered in `list` or `browse`.
	all, err := manifest.LoadSupported()
	if err != nil {
		return nil, fmt.Errorf("load manifests: %w", err)
	}
	var out []string
	for _, t := range all {
		if f.Category != "" && t.Theme.Category != f.Category {
			continue
		}
		out = append(out, t.Theme.Slug)
	}
	if len(out) == 0 {
		if f.Category != "" {
			return nil, fmt.Errorf("no themes in category %q", f.Category)
		}
		return nil, fmt.Errorf("no themes available")
	}
	return out, nil
}

// noManifestError builds the standard "no manifest for X" error, appending a "did you mean Y?" hint when manifest.SuggestSlug finds a close match. Shared across install / installInteractiveTUI so the suggestion behavior stays consistent.
func noManifestError(slug string) error {
	if hint := manifest.SuggestSlug(slug); hint != "" {
		return fmt.Errorf("no manifest for %q — did you mean %q? (run `slatewave list` to see all)", slug, hint)
	}
	return fmt.Errorf("no manifest for %q (run `slatewave list` to see available themes)", slug)
}

// installInteractiveTUI runs the install pipeline through the bubbletea dashboard. It loads each slug's manifest, drops themes already recorded in state (matching installBulk's skip behavior), and hands the rest to tui.RunInstall — which renders live progress and surfaces failures in the summary line rather than per-theme errors.
//
// ctx flows into tui.RunInstall so a Ctrl-C inside the dashboard cancels the in-flight install subprocess (git clone, post-hook) instead of orphaning it. Currently a no-op until tui.RunInstall wires the cancel — the TUI layer is the next commit.
func installInteractiveTUI(ctx context.Context, slugs []string, f installFlags, out io.Writer) error {
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var themes []manifest.Theme
	var skipped []string
	for _, slug := range slugs {
		if _, ok := s.Get(slug); ok {
			skipped = append(skipped, slug)
			continue
		}
		t, err := manifest.LoadOne(slug)
		if err != nil {
			return noManifestError(slug)
		}
		if !manifest.SupportsCurrentOS(t) {
			return fmt.Errorf("%s is not supported on %s", t.Theme.Name, manifest.CurrentGOOS())
		}
		themes = append(themes, t)
	}

	for _, slug := range skipped {
		ui.MutedLn(out, fmt.Sprintf("Skipping %s — already installed.", slug))
	}
	if len(skipped) > 0 {
		fmt.Fprintln(out)
	}

	if len(themes) == 0 {
		ui.Done(out, "Nothing to install — every requested theme is already installed.")
		return nil
	}

	runErr := tui.RunInstall(ctx, themes, tui.InstallOptions{DryRun: f.DryRun})
	// Print post-install instructions for every theme we tried, after the
	// dashboard exits. Static-mode installOne prints these inline; TUI mode
	// can't (multi-line guidance won't fit in a one-row dashboard) so we
	// surface them as a "Next steps:" block once the live render is done.
	// We print even when runErr != nil — the dashboard already shows which
	// themes failed; instructions for failed ones are ignorable noise but
	// instructions for the successes still need to land.
	printPostInstallInstructions(themes, out)
	return runErr
}

// printPostInstallInstructions emits each theme's install.instructions block under a "Next steps for <name>:" header. No-op for themes that ship no instructions, so the function is safe to call against a mixed list.
func printPostInstallInstructions(themes []manifest.Theme, out io.Writer) {
	any := false
	for _, t := range themes {
		if len(t.Install.Instructions) == 0 {
			continue
		}
		if !any {
			fmt.Fprintln(out)
			any = true
		}
		ui.MutedLn(out, fmt.Sprintf("Next steps for %s:", t.Theme.Name))
		for _, line := range t.Install.Instructions {
			ui.MutedLn(out, "  "+line)
		}
		fmt.Fprintln(out)
	}
}

// installBulk runs installOne for each slug, skipping themes already
// recorded in state and continuing past individual failures so one
// broken theme doesn't bail the rest of the run.
func installBulk(ctx context.Context, slugs []string, f installFlags, out io.Writer) error {
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var installed, skipped, failed int
	for i, slug := range slugs {
		if _, ok := s.Get(slug); ok {
			ui.MutedLn(out, fmt.Sprintf("Skipping %s — already installed.", slug))
			skipped++
			continue
		}
		if i > 0 {
			fmt.Fprintln(out)
		}
		if err := installOne(ctx, slug, true, f, out); err != nil {
			ui.Errorf(out, "%s: %v", slug, err)
			failed++
			continue
		}
		installed++
	}

	fmt.Fprintln(out)
	switch {
	case failed > 0:
		ui.Done(out, fmt.Sprintf("%d installed, %d skipped, %d failed.", installed, skipped, failed))
	case installed == 0:
		ui.Done(out, fmt.Sprintf("Nothing to install — %d already installed.", skipped))
	default:
		ui.Done(out, fmt.Sprintf("%d installed, %d skipped.", installed, skipped))
	}
	return nil
}

// installOne is the per-theme install pipeline: detect → install →
// post-hook → activate → persist → instructions. Returns an error
// rather than calling os.Exit so bulk mode can keep going past
// individual failures.
//
// suppressFinal — when true (bulk mode), skip the per-theme final
// "Done." line so the bulk caller can render its own summary.
//
// ctx threads into installer.Install so a SIGINT (Ctrl-C from the
// streaming CLI) kills the in-flight git clone / post-hook instead of
// orphaning it.
func installOne(ctx context.Context, slug string, suppressFinal bool, f installFlags, out io.Writer) error {
	t, err := manifest.LoadOne(slug)
	if err != nil {
		return noManifestError(slug)
	}
	if !manifest.SupportsCurrentOS(t) {
		return fmt.Errorf("%s is not supported on %s", t.Theme.Name, manifest.CurrentGOOS())
	}

	ui.Header(out, "Installing", t.Theme.Name)
	opts := installer.Options{DryRun: f.DryRun}
	actOpts := activator.Options{DryRun: f.DryRun}

	if t.Install.Type != "marketplace" && t.Install.Type != "manual" {
		done := ui.StepStart(out, fmt.Sprintf("Detecting %s", t.Theme.Slug))
		if err := installer.Detect(t); err != nil {
			done(err)
			return err
		}
		done(nil)
	}

	done := ui.StepStart(out, installLabel(t))
	rec, err := installer.Install(ctx, t, opts)
	if err != nil {
		done(err)
		return err
	}
	done(nil)

	if t.Install.Post != nil {
		done := ui.StepStart(out, t.Install.Post.Description)
		done(nil)
	}

	if t.Activate.Type != "" && t.Activate.Type != "none" {
		done := ui.StepStart(out, activateLabel(t))
		if err := activator.Activate(t, &rec, actOpts); err != nil {
			done(err)
			return err
		}
		done(nil)
	}

	if !f.DryRun {
		if err := state.Update(func(s *state.Store) error {
			s.Put(rec)
			return nil
		}); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	if t.Install.Type == "manual" || len(t.Install.Instructions) > 0 {
		fmt.Fprintln(out)
		for _, line := range t.Install.Instructions {
			ui.MutedLn(out, "  "+line)
		}
	}

	if suppressFinal {
		return nil
	}
	if f.DryRun {
		ui.Done(out, "Dry run — no files written.")
	} else {
		ui.Done(out, installDoneMessage(t))
	}
	return nil
}

// installDoneMessage returns the success line for `slatewave install <slug>`.
// Honors the manifest's optional install.done_message; falls back to the
// generic "Slatewave is installed." for themes that don't need
// tool-specific guidance.
func installDoneMessage(t manifest.Theme) string {
	if t.Install.DoneMessage != "" {
		return t.Install.DoneMessage
	}
	return "Slatewave is installed."
}

// installLabel produces the step label per install type.
func installLabel(t manifest.Theme) string {
	switch t.Install.Type {
	case "curl":
		return "Fetching theme file"
	case "clone":
		return "Cloning " + t.Install.Repo
	case "vscode-ext":
		return fmt.Sprintf("Installing %s extension %s", installer.VSCodeExtCLI(t), t.Install.Identifier)
	case "marketplace":
		return "Opening Marketplace in your browser"
	case "gui-import":
		return "Fetching theme file (GUI import follows)"
	case "manual":
		return "Manual install — see instructions below"
	default:
		return "Running install step"
	}
}

func activateLabel(t manifest.Theme) string {
	switch t.Activate.Type {
	case "ini-key":
		return fmt.Sprintf("Setting %s = %s in %s", t.Activate.Key, t.Activate.Value, t.Activate.File)
	case "gitconfig-include":
		return "Adding include.path to ~/.gitconfig"
	case "shell-rc":
		if len(t.Activate.Files) > 0 {
			return "Appending to " + t.Activate.Files[0]
		}
		return "Appending activation line"
	case "toml-import":
		return "Importing into " + t.Activate.TOMLPath
	case "yaml-set":
		return "Setting keys in " + t.Activate.YAMLPath
	default:
		return "Activating"
	}
}
