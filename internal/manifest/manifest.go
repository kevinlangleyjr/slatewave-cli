// Package manifest models the per-theme slatewave.toml file each
// Slatewave theme repo ships at its root. The manifest tells the
// CLI everything it needs to install, activate, verify, and reverse
// the theme — without reading the theme's prose docs.
//
// See the v0.1 manifests in slatewave-cli/manifests/ for examples.
package manifest

// Theme is the top-level shape of a slatewave.toml file.
type Theme struct {
	Theme    Meta     `toml:"theme"`
	Install  Install  `toml:"install"`
	Activate Activate `toml:"activate"`
	Verify   Verify   `toml:"verify"`
}

// Meta holds the theme's identity + tool detection.
type Meta struct {
	// Slug is the canonical CLI identifier (e.g. "bat", "btop"). Must
	// match the theme's slug on getslatewave.
	Slug string `toml:"slug"`
	// Name is the human-readable display name ("Slatewave for bat").
	Name string `toml:"name"`
	// Category mirrors the website's category enum.
	Category string `toml:"category"`
	// DetectCommand runs to verify the underlying tool is present
	// before any install action ("bat --version", "btop --version").
	// If non-zero exit → CLI errors with "<tool> not detected" and
	// does NOT fall back to installing the tool itself (per design).
	DetectCommand string `toml:"detect_command"`
}

// Install describes how to put the theme files in place.
type Install struct {
	// Type is the install dispatch tag. Each value maps to a function
	// in internal/installer.
	//
	//   curl              — fetch URL → write to Dest
	//   clone             — git clone Repo → CloneDest
	//   vscode-ext        — code --install-extension Identifier
	//   marketplace       — open URL in browser, exit (no automation)
	//   gui-import        — write file to Dest, then `open Dest`
	//   manual            — print Instructions and exit
	Type string `toml:"type"`

	// Common fields (used by multiple types)
	URL  string `toml:"url"`  // curl, marketplace
	Dest string `toml:"dest"` // curl, gui-import — destination path on disk

	// clone-specific
	Repo      string `toml:"repo"`       // git URL
	CloneDest string `toml:"clone_dest"` // default destination (used when no per-OS override fits)
	// Per-platform overrides — when set, take precedence over CloneDest
	// on the matching runtime.GOOS. Lets one manifest target tools whose
	// config dirs differ between macOS and Linux (sublime-text is the
	// canonical case: ~/Library/... on macOS, ~/.config/... on Linux).
	CloneDestDarwin  string `toml:"clone_dest_darwin"`
	CloneDestLinux   string `toml:"clone_dest_linux"`
	CloneDestWindows string `toml:"clone_dest_windows"`

	// vscode-ext-specific
	Identifier string `toml:"identifier"` // e.g. "kevinlangleyjr.slatewave"

	// manual-specific
	Instructions []string `toml:"instructions"`

	// Optional post-install hook — a command to run after the file is
	// in place (e.g. `bat cache --build`).
	Post *PostHook `toml:"post"`
}

// PostHook runs after a successful install.
type PostHook struct {
	// Description is shown as the step label ("Rebuilding bat cache").
	Description string `toml:"description"`
	// Command is the shell command to run.
	Command string `toml:"command"`
}

// Activate describes how to flip the user's config to point at the
// installed theme. Some themes have no separate activation step (the
// install step also activates) — those use Type = "none".
type Activate struct {
	// Type is the activate dispatch tag.
	//
	//   none                 — install step also activates
	//   ini-key              — set Key=Value in File
	//   gitconfig-include    — add include.path to ~/.gitconfig
	//   shell-rc             — append Line to ~/.zshrc / ~/.bashrc
	//   toml-import          — add an import line to a TOML config
	Type string `toml:"type"`

	// ini-key fields
	File  string `toml:"file"`
	Key   string `toml:"key"`
	Value string `toml:"value"`

	// gitconfig-include fields
	IncludePath string `toml:"include_path"`

	// shell-rc fields
	Files []string `toml:"files"` // candidate rc files (CLI picks the user's)
	Line  string   `toml:"line"`

	// toml-import fields
	TOMLPath string `toml:"toml_path"` // e.g. ~/.config/alacritty/alacritty.toml
	Import   string `toml:"import"`    // e.g. import = ["~/.config/alacritty/themes/slatewave.toml"]
}

// Verify holds an optional smoke-test command that doctor + list reconcile
// run to detect drift. Empty Command → CLI trusts the state record (the
// install was recorded, no way to check, assume good).
type Verify struct {
	Command string `toml:"command"`
	Expect  string `toml:"expect"` // optional: expected substring in stdout
	// TrustState short-circuits verify to "always passes if state has the
	// record." Use for installs where the post-install location is opaque
	// to us — Alfred imports a .alfredappearance into Alfred's own internal
	// preferences package under a UUID-named subdir we shouldn't peek at,
	// marketplace plugins live in IDE-managed dirs, etc. Setting this
	// avoids false-stale doctor reports when the user later cleans the
	// transient install asset (e.g. the file we dropped in ~/Downloads).
	//
	// Prefer a real Command when there's any way to write one — TrustState
	// is the escape hatch, not the default.
	TrustState bool `toml:"trust_state"`
}
