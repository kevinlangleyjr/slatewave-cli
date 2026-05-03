// Package cmd holds the cobra command tree for the slatewave CLI.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set via -ldflags at release time.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:           "slatewave",
	Short:         "One palette across every tool you live in.",
	Long:          "slatewave installs and manages the Slatewave family of themes — editor, terminal, prompt, notes, launcher, chat — from one CLI.",
	Version:       Version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the cobra root. main.go calls this.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
}
