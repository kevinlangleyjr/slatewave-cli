package tui

import (
	"errors"
	"sort"

	"github.com/charmbracelet/huh"
)

// ErrAborted is returned when the user cancels the wizard (Ctrl+C or
// equivalent). Callers should treat this as a clean exit, not a real
// error. Mirrors huh.ErrUserAborted but stays inside our package so
// cmd/ doesn't have to import huh directly.
var ErrAborted = errors.New("wizard aborted by user")

// PickThemes shows a multi-select grouped by category so the user can
// check which themes they want to install. Already-installed themes
// and themes whose tool is missing are filtered out — the wizard only
// offers what's actually installable right now.
//
// Returns the selected slugs in the order huh produced them. Returns
// (nil, nil) if there's nothing to offer (all tools missing or already
// installed). Returns (nil, ErrAborted) if the user cancels.
func PickThemes(detected []DetectResult) ([]string, error) {
	available := offerable(detected)
	if len(available) == 0 {
		return nil, nil
	}

	options := make([]huh.Option[string], len(available))
	for i, d := range available {
		label := d.Theme.Theme.Name + "  " + paren(d.Theme.Theme.Category)
		options[i] = huh.NewOption(label, d.Theme.Theme.Slug)
	}

	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Slatewave for which tools?").
				Description("Spacebar toggles, enter confirms.").
				Options(options...).
				Value(&selected),
		),
	)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ErrAborted
		}
		return nil, err
	}
	return selected, nil
}

// offerable returns the detected themes worth offering — tool present,
// not already installed — sorted by category then slug for stable
// presentation order.
func offerable(detected []DetectResult) []DetectResult {
	var out []DetectResult
	for _, d := range detected {
		if d.Present && !d.Installed {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Theme.Theme.Category != out[j].Theme.Theme.Category {
			return out[i].Theme.Theme.Category < out[j].Theme.Theme.Category
		}
		return out[i].Theme.Theme.Slug < out[j].Theme.Theme.Slug
	})
	return out
}

func paren(s string) string { return "(" + s + ")" }
