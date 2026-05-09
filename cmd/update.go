package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/shell"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/tui"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// updateFlags bundles update's parsed flag values.
type updateFlags struct {
	DryRun        bool
	All           bool
	Category      string
	Interactive   bool
	NoInteractive bool
}

func parseUpdateFlags(cmd *cobra.Command) updateFlags {
	f := cmd.Flags()
	return updateFlags{
		DryRun:        flagBool(f, "dry-run"),
		All:           flagBool(f, "all"),
		Category:      flagString(f, "category"),
		Interactive:   flagBool(f, "interactive"),
		NoInteractive: flagBool(f, "no-interactive"),
	}
}

var updateCmd = &cobra.Command{
	Use:   "update [theme]",
	Short: "Update one Slatewave theme, an entire category, or every installed theme",
	Long: `Re-fetch a theme's assets without re-running activation.

  slatewave update bat                   # one theme
  slatewave update --category=editor     # every installed theme in a category
  slatewave update --all                 # every installed theme

Update only refreshes assets — the theme stays activated, your config
stays edited. The state record's install timestamp is bumped so ` + "`slatewave status`" + `
shows when each theme was last refreshed.

Themes that aren't installed are skipped silently. Themes whose install
type has no automated update path (` + "`marketplace`" + `, ` + "`manual`" + `) are reported
with a one-line hint and continue.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: validInstalledArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := parseUpdateFlags(cmd)
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

		// Same dispatch pattern as install: bulk on a TTY defaults to
		// the dashboard; piped / CI runs fall back to the streaming
		// summary; --interactive / --no-interactive override either way.
		switch {
		case f.Interactive:
			return updateInteractiveTUI(args, bulk, f)
		case f.NoInteractive:
			if !bulk {
				return updateOne(args[0], false, f)
			}
			return updateBulk(f)
		case !bulk:
			return updateOne(args[0], false, f)
		case isTerminal():
			return updateInteractiveTUI(args, bulk, f)
		default:
			return updateBulk(f)
		}
	},
}

func init() {
	updateCmd.Flags().Bool("dry-run", false, "Print what would happen without re-fetching")
	updateCmd.Flags().Bool("all", false, "Update every installed theme")
	updateCmd.Flags().String("category", "", "Update every installed theme in this category")
	updateCmd.Flags().Bool("interactive", false, "Force the live progress dashboard (default for bulk updates on a TTY)")
	updateCmd.Flags().Bool("no-interactive", false, "Force streaming output instead of the dashboard (useful for CI / log capture)")
	_ = updateCmd.RegisterFlagCompletionFunc("category", validCategories)
}

// updateInteractiveTUI runs the update pipeline through the bubbletea dashboard. Loads each requested slug's manifest, drops marketplace + manual themes (no automated update path — would just clutter the dashboard with "failed: no automated update" rows), and hands the rest to tui.RunFix with FixUpdate. Reuses the fix dashboard since the pipeline is identical (refresh assets + post-hook + bump InstalledAt).
func updateInteractiveTUI(args []string, bulk bool, f updateFlags) error {
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var slugs []string
	switch {
	case bulk:
		for _, slug := range s.AllSlugs() {
			if f.Category != "" {
				th, err := manifest.LoadOne(slug)
				if err != nil || th.Theme.Category != f.Category {
					continue
				}
			}
			slugs = append(slugs, slug)
		}
	default:
		if _, ok := s.Get(args[0]); !ok {
			return fmt.Errorf("%s is not installed (run `slatewave install %s` first)", args[0], args[0])
		}
		slugs = []string{args[0]}
	}

	var fixes []tui.Fix
	var skipped []string
	for _, slug := range slugs {
		t, err := manifest.LoadOne(slug)
		if err != nil {
			ui.Errorf("%s: no manifest — skipping", slug)
			continue
		}
		if !manifest.SupportsCurrentOS(t) {
			if !bulk {
				return fmt.Errorf("%s is not supported on %s", t.Theme.Name, manifest.CurrentGOOS())
			}
			continue
		}
		if t.Install.Type == "marketplace" || t.Install.Type == "manual" {
			skipped = append(skipped, slug)
			continue
		}
		fixes = append(fixes, tui.Fix{
			Slug:  slug,
			Name:  t.Theme.Name,
			Kind:  tui.FixUpdate,
			Theme: t,
		})
	}

	for _, slug := range skipped {
		ui.MutedLn(fmt.Sprintf("Skipping %s — install type has no automated update.", slug))
	}
	if len(skipped) > 0 {
		fmt.Fprintln(ui.W)
	}

	if len(fixes) == 0 {
		ui.Done("Nothing to update.")
		return nil
	}

	return tui.RunFix(fixes, tui.FixOptions{DryRun: f.DryRun, Title: "Updating"})
}

// updateBulk iterates every installed slug, optionally filtered by
// category, calling updateOne for each. Individual failures are reported
// and the run continues.
func updateBulk(f updateFlags) error {
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var slugs []string
	for _, slug := range s.AllSlugs() {
		th, err := manifest.LoadOne(slug)
		if err != nil {
			// State references a slug we no longer ship a manifest for —
			// leave it for `slatewave doctor` to surface; bulk update just
			// skips it silently.
			continue
		}
		if f.Category != "" && th.Theme.Category != f.Category {
			continue
		}
		// State.json may have been seeded on a different OS (dual boot,
		// machine migration). Silently skip themes whose manifest no
		// longer claims this OS — updateOne would error out per-theme
		// anyway, and a bulk run shouldn't surface that as a failure.
		if !manifest.SupportsCurrentOS(th) {
			continue
		}
		slugs = append(slugs, slug)
	}
	if len(slugs) == 0 {
		if f.Category != "" {
			return fmt.Errorf("no installed themes in category %q", f.Category)
		}
		ui.MutedLn("Nothing to update — no themes installed.")
		return nil
	}

	var updated, skipped, failed int
	for i, slug := range slugs {
		if i > 0 {
			fmt.Fprintln(ui.W)
		}
		err := updateOne(slug, true, f)
		switch {
		case errors.Is(err, installer.ErrNoAutomatedUpdate):
			skipped++
		case err != nil:
			ui.Errorf("%s: %v", slug, err)
			failed++
		default:
			updated++
		}
	}

	fmt.Fprintln(ui.W)
	switch {
	case failed > 0:
		ui.Done(fmt.Sprintf("%d updated, %d skipped, %d failed.", updated, skipped, failed))
	default:
		ui.Done(fmt.Sprintf("%d updated, %d skipped.", updated, skipped))
	}
	return nil
}

// updateOne re-fetches assets for one installed theme.
//
// suppressFinal — when true (bulk mode), skip the per-theme final
// "Up to date." line so the bulk caller can render its own summary.
func updateOne(slug string, suppressFinal bool, f updateFlags) error {
	t, err := manifest.LoadOne(slug)
	if err != nil {
		return fmt.Errorf("no manifest for %q", slug)
	}
	if !manifest.SupportsCurrentOS(t) {
		return fmt.Errorf("%s is not supported on %s", t.Theme.Name, manifest.CurrentGOOS())
	}

	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	rec, ok := s.Get(slug)
	if !ok {
		return fmt.Errorf("%s is not installed (run `slatewave install %s` first)", slug, slug)
	}

	ui.Header("Updating", t.Theme.Name)

	opts := installer.Options{DryRun: f.DryRun}
	done := ui.StepStart(updateLabel(t))
	if err := installer.Update(t, opts); err != nil {
		if errors.Is(err, installer.ErrNoAutomatedUpdate) {
			done(nil)
			ui.MutedLn(fmt.Sprintf("  No automated update for install type %q — check getslatewave.com/themes/%s for the latest steps.", t.Install.Type, slug))
			return err
		}
		done(err)
		return err
	}
	done(nil)

	// Re-run the post-hook so derived caches (e.g., `bat cache --build`)
	// reflect the refreshed asset.
	if t.Install.Post != nil {
		done := ui.StepStart(t.Install.Post.Description)
		if !f.DryRun {
			if err := shell.RunInherit(context.Background(), t.Install.Post.Command); err != nil {
				done(err)
				return fmt.Errorf("post-hook: %w", err)
			}
		}
		done(nil)
	}

	// Bump the install timestamp so `slatewave status` reflects the
	// last refresh.
	if !f.DryRun {
		if err := state.Update(func(s *state.Store) error {
			rec.InstalledAt = time.Now().UTC()
			s.Put(rec)
			return nil
		}); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	if suppressFinal {
		return nil
	}
	if f.DryRun {
		ui.Done("Dry run — no files written.")
	} else {
		ui.Done("Up to date.")
	}
	return nil
}

func updateLabel(t manifest.Theme) string {
	switch t.Install.Type {
	case "curl", "gui-import":
		return "Re-fetching theme file"
	case "clone":
		return "git pull --ff-only on " + t.Install.CloneDest
	case "vscode-ext":
		return fmt.Sprintf("Reinstalling %s extension %s", installer.VSCodeExtCLI(t), t.Install.Identifier)
	case "marketplace":
		return "Marketplace install — manual update"
	case "manual":
		return "Manual install — manual update"
	default:
		return "Updating"
	}
}
