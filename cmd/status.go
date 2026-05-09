package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/jsonout"
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
  slatewave status --json       # machine-readable output

Per theme: install timestamp, install + activate types, every file the
CLI created, every config line the CLI appended, every backup the CLI
made before editing. This is what ` + "`slatewave uninstall`" + ` would reverse.

Read-only — status never touches state, install, or uninstall.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: validInstalledArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}
		if flagBool(cmd.Flags(), "json") {
			return renderStatusJSON(s, args)
		}
		if len(args) == 0 {
			return statusAll(s)
		}
		statusOne(s, args[0])
		return nil
	},
}

func init() {
	statusCmd.Flags().Bool("json", false, "Emit machine-readable JSON to stdout (see internal/jsonout for the schema)")
}

// renderStatusJSON emits the status footprint as JSON. With no slug,
// every installed theme is listed; with a slug, only that one (or an
// error if the slug isn't installed — same shape as the human path's
// ui.Errorf, just promoted to a real error since --json consumers need
// non-zero exit on missing slug to short-circuit their script).
func renderStatusJSON(s *state.Store, args []string) error {
	out := jsonout.StatusOutput{Themes: make([]jsonout.StatusEntry, 0)}
	switch {
	case len(args) == 0:
		for _, slug := range s.AllSlugs() {
			rec, _ := s.Get(slug)
			out.Themes = append(out.Themes, statusEntry(rec))
		}
	default:
		rec, ok := s.Get(args[0])
		if !ok {
			return fmt.Errorf("%s is not installed", args[0])
		}
		out.Themes = append(out.Themes, statusEntry(rec))
	}
	enc := json.NewEncoder(ui.W)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// statusEntry converts a state.Record into the jsonout shape, looking
// up the manifest's display name when available (state.Record stores
// only the slug; the name comes from the manifest).
func statusEntry(rec state.Record) jsonout.StatusEntry {
	name := rec.Slug
	if t, err := manifest.LoadOne(rec.Slug); err == nil {
		name = t.Theme.Name
	}
	entry := jsonout.StatusEntry{
		Slug:         rec.Slug,
		Name:         name,
		InstalledAt:  rec.InstalledAt,
		InstallType:  rec.InstallType,
		ActivateType: rec.ActivateType,
		CreatedPaths: rec.CreatedPaths,
	}
	if rec.AppendedLine != nil {
		entry.AppendedLine = &jsonout.AppendedLine{
			File: rec.AppendedLine.File,
			Line: rec.AppendedLine.Line,
		}
	}
	for _, b := range rec.Backups {
		entry.Backups = append(entry.Backups, jsonout.Backup{Original: b.Original, Path: b.Path})
	}
	return entry
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
