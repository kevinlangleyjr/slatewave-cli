package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Slatewave wordmark in figlet "slant" style. Stored line-by-line so
// the slant geometry is easy to read in source — eyeballing changes
// is harder in a single multi-line raw literal.
var bannerLines = []string{
	`    _____ __      __`,
	`   / ___// /___ _/ /____ _      ______ __   _____`,
	"   \\__ \\/ / __ `/ __/ _ \\ | /| / / __ `/ | / / _ \\",
	`  ___/ / / /_/ / /_/  __/ |/ |/ / /_/ /| |/ /  __/`,
	` /____/_/\__,_/\__/\___/|__/|__/\__,_/ |___/\___/`,
}

const (
	bannerWave    = "  ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~"
	bannerTagline = "      one palette across every tool"
)

// Letter-start columns on the bottom (un-slanted) row of the wordmark.
// "Slate" sits in cols 0-23; the four "wave" letters start at these
// offsets. Each row above is shifted +1 col due to the slant, so we
// compute per-line offsets as base + (rows-1 - lineIdx).
const (
	waveColW = 24
	waveColA = 32
	waveColV = 38
	waveColE = 44
)

// Per-letter styles for the "wave" portion. Mirrors the teal → rose →
// purple → amber gradient on the brand wordmark
// (getslatewave.com/brand/wordmark-light.png).
var (
	waveStyleW = lipgloss.NewStyle().Foreground(Teal400).Bold(true)
	waveStyleA = lipgloss.NewStyle().Foreground(Rose400).Bold(true)
	waveStyleV = lipgloss.NewStyle().Foreground(Purple).Bold(true)
	waveStyleE = lipgloss.NewStyle().Foreground(Amber400).Bold(true)
)

// BannerHeight is the visible row count of Banner() — wordmark rows
// plus wave underline plus tagline. TUIs embedding the banner in
// View() subtract this (plus any extra separator rows they add) from
// the height they pass to list/viewport sizing.
var BannerHeight = len(bannerLines) + 2

// colorizeWordmarkLine renders a single wordmark row, painting "Slate"
// in Title (slate-200 bold) and the four "wave" letters in their brand
// gradient colors. The slant geometry shifts each row's letter
// boundaries by one column relative to the bottom row.
func colorizeWordmarkLine(line string, lineIdx int) string {
	shift := len(bannerLines) - 1 - lineIdx
	clamp := func(c int) int {
		if c < 0 {
			return 0
		}
		if c > len(line) {
			return len(line)
		}
		return c
	}
	wStart := clamp(waveColW + shift)
	aStart := clamp(waveColA + shift)
	vStart := clamp(waveColV + shift)
	eStart := clamp(waveColE + shift)

	var b strings.Builder
	b.WriteString(Title.Render(line[:wStart]))
	b.WriteString(waveStyleW.Render(line[wStart:aStart]))
	b.WriteString(waveStyleA.Render(line[aStart:vStart]))
	b.WriteString(waveStyleV.Render(line[vStart:eStart]))
	b.WriteString(waveStyleE.Render(line[eStart:]))
	return b.String()
}

// Banner returns the colorized startup banner without a trailing
// newline: "Slate" in Title (slate-200 bold), "wave" in the brand
// teal → rose → purple → amber gradient, wave underline in Accent
// (teal), tagline in Muted.
func Banner() string {
	var b strings.Builder
	for i, line := range bannerLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(colorizeWordmarkLine(line, i))
	}
	b.WriteByte('\n')
	b.WriteString(Accent.Render(bannerWave))
	b.WriteByte('\n')
	b.WriteString(Muted.Render(bannerTagline))
	return b.String()
}

// PrintBanner writes the banner followed by a blank-line separator
// to w. Command entry points call this for the startup brand moment.
func PrintBanner(w io.Writer) {
	fmt.Fprintln(w, Banner())
	fmt.Fprintln(w)
}
