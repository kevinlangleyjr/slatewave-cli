package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// uninstallFlags bundles uninstall's parsed flag values. browse.go's
// uninstall path passes a zero-value here since flags are CLI-only;
// the browser doesn't surface --dry-run / --category as TUI options.
type uninstallFlags struct {
	DryRun   bool
	All      bool
	Category string
}

func parseUninstallFlags(cmd *cobra.Command) uninstallFlags {
	f := cmd.Flags()
	return uninstallFlags{
		DryRun:   flagBool(f, "dry-run"),
		All:      flagBool(f, "all"),
		Category: flagString(f, "category"),
	}
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall [theme]",
	Short: "Remove one Slatewave theme, an entire category, or every installed theme",
	Long: `Remove a Slatewave theme and revert config edits.

  slatewave uninstall bat                 # one theme
  slatewave uninstall --category=editor   # every installed theme in a category
  slatewave uninstall --all               # every installed theme

Bulk uninstall walks installed themes (filtered by category if set) and
runs the same reversal pipeline ` + "`slatewave uninstall <slug>`" + ` uses for each.
Individual failures don't bail the rest — bulk uninstall reports a summary
at the end. Themes whose manifest has disappeared since install can't be
reversed safely; they're skipped with a warning so the run keeps going.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: validInstalledArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := parseUninstallFlags(cmd)
		if f.All && f.Category != "" {
			return fmt.Errorf("--all and --category are mutually exclusive")
		}
		bulk := f.All || f.Category != ""
		if bulk && len(args) > 0 {
			return fmt.Errorf("don't pass a theme name with --all or --category")
		}
		if !bulk && len(args) == 0 {
			return fmt.Errorf("specify a theme name, --all, or --category=<name>")
		}

		if bulk {
			return uninstallBulk(f)
		}
		return uninstallOne(args[0], f)
	},
}

// uninstallOne is the shared uninstall pipeline — used by `slatewave uninstall <slug>` and by `slatewave browse` when the user picks the uninstall action. Honors f.DryRun.
func uninstallOne(slug string, f uninstallFlags) error {
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	rec, ok := s.Get(slug)
	if !ok {
		return fmt.Errorf("%s is not installed (run `slatewave list` to see what is)", slug)
	}

	t, err := manifest.LoadOne(slug)
	if err != nil {
		return fmt.Errorf("no manifest for %q — cannot uninstall safely", slug)
	}

	ui.Header("Uninstalling", t.Theme.Name)

	opts := installer.Options{DryRun: f.DryRun}
	done := ui.StepStart("Reversing install footprint")
	if err := installer.Uninstall(rec, t, opts); err != nil {
		done(err)
		return err
	}
	done(nil)

	if !f.DryRun {
		if err := state.Update(func(s *state.Store) error {
			s.Remove(slug)
			return nil
		}); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	if f.DryRun {
		ui.Done("Dry run — nothing reverted.")
	} else {
		ui.Done(uninstallDoneMessage(t))
	}
	return nil
}

// uninstallDoneMessage returns the success line for `slatewave uninstall <slug>`.
// Honors the manifest's optional uninstall.done_message; falls back to the
// generic "Reverted." for themes that re-read config per-invocation (bat,
// delta, git diff — the next run picks up the empty state automatically).
func uninstallDoneMessage(t manifest.Theme) string {
	if t.Uninstall.DoneMessage != "" {
		return t.Uninstall.DoneMessage
	}
	return "Reverted."
}

// uninstallBulk iterates installed slugs (filtered by category if set), running uninstallOne for each. Mirrors updateBulk's shape — individual failures are reported and the run continues so one broken reversal doesn't strand the rest installed.
func uninstallBulk(f uninstallFlags) error {
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var slugs []string
	for _, slug := range s.AllSlugs() {
		if f.Category != "" {
			th, err := manifest.LoadOne(slug)
			if err != nil || th.Theme.Category != f.Category {
				continue
			}
		}
		slugs = append(slugs, slug)
	}
	if len(slugs) == 0 {
		if f.Category != "" {
			return fmt.Errorf("no installed themes in category %q", f.Category)
		}
		ui.MutedLn("Nothing to uninstall — no themes installed.")
		return nil
	}

	var removed, failed int
	for i, slug := range slugs {
		if i > 0 {
			fmt.Fprintln(ui.W)
		}
		if err := uninstallOne(slug, f); err != nil {
			ui.Errorf("%s: %v", slug, err)
			failed++
			continue
		}
		removed++
	}

	fmt.Fprintln(ui.W)
	switch {
	case failed > 0:
		ui.Done(fmt.Sprintf("%d uninstalled, %d failed.", removed, failed))
	default:
		ui.Done(fmt.Sprintf("%d uninstalled.", removed))
	}
	return nil
}

func init() {
	uninstallCmd.Flags().Bool("dry-run", false, "Print what would happen without reverting")
	uninstallCmd.Flags().Bool("all", false, "Uninstall every installed theme")
	uninstallCmd.Flags().String("category", "", "Uninstall every installed theme in this category")
	_ = uninstallCmd.RegisterFlagCompletionFunc("category", validCategories)
}
