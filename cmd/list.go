package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

var listCategory string

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
	RunE: func(_ *cobra.Command, _ []string) error {
		themes, err := manifest.LoadAll()
		if err != nil {
			return fmt.Errorf("load manifests: %w", err)
		}
		s, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}

		// Reconcile recorded state with reality. If a theme was uninstalled
		// outside the CLI (e.g. `code --uninstall-extension` from VSCode's
		// own UI), the install record lingers until we notice via the
		// theme's verify command. Drop stale records and persist.
		if reconciled := reconcileWithReality(s); reconciled > 0 {
			_ = s.Save() // best-effort; if save fails, render still reflects reality
		}

		// Group by category in a stable order.
		order := []string{"editor", "terminal", "notes", "productivity", "chat"}
		groups := map[string][]manifest.Theme{}
		for _, t := range themes {
			if listCategory != "" && t.Theme.Category != listCategory {
				continue
			}
			groups[t.Theme.Category] = append(groups[t.Theme.Category], t)
		}

		// If --category was set and matched nothing, error rather than
		// printing an empty box (which would look like a render bug).
		if listCategory != "" && len(groups) == 0 {
			return fmt.Errorf("no themes in category %q (try one of: %s)", listCategory, strings.Join(order, ", "))
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
		if listCategory != "" {
			footerThemes = nil
			for _, t := range themes {
				if t.Theme.Category == listCategory {
					footerThemes = append(footerThemes, t)
				}
			}
		}
		footer := summary(footerThemes, s)

		fmt.Fprintln(ui.W, ui.Box.Render(header+"\n\n"+body+"\n\n"+footer))
		return nil
	},
}

func init() {
	listCmd.Flags().StringVar(&listCategory, "category", "", "Only show themes in this category (editor / terminal / notes / productivity / chat)")
	_ = listCmd.RegisterFlagCompletionFunc("category", validCategories)
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

// verifyInstalled returns true if the theme's verify.command exits 0
// and (when verify.expect is set) its output contains the expected
// substring. An empty verify.command means "no way to check" — in
// that case we trust the state record.
func verifyInstalled(th manifest.Theme) bool {
	if th.Verify.Command == "" {
		return true
	}
	out, err := exec.Command("sh", "-c", th.Verify.Command).CombinedOutput()
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
