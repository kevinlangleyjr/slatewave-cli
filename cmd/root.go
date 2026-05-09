// Package cmd holds the cobra command tree for the slatewave CLI.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/verbose"
)

// Version is set via -ldflags at release time.
var Version = "dev"

// verboseFlag is the parsed value of the persistent --verbose flag.
// PersistentPreRun copies it into internal/verbose so lower-level
// packages can call verbose.Log without re-reading flag state.
var verboseFlag bool

var rootCmd = &cobra.Command{
	Use:           "slatewave",
	Short:         "One palette across every tool you live in.",
	Long:          "slatewave installs and manages the Slatewave family of themes — editor, terminal, prompt, notes, launcher, chat — from one CLI.",
	Version:       Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		verbose.SetEnabled(verboseFlag)
	},
}

// Execute runs the cobra root. main.go calls this.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Stream every shell command, URL fetch, and file write to stderr")

	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
}
