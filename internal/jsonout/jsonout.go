// Package jsonout owns the wire-format types every cobra command
// emits when --json is set. Each command has one *Output struct here;
// they're the contract automation tools (CI dotfiles bootstrap, status
// dashboards, scripts piping through jq) write against. Adding fields
// is fine; renaming or removing them is a breaking change reviewers
// should catch via `git blame` on this file.
//
// All times use RFC 3339 / UTC via the default time.Time JSON marshal.
// All paths are pre-expanded (~ resolved, $ENV substituted) so JSON
// consumers don't need to re-implement the activator's expandPath.
package jsonout

import "time"

// ListOutput is what `slatewave list --json` prints. Themes is the
// OS-supported set, optionally filtered by --category. Counts mirrors
// the human-readable "N of M installed" footer.
type ListOutput struct {
	Themes []ThemeRow `json:"themes"`
	Counts ListCounts `json:"counts"`
}

// ThemeRow is one row of the list. Optional fields are omitted via
// omitempty when the theme isn't installed (zero-value Time, empty
// install_type / activate_type).
type ThemeRow struct {
	Slug         string     `json:"slug"`
	Name         string     `json:"name"`
	Category     string     `json:"category"`
	Installed    bool       `json:"installed"`
	InstalledAt  *time.Time `json:"installed_at,omitempty"`
	InstallType  string     `json:"install_type,omitempty"`
	ActivateType string     `json:"activate_type,omitempty"`
}

// ListCounts is the footer summary.
type ListCounts struct {
	Total     int `json:"total"`
	Installed int `json:"installed"`
}
