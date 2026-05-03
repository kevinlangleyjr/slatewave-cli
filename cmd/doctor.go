package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// doctor walks every state record and classifies it. Read-only: no
// state mutations, no installs, no uninstalls. A separate command so
// users can audit before `slatewave list` silently reconciles drift
// out from under them.

type doctorStatus int

const (
	statusHealthy     doctorStatus = iota
	statusStale                    // verify failed — install asset / activation drifted
	statusMissingTool              // detect failed — underlying tool removed
	statusOrphan                   // state record but no manifest — possibly renamed slug
)

type doctorRow struct {
	slug   string
	name   string
	status doctorStatus
	detail string
	remedy string
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose installed Slatewave themes",
	Long: `Walk every recorded install and report drift:

  ✓ healthy       — verify command passes
  ⚠ stale         — verify failed; install asset or activation has drifted
  ⚠ missing-tool  — underlying tool no longer detected
  ✗ orphan        — state record but no matching manifest

Doctor is read-only — it doesn't touch state, install, or uninstall.
Run the suggested remedy command to fix each issue, or run ` + "`slatewave list`" + `
to silently reconcile stale + orphan records.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		s, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}

		if len(s.Records) == 0 {
			ui.MutedLn("Nothing installed yet — `slatewave install <theme>` to get started.")
			return nil
		}

		rows := diagnose(s)
		fmt.Fprintln(ui.W, ui.Title.Render("slatewave doctor"))
		fmt.Fprintln(ui.W)

		var healthy, stale, missing, orphan int
		for _, r := range rows {
			fmt.Fprintln(ui.W, renderDoctorRow(r))
			switch r.status {
			case statusHealthy:
				healthy++
			case statusStale:
				stale++
			case statusMissingTool:
				missing++
			case statusOrphan:
				orphan++
			}
		}

		fmt.Fprintln(ui.W)
		fmt.Fprintln(ui.W, doctorSummary(healthy, stale, missing, orphan))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func diagnose(s *state.Store) []doctorRow {
	slugs := s.AllSlugs()
	sort.Strings(slugs)

	rows := make([]doctorRow, 0, len(slugs))
	for _, slug := range slugs {
		th, err := manifest.LoadOne(slug)
		if err != nil {
			rows = append(rows, doctorRow{
				slug:   slug,
				name:   slug,
				status: statusOrphan,
				detail: "no manifest in the embedded set",
				remedy: fmt.Sprintf("slatewave uninstall %s", slug),
			})
			continue
		}
		// Detect runs first — if the underlying tool is gone, verify
		// would fail too but the better remedy is "reinstall the tool"
		// not "rerun update".
		if err := installer.Detect(th); err != nil {
			rows = append(rows, doctorRow{
				slug:   slug,
				name:   th.Theme.Name,
				status: statusMissingTool,
				detail: strings.TrimSpace(strings.SplitN(err.Error(), "\n", 2)[0]),
				remedy: fmt.Sprintf("reinstall the tool, or `slatewave uninstall %s`", slug),
			})
			continue
		}
		if !verifyInstalled(th) {
			rows = append(rows, doctorRow{
				slug:   slug,
				name:   th.Theme.Name,
				status: statusStale,
				detail: "verify command failed — install asset or activation drifted",
				remedy: fmt.Sprintf("slatewave update %s", slug),
			})
			continue
		}
		rows = append(rows, doctorRow{
			slug:   slug,
			name:   th.Theme.Name,
			status: statusHealthy,
		})
	}
	return rows
}

func renderDoctorRow(r doctorRow) string {
	const slugWidth = 18
	slug := lipgloss.NewStyle().Width(slugWidth).Render(r.slug)

	switch r.status {
	case statusHealthy:
		return "  " + ui.Accent.Render("✓") + "  " + slug + "  " + ui.Faint.Render("healthy")
	case statusStale:
		return "  " + ui.Warn.Render("⚠") + "  " + slug + "  " +
			ui.Warn.Render("stale") + "  " + ui.Faint.Render(r.detail) +
			"\n     " + ui.Muted.Render("→ ") + ui.Code.Render(r.remedy)
	case statusMissingTool:
		return "  " + ui.Warn.Render("⚠") + "  " + slug + "  " +
			ui.Warn.Render("missing tool") + "  " + ui.Faint.Render(r.detail) +
			"\n     " + ui.Muted.Render("→ ") + ui.Faint.Render(r.remedy)
	case statusOrphan:
		return "  " + ui.Danger.Render("✗") + "  " + slug + "  " +
			ui.Danger.Render("orphan") + "  " + ui.Faint.Render(r.detail) +
			"\n     " + ui.Muted.Render("→ ") + ui.Code.Render(r.remedy)
	}
	return slug
}

func doctorSummary(healthy, stale, missing, orphan int) string {
	parts := []string{
		ui.Accent.Render(fmt.Sprintf("%d healthy", healthy)),
	}
	if stale > 0 {
		parts = append(parts, ui.Warn.Render(fmt.Sprintf("%d stale", stale)))
	}
	if missing > 0 {
		parts = append(parts, ui.Warn.Render(fmt.Sprintf("%d missing tool", missing)))
	}
	if orphan > 0 {
		parts = append(parts, ui.Danger.Render(fmt.Sprintf("%d orphan", orphan)))
	}
	return "  " + strings.Join(parts, ui.Muted.Render(" · "))
}
