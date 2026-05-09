// Package verbose owns the CLI's diagnostic logging — a single global
// writer that lower-level packages (shell, installer, activator)
// route narration through, and a flag-driven on/off switch the root
// cobra command flips at startup.
//
// When off (default), Log is a no-op (writer is io.Discard) so there's
// no measurable cost in the hot path. When on, every shell command,
// HTTP fetch, and file write the CLI performs gets a one-line log to
// stderr — making it possible to diagnose a user's failure without
// running them through repro steps.
package verbose

import (
	"fmt"
	"io"
	"os"
)

// w is where verbose lines go. Discards by default; the root command
// flips this to os.Stderr when --verbose is set. Held package-level
// since the CLI is single-process and the verbose flag is a global
// concern.
var w io.Writer = io.Discard

// SetEnabled switches verbose logging on or off. Called once from the
// root command's PersistentPreRun after cobra resolves --verbose.
func SetEnabled(enabled bool) {
	if enabled {
		w = os.Stderr
		return
	}
	w = io.Discard
}

// SetWriter replaces the verbose writer outright. Tests use this to
// capture verbose output into a buffer for assertions; production
// callers should use SetEnabled.
func SetWriter(out io.Writer) {
	w = out
}

// Log writes one formatted line to the verbose writer. Each line is
// prefixed with "↪ " so verbose noise is visually distinct from
// regular CLI output (which goes to stdout via internal/ui).
//
// No-op when verbose mode is off — the io.Discard writer drops the
// formatted bytes without allocating, so call sites can sprinkle
// Log calls freely without measuring impact on a non-verbose run.
func Log(format string, args ...any) {
	fmt.Fprintf(w, "↪ "+format+"\n", args...)
}
