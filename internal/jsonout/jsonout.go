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

// StatusOutput is what `slatewave status [theme] --json` prints. When
// no slug is given, every installed theme is listed; with a slug,
// the slice has length 1 (or is omitted entirely if the slug isn't
// installed and the command surfaces an error to stderr instead).
type StatusOutput struct {
	Themes []StatusEntry `json:"themes"`
}

// StatusEntry mirrors what `slatewave status` prints per theme: the
// recorded install footprint plus enough identity for downstream tools
// to cross-reference with a manifest registry.
type StatusEntry struct {
	Slug         string        `json:"slug"`
	Name         string        `json:"name"`
	InstalledAt  time.Time     `json:"installed_at"`
	InstallType  string        `json:"install_type"`
	ActivateType string        `json:"activate_type,omitempty"`
	CreatedPaths []string      `json:"created_paths,omitempty"`
	AppendedLine *AppendedLine `json:"appended_line,omitempty"`
	Backups      []Backup      `json:"backups,omitempty"`
}

// AppendedLine is the JSON twin of state.Appended.
type AppendedLine struct {
	File string `json:"file"`
	Line string `json:"line"`
}

// Backup is the JSON twin of state.Backup.
type Backup struct {
	Original string `json:"original"`
	Path     string `json:"path"`
}

// DoctorOutput is what `slatewave doctor --json` prints. Per-theme
// rows carry the diagnosis verbatim; summary mirrors the human-
// readable footer counts so a tool can act on either shape.
type DoctorOutput struct {
	Summary DoctorSummary `json:"summary"`
	Themes  []DoctorRow   `json:"themes"`
}

// DoctorRow is one diagnosed theme. Status is one of the canonical
// strings: "healthy", "stale", "missing-tool", "orphan". Detail and
// remedy are empty for healthy rows.
type DoctorRow struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Remedy string `json:"remedy,omitempty"`
}

// DoctorSummary is the footer counts (one int per status bucket).
type DoctorSummary struct {
	Healthy     int `json:"healthy"`
	Stale       int `json:"stale"`
	MissingTool int `json:"missing_tool"`
	Orphan      int `json:"orphan"`
}
