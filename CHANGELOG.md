# Changelog

All notable changes to slatewave land here. Format roughly follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); the project follows [SemVer](https://semver.org/) once 1.0.0 ships — 0.x releases are still allowed to make small breaking changes when the audit calls for it.

## [Unreleased]

## [0.0.22] — 2026-05-13

### Added

- Adaptive light/dark palette. The CLI auto-adapts to the terminal's background via OSC 11 — slate text goes dark on a light terminal and light on a dark one, accent stops deepen on light backgrounds so the brand colors stay readable on either side.
- `SLATEWAVE_THEME` environment variable to force `light` / `dark` / `auto`. Useful when a terminal lies about its background (SSH paths, older Windows consoles), for CI determinism, or when a user's preference inverts the detected mode.
- `SECURITY.md` policy: reporting channels, supported versions, scope, and the hardening already in place.
- `CONTRIBUTING.md` covering setup, adding a theme manifest, adding new install / activate types, the `make check` gate, and commit / PR conventions.
- `CHANGELOG.md` seeded from full git history (this file).
- `slatewave --help` (and every subcommand's `--help`) ends with a single-line footer pointing at https://getslatewave.com so users discovering the CLI find the theme catalog.
- `install.done_message_windows` and `uninstall.done_message_windows` manifest fields override the cross-OS `done_message` on Windows. Mirrors the existing per-OS overrides on `detect_command` and `verify.command`. Used by `oh-my-posh` and `starship` to surface the PowerShell execution-policy hint (`Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned`) that bites every Windows user on a fresh profile.

### Changed

- README surfaces `slatewave info <theme>` in the Commands section and notes that `list` / `status` / `doctor` / `info` accept `--json`. Both features shipped earlier but the README hadn't caught up.

### Removed

- `--interactive` flag on `install` and `update`. The dashboard has been the default for bulk runs on a TTY since v0.0.10; the flag's only remaining behavior was forcing the dashboard on a single-theme install — a narrow case that didn't earn its keep. Wrappers that pass `--interactive` will now fail with cobra's "unknown flag" error. Use the natural defaults for bulk + TTY, or `--no-interactive` to force streaming output. (The deprecation warning landed in the prior release; this is the actual removal.)

## [0.0.21] — 2026-05-12

### Fixed

- Windows CI: jsonout golden files now stay LF on checkout under `autocrlf=true`. `.gitattributes` patterns extended to cover `*.golden.json`; the test also normalizes line endings before comparing as defense in depth.

### Changed

- Dependency bumps: `golang.org/x/sys` 0.38.0 → 0.44.0, `golang.org/x/mod` 0.21.0 → 0.36.0, `spf13/pflag` (transitive).

## [0.0.20] — 2026-05-12

A focused hardening pass against the v0.0.19 audit. Every install / TUI / output-shape concern from `IMPROVEMENTS.md` is closed.

### Added

- `--interactive` is now on a deprecation clock. The flag still works (script wrappers won't break) but emits a one-line stderr nudge pointing users at the default-dashboard behavior; `--no-interactive` is the supported opt-out going forward.
- Golden-file tests for every `--json` shape (list / status / doctor / info). The wire schemas now have an enforceable contract — a field rename or `omitempty` regression fails CI, with a `-update` flag for intentional schema changes.
- Fuzz coverage for the variant version-constraint parser (`matchConstraint`).
- Verbose logging extends to every directory creation and backup operation in the activator, not just the file writes.

### Changed

- TUI install / fix / detect pipelines run under a cancellable `context.Context`. Ctrl-C in the dashboard now kills the in-flight `git clone`, `code --install-extension`, or post-hook subprocess instead of orphaning it; same context cancels on SIGINT for the streaming CLI path via `signal.NotifyContext` at the root.
- `recover()` guards landed on the three TUI worker goroutines so a panic surfaces on the failing row's progress message + cleanly quits the dashboard instead of hanging on a never-sent completion message.
- Goroutine ↔ Update concurrency contract documented inline on `installModel` / `fixModel`.
- `quoteIfNeeded` collapsed to its actual behavior (always-quote unless already quoted); the dead-branch noise is gone.
- Upgrade nag in `cmd/root.go` documented and tested as stderr-only so `slatewave list --json | jq` pipelines stay clean.

### Fixed

- Type assertions on the final TUI model (`final.(installModel)` / `(fixModel)` / `(browseModel)`) now use comma-ok with an explicit error path — fragile invariant turned into a real failure mode if a future refactor swaps model types.

## [0.0.19] — 2026-05-12

### Added

- Bare `slatewave` invocation now prints the banner + help instead of exiting silently (`SilenceUsage` + `SilenceErrors` made the previous behavior invisible).

## [0.0.18] — 2026-05-10

### Added

- Brand-gradient wordmark in the startup banner. The "wave" portion paints teal → rose → purple → amber in slant ASCII while "Slate" stays in slate-200 bold.

## [0.0.17] — 2026-05-09

### Added

- Version-aware install variants. A manifest can dispatch a different install URL / dest / files based on the installed tool's version, driven by `theme.version_regex` against the detect_command output and `install.variants` with simple semver constraints (`<X.Y.Z`, `>=X.Y.Z`, bare `X.Y.Z`, etc.). LSD ships the first real consumer — 1.0.x picks `colors-256.yaml`, 1.1+ picks the hex `colors.yaml`.

### Fixed

- CI: `TestDetectVersion_Regex*` skips on Windows where the shebang-script test seam doesn't apply.

## [0.0.16] — 2026-05-09

### Fixed

- CI: Linux runner stdio-drain hang and three Windows-only test failures from the v0.0.15 push.

### Changed

- `pflag` promoted from indirect to direct dependency.

## [0.0.15] — 2026-05-09

The largest release in the project's history — a multi-week hardening pass landing the entire v0.0.14 audit (`IMPROVEMENTS.md`) in one bundle.

### Added

- `slatewave info <theme>` — read-only manifest inspection with `--json` flag.
- `--json` output on `list`, `status`, `doctor` for scripting / CI dotfiles bootstrap.
- `--verbose` (`-v`) flag streams shell commands, URL fetches, and file writes to stderr for diagnosis.
- Once-a-day update nag against the GitHub Releases API, cached at `~/.config/slatewave/version-check.json`. Suppressed via `SLATEWAVE_NO_UPDATE_CHECK=1` for hermetic CI.
- Bulk `install` / `update` default to the live TUI dashboard on a TTY. Use `--no-interactive` to force streaming output.
- Reconcile in `slatewave list` surfaces a one-line "dropped N stale records" footer when it mutates state.
- Tool-specific done messages now live on the manifest (`[install].done_message`, `[uninstall].done_message`) instead of slug-keyed switches in `cmd/`.
- Windows CI matrix + `make smoke` step.
- JSON-Schema validation of every embedded manifest as a test.
- Fuzz tests for the line-based parsers (`removeShellRCLine`, `tomlImportRewrite`, `yamlSetRewrite`).
- Golden-file regression test for the slant-ASCII banner.

### Changed

- Install / activate / uninstall dispatch consolidated into registry maps instead of parallel switch statements — adding a new install type is now one entry instead of four switch edits.
- The `ui` writer threads through `cobra.Command.Context()` instead of a package-level mutable global.
- Cobra flag globals replaced with `RunE`-scoped reads via a per-command flags struct.
- Stale `go.mod` pin comment dropped.

### Fixed

- `installer.fetchToFile` is now atomic (temp file + rename) — a Ctrl-C mid-write no longer leaves a partial theme file the state record would think is installed.
- Backup restores in the uninstaller go through the same atomic helper.
- File-mode preservation: rewriting a user-owned config (e.g. `.gitconfig` at 0o600) no longer silently downgrades it to 0o644.
- `git config --unset-all include.path <path>` value is now `regexp.QuoteMeta`'d + anchored, so a user with two similar include paths can't lose the wrong one on uninstall.
- `removeShellRCLine` loops until every occurrence is gone — a buggy older version that landed our line twice doesn't leave a duplicate.
- State writes are serialized with an exclusive file lock so two concurrent `slatewave install` runs can't clobber each other.
- HTTP fetch bounded to 60 s and to 100 MiB; text/html responses are rejected before any write.
- `installer.Detect` bounded to 5 s.
- `pickShellRC` matches by exact basename instead of substring contains — `.bashrc` no longer matches `SHELL=/bin/sh` accidentally.

## [0.0.14] — 2026-05-08

### Added

- Slatewave banner displays in `slatewave init` and `slatewave browse`.

## [0.0.13] — 2026-05-07

### Fixed

- Obsidian: theme files are symlinked into the vault rather than copied, so plugin updates reach the vault automatically.

## [0.0.12] — 2026-05-04

### Added

- VSCodium and Antigravity as Open VSX-based `vscode-ext` variants.

### Changed

- README commands list + schema notes synced with what the CLI actually ships.

### Removed

- "Publication pending review" caveats on theme manifests now that Slatewave is live on Open VSX.

## [0.0.11] — 2026-05-04

### Added

- Native Linux packages on every release: `.deb`, `.rpm`, `.apk`, Arch `.pkg.tar.zst`. Unsigned repo-less artifacts you install by hand.
- Cursor as a separate `vscode-ext`-flavored theme (uses `cursor --install-extension`).

### Changed

- README documents the Windows install path and the OS-aware filtering behavior.

## [0.0.10] — 2026-05-04

Windows support lands.

### Added

- `supported_os` manifest field gates a theme to specific OSes. Empty defaults to `["darwin", "linux"]` for backward compatibility; cross-platform manifests must list `"windows"` explicitly.
- Per-OS overrides: `detect_command_windows`, `clone_dest_windows`, `verify.command_windows`, `files_windows` / `line_windows` for PowerShell-profile shell-rc activates.
- VSCode, Starship, Oh My Posh, and Windows Terminal opt in to `supported_os = ["windows"]`.
- `internal/shell` package: command execution routes through `sh -c` on Unix and `cmd /C` on Windows.

### Changed

- `slatewave list` / `browse` / `init` filter to themes supported on the current OS. Tab-completion likewise scopes to the OS.
- Explicit `install <slug>` for an unsupported theme errors out with a clean message before any side effects.

### Fixed

- Sublime Text manifest now points users at the actual activation steps.

## [0.0.9] — 2026-05-04

### Added

- Kitty terminal emulator support.
- Anytype theme.

(Consolidates v0.0.5–v0.0.8 work into a single release for shipping.)

## [0.0.8] — 2026-05-03

### Added

- Powerlevel10k support.

## [0.0.7] — 2026-05-03

### Added

- Multi-file `curl` install — a manifest can fetch multiple URL/dest pairs in one operation.
- Shell-rc activator gains `scaffold` and `insert_before` options for more granular config-file edits.
- Custom comment markers for shell-rc activates (default `# slatewave`).

## [0.0.6] — 2026-05-03

### Added

- `yaml-set` activator for depth-2 YAML config edits. Drives `lsd` automation.
- Raycast theme uses `themes.ray.so` + `verify.trust_state` for opaque post-install storage.
- Xcode support.

## [0.0.5] — 2026-05-03

### Changed

- Dependency bumps: `goreleaser-action` 6→7, `actions/checkout` 4→6, `actions/setup-go` 5→6, `golangci-lint-action` 8→9.

## [0.0.4] — 2026-05-03

### Fixed

- Apply `gofmt` to test files to unblock CI.

## [0.0.3] — 2026-05-03

### Added

- `slatewave update` command — re-fetch curl assets / `git pull` clones without re-running activation.
- `slatewave browse` command — interactive TUI list with filter, install, uninstall.
- `slatewave doctor` for read-only drift diagnostics across installed themes.
- `slatewave doctor --fix` interactively remediates stale / missing-tool / orphan rows via the TUI fix dashboard.
- `slatewave init` first-run wizard — detect, multi-select, install.
- TUI install dashboard for live progress rendering during bulk operations.
- TUI fix dashboard, parameterized header (reused by `slatewave update --interactive`).
- Shell completions with theme-aware suggestions (zsh / bash / fish).
- `uninstall --all` and `--category` for bulk reversal.
- Per-platform `clone_dest_*` manifest field.
- Tool-specific done messages after uninstall to mirror install's `done_message`.
- Closest-slug suggestion on install typo via Levenshtein.
- `--category` flag on `slatewave list` for parity.
- JSON Schema + taplo config for the manifest format.
- Bubbletea / huh / bubbles dependencies for the TUI surface.
- Multi-OS detection: Logseq, Slack, Obsidian on Linux as well as macOS; alacritty / wezterm / ghostty via PATH or `.app` bundle.
- CI / release / license badges in README.
- Dependabot config for go-modules and github-actions.
- CI test matrix across Ubuntu + macOS.

### Changed

- `cmd/install.go` `--interactive` routes through the dashboard.
- `state.AllSlugs` returns sorted slugs.
- `slatewave list`, `status` gain `Long` descriptions.
- `make smoke` iterates every embedded manifest, not just five.
- `goreleaser` ships `windows/amd64` binaries.
- README documents `init`, `browse`, `doctor`, `doctor --fix`, `--interactive`.
- Alfred's verify command checks Alfred's imported theme location, not `~/Downloads`.
- `verify.trust_state` escape hatch for manifests whose post-install storage is opaque.

### Fixed

- `vscode-ext` uninstall tolerates already-removed extensions.
- TUI install prints post-install instructions block after the dashboard exits.
- `ini-key` activator handles missing files and missing keys.

### Removed

- Unimplemented `clipboard` install type and `neovim-plugin-spec` activate type.
- Stale `sublime-text` TODO.

## [0.0.2] — 2026-05-02

### Added

- `install --all` and `--category` for bulk install.
- Auto-publish Homebrew formula on each release.
- `toml-import` activator for TOML import-array configs.
- Manifest authoring for the marketplace + manual-paste, clone-based, gui-import (iTerm2, Alfred), and curl-and-activate theme batches.

## [0.0.1] — 2026-05-02

The initial scaffold.

### Added

- Cobra command tree with `install`, `uninstall`, `list`.
- Manifest schema (`slatewave.toml`) and 5 v0.1 manifests.
- Install dispatch (`curl`, `clone`, `vscode-ext`, `marketplace`, `manual`).
- Activate dispatch (`ini-key`, `gitconfig-include`, `shell-rc`).
- Uninstaller — files-created reversal, backup restore, appended-line removal, gitconfig-include unset, vscode-ext removal.
- `state` package recording every install's footprint to `~/.config/slatewave/installed.toml`.
- `ui` package — palette, lipgloss styles, log helpers.
- Test coverage for the installer, activator, manifest, and state packages.
- `Makefile` with common dev targets.
- CI workflow — gofmt, vet, race tests, golangci-lint.
- Release pipeline — goreleaser on tag push.
- `slatewave list` reconciles state with reality (drops verify-failing records).

[Unreleased]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.22...HEAD
[0.0.22]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.21...v0.0.22
[0.0.21]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.20...v0.0.21
[0.0.20]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.19...v0.0.20
[0.0.19]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.18...v0.0.19
[0.0.18]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.17...v0.0.18
[0.0.17]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.16...v0.0.17
[0.0.16]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.15...v0.0.16
[0.0.15]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.14...v0.0.15
[0.0.14]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.13...v0.0.14
[0.0.13]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.12...v0.0.13
[0.0.12]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.11...v0.0.12
[0.0.11]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.10...v0.0.11
[0.0.10]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.9...v0.0.10
[0.0.9]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.8...v0.0.9
[0.0.8]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.7...v0.0.8
[0.0.7]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.6...v0.0.7
[0.0.6]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.5...v0.0.6
[0.0.5]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/kevinlangleyjr/slatewave-cli/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/kevinlangleyjr/slatewave-cli/releases/tag/v0.0.1
