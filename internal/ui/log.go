package ui

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// writerKey is the cobra.Command.Context() key under which a writer is
// stored. Unexported so the only way to set / read the writer is via
// WithWriter / Writer below — keeps the indirection contained.
type writerKey struct{}

// WithWriter stashes w on ctx so a downstream cobra command can pull
// it back out via Writer(cmd). The root command's PersistentPreRun
// sets this once at startup; tests inject a *bytes.Buffer the same
// way to capture command output.
func WithWriter(ctx context.Context, w io.Writer) context.Context {
	return context.WithValue(ctx, writerKey{}, w)
}

// Writer returns the writer attached to cmd's context, falling back
// to os.Stdout when no writer was injected. Every cmd RunE that
// produces output should pull from here at the top of its body and
// thread the result through helpers, instead of reaching for a
// package-level global.
func Writer(cmd *cobra.Command) io.Writer {
	if cmd == nil {
		return os.Stdout
	}
	if w, ok := cmd.Context().Value(writerKey{}).(io.Writer); ok && w != nil {
		return w
	}
	return os.Stdout
}

// Header prints a "Slatewave → <theme>" header above an install /
// uninstall flow.
func Header(w io.Writer, action, themeName string) {
	fmt.Fprintln(w, Title.Render("Slatewave")+Muted.Render(" → ")+AccentBold.Render(themeName))
	fmt.Fprintln(w, Muted.Render(action))
	fmt.Fprintln(w)
}

// StepStart prints "  ▸  <message>" in a step list. Returns a closure
// that finalizes the step with ✓ (success) or ✗ (failure) when called.
func StepStart(w io.Writer, message string) func(err error) {
	prefix := "  " + Step.Render("▸") + "  " + message
	fmt.Fprint(w, prefix)
	return func(err error) {
		// pad message to a column for alignment
		const col = 50
		pad := col - len(message)
		if pad < 1 {
			pad = 1
		}
		fmt.Fprintf(w, "%*s", pad, "")
		if err == nil {
			fmt.Fprintln(w, Success.Render("✓"))
		} else {
			fmt.Fprintln(w, Danger.Render("✗")+"  "+Faint.Render(err.Error()))
		}
	}
}

// Info prints a plain narrative line in slate-200 (default).
func Info(w io.Writer, message string) {
	fmt.Fprintln(w, message)
}

// MutedLn prints a slate-400 line (e.g. "Restart your shell or run: source ~/.zshrc").
func MutedLn(w io.Writer, message string) {
	fmt.Fprintln(w, Muted.Render(message))
}

// Errorf prints a rose error message with a leading ✗ glyph.
func Errorf(w io.Writer, format string, args ...any) {
	fmt.Fprintln(w, Danger.Render("✗ ")+fmt.Sprintf(format, args...))
}

// Done prints a final success line with a teal "Done." prefix.
func Done(w io.Writer, message string) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, Success.Render("Done.")+"  "+message)
}
