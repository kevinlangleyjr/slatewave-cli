package ui

import (
	"fmt"
	"strings"
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

// BannerHeight is the visible row count of Banner() — wordmark rows
// plus wave underline plus tagline. TUIs embedding the banner in
// View() subtract this (plus any extra separator rows they add) from
// the height they pass to list/viewport sizing.
var BannerHeight = len(bannerLines) + 2

// Banner returns the colorized startup banner without a trailing
// newline: wordmark in Title (slate-200 bold), wave in Accent (teal),
// tagline in Muted.
func Banner() string {
	var b strings.Builder
	for i, line := range bannerLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(Title.Render(line))
	}
	b.WriteByte('\n')
	b.WriteString(Accent.Render(bannerWave))
	b.WriteByte('\n')
	b.WriteString(Muted.Render(bannerTagline))
	return b.String()
}

// PrintBanner writes the banner followed by a blank-line separator
// to W. Command entry points call this for the startup brand moment.
func PrintBanner() {
	fmt.Fprintln(W, Banner())
	fmt.Fprintln(W)
}
