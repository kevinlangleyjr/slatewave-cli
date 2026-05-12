package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/jsonout"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/tui"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// doctorFlags bundles doctor's parsed flag values.
type doctorFlags struct {
	Fix    bool
	DryRun bool
	JSON   bool
}

func parseDoctorFlags(cmd *cobra.Command) doctorFlags {
	f := cmd.Flags()
	return doctorFlags{
		Fix:    flagBool(f, "fix"),
		DryRun: flagBool(f, "dry-run"),
		JSON:   flagBool(f, "json"),
	}
}

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

By default doctor is read-only — it doesn't touch state, install, or uninstall.
Pass --fix to launch an interactive remediation flow: pick which drift rows
to fix and the dashboard runs the matching remedy (update for stale, uninstall
for missing-tool, drop for orphan). Without --fix, copy a suggested remedy from
the report or run ` + "`slatewave list`" + ` to silently reconcile stale + orphan records.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := ui.Writer(cmd)
		f := parseDoctorFlags(cmd)
		s, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}

		if f.JSON {
			return renderDoctorJSON(s, out)
		}

		if len(s.Records) == 0 {
			ui.MutedLn(out, "Nothing installed yet — `slatewave install <theme>` to get started.")
			return nil
		}

		rows := diagnose(s)
		fmt.Fprintln(out, ui.Title.Render("slatewave doctor"))
		fmt.Fprintln(out)

		var healthy, stale, missing, orphan int
		for _, r := range rows {
			fmt.Fprintln(out, renderDoctorRow(r))
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

		fmt.Fprintln(out)
		fmt.Fprintln(out, doctorSummary(healthy, stale, missing, orphan))

		if f.Fix {
			fmt.Fprintln(out)
			return runDoctorFix(cmd.Context(), rows, f, out)
		}
		return nil
	},
}

func init() {
	doctorCmd.Flags().Bool("fix", false, "Interactively remediate stale, missing-tool, and orphan rows")
	doctorCmd.Flags().Bool("dry-run", false, "With --fix, show the dashboard without writing")
	doctorCmd.Flags().Bool("json", false, "Emit machine-readable JSON to stdout (incompatible with --fix)")
	rootCmd.AddCommand(doctorCmd)
}

// renderDoctorJSON emits the diagnose() output as JSON: per-theme
// rows with status / detail / remedy plus a summary count. Empty
// state still produces a well-formed object (themes: [], summary
// all-zero) so consumers can tell "no records" apart from a parse
// failure.
func renderDoctorJSON(s *state.Store, out io.Writer) error {
	doc := jsonout.DoctorOutput{Themes: make([]jsonout.DoctorRow, 0)}
	if len(s.Records) > 0 {
		for _, r := range diagnose(s) {
			doc.Themes = append(doc.Themes, jsonout.DoctorRow{
				Slug:   r.slug,
				Name:   r.name,
				Status: doctorStatusString(r.status),
				Detail: r.detail,
				Remedy: r.remedy,
			})
			switch r.status {
			case statusHealthy:
				doc.Summary.Healthy++
			case statusStale:
				doc.Summary.Stale++
			case statusMissingTool:
				doc.Summary.MissingTool++
			case statusOrphan:
				doc.Summary.Orphan++
			}
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// doctorStatusString maps the internal iota enum to the wire-format
// string. Pinned here so renaming the enum or rebalancing values
// can't accidentally change the JSON contract.
func doctorStatusString(s doctorStatus) string {
	switch s {
	case statusHealthy:
		return "healthy"
	case statusStale:
		return "stale"
	case statusMissingTool:
		return "missing-tool"
	case statusOrphan:
		return "orphan"
	}
	return "unknown"
}

// runDoctorFix maps fixable doctor rows to tui.Fix entries, hands them to the picker so the user can confirm or deselect, then runs the dashboard. Healthy rows are filtered out before the picker — there's nothing to fix.
func runDoctorFix(ctx context.Context, rows []doctorRow, f doctorFlags, out io.Writer) error {
	fixes := buildFixes(rows)
	if len(fixes) == 0 {
		ui.Done(out, "Nothing to fix.")
		return nil
	}

	selected, err := tui.PickFixes(fixes)
	if err != nil {
		if errors.Is(err, tui.ErrAborted) {
			ui.MutedLn(out, "Aborted. Nothing changed.")
			return nil
		}
		return err
	}
	if len(selected) == 0 {
		ui.MutedLn(out, "No fixes selected. Nothing changed.")
		return nil
	}

	fmt.Fprintln(out)
	return tui.RunFix(ctx, selected, tui.FixOptions{DryRun: f.DryRun})
}

// buildFixes converts diagnose() rows into tui.Fix entries. Healthy rows are dropped. For stale and missing-tool rows we re-load the manifest so the fix pipeline has it (avoids a second LoadOne in the dashboard); orphan rows ship without a manifest since that's the diagnosis.
func buildFixes(rows []doctorRow) []tui.Fix {
	var out []tui.Fix
	for _, r := range rows {
		switch r.status {
		case statusStale:
			th, err := manifest.LoadOne(r.slug)
			if err != nil {
				continue
			}
			out = append(out, tui.Fix{Slug: r.slug, Name: r.name, Kind: tui.FixUpdate, Theme: th})
		case statusMissingTool:
			th, err := manifest.LoadOne(r.slug)
			if err != nil {
				continue
			}
			out = append(out, tui.Fix{Slug: r.slug, Name: r.name, Kind: tui.FixUninstall, Theme: th})
		case statusOrphan:
			out = append(out, tui.Fix{Slug: r.slug, Name: r.name, Kind: tui.FixDropOrphan})
		}
	}
	return out
}

func diagnose(s *state.Store) []doctorRow {
	// AllSlugs already returns sorted, no need to re-sort here.
	slugs := s.AllSlugs()

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
