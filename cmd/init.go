package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/tui"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive setup wizard — pick themes for the tools you have",
	Long: `Slatewave's first-run wizard:

  1. Detect every Slatewave-supported tool present on this machine
     (parallel sh probes — typically completes in well under a second).
  2. Skip tools that aren't installed and themes you already have.
  3. Show a multi-select of what's left, grouped by category.
  4. Install the picks via the same pipeline as ` + "`slatewave install`" + `.

Re-run anytime — already-installed themes are filtered out automatically
so re-running after adding new tools just installs Slatewave for the new
ones.`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(_ *cobra.Command, _ []string) error {
	ui.PrintBanner()

	themes, err := manifest.LoadSupported()
	if err != nil {
		return fmt.Errorf("load manifests: %w", err)
	}
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	installedSet := map[string]bool{}
	for _, slug := range s.AllSlugs() {
		installedSet[slug] = true
	}

	ui.MutedLn("Detecting tools on your machine…")
	detected := tui.DetectAll(themes, installedSet)

	available, alreadyInstalled, missingTool := summarize(detected)
	ui.MutedLn(fmt.Sprintf("  %d available · %d already installed · %d tool not detected",
		available, alreadyInstalled, missingTool))
	fmt.Fprintln(ui.W)

	if available == 0 {
		ui.Done("Nothing to install — every detected tool already has Slatewave applied.")
		return nil
	}

	slugs, err := tui.PickThemes(detected)
	if err != nil {
		if errors.Is(err, tui.ErrAborted) {
			ui.MutedLn("Aborted. Nothing installed.")
			return nil
		}
		return err
	}
	if len(slugs) == 0 {
		ui.MutedLn("No themes selected. Nothing installed.")
		return nil
	}

	fmt.Fprintln(ui.W)
	if err := installInteractiveTUI(slugs); err != nil {
		return err
	}

	ui.Done(fmt.Sprintf("%d theme(s) processed. Welcome to Slatewave.", len(slugs)))
	return nil
}

func summarize(detected []tui.DetectResult) (available, alreadyInstalled, missingTool int) {
	for _, d := range detected {
		switch {
		case d.Installed:
			alreadyInstalled++
		case d.Present:
			available++
		default:
			missingTool++
		}
	}
	return
}
