// Package shell runs manifest-supplied shell commands through the OS-
// appropriate interpreter — `sh -c` on unix, `cmd /C` on windows. Every
// place the engine used to call exec.Command("sh", "-c", ...) directly
// should route through here so Windows builds don't depend on a POSIX
// shell being on PATH.
//
// Manifests targeting Windows must write commands cmd.exe can parse:
// `where <tool>` instead of `command -v <tool>`, `if exist ... (exit 0)`
// instead of `test -f ...`, etc. The OS-specific overrides on Theme.Meta
// (DetectCommandWindows) and Theme.Verify (CommandWindows) exist for
// exactly this reason — pick them via manifest.DetectCommandFor /
// manifest.VerifyCommandFor before passing to Run.
package shell

import (
	"context"
	"os/exec"
	"runtime"
	"time"

	"github.com/kevinlangleyjr/slatewave-cli/internal/verbose"
)

// stdioWaitDelay caps how long exec waits for stdio pipes to drain
// after the process is terminated. Without this (zero default), a
// command like `sh -c "sleep 60"` whose child grabs the parent's
// stdout would hang CombinedOutput forever: ctx cancel kills sh,
// but sleep gets reparented to init and keeps the pipes open.
// 100ms is plenty for any legit drain; anything beyond is a
// detached child the timeout was supposed to cut loose.
const stdioWaitDelay = 100 * time.Millisecond

// Run executes command via the OS-appropriate shell and returns the
// combined stdout+stderr output. ctx cancels the underlying process
// (used by the TUI's per-detect timeout).
func Run(ctx context.Context, command string) ([]byte, error) {
	verbose.Log("shell: %s", command)
	return cmdFor(ctx, command).CombinedOutput()
}

// RunInherit runs command with stdout/stderr wired to the parent process
// instead of capturing them. Used for post-install hooks where the user
// expects to see the underlying tool's output (e.g. `bat cache --build`)
// rather than have it disappear into a buffer that's only printed on
// failure.
func RunInherit(ctx context.Context, command string) error {
	verbose.Log("shell: %s", command)
	return cmdFor(ctx, command).Run()
}

func cmdFor(ctx context.Context, command string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.WaitDelay = stdioWaitDelay
	return cmd
}
