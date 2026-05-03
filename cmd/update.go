package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/tui"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

var (
	updateDryRun      bool
	updateAll         bool
	updateCategory    string
	updateInteractive bool
)

var updateCmd = &cobra.Command{
	Use:   "update [theme]",
	Short: "Update one Slatewave theme, an entire category, or every installed theme",
	Long: `Re-fetch a theme's assets without re-running activation.

  slatewave update bat                   # one theme
  slatewave update --category=editor     # every installed theme in a category
  slatewave update --all                 # every installed theme

Update only refreshes assets — the theme stays activated, your config
stays edited. The state record's install timestamp is bumped so ` + "`slatewave status`" + `
shows when each theme was last refreshed.

Themes that aren't installed are skipped silently. Themes whose install
type has no automated update path (` + "`marketplace`" + `, ` + "`manual`" + `) are reported
with a one-line hint and continue.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: validInstalledArgs,
	RunE: func(_ *cobra.Command, args []string) error {
		if updateAll && updateCategory != "" {
			return fmt.Errorf("--all and --category are mutually exclusive")
		}
		bulk := updateAll || updateCategory != ""
		if bulk && len(args) > 0 {
			return fmt.Errorf("don't pass a theme name with --all or --category")
		}
		if !bulk && len(args) == 0 {
			return fmt.Errorf("specify a theme name, --all, or --category=<name>")
		}

		if updateInteractive {
			return updateInteractiveTUI(args, bulk)
		}

		if !bulk {
			return updateOne(args[0], false)
		}
		return updateBulk()
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Print what would happen without re-fetching")
	updateCmd.Flags().BoolVar(&updateAll, "all", false, "Update every installed theme")
	updateCmd.Flags().StringVar(&updateCategory, "category", "", "Update every installed theme in this category")
	updateCmd.Flags().BoolVar(&updateInteractive, "interactive", false, "Show a live progress dashboard instead of streamed step output")
	_ = updateCmd.RegisterFlagCompletionFunc("category", validCategories)
}

// updateInteractiveTUI runs the update pipeline through the bubbletea dashboard. Loads each requested slug's manifest, drops marketplace + manual themes (no automated update path — would just clutter the dashboard with "failed: no automated update" rows), and hands the rest to tui.RunFix with FixUpdate. Reuses the fix dashboard since the pipeline is identical (refresh assets + post-hook + bump InstalledAt).
func updateInteractiveTUI(args []string, bulk bool) error {
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var slugs []string
	switch {
	case bulk:
		for _, slug := range s.AllSlugs() {
			if updateCategory != "" {
				th, err := manifest.LoadOne(slug)
				if err != nil || th.Theme.Category != updateCategory {
					continue
				}
			}
			slugs = append(slugs, slug)
		}
	default:
		if _, ok := s.Get(args[0]); !ok {
			return fmt.Errorf("%s is not installed (run `slatewave install %s` first)", args[0], args[0])
		}
		slugs = []string{args[0]}
	}

	var fixes []tui.Fix
	var skipped []string
	for _, slug := range slugs {
		t, err := manifest.LoadOne(slug)
		if err != nil {
			ui.Errorf("%s: no manifest — skipping", slug)
			continue
		}
		if t.Install.Type == "marketplace" || t.Install.Type == "manual" {
			skipped = append(skipped, slug)
			continue
		}
		fixes = append(fixes, tui.Fix{
			Slug:  slug,
			Name:  t.Theme.Name,
			Kind:  tui.FixUpdate,
			Theme: t,
		})
	}

	for _, slug := range skipped {
		ui.MutedLn(fmt.Sprintf("Skipping %s — install type has no automated update.", slug))
	}
	if len(skipped) > 0 {
		fmt.Fprintln(ui.W)
	}

	if len(fixes) == 0 {
		ui.Done("Nothing to update.")
		return nil
	}

	return tui.RunFix(fixes, tui.FixOptions{DryRun: updateDryRun, Title: "Updating"})
}

// updateBulk iterates every installed slug, optionally filtered by
// category, calling updateOne for each. Individual failures are reported
// and the run continues.
func updateBulk() error {
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var slugs []string
	for _, slug := range s.AllSlugs() {
		if updateCategory != "" {
			th, err := manifest.LoadOne(slug)
			if err != nil || th.Theme.Category != updateCategory {
				continue
			}
		}
		slugs = append(slugs, slug)
	}
	if len(slugs) == 0 {
		if updateCategory != "" {
			return fmt.Errorf("no installed themes in category %q", updateCategory)
		}
		ui.MutedLn("Nothing to update — no themes installed.")
		return nil
	}

	var updated, skipped, failed int
	for i, slug := range slugs {
		if i > 0 {
			fmt.Fprintln(ui.W)
		}
		err := updateOne(slug, true)
		switch {
		case errors.Is(err, installer.ErrNoAutomatedUpdate):
			skipped++
		case err != nil:
			ui.Errorf("%s: %v", slug, err)
			failed++
		default:
			updated++
		}
	}

	fmt.Fprintln(ui.W)
	switch {
	case failed > 0:
		ui.Done(fmt.Sprintf("%d updated, %d skipped, %d failed.", updated, skipped, failed))
	default:
		ui.Done(fmt.Sprintf("%d updated, %d skipped.", updated, skipped))
	}
	return nil
}

// updateOne re-fetches assets for one installed theme.
//
// suppressFinal — when true (bulk mode), skip the per-theme final
// "Up to date." line so the bulk caller can render its own summary.
func updateOne(slug string, suppressFinal bool) error {
	t, err := manifest.LoadOne(slug)
	if err != nil {
		return fmt.Errorf("no manifest for %q", slug)
	}

	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	rec, ok := s.Get(slug)
	if !ok {
		return fmt.Errorf("%s is not installed (run `slatewave install %s` first)", slug, slug)
	}

	ui.Header("Updating", t.Theme.Name)

	opts := installer.Options{DryRun: updateDryRun}
	done := ui.StepStart(updateLabel(t))
	if err := installer.Update(t, opts); err != nil {
		if errors.Is(err, installer.ErrNoAutomatedUpdate) {
			done(nil)
			ui.MutedLn(fmt.Sprintf("  No automated update for install type %q — check getslatewave.com/themes/%s for the latest steps.", t.Install.Type, slug))
			return err
		}
		done(err)
		return err
	}
	done(nil)

	// Re-run the post-hook so derived caches (e.g., `bat cache --build`)
	// reflect the refreshed asset.
	if t.Install.Post != nil {
		done := ui.StepStart(t.Install.Post.Description)
		if !updateDryRun {
			if err := exec.Command("sh", "-c", t.Install.Post.Command).Run(); err != nil {
				done(err)
				return fmt.Errorf("post-hook: %w", err)
			}
		}
		done(nil)
	}

	// Bump the install timestamp so `slatewave status` reflects the
	// last refresh.
	if !updateDryRun {
		rec.InstalledAt = time.Now().UTC()
		s.Put(rec)
		if err := s.Save(); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	if suppressFinal {
		return nil
	}
	if updateDryRun {
		ui.Done("Dry run — no files written.")
	} else {
		ui.Done("Up to date.")
	}
	return nil
}

func updateLabel(t manifest.Theme) string {
	switch t.Install.Type {
	case "curl", "gui-import":
		return "Re-fetching theme file"
	case "clone":
		return "git pull --ff-only on " + t.Install.CloneDest
	case "vscode-ext":
		return "Reinstalling VSCode extension " + t.Install.Identifier
	case "marketplace":
		return "Marketplace install — manual update"
	case "manual":
		return "Manual install — manual update"
	default:
		return "Updating"
	}
}
