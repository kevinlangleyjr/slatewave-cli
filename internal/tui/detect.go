// Package tui owns the interactive surfaces of the slatewave CLI —
// `slatewave init` today, more in later releases. It depends on
// bubbletea / huh / bubbles. Everything below internal/installer,
// internal/activator, internal/manifest, internal/state stays
// TUI-free so the core remains testable without bubbletea mocks.
package tui

import (
	"context"
	"sync"
	"time"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/shell"
)

// detectTimeout caps each individual detect_command so a hung shell
// doesn't freeze the wizard. Three seconds is generous — most detect
// commands are `command -v <tool>` or `test -d <path>`.
const detectTimeout = 3 * time.Second

// DetectResult is one row in the wizard's input table: a theme plus
// whether its underlying tool is present on this machine and whether
// the user has already installed Slatewave for it.
type DetectResult struct {
	Theme     manifest.Theme
	Present   bool
	Installed bool
}

// DetectAll runs every theme's detect_command in parallel and returns
// the results in the same order as the input slice. Used by `slatewave
// init` to figure out which themes are even worth offering — there's
// no point asking the user about Slatewave for btop if they don't
// have btop installed.
func DetectAll(themes []manifest.Theme, installed map[string]bool) []DetectResult {
	out := make([]DetectResult, len(themes))
	var wg sync.WaitGroup
	for i, th := range themes {
		i, th := i, th
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = DetectResult{
				Theme:     th,
				Present:   detectOne(th),
				Installed: installed[th.Theme.Slug],
			}
		}()
	}
	wg.Wait()
	return out
}

func detectOne(th manifest.Theme) bool {
	cmd := manifest.DetectCommandFor(th)
	if cmd == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), detectTimeout)
	defer cancel()
	_, err := shell.Run(ctx, cmd)
	return err == nil
}
