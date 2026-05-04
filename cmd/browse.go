package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/tui"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

var browseCmd = &cobra.Command{
	Use:   "browse",
	Short: "Interactively browse, search, and act on Slatewave themes",
	Long: `Open a TUI list of every Slatewave theme:

  ↑/↓ or j/k    navigate
  /             filter (name, slug, or category)
  i             install the focused theme (if not already installed)
  u             uninstall the focused theme (if installed)
  q / esc       quit without acting

The browser shows install state (●/○) and a "tool not detected" hint
for themes whose underlying tool isn't on this machine. Picking install
or uninstall closes the browser and runs the action through the same
pipeline ` + "`slatewave install`" + ` / ` + "`slatewave uninstall`" + ` use.`,
	Args: cobra.NoArgs,
	RunE: runBrowse,
}

func init() {
	rootCmd.AddCommand(browseCmd)
}

func runBrowse(_ *cobra.Command, _ []string) error {
	themes, err := manifest.LoadSupported()
	if err != nil {
		return fmt.Errorf("load manifests: %w", err)
	}
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	installed := map[string]bool{}
	for _, slug := range s.AllSlugs() {
		installed[slug] = true
	}

	// Detect runs in parallel and is bounded by tui.detectTimeout, so the
	// startup latency is the slowest-tool's detect command (~hundreds of ms
	// in practice). Worth it — the "tool not detected" hint is the main
	// reason a user would skip a row.
	detected := map[string]bool{}
	for _, d := range tui.DetectAll(themes, installed) {
		detected[d.Theme.Theme.Slug] = d.Present
	}

	action, err := tui.RunBrowse(themes, installed, detected)
	if err != nil {
		return err
	}

	switch action.Kind {
	case tui.BrowseInstall:
		fmt.Fprintln(ui.W)
		return installInteractiveTUI([]string{action.Slug})
	case tui.BrowseUninstall:
		fmt.Fprintln(ui.W)
		return uninstallOne(action.Slug)
	}
	return nil
}
