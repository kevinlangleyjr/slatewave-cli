package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/activator"
	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

var installDryRun bool

var installCmd = &cobra.Command{
	Use:   "install <theme>",
	Short: "Install a Slatewave theme",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		slug := args[0]
		t, err := manifest.LoadOne(slug)
		if err != nil {
			return fmt.Errorf("no manifest for %q (run `slatewave list` to see available themes)", slug)
		}

		ui.Header("Installing", t.Theme.Name)
		opts := installer.Options{DryRun: installDryRun}
		actOpts := activator.Options{DryRun: installDryRun}

		// Detect the underlying tool. Skip for marketplace-only and
		// manual install types where the tool detection is the user's job.
		if t.Install.Type != "marketplace" && t.Install.Type != "manual" {
			done := ui.StepStart(fmt.Sprintf("Detecting %s", t.Theme.Slug))
			if err := installer.Detect(t); err != nil {
				done(err)
				return err
			}
			done(nil)
		}

		// Install
		done := ui.StepStart(installLabel(t))
		rec, err := installer.Install(t, opts)
		if err != nil {
			done(err)
			return err
		}
		done(nil)

		// Post-hook (folded into install logic; surface its own line)
		if t.Install.Post != nil {
			done := ui.StepStart(t.Install.Post.Description)
			done(nil)
		}

		// Activate
		if t.Activate.Type != "" && t.Activate.Type != "none" {
			done := ui.StepStart(activateLabel(t))
			if err := activator.Activate(t, &rec, actOpts); err != nil {
				done(err)
				return err
			}
			done(nil)
		}

		// Persist state (skip on dry-run — no real install happened)
		if !installDryRun {
			s, err := state.Load()
			if err != nil {
				return fmt.Errorf("load state: %w", err)
			}
			s.Put(rec)
			if err := s.Save(); err != nil {
				return fmt.Errorf("save state: %w", err)
			}
		}

		// Manual-install themes carry final instructions.
		if t.Install.Type == "manual" || len(t.Install.Instructions) > 0 {
			fmt.Fprintln(ui.W)
			for _, line := range t.Install.Instructions {
				ui.MutedLn("  " + line)
			}
		}

		if installDryRun {
			ui.Done("Dry run — no files written.")
		} else {
			ui.Done(doneMessage(t))
		}
		return nil
	},
}

func init() {
	installCmd.Flags().BoolVar(&installDryRun, "dry-run", false, "Print what would happen without writing files")
}

// installLabel produces the step label per install type.
func installLabel(t manifest.Theme) string {
	switch t.Install.Type {
	case "curl":
		return "Fetching theme file"
	case "clone":
		return "Cloning " + t.Install.Repo
	case "vscode-ext":
		return "Installing VSCode extension " + t.Install.Identifier
	case "marketplace":
		return "Opening Marketplace in your browser"
	case "gui-import":
		return "Fetching theme file (GUI import follows)"
	case "manual":
		return "Manual install — see instructions below"
	default:
		return "Running install step"
	}
}

func activateLabel(t manifest.Theme) string {
	switch t.Activate.Type {
	case "ini-key":
		return fmt.Sprintf("Setting %s = %s in %s", t.Activate.Key, t.Activate.Value, t.Activate.File)
	case "gitconfig-include":
		return "Adding include.path to ~/.gitconfig"
	case "shell-rc":
		if len(t.Activate.Files) > 0 {
			return "Appending to " + t.Activate.Files[0]
		}
		return "Appending activation line"
	default:
		return "Activating"
	}
}

func doneMessage(t manifest.Theme) string {
	// Tool-specific guidance — bat picks up its config on next
	// invocation, but a prompt change needs a shell restart.
	switch t.Theme.Slug {
	case "bat":
		return "bat picks up the new theme on its next invocation."
	case "btop":
		return "Launch `btop` to see Slatewave applied."
	case "delta":
		return "Run a `git diff` in any repo to see Slatewave applied."
	case "oh-my-posh", "starship":
		return "Restart your shell or `source` your rc file."
	}
	return "Slatewave is installed."
}
