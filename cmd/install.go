package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/activator"
	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/tui"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

var (
	installDryRun      bool
	installAll         bool
	installCategory    string
	installInteractive bool
)

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
	RunE: func(_ *cobra.Command, args []string) error {
		if installAll && installCategory != "" {
			return fmt.Errorf("--all and --category are mutually exclusive")
		}
		bulk := installAll || installCategory != ""
		if bulk && len(args) > 0 {
			return fmt.Errorf("don't pass a theme name with --all or --category")
		}
		if !bulk && len(args) == 0 {
			return fmt.Errorf("specify a theme name, --all, or --category=<name>")
		}

		slugs, err := resolveSlugs(args, bulk)
		if err != nil {
			return err
		}

		if installInteractive {
			return installInteractiveTUI(slugs)
		}

		if !bulk {
			return installOne(slugs[0], false)
		}
		return installBulk(slugs)
	},
}

func init() {
	installCmd.Flags().BoolVar(&installDryRun, "dry-run", false, "Print what would happen without writing files")
	installCmd.Flags().BoolVar(&installAll, "all", false, "Install every shipping theme")
	installCmd.Flags().StringVar(&installCategory, "category", "", "Install every theme in this category (editor / terminal / notes / productivity / chat)")
	installCmd.Flags().BoolVar(&installInteractive, "interactive", false, "Show a live progress dashboard instead of streamed step output")
	_ = installCmd.RegisterFlagCompletionFunc("category", validCategories)
}

// resolveSlugs returns the slugs to install. For single-theme mode it's
// just args[0]; for bulk it filters by category if set.
func resolveSlugs(args []string, bulk bool) ([]string, error) {
	if !bulk {
		return []string{args[0]}, nil
	}
	all, err := manifest.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("load manifests: %w", err)
	}
	var out []string
	for _, t := range all {
		if installCategory != "" && t.Theme.Category != installCategory {
			continue
		}
		out = append(out, t.Theme.Slug)
	}
	if len(out) == 0 {
		if installCategory != "" {
			return nil, fmt.Errorf("no themes in category %q", installCategory)
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
func installInteractiveTUI(slugs []string) error {
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
		themes = append(themes, t)
	}

	for _, slug := range skipped {
		ui.MutedLn(fmt.Sprintf("Skipping %s — already installed.", slug))
	}
	if len(skipped) > 0 {
		fmt.Fprintln(ui.W)
	}

	if len(themes) == 0 {
		ui.Done("Nothing to install — every requested theme is already installed.")
		return nil
	}

	runErr := tui.RunInstall(themes, tui.InstallOptions{DryRun: installDryRun})
	// Print post-install instructions for every theme we tried, after the
	// dashboard exits. Static-mode installOne prints these inline; TUI mode
	// can't (multi-line guidance won't fit in a one-row dashboard) so we
	// surface them as a "Next steps:" block once the live render is done.
	// We print even when runErr != nil — the dashboard already shows which
	// themes failed; instructions for failed ones are ignorable noise but
	// instructions for the successes still need to land.
	printPostInstallInstructions(themes)
	return runErr
}

// printPostInstallInstructions emits each theme's install.instructions block under a "Next steps for <name>:" header. No-op for themes that ship no instructions, so the function is safe to call against a mixed list.
func printPostInstallInstructions(themes []manifest.Theme) {
	any := false
	for _, t := range themes {
		if len(t.Install.Instructions) == 0 {
			continue
		}
		if !any {
			fmt.Fprintln(ui.W)
			any = true
		}
		ui.MutedLn(fmt.Sprintf("Next steps for %s:", t.Theme.Name))
		for _, line := range t.Install.Instructions {
			ui.MutedLn("  " + line)
		}
		fmt.Fprintln(ui.W)
	}
}

// installBulk runs installOne for each slug, skipping themes already
// recorded in state and continuing past individual failures so one
// broken theme doesn't bail the rest of the run.
func installBulk(slugs []string) error {
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var installed, skipped, failed int
	for i, slug := range slugs {
		if _, ok := s.Get(slug); ok {
			ui.MutedLn(fmt.Sprintf("Skipping %s — already installed.", slug))
			skipped++
			continue
		}
		if i > 0 {
			fmt.Fprintln(ui.W)
		}
		if err := installOne(slug, true); err != nil {
			ui.Errorf("%s: %v", slug, err)
			failed++
			continue
		}
		installed++
	}

	fmt.Fprintln(ui.W)
	switch {
	case failed > 0:
		ui.Done(fmt.Sprintf("%d installed, %d skipped, %d failed.", installed, skipped, failed))
	case installed == 0:
		ui.Done(fmt.Sprintf("Nothing to install — %d already installed.", skipped))
	default:
		ui.Done(fmt.Sprintf("%d installed, %d skipped.", installed, skipped))
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
func installOne(slug string, suppressFinal bool) error {
	t, err := manifest.LoadOne(slug)
	if err != nil {
		return noManifestError(slug)
	}

	ui.Header("Installing", t.Theme.Name)
	opts := installer.Options{DryRun: installDryRun}
	actOpts := activator.Options{DryRun: installDryRun}

	if t.Install.Type != "marketplace" && t.Install.Type != "manual" {
		done := ui.StepStart(fmt.Sprintf("Detecting %s", t.Theme.Slug))
		if err := installer.Detect(t); err != nil {
			done(err)
			return err
		}
		done(nil)
	}

	done := ui.StepStart(installLabel(t))
	rec, err := installer.Install(t, opts)
	if err != nil {
		done(err)
		return err
	}
	done(nil)

	if t.Install.Post != nil {
		done := ui.StepStart(t.Install.Post.Description)
		done(nil)
	}

	if t.Activate.Type != "" && t.Activate.Type != "none" {
		done := ui.StepStart(activateLabel(t))
		if err := activator.Activate(t, &rec, actOpts); err != nil {
			done(err)
			return err
		}
		done(nil)
	}

	if !installDryRun {
		s, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}
		s.Put(rec)
		if err := s.Save(); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	if t.Install.Type == "manual" || len(t.Install.Instructions) > 0 {
		fmt.Fprintln(ui.W)
		for _, line := range t.Install.Instructions {
			ui.MutedLn("  " + line)
		}
	}

	if suppressFinal {
		return nil
	}
	if installDryRun {
		ui.Done("Dry run — no files written.")
	} else {
		ui.Done(doneMessage(t))
	}
	return nil
}

// installLabel produces the step label per install type.
func installLabel(t manifest.Theme) string {
	switch t.Install.Type {
	case "curl":
		return "Fetching theme file"
	case "clone":
		return "Cloning " + t.Install.Repo
	case "vscode-ext":
		return "Installing VSCode extension " + t.Install.Identifier
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

func doneMessage(t manifest.Theme) string {
	// Tool-specific guidance — bat picks up its config on next
	// invocation, but a prompt change needs a shell restart.
	switch t.Theme.Slug {
	case "bat":
		return "bat picks up the new theme on its next invocation."
	case "btop":
		return "Launch `btop` to see Slatewave applied."
	case "delta":
		return "Run a `git diff` in any repo to see Slatewave applied."
	case "oh-my-posh", "starship":
		return "Restart your shell or `source` your rc file."
	}
	return "Slatewave is installed."
}
