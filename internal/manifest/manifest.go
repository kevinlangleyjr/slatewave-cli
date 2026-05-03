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

	// Files lets a curl install fetch multiple files in one shot — used
	// when a theme ships more than one asset that has to land alongside
	// the user's config (wezterm's `slatewave-full.lua` + its `slatewave.lua`
	// dependency, for example). When set, URL/Dest must be empty; doCurl
	// iterates Files and records each dest as a CreatedPath so uninstall
	// reverses cleanly.
	Files []InstallFile `toml:"files"`

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

// InstallFile is one entry in a multi-file curl install. URL is the
// source, Dest is the on-disk path (supports ~ and $ENV expansion).
type InstallFile struct {
	URL  string `toml:"url"`
	Dest string `toml:"dest"`
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
	//   yaml-set             — set one or more nested keys in a YAML file
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
	// Scaffold is written verbatim as the full file contents when the
	// chosen target is missing or empty (whitespace-only). For configs
	// like wezterm.lua where appending a single line to an empty file
	// produces invalid syntax — `config.color_scheme = "Slatewave"` needs
	// `local config = wezterm.config_builder()` to be defined first.
	// Scaffold should already include the activation line, since the line
	// is only re-appended when the file already had real content.
	Scaffold string `toml:"scaffold"`
	// InsertBefore picks where to splice Line into a file that already
	// has real content. Matched as a substring against each trimmed line;
	// the activation line goes immediately above the first hit. Used for
	// configs where "append to end of file" is wrong because control flow
	// would never reach the appended line — e.g. wezterm.lua's `return
	// config` stops Lua from seeing anything below it. Falls back to the
	// append-with-marker behavior when the anchor isn't found.
	InsertBefore string `toml:"insert_before"`
	// CommentPrefix overrides the marker comment style for shell-rc.
	// Defaults to "#" — correct for shell rc files, gitconfig, ssh
	// config, etc. Lua targets must set this to "--" so the marker
	// (`-- slatewave`) is a valid comment in the host file. Anything
	// else (";", "//") is fine too.
	CommentPrefix string `toml:"comment_prefix"`

	// toml-import fields
	TOMLPath string `toml:"toml_path"` // e.g. ~/.config/alacritty/alacritty.toml
	Import   string `toml:"import"`    // e.g. import = ["~/.config/alacritty/themes/slatewave.toml"]

	// yaml-set fields
	YAMLPath string     `toml:"yaml_path"` // e.g. ~/.config/lsd/config.yaml
	YAMLSet  []YAMLPair `toml:"yaml_set"`  // nested-key sets to apply
}

// YAMLPair is one (path, value) entry in a yaml-set activator. Path is a
// dotted path of depth 2 — "parent.child" — and Value is written as the
// child's scalar value.
type YAMLPair struct {
	Path  string `toml:"path"`
	Value string `toml:"value"`
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
