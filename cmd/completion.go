package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
)

// validInstallArgs completes from every manifest slug supported on the
// current OS. Suggesting an unsupported slug would lead the user into
// an immediate "is not supported on <os>" error — better to leave it
// out of completion entirely.
func validInstallArgs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	all, err := manifest.LoadSupported()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var out []string
	for _, t := range all {
		if strings.HasPrefix(t.Theme.Slug, toComplete) {
			out = append(out, t.Theme.Slug)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// validInstalledArgs completes only currently-installed slugs. Used by
// `uninstall` and `update` since both only make sense for already-
// installed themes.
func validInstalledArgs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	s, err := state.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var out []string
	for _, slug := range s.AllSlugs() {
		if strings.HasPrefix(slug, toComplete) {
			out = append(out, slug)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// validCategories completes the --category flag with the manifest
// schema's category enum, narrowed to categories actually present in
// the OS-supported set (so a category with zero installable themes on
// the current OS doesn't get suggested as a dead-end).
func validCategories(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	all, err := manifest.LoadSupported()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range all {
		c := t.Theme.Category
		if seen[c] || !strings.HasPrefix(c, toComplete) {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
