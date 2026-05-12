// Package cmd holds the cobra command tree for the slatewave CLI.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
	"github.com/kevinlangleyjr/slatewave-cli/internal/verbose"
	"github.com/kevinlangleyjr/slatewave-cli/internal/version"
)

// Version is set via -ldflags at release time.
var Version = "dev"

// verboseFlag is the parsed value of the persistent --verbose flag.
// PersistentPreRun copies it into internal/verbose so lower-level
// packages can call verbose.Log without re-reading flag state.
var verboseFlag bool

// versionCheckCh holds the result channel from the async version check
// kicked off in PersistentPreRun. PersistentPostRun waits on it for
// up to nagWait before emitting the upgrade nag — slow API responses
// don't block the user's command, they just miss the nag this run
// (the cache will catch them next time).
var versionCheckCh <-chan *version.Result

// nagWait is the maximum time PersistentPostRun blocks waiting for
// the version-check goroutine to return. Short enough to be invisible
// on every command, long enough to surface a fresh-cache result
// without round-tripping the API.
const nagWait = 200 * time.Millisecond

var rootCmd = &cobra.Command{
	Use:           "slatewave",
	Short:         "One palette across every tool you live in.",
	Long:          "slatewave installs and manages the Slatewave family of themes — editor, terminal, prompt, notes, launcher, chat — from one CLI.",
	Version:       Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, _ []string) {
		out := ui.Writer(cmd)
		ui.PrintBanner(out)
		cmd.SetOut(out)
		_ = cmd.Help()
	},
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		verbose.SetEnabled(verboseFlag)
		versionCheckCh = version.Check(Version)
		// Inject os.Stdout as the per-command writer so subcommands
		// reading via ui.Writer(cmd) get the real terminal. Tests
		// override this by calling SetContext before invoking RunE.
		cmd.SetContext(ui.WithWriter(cmd.Context(), os.Stdout))
	},
	PersistentPostRun: func(_ *cobra.Command, _ []string) {
		emitUpgradeNag()
	},
}

// emitUpgradeNag waits briefly for the version check goroutine to
// finish, then prints a one-line nag to stderr if the user is on an
// out-of-date binary. Silent on no-update / no-result / wait-expired
// so a healthy run produces zero noise.
//
// Always writes to os.Stderr (never the cmd's writer or os.Stdout).
// `slatewave list --json | jq` and similar piping idioms send only
// stdout through the consumer; stderr stays attached to the user's
// terminal, so the nag never contaminates a JSON payload. Users who
// merge with `2>&1` opt into mixed output by choice — the nag isn't
// suppressed in that case because doing so would also hide it from
// every other reasonable invocation. The lock-in test for this
// contract lives in root_test.go.
func emitUpgradeNag() {
	if versionCheckCh == nil {
		return
	}
	select {
	case res := <-versionCheckCh:
		if res == nil {
			return
		}
		fmt.Fprintln(os.Stderr,
			ui.Muted.Render("➜ slatewave ")+
				ui.Accent.Render(res.Latest)+
				ui.Muted.Render(" is out (you're on "+Version+"): "+res.URL))
	case <-time.After(nagWait):
		// Background check didn't finish in time; the cache will be
		// updated whenever the goroutine completes (or not — main
		// will exit and kill it). Either way, next run sees fresh
		// state.
	}
}

// Execute runs the cobra root. main.go calls this.
//
// A signal-aware context is plumbed into the command tree so SIGINT
// (Ctrl-C from a streaming CLI run) and SIGTERM cancel anything reading
// cmd.Context() — most importantly the installer's git clone /
// post-hook / VS Code extension shell-outs, which propagate the cancel
// to the child process instead of orphaning it.
//
// In TUI mode bubbletea grabs raw stdin and Ctrl-C arrives as a
// KeyMsg, not a signal — the TUI's own KeyCtrlC handler is responsible
// for cancelling its install context. Both layers are needed; this one
// covers the streaming path.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
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
