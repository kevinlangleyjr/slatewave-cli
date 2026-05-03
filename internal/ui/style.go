package ui

import "github.com/charmbracelet/lipgloss"

// Pre-defined styles. Commands compose these — they should not call
// lipgloss.NewStyle() inline. Keeps the visual vocabulary consistent.
var (
	// Title — for command headers like "Slatewave → bat"
	Title = lipgloss.NewStyle().Foreground(Slate200).Bold(true)

	// Accent — the slatewave teal-300 signature; theme names, success
	// markers, the active state in lists.
	Accent = lipgloss.NewStyle().Foreground(Teal300)

	// AccentBold — same teal as Accent but bold; used for theme names
	// inside narrative output.
	AccentBold = lipgloss.NewStyle().Foreground(Teal300).Bold(true)

	// Muted — slate-400, the subtle secondary tone (categories,
	// timestamps, "via Marketplace" annotations).
	Muted = lipgloss.NewStyle().Foreground(Slate400)

	// Faint — slate-500, the dimmest readable tone (hex labels next to
	// swatches, "not installed" status).
	Faint = lipgloss.NewStyle().Foreground(Slate500)

	// Success — teal-300, used by the ✓ marker.
	Success = lipgloss.NewStyle().Foreground(Teal300)

	// Warn — amber-400, for warnings the user should see but not
	// errors that block.
	Warn = lipgloss.NewStyle().Foreground(Amber400)

	// Danger — rose-400, for errors and the ✗ marker.
	Danger = lipgloss.NewStyle().Foreground(Rose400)

	// Step — for the ▸ glyph leading each install step.
	Step = lipgloss.NewStyle().Foreground(Teal300)

	// Box — slate-600 1px border with internal padding. Used for
	// boxed list output and `slatewave doctor` panels.
	Box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Slate600).
		Padding(0, 2)

	// Code — for commands embedded in narrative output ("run
	// `:colorscheme slatewave`"). Slate-200 on slate-800.
	Code = lipgloss.NewStyle().Foreground(Slate200).Background(Slate800).Padding(0, 1)
)
