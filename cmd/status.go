package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

var statusCmd = &cobra.Command{
	Use:   "status [theme]",
	Short: "Show what's installed (and where the files live)",
	Long: `Print the install footprint for one theme or every installed theme:

  slatewave status              # every installed theme
  slatewave status bat          # one theme

Per theme: install timestamp, install + activate types, every file the
CLI created, every config line the CLI appended, every backup the CLI
made before editing. This is what ` + "`slatewave uninstall`" + ` would reverse.

Read-only — status never touches state, install, or uninstall.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: validInstalledArgs,
	RunE: func(_ *cobra.Command, args []string) error {
		s, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}
		if len(args) == 0 {
			return statusAll(s)
		}
		statusOne(s, args[0])
		return nil
	},
}

func statusAll(s *state.Store) error {
	if len(s.Records) == 0 {
		ui.MutedLn("Nothing installed yet. Run `slatewave list` to see available themes.")
		return nil
	}
	for _, slug := range s.AllSlugs() {
		statusOne(s, slug)
	}
	return nil
}

// statusOne prints the install footprint for one slug. It never errors
// (missing-slug case is reported via ui.Errorf and short-circuits) so
// the signature is void — callers don't need an error-check ceremony.
func statusOne(s *state.Store, slug string) {
	rec, ok := s.Get(slug)
	if !ok {
		ui.Errorf("%s is not installed.", slug)
		return
	}
	name := slug
	if t, err := manifest.LoadOne(slug); err == nil {
		name = t.Theme.Name
	}
	fmt.Fprintln(ui.W, ui.AccentBold.Render(name))
	fmt.Fprintln(ui.W, ui.Muted.Render(fmt.Sprintf("  installed %s", rec.InstalledAt.Local().Format("2006-01-02 15:04"))))
	fmt.Fprintln(ui.W, ui.Muted.Render(fmt.Sprintf("  install: %s   activate: %s", rec.InstallType, fallback(rec.ActivateType, "none"))))
	if len(rec.CreatedPaths) > 0 {
		fmt.Fprintln(ui.W, ui.Muted.Render("  files:"))
		for _, p := range rec.CreatedPaths {
			fmt.Fprintln(ui.W, "    "+ui.Faint.Render(p))
		}
	}
	if rec.AppendedLine != nil {
		fmt.Fprintln(ui.W, ui.Muted.Render(fmt.Sprintf("  appended to %s:", rec.AppendedLine.File)))
		fmt.Fprintln(ui.W, "    "+ui.Code.Render(rec.AppendedLine.Line))
	}
	if len(rec.Backups) > 0 {
		fmt.Fprintln(ui.W, ui.Muted.Render("  backups:"))
		for _, b := range rec.Backups {
			fmt.Fprintln(ui.W, "    "+ui.Faint.Render(b.Path))
		}
	}
	fmt.Fprintln(ui.W)
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
