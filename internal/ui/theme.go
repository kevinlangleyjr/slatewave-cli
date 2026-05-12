package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ThemeEnvVar names the env var users can set to force a specific
// terminal-background mode. Accepted values (case-insensitive):
//
//	light   force the light-bg palette (deep accents, dark text)
//	dark    force the dark-bg palette  (bright accents, light text)
//	auto    (default) let lipgloss detect via OSC 11
//
// Useful when a terminal lies about its background (certain SSH paths
// where the host can't query the client's emulator, older Windows
// consoles), when CI or golden tests need deterministic colors, and
// when the user's preference inverts the detected mode.
//
// Detection without this override runs once per process at first style
// render and is cached, so unsetting the env var mid-session has no
// effect until the next invocation.
const ThemeEnvVar = "SLATEWAVE_THEME"

// init reads SLATEWAVE_THEME at package init so the override applies
// before any style hits a terminal. lipgloss.SetHasDarkBackground
// short-circuits the OSC 11 query path entirely — explicit user intent
// always wins over auto-detection.
func init() {
	applyThemeOverride(os.Getenv(ThemeEnvVar))
}

// applyThemeOverride is the testable seam behind init. Unknown / empty
// values fall through to lipgloss's automatic detection — the function
// only ever forces a mode, never clears one that was already set.
func applyThemeOverride(value string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "light":
		lipgloss.SetHasDarkBackground(false)
	case "dark":
		lipgloss.SetHasDarkBackground(true)
	}
}
