package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/shell"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/tui"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// updateFlags bundles update's parsed flag values.
type updateFlags struct {
	DryRun        bool
	All           bool
	Category      string
	Interactive   bool
	NoInteractive bool
}

func parseUpdateFlags(cmd *cobra.Command) updateFlags {
	f := cmd.Flags()
	return updateFlags{
		DryRun:        flagBool(f, "dry-run"),
		All:           flagBool(f, "all"),
		Category:      flagString(f, "category"),
		Interactive:   flagBool(f, "interactive"),
		NoInteractive: flagBool(f, "no-interactive"),
	}
}

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
	RunE: func(cmd *cobra.Command, args []string) error {
		out := ui.Writer(cmd)
		f := parseUpdateFlags(cmd)
		if f.All && f.Category != "" {
			return fmt.Errorf("--all and --category are mutually exclusive")
		}
		if f.Interactive && f.NoInteractive {
			return fmt.Errorf("--interactive and --no-interactive are mutually exclusive")
		}
		bulk := f.All || f.Category != ""
		if bulk && len(args) > 0 {
			return fmt.Errorf("don't pass a theme name with --all or --category")
		}
		if !bulk && len(args) == 0 {
			return fmt.Errorf("specify a theme name, --all, or --category=<name>")
		}

		ctx := cmd.Context()
		if f.Interactive {
			emitInteractiveDeprecationWarning()
		}
		switch {
		case f.Interactive:
			return updateInteractiveTUI(ctx, args, bulk, f, out)
		case f.NoInteractive:
			if !bulk {
				return updateOne(ctx, args[0], false, f, out)
			}
			return updateBulk(ctx, f, out)
		case !bulk:
			return updateOne(ctx, args[0], false, f, out)
		case isTerminal():
			return updateInteractiveTUI(ctx, args, bulk, f, out)
		default:
			return updateBulk(ctx, f, out)
		}
	},
}

func init() {
	updateCmd.Flags().Bool("dry-run", false, "Print what would happen without re-fetching")
	updateCmd.Flags().Bool("all", false, "Update every installed theme")
	updateCmd.Flags().String("category", "", "Update every installed theme in this category")
	// See cmd/install.go for why this is DIY'd instead of using cobra's
	// pflag.MarkDeprecated — the cobra plumbing eats the warning before
	// it can reach stderr. emitInteractiveDeprecationWarning lives in
	// install.go and is shared by both commands.
	updateCmd.Flags().Bool("interactive", false, "(deprecated) Force the live progress dashboard — now the default for bulk updates on a TTY")
	updateCmd.Flags().Bool("no-interactive", false, "Force streaming output instead of the dashboard (useful for CI / log capture)")
	_ = updateCmd.RegisterFlagCompletionFunc("category", validCategories)
}

func updateInteractiveTUI(ctx context.Context, args []string, bulk bool, f updateFlags, out io.Writer) error {
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var slugs []string
	switch {
	case bulk:
		for _, slug := range s.AllSlugs() {
			if f.Category != "" {
				th, err := manifest.LoadOne(slug)
				if err != nil || th.Theme.Category != f.Category {
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
			ui.Errorf(out, "%s: no manifest — skipping", slug)
			continue
		}
		if !manifest.SupportsCurrentOS(t) {
			if !bulk {
				return fmt.Errorf("%s is not supported on %s", t.Theme.Name, manifest.CurrentGOOS())
			}
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
		ui.MutedLn(out, fmt.Sprintf("Skipping %s — install type has no automated update.", slug))
	}
	if len(skipped) > 0 {
		fmt.Fprintln(out)
	}

	if len(fixes) == 0 {
		ui.Done(out, "Nothing to update.")
		return nil
	}

	return tui.RunFix(ctx, fixes, tui.FixOptions{DryRun: f.DryRun, Title: "Updating"})
}

func updateBulk(ctx context.Context, f updateFlags, out io.Writer) error {
	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var slugs []string
	for _, slug := range s.AllSlugs() {
		th, err := manifest.LoadOne(slug)
		if err != nil {
			continue
		}
		if f.Category != "" && th.Theme.Category != f.Category {
			continue
		}
		if !manifest.SupportsCurrentOS(th) {
			continue
		}
		slugs = append(slugs, slug)
	}
	if len(slugs) == 0 {
		if f.Category != "" {
			return fmt.Errorf("no installed themes in category %q", f.Category)
		}
		ui.MutedLn(out, "Nothing to update — no themes installed.")
		return nil
	}

	var updated, skipped, failed int
	for i, slug := range slugs {
		if i > 0 {
			fmt.Fprintln(out)
		}
		err := updateOne(ctx, slug, true, f, out)
		switch {
		case errors.Is(err, installer.ErrNoAutomatedUpdate):
			skipped++
		case err != nil:
			ui.Errorf(out, "%s: %v", slug, err)
			failed++
		default:
			updated++
		}
	}

	fmt.Fprintln(out)
	switch {
	case failed > 0:
		ui.Done(out, fmt.Sprintf("%d updated, %d skipped, %d failed.", updated, skipped, failed))
	default:
		ui.Done(out, fmt.Sprintf("%d updated, %d skipped.", updated, skipped))
	}
	return nil
}

func updateOne(ctx context.Context, slug string, suppressFinal bool, f updateFlags, out io.Writer) error {
	t, err := manifest.LoadOne(slug)
	if err != nil {
		return fmt.Errorf("no manifest for %q", slug)
	}
	if !manifest.SupportsCurrentOS(t) {
		return fmt.Errorf("%s is not supported on %s", t.Theme.Name, manifest.CurrentGOOS())
	}

	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	rec, ok := s.Get(slug)
	if !ok {
		return fmt.Errorf("%s is not installed (run `slatewave install %s` first)", slug, slug)
	}

	ui.Header(out, "Updating", t.Theme.Name)

	opts := installer.Options{DryRun: f.DryRun}
	done := ui.StepStart(out, updateLabel(t))
	if err := installer.Update(ctx, t, opts); err != nil {
		if errors.Is(err, installer.ErrNoAutomatedUpdate) {
			done(nil)
			ui.MutedLn(out, fmt.Sprintf("  No automated update for install type %q — check getslatewave.com/themes/%s for the latest steps.", t.Install.Type, slug))
			return err
		}
		done(err)
		return err
	}
	done(nil)

	if t.Install.Post != nil {
		done := ui.StepStart(out, t.Install.Post.Description)
		if !f.DryRun {
			if err := shell.RunInherit(ctx, t.Install.Post.Command); err != nil {
				done(err)
				return fmt.Errorf("post-hook: %w", err)
			}
		}
		done(nil)
	}

	if !f.DryRun {
		if err := state.Update(func(s *state.Store) error {
			rec.InstalledAt = time.Now().UTC()
			s.Put(rec)
			return nil
		}); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	if suppressFinal {
		return nil
	}
	if f.DryRun {
		ui.Done(out, "Dry run — no files written.")
	} else {
		ui.Done(out, "Up to date.")
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
		return fmt.Sprintf("Reinstalling %s extension %s", installer.VSCodeExtCLI(t), t.Install.Identifier)
	case "marketplace":
		return "Marketplace install — manual update"
	case "manual":
		return "Manual install — manual update"
	default:
		return "Updating"
	}
}
