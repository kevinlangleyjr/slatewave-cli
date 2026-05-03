package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

var uninstallDryRun bool

var uninstallCmd = &cobra.Command{
	Use:               "uninstall <theme>",
	Short:             "Remove a Slatewave theme and revert config edits",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: validInstalledArgs,
	RunE: func(_ *cobra.Command, args []string) error {
		return uninstallOne(args[0])
	},
}

// uninstallOne is the shared uninstall pipeline — used by `slatewave uninstall <slug>` and by `slatewave browse` when the user picks the uninstall action. Honors uninstallDryRun.
func uninstallOne(slug string) error {
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

	opts := installer.Options{DryRun: uninstallDryRun}
	done := ui.StepStart("Reversing install footprint")
	if err := installer.Uninstall(rec, t, opts); err != nil {
		done(err)
		return err
	}
	done(nil)

	if !uninstallDryRun {
		s.Remove(slug)
		if err := s.Save(); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	if uninstallDryRun {
		ui.Done("Dry run — nothing reverted.")
	} else {
		ui.Done("Reverted.")
	}
	return nil
}

func init() {
	uninstallCmd.Flags().BoolVar(&uninstallDryRun, "dry-run", false, "Print what would happen without reverting")
}
