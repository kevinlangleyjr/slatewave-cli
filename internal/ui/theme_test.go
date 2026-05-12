package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestApplyThemeOverride pins the env-value-to-lipgloss-state mapping
// that SLATEWAVE_THEME is built on. A regression here would mean users
// who set SLATEWAVE_THEME=light still get the dark palette (or vice
// versa) — the bug case the env var exists to fix.
func TestApplyThemeOverride(t *testing.T) {
	prev := lipgloss.HasDarkBackground()
	t.Cleanup(func() { lipgloss.SetHasDarkBackground(prev) })

	cases := []struct {
		in   string
		want bool // expected HasDarkBackground after applyThemeOverride
		// pre flips the state before the call so we can prove the
		// override sticks (true→false / false→true) vs no-op cases
		// that should preserve whatever was set before.
		pre bool
	}{
		{in: "light", want: false, pre: true},     // dark→light
		{in: "dark", want: true, pre: false},      // light→dark
		{in: "LIGHT", want: false, pre: true},     // case-insensitive
		{in: "Dark", want: true, pre: false},      //
		{in: "  light  ", want: false, pre: true}, // trims whitespace
		{in: "auto", want: true, pre: true},       // unknown → no-op preserves pre
		{in: "", want: true, pre: true},           // empty → no-op preserves pre
		{in: "garbage", want: false, pre: false},  // garbage → no-op preserves pre
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			lipgloss.SetHasDarkBackground(c.pre)
			applyThemeOverride(c.in)
			if got := lipgloss.HasDarkBackground(); got != c.want {
				t.Errorf("applyThemeOverride(%q) with pre=%v: HasDarkBackground=%v, want %v", c.in, c.pre, got, c.want)
			}
		})
	}
}
