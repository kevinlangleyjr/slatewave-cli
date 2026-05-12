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
//
// Each constant is an AdaptiveColor: the Dark stop is the canonical
// palette hex (what shipped before any of this), the Light stop is the
// equivalent role in a light-background terminal. lipgloss queries
// terminal background via OSC 11 at first style render and picks the
// appropriate stop; piped output / non-TTY falls back to Dark so the
// stripped-color path stays identical.
//
// The mapping rules:
//
//   - Slate ladder inverts luminance — what's "light text on dark bg"
//     becomes "dark text on light bg" with the same semantic role.
//     Slate500 is the true mid-gray and stays neutral on both.
//   - Accent stops (Teal / Sky / Rose / Purple / Amber) deepen rather
//     than flip — the dark-mode bright glows against #1e293b-ish bg;
//     on a white terminal the same hex washes out, so light pairs
//     move into the 600/700 range to preserve hue + restore contrast.
//
// Names keep their dark-mode hex anchor (Slate200 etc.) because they
// describe the semantic role, not the literal hex — Slate200 is
// "brightest slate text" regardless of which side of the inversion
// the user's terminal sits on.
var (
	// Foundation — slate
	Slate200    = lipgloss.AdaptiveColor{Light: "#1e293b", Dark: "#e2e8f0"}
	Slate300    = lipgloss.AdaptiveColor{Light: "#334155", Dark: "#cbd5e1"}
	Slate400    = lipgloss.AdaptiveColor{Light: "#475569", Dark: "#94a3b8"}
	Slate500    = lipgloss.AdaptiveColor{Light: "#64748b", Dark: "#64748b"}
	Slate600    = lipgloss.AdaptiveColor{Light: "#94a3b8", Dark: "#475569"}
	Slate700    = lipgloss.AdaptiveColor{Light: "#cbd5e1", Dark: "#334155"}
	Slate800    = lipgloss.AdaptiveColor{Light: "#e2e8f0", Dark: "#1e293b"}
	Slate900    = lipgloss.AdaptiveColor{Light: "#f1f5f9", Dark: "#0f172a"}
	SlateEditor = lipgloss.AdaptiveColor{Light: "#f5f7fa", Dark: "#282c34"}
	SlateChrome = lipgloss.AdaptiveColor{Light: "#eef2f7", Dark: "#21252b"}

	// Signature — teal
	Teal200 = lipgloss.AdaptiveColor{Light: "#115e59", Dark: "#99f6e4"} // teal-800 ↔ teal-200
	Teal300 = lipgloss.AdaptiveColor{Light: "#0f766e", Dark: "#5eead4"} // teal-700 ↔ teal-300
	Teal400 = lipgloss.AdaptiveColor{Light: "#0d9488", Dark: "#2dd4bf"} // teal-600 ↔ teal-400

	// Accents
	Sky300   = lipgloss.AdaptiveColor{Light: "#0369a1", Dark: "#7dd3fc"} // sky-700 ↔ sky-300
	Sky400   = lipgloss.AdaptiveColor{Light: "#0284c7", Dark: "#38bdf8"} // sky-600 ↔ sky-400
	Rose400  = lipgloss.AdaptiveColor{Light: "#e11d48", Dark: "#fb7185"} // rose-600 ↔ rose-400
	Purple   = lipgloss.AdaptiveColor{Light: "#6d28d9", Dark: "#b388ff"} // violet-700 ↔ material A100
	Amber400 = lipgloss.AdaptiveColor{Light: "#d97706", Dark: "#fbbf24"} // amber-600 ↔ amber-400
	Amber700 = lipgloss.AdaptiveColor{Light: "#fbbf24", Dark: "#b45309"} // already deep on dark — flip
)
