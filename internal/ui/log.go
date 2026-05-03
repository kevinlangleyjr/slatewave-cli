package ui

import (
	"fmt"
	"io"
	"os"
)

// W is the package-level writer. Tests can swap it for an in-memory
// buffer. CLI sets it to os.Stdout in main.
var W io.Writer = os.Stdout

// Header prints a "Slatewave → <theme>" header above an install /
// uninstall flow.
func Header(action, themeName string) {
	fmt.Fprintln(W, Title.Render("Slatewave")+Muted.Render(" → ")+AccentBold.Render(themeName))
	fmt.Fprintln(W, Muted.Render(action))
	fmt.Fprintln(W)
}

// StepStart prints "  ▸  <message>" in a step list. Returns a closure
// that finalizes the step with ✓ (success) or ✗ (failure) when called.
func StepStart(message string) func(err error) {
	prefix := "  " + Step.Render("▸") + "  " + message
	fmt.Fprint(W, prefix)
	return func(err error) {
		// pad message to a column for alignment
		const col = 50
		pad := col - len(message)
		if pad < 1 {
			pad = 1
		}
		fmt.Fprint(W, fmt.Sprintf("%*s", pad, ""))
		if err == nil {
			fmt.Fprintln(W, Success.Render("✓"))
		} else {
			fmt.Fprintln(W, Danger.Render("✗")+"  "+Faint.Render(err.Error()))
		}
	}
}

// Info prints a plain narrative line in slate-200 (default).
func Info(message string) {
	fmt.Fprintln(W, message)
}

// MutedLn prints a slate-400 line (e.g. "Restart your shell or run: source ~/.zshrc").
func MutedLn(message string) {
	fmt.Fprintln(W, Muted.Render(message))
}

// Errorf prints a rose error message with a leading ✗ glyph.
func Errorf(format string, args ...any) {
	fmt.Fprintln(W, Danger.Render("✗ ")+fmt.Sprintf(format, args...))
}

// Done prints a final success line with a teal "Done." prefix.
func Done(message string) {
	fmt.Fprintln(W)
	fmt.Fprintln(W, Success.Render("Done.")+"  "+message)
}
