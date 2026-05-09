package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/jsonout"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

var infoCmd = &cobra.Command{
	Use:   "info <theme>",
	Short: "Show what `slatewave install <theme>` would actually do",
	Long: `Print the manifest details for one theme — install type, target
paths, activation method, OS support, and the website's source URL.

  slatewave info bat
  slatewave info bat --json     # machine-readable output

Useful for "what does this thing actually do" before running install,
or for confirming which file slatewave would edit during activation.
Read-only — info never touches state, install, or uninstall.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: validInstallArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := manifest.LoadOne(args[0])
		if err != nil {
			return noManifestError(args[0])
		}
		if flagBool(cmd.Flags(), "json") {
			return renderInfoJSON(t)
		}
		renderInfoHuman(t)
		return nil
	},
}

func init() {
	infoCmd.Flags().Bool("json", false, "Emit machine-readable JSON to stdout (see internal/jsonout for the schema)")
	rootCmd.AddCommand(infoCmd)
}

// renderInfoHuman prints the manifest as a styled report. Mirrors the
// shape of `slatewave status` so users have a consistent visual
// vocabulary across read-only inspection commands.
func renderInfoHuman(t manifest.Theme) {
	fmt.Fprintln(ui.W, ui.AccentBold.Render(t.Theme.Name))
	osList := strings.Join(supportedOSList(t), ", ")
	fmt.Fprintln(ui.W, ui.Muted.Render(fmt.Sprintf("  %s · %s · %s", t.Theme.Slug, t.Theme.Category, osList)))
	fmt.Fprintln(ui.W)

	fmt.Fprintln(ui.W, ui.Title.Render("Install"))
	fmt.Fprintln(ui.W, ui.Muted.Render("  type: ")+t.Install.Type)
	switch t.Install.Type {
	case "curl", "gui-import", "marketplace":
		if len(t.Install.Files) > 0 {
			fmt.Fprintln(ui.W, ui.Muted.Render("  files:"))
			for _, f := range t.Install.Files {
				fmt.Fprintln(ui.W, "    "+ui.Faint.Render(f.URL+" → "+f.Dest))
			}
		} else {
			if t.Install.URL != "" {
				fmt.Fprintln(ui.W, ui.Muted.Render("  url:  ")+ui.Faint.Render(t.Install.URL))
			}
			if t.Install.Dest != "" {
				fmt.Fprintln(ui.W, ui.Muted.Render("  dest: ")+ui.Faint.Render(t.Install.Dest))
			}
		}
	case "clone":
		fmt.Fprintln(ui.W, ui.Muted.Render("  repo: ")+ui.Faint.Render(t.Install.Repo))
		if t.Install.CloneDest != "" {
			fmt.Fprintln(ui.W, ui.Muted.Render("  dest: ")+ui.Faint.Render(t.Install.CloneDest))
		}
	case "vscode-ext":
		fmt.Fprintln(ui.W, ui.Muted.Render("  identifier: ")+ui.Faint.Render(t.Install.Identifier))
		if t.Install.CLI != "" {
			fmt.Fprintln(ui.W, ui.Muted.Render("  cli:        ")+ui.Faint.Render(t.Install.CLI))
		}
	}
	if t.Install.DoneMessage != "" {
		fmt.Fprintln(ui.W, ui.Muted.Render("  after: ")+ui.Faint.Render(t.Install.DoneMessage))
	}
	fmt.Fprintln(ui.W)

	if t.Activate.Type != "" && t.Activate.Type != "none" {
		fmt.Fprintln(ui.W, ui.Title.Render("Activate"))
		fmt.Fprintln(ui.W, ui.Muted.Render("  type: ")+t.Activate.Type)
		switch t.Activate.Type {
		case "ini-key":
			fmt.Fprintln(ui.W, ui.Muted.Render("  file: ")+ui.Faint.Render(t.Activate.File))
			fmt.Fprintln(ui.W, ui.Muted.Render("  set:  ")+ui.Faint.Render(t.Activate.Key+" = "+t.Activate.Value))
		case "gitconfig-include":
			fmt.Fprintln(ui.W, ui.Muted.Render("  include: ")+ui.Faint.Render(t.Activate.IncludePath))
		case "shell-rc":
			fmt.Fprintln(ui.W, ui.Muted.Render("  files: ")+ui.Faint.Render(strings.Join(t.Activate.Files, ", ")))
			fmt.Fprintln(ui.W, ui.Muted.Render("  line:  ")+ui.Code.Render(t.Activate.Line))
		case "toml-import":
			fmt.Fprintln(ui.W, ui.Muted.Render("  toml:   ")+ui.Faint.Render(t.Activate.TOMLPath))
			fmt.Fprintln(ui.W, ui.Muted.Render("  import: ")+ui.Faint.Render(t.Activate.Import))
		case "yaml-set":
			fmt.Fprintln(ui.W, ui.Muted.Render("  yaml: ")+ui.Faint.Render(t.Activate.YAMLPath))
			for _, p := range t.Activate.YAMLSet {
				fmt.Fprintln(ui.W, "    "+ui.Faint.Render(p.Path+" = "+p.Value))
			}
		}
		fmt.Fprintln(ui.W)
	}

	if t.Verify.Command != "" {
		fmt.Fprintln(ui.W, ui.Title.Render("Verify"))
		fmt.Fprintln(ui.W, ui.Muted.Render("  command: ")+ui.Code.Render(t.Verify.Command))
		if t.Verify.Expect != "" {
			fmt.Fprintln(ui.W, ui.Muted.Render("  expect:  ")+ui.Faint.Render(t.Verify.Expect))
		}
		fmt.Fprintln(ui.W)
	}

	fmt.Fprintln(ui.W, ui.Title.Render("Source"))
	fmt.Fprintln(ui.W, "  "+ui.Faint.Render(sourceURL(t.Theme.Slug)))
}

// renderInfoJSON emits the same details as renderInfoHuman in machine-
// readable form. Empty fields are omitted so consumers don't see a
// noisy mix of "" and real values.
func renderInfoJSON(t manifest.Theme) error {
	out := jsonout.InfoOutput{
		Slug:        t.Theme.Slug,
		Name:        t.Theme.Name,
		Category:    t.Theme.Category,
		SupportedOS: supportedOSList(t),
		SourceURL:   sourceURL(t.Theme.Slug),
		Install: jsonout.InfoInstall{
			Type:        t.Install.Type,
			URL:         t.Install.URL,
			Dest:        t.Install.Dest,
			Repo:        t.Install.Repo,
			CloneDest:   t.Install.CloneDest,
			Identifier:  t.Install.Identifier,
			CLI:         t.Install.CLI,
			DoneMessage: t.Install.DoneMessage,
		},
	}
	for _, f := range t.Install.Files {
		out.Install.Files = append(out.Install.Files, f.URL+" → "+f.Dest)
	}
	if t.Activate.Type != "" && t.Activate.Type != "none" {
		out.Activate = jsonout.InfoActivate{
			Type:        t.Activate.Type,
			File:        t.Activate.File,
			Key:         t.Activate.Key,
			Value:       t.Activate.Value,
			IncludePath: t.Activate.IncludePath,
			Files:       t.Activate.Files,
			Line:        t.Activate.Line,
			TOMLPath:    t.Activate.TOMLPath,
			Import:      t.Activate.Import,
			YAMLPath:    t.Activate.YAMLPath,
		}
	}
	if t.Verify.Command != "" {
		out.Verify = jsonout.InfoVerify{Command: t.Verify.Command, Expect: t.Verify.Expect}
	}
	enc := json.NewEncoder(ui.W)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// supportedOSList returns the manifest's SupportedOS, filling in the
// default ("darwin", "linux") when unset. Used for both the human and
// JSON renderers so they can't drift.
func supportedOSList(t manifest.Theme) []string {
	if len(t.Theme.SupportedOS) == 0 {
		return []string{"darwin", "linux"}
	}
	out := make([]string, len(t.Theme.SupportedOS))
	copy(out, t.Theme.SupportedOS)
	return out
}

// sourceURL builds the canonical website URL for a slug. Pinned to
// getslatewave.com so info output always points at the published
// theme page (not the GitHub repo, which may move).
func sourceURL(slug string) string {
	return "https://getslatewave.com/themes/" + slug
}
