// Package ui owns the CLI's visual presentation — pre-styled lipgloss
// styles, palette constants, and printing helpers. The palette mirrors
// the canonical Slatewave palette in getslatewave/src/styles/tokens.css
// so this CLI's output speaks the same color vocabulary as the themes
// it installs.
package ui

import "github.com/charmbracelet/lipgloss"

// Slatewave palette constants. Names mirror the --sw-* CSS variables
// in the website's tokens.css. Update both together when the canonical
// palette changes.
var (
	// Foundation — slate
	Slate200    = lipgloss.Color("#e2e8f0")
	Slate300    = lipgloss.Color("#cbd5e1")
	Slate400    = lipgloss.Color("#94a3b8")
	Slate500    = lipgloss.Color("#64748b")
	Slate600    = lipgloss.Color("#475569")
	Slate700    = lipgloss.Color("#334155")
	Slate800    = lipgloss.Color("#1e293b")
	Slate900    = lipgloss.Color("#0f172a")
	SlateEditor = lipgloss.Color("#282c34")
	SlateChrome = lipgloss.Color("#21252b")

	// Signature — teal
	Teal200 = lipgloss.Color("#99f6e4")
	Teal300 = lipgloss.Color("#5eead4")
	Teal400 = lipgloss.Color("#2dd4bf")

	// Accents
	Sky300   = lipgloss.Color("#7dd3fc")
	Sky400   = lipgloss.Color("#38bdf8")
	Rose400  = lipgloss.Color("#fb7185")
	Purple   = lipgloss.Color("#b388ff")
	Amber400 = lipgloss.Color("#fbbf24")
	Amber700 = lipgloss.Color("#b45309")
)
