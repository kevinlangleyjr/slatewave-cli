// Package manifest models the per-theme slatewave.toml file each
// Slatewave theme repo ships at its root. The manifest tells the
// CLI everything it needs to install, activate, verify, and reverse
// the theme — without reading the theme's prose docs.
//
// See the v0.1 manifests in slatewave-cli/manifests/ for examples.
package manifest

// Theme is the top-level shape of a slatewave.toml file.
type Theme struct {
	Theme     Meta      `toml:"theme"`
	Install   Install   `toml:"install"`
	Activate  Activate  `toml:"activate"`
	Verify    Verify    `toml:"verify"`
	Uninstall Uninstall `toml:"uninstall"`
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
	// SupportedOS lists the runtime.GOOS values this manifest works on.
	// Empty/unset → defaults to ["darwin", "linux"], preserving the
	// pre-Windows behavior of every existing manifest. Cross-platform
	// manifests must opt in to "windows" explicitly so that unaudited
	// manifests can't accidentally be offered (and fail mid-install)
	// to Windows users.
	SupportedOS []string `toml:"supported_os"`
	// DetectCommand runs to verify the underlying tool is present
	// before any install action ("bat --version", "btop --version").
	// If non-zero exit → CLI errors with "<tool> not detected" and
	// does NOT fall back to installing the tool itself (per design).
	DetectCommand string `toml:"detect_command"`
	// DetectCommandWindows overrides DetectCommand on Windows. Empty
	// → fall back to DetectCommand. Required for manifests whose
	// unix detect uses POSIX-only forms like `command -v <tool>`;
	// use `where <tool>` or `<tool> --version` on Windows.
	DetectCommandWindows string `toml:"detect_command_windows"`
	// VersionRegex extracts an X.Y.Z version from DetectCommand stdout.
	// Capture group 1 must hold the version. Required when Install.Variants
	// is non-empty (manifest validation rejects variants without it);
	// otherwise unused. Lets one manifest dispatch its install URL based
	// on the installed tool's version — e.g. lsd 1.0.x silently ignores
	// hex colors and needs an ANSI-256 variant.
	VersionRegex string `toml:"version_regex"`
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
	// CLI overrides which binary the vscode-ext handler shells out to.
	// Defaults to "code" (VSCode and most VSCode-derived editors honor
	// `code` as a symlink). Set to "cursor" for Cursor, "codium" for
	// VSCodium, etc. — every editor in the family accepts the same
	// `--install-extension <id>` / `--list-extensions` / `--uninstall-extension <id>`
	// flags, so the only thing that varies is the binary name.
	CLI string `toml:"cli"`

	// manual-specific
	Instructions []string `toml:"instructions"`

	// Variants are version-conditional install overrides. When a variant's
	// WhenVersion constraint matches the version captured via
	// Meta.VersionRegex, its URL/Dest/Files override the parent fields.
	// First match wins (declaration order). Empty slice = no version logic.
	// Backward-compatible: every existing manifest leaves this empty and
	// behavior is unchanged.
	Variants []InstallVariant `toml:"variants"`

	// Optional post-install hook — a command to run after the file is
	// in place (e.g. `bat cache --build`).
	Post *PostHook `toml:"post"`

	// DoneMessage is the success line printed after a non-bulk install
	// completes ("bat picks up the new theme on its next invocation.",
	// "Restart your shell or `source` your rc file."). Empty → CLI
	// prints the generic "Slatewave is installed." default. Lets a
	// theme give tool-specific guidance without a code change.
	DoneMessage string `toml:"done_message"`

	// DoneMessageWindows overrides DoneMessage on Windows. Empty → fall
	// back to DoneMessage. Used when the post-install guidance is
	// genuinely different on Windows — most commonly shell-rc activates
	// targeting a PowerShell profile, where users hit Set-ExecutionPolicy
	// before the appended line runs.
	DoneMessageWindows string `toml:"done_message_windows"`
}

// InstallFile is one entry in a multi-file curl install. URL is the
// source, Dest is the on-disk path (supports ~ and $ENV expansion).
type InstallFile struct {
	URL  string `toml:"url"`
	Dest string `toml:"dest"`
}

// InstallVariant overrides Install.URL, Install.Dest, and/or Install.Files
// when WhenVersion matches the detected tool version. Only those three
// fields are honored as overrides in v1; everything else falls through to
// the parent Install. When a variant sets Files, the parent's URL/Dest
// are cleared (variants flip the install between single-file and
// multi-file shape as a unit).
type InstallVariant struct {
	// WhenVersion is the constraint expression: "<X.Y.Z", "<=X.Y.Z",
	// ">X.Y.Z", ">=X.Y.Z", "=X.Y.Z", or bare "X.Y.Z" (treated as exact
	// match). Anything else is rejected at install time.
	WhenVersion string        `toml:"when_version"`
	URL         string        `toml:"url"`
	Dest        string        `toml:"dest"`
	Files       []InstallFile `toml:"files"`
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
	// FilesWindows / LineWindows are the PowerShell-profile equivalents,
	// used when runtime.GOOS == "windows". A shell-rc manifest that
	// declares supported_os = [..., "windows"] must set both — without
	// them the activator errors out cleanly rather than appending bash
	// syntax to a .ps1 file.
	FilesWindows []string `toml:"files_windows"`
	LineWindows  string   `toml:"line_windows"`
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

// Uninstall carries optional per-theme post-uninstall guidance. The
// uninstall pipeline is fully manifest-driven via Install + Activate;
// this block only exists today for DoneMessage. Add more fields here
// as new uninstall-side concerns appear (custom warnings, conditional
// cleanup hints, etc.).
type Uninstall struct {
	// DoneMessage is the success line printed after `slatewave uninstall <slug>`
	// completes ("Reverted. Quit and relaunch Ghostty to see your
	// original colors..."). Empty → CLI prints the generic "Reverted."
	// default. Themes for tools that load config once at launch (terminals,
	// editor processes) should set this to remind the user to relaunch.
	DoneMessage string `toml:"done_message"`

	// DoneMessageWindows overrides DoneMessage on Windows. Same shape as
	// Install.DoneMessageWindows — used when uninstall guidance is
	// Windows-specific (PowerShell session reset, etc.).
	DoneMessageWindows string `toml:"done_message_windows"`
}

// Verify holds an optional smoke-test command that doctor + list reconcile
// run to detect drift. Empty Command → CLI trusts the state record (the
// install was recorded, no way to check, assume good).
type Verify struct {
	Command string `toml:"command"`
	// CommandWindows overrides Command on Windows. Empty → fall back
	// to Command. Use when the unix verify shells out to `test -f` /
	// `grep` / etc. and a cmd.exe-parseable equivalent is needed.
	CommandWindows string `toml:"command_windows"`
	Expect         string `toml:"expect"` // optional: expected substring in stdout
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
