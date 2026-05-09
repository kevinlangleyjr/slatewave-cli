package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/jsonout"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/shell"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// listFlags bundles list's parsed flag values.
type listFlags struct {
	Category string
	JSON     bool
}

func parseListFlags(cmd *cobra.Command) listFlags {
	f := cmd.Flags()
	return listFlags{
		Category: flagString(f, "category"),
		JSON:     flagBool(f, "json"),
	}
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List every Slatewave theme and what's installed",
	Long: `Show every theme in the Slatewave family, grouped by category, with
` + "`●`" + ` for installed and ` + "`○`" + ` for not. Footer shows the install count.

  slatewave list                       # every theme
  slatewave list --category=editor     # only one category

Before rendering, list silently re-runs each installed theme's verify
command. If a theme was uninstalled outside the CLI (e.g. ` + "`code --uninstall-extension`" + `
from VSCode's UI) the stale state record is dropped and the row renders
as not-installed. To audit drift without mutating state, use ` + "`slatewave doctor`" + `.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := ui.Writer(cmd)
		f := parseListFlags(cmd)
		// LoadSupported hides themes that don't claim the current OS.
		// On Windows the list shrinks to the four manifests that opt
		// in; on darwin/linux it's identical to LoadAll today.
		themes, err := manifest.LoadSupported()
		if err != nil {
			return fmt.Errorf("load manifests: %w", err)
		}
		// Reconcile recorded state with reality. If a theme was uninstalled
		// outside the CLI (e.g. `code --uninstall-extension` from VSCode's
		// own UI), the install record lingers until we notice via the
		// theme's verify command. Drop stale records and persist.
		// state.Update brackets the load/mutate/save under a file lock so
		// a concurrent install can't clobber our reconcile.
		var reconciled int
		_ = state.Update(func(s *state.Store) error {
			reconciled = reconcileWithReality(s)
			return nil
		})
		s, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}

		// Group by category in a stable order.
		order := []string{"editor", "terminal", "notes", "productivity", "chat"}
		groups := map[string][]manifest.Theme{}
		for _, t := range themes {
			if f.Category != "" && t.Theme.Category != f.Category {
				continue
			}
			groups[t.Theme.Category] = append(groups[t.Theme.Category], t)
		}

		// If --category was set and matched nothing, error rather than
		// printing an empty box (which would look like a render bug).
		if f.Category != "" && len(groups) == 0 {
			return fmt.Errorf("no themes in category %q (try one of: %s)", f.Category, strings.Join(order, ", "))
		}

		if f.JSON {
			return renderListJSON(themes, s, order, groups, f, out)
		}

		var rows []string
		for _, cat := range order {
			ts := groups[cat]
			if len(ts) == 0 {
				continue
			}
			for _, t := range ts {
				rows = append(rows, renderRow(t, s))
			}
			rows = append(rows, "") // blank line between categories
		}
		// Trim trailing blank
		for len(rows) > 0 && rows[len(rows)-1] == "" {
			rows = rows[:len(rows)-1]
		}

		body := strings.Join(rows, "\n")
		header := ui.Title.Render("Slatewave themes")
		// Footer counts use the filtered theme set so "N of M installed"
		// matches what the user is actually looking at.
		footerThemes := themes
		if f.Category != "" {
			footerThemes = nil
			for _, t := range themes {
				if t.Theme.Category == f.Category {
					footerThemes = append(footerThemes, t)
				}
			}
		}
		footer := summary(footerThemes, s)

		fmt.Fprintln(out, ui.Box.Render(header+"\n\n"+body+"\n\n"+footer))
		if reconciled > 0 {
			ui.MutedLn(out, fmt.Sprintf("Dropped %s from state — verify failed (theme uninstalled outside the CLI).",
				pluralize(reconciled, "stale record", "stale records")))
		}
		return nil
	},
}

// pluralize returns "<n> singular" for n=1 and "<n> plural" otherwise.
// Used to keep the reconcile footer human-readable for both the
// one-record case ("1 stale record") and the many-records case
// ("3 stale records").
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

func init() {
	listCmd.Flags().String("category", "", "Only show themes in this category (editor / terminal / notes / productivity / chat)")
	listCmd.Flags().Bool("json", false, "Emit machine-readable JSON to stdout (see internal/jsonout for the schema)")
	_ = listCmd.RegisterFlagCompletionFunc("category", validCategories)
}

// renderListJSON emits the same theme set / counts as the human-readable
// list, just in machine-readable shape. order + groups are passed in so
// JSON output preserves the same category-based stable ordering as the
// rendered version (callers shouldn't see a different theme order
// between --json and not).
func renderListJSON(themes []manifest.Theme, s *state.Store, order []string, groups map[string][]manifest.Theme, f listFlags, out io.Writer) error {
	doc := jsonout.ListOutput{Themes: make([]jsonout.ThemeRow, 0)}
	for _, cat := range order {
		for _, t := range groups[cat] {
			row := jsonout.ThemeRow{
				Slug:     t.Theme.Slug,
				Name:     t.Theme.Name,
				Category: t.Theme.Category,
			}
			if rec, ok := s.Get(t.Theme.Slug); ok {
				installed := rec.InstalledAt
				row.Installed = true
				row.InstalledAt = &installed
				row.InstallType = rec.InstallType
				row.ActivateType = rec.ActivateType
			}
			doc.Themes = append(doc.Themes, row)
		}
	}
	// Footer counts mirror the human path: filtered theme set if a
	// category was requested, full set otherwise.
	footerThemes := themes
	if f.Category != "" {
		footerThemes = nil
		for _, t := range themes {
			if t.Theme.Category == f.Category {
				footerThemes = append(footerThemes, t)
			}
		}
	}
	doc.Counts.Total = len(footerThemes)
	for _, t := range footerThemes {
		if _, ok := s.Get(t.Theme.Slug); ok {
			doc.Counts.Installed++
		}
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func renderRow(t manifest.Theme, s *state.Store) string {
	const (
		dotInstalled    = "●"
		dotNotInstalled = "○"
	)
	rec, installed := s.Get(t.Theme.Slug)

	var marker, statusText string
	if installed {
		marker = ui.Accent.Render(dotInstalled)
		statusText = ui.Accent.Render("installed") + ui.Faint.Render(installSuffix(rec))
	} else {
		marker = ui.Faint.Render(dotNotInstalled)
		statusText = ui.Faint.Render("")
	}

	slug := lipgloss.NewStyle().Width(16).Render(t.Theme.Slug)
	cat := ui.Muted.Width(12).Render(t.Theme.Category)
	return fmt.Sprintf("  %s  %s  %s  %s", marker, slug, cat, statusText)
}

func installSuffix(rec state.Record) string {
	switch rec.ActivateType {
	case "ini-key":
		return ""
	case "shell-rc":
		if rec.AppendedLine != nil {
			return " (via " + rec.AppendedLine.File + ")"
		}
	case "gitconfig-include":
		return " (gitconfig include)"
	}
	return ""
}

// reconcileWithReality runs each recorded install's verify.command and
// drops the record from state if the install is no longer detectable.
// Returns the number of records removed.
func reconcileWithReality(s *state.Store) int {
	removed := 0
	for _, slug := range s.AllSlugs() {
		th, err := manifest.LoadOne(slug)
		if err != nil {
			// Manifest disappeared — our install record can't be reversed
			// safely from here, but it's also not real anymore. Drop it.
			s.Remove(slug)
			removed++
			continue
		}
		if !verifyInstalled(th) {
			s.Remove(slug)
			removed++
		}
	}
	return removed
}

// verifyInstalled returns true if the theme's verify command passes (or
// if there's nothing to verify against). Three cases, in order:
//
//  1. verify.trust_state = true → the manifest is opting out of checks
//     entirely (post-install location is opaque to us). Trust state.
//  2. verify.command is empty → no way to check; trust state.
//  3. otherwise run the command, return false on non-zero exit; if
//     verify.expect is set, also require the expected substring in stdout.
func verifyInstalled(th manifest.Theme) bool {
	if th.Verify.TrustState {
		return true
	}
	cmd := manifest.VerifyCommandFor(th)
	if cmd == "" {
		return true
	}
	out, err := shell.Run(context.Background(), cmd)
	if err != nil {
		return false
	}
	if th.Verify.Expect != "" && !strings.Contains(string(out), th.Verify.Expect) {
		return false
	}
	return true
}

func summary(all []manifest.Theme, s *state.Store) string {
	count := 0
	for _, t := range all {
		if _, ok := s.Get(t.Theme.Slug); ok {
			count++
		}
	}
	left := ui.Muted.Render(fmt.Sprintf("%d of %d installed", count, len(all)))
	right := ui.Faint.Render("slatewave install --all  · slatewave install <theme>")
	return left + "    " + right
}
