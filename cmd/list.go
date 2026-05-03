package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List every Slatewave theme and what's installed",
	RunE: func(_ *cobra.Command, _ []string) error {
		themes, err := manifest.LoadAll()
		if err != nil {
			return fmt.Errorf("load manifests: %w", err)
		}
		s, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}

		// Group by category in a stable order.
		order := []string{"editor", "terminal", "notes", "productivity", "chat"}
		groups := map[string][]manifest.Theme{}
		for _, t := range themes {
			groups[t.Theme.Category] = append(groups[t.Theme.Category], t)
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
		footer := summary(themes, s)

		fmt.Fprintln(ui.W, ui.Box.Render(header+"\n\n"+body+"\n\n"+footer))
		return nil
	},
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
	cat := ui.Muted.Copy().Width(12).Render(t.Theme.Category)
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
