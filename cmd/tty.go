package cmd

import "os"

// isTerminal reports whether stdout is wired to a real terminal. Used
// by install / update to pick the TUI dashboard by default for bulk
// operations on a TTY, and gracefully fall back to streaming output
// when piped into a file or running in CI.
//
// Pure stdlib: checks os.ModeCharDevice on the stat result. Windows
// runners report this correctly for the default console host; non-TTY
// pipes report 0 on every platform.
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
