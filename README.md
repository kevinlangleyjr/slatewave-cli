<div align="center">

<img src="https://getslatewave.com/brand/icon.png" alt="" height="64" align="middle">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://getslatewave.com/brand/wordmark-light.png">
  <img alt="Slatewave" src="https://getslatewave.com/brand/wordmark.png" height="64" align="middle">
</picture>

# slatewave

The family installer for the [Slatewave](https://getslatewave.com) themes — one CLI to install, activate, and uninstall every theme in the family.

> _Slate below, teal above._

[![CI](https://github.com/kevinlangleyjr/slatewave-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/kevinlangleyjr/slatewave-cli/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/kevinlangleyjr/slatewave-cli?label=release)](https://github.com/kevinlangleyjr/slatewave-cli/releases/latest)
[![License: WTFPL](https://img.shields.io/badge/license-WTFPL-blue.svg)](LICENSE)

</div>

---

## What it does

Replace this:

```sh
mkdir -p "$(bat --config-dir)/themes" && curl -fsSL https://raw.githubusercontent.com/kevinlangleyjr/bat-slatewave/main/Slatewave.tmTheme -o "$(bat --config-dir)/themes/Slatewave.tmTheme" && bat cache --build && echo 'export BAT_THEME=Slatewave' >> ~/.zshrc
```

With this:

```sh
slatewave install bat
```

…and same shape for every theme in the family. Install paths, activation steps, and post-hooks are encoded as **per-theme manifests** (`slatewave.toml`) so the CLI knows exactly what to do for each tool.

## Install

**macOS / Linux** — via Homebrew:

```sh
brew tap kevinlangleyjr/slatewave
brew install slatewave
```

**Linux distros** — native packages on each release:

```sh
# Debian / Ubuntu
sudo dpkg -i slatewave_<version>_linux_<arch>.deb

# Fedora / RHEL / openSUSE
sudo rpm -i slatewave_<version>_linux_<arch>.rpm

# Alpine
sudo apk add --allow-untrusted slatewave_<version>_linux_<arch>.apk

# Arch
sudo pacman -U slatewave-<version>-linux_<arch>.pkg.tar.zst
```

Grab the matching file from the [Releases page](https://github.com/kevinlangleyjr/slatewave-cli/releases). These are unsigned, repo-less artifacts — they install just like a hosted package but you fetch them manually. Hosted apt / rpm / AUR repositories may come later if there's demand.

**Windows** — download the latest `slatewave_windows_*.zip` from the [Releases page](https://github.com/kevinlangleyjr/slatewave-cli/releases) and put `slatewave.exe` somewhere on `%PATH%`.

All platforms can also grab the matching tarball or zip from Releases.

## Commands

```
slatewave init                            # interactive setup wizard — detect + pick + install
slatewave browse                          # interactive TUI list with filter, install, uninstall
slatewave list                            # every theme, with ● / ○ install markers
slatewave list --category=editor          # only one category

slatewave install <theme>                 # install + activate one theme
slatewave install --all                   # bulk install — live dashboard on a TTY, streaming otherwise
slatewave install --category=editor       # bulk install scoped to a category
slatewave install --all --no-interactive  # force streaming output (e.g. for CI logs)
slatewave install <theme> --dry-run       # preview the plan

slatewave update <theme>                  # re-fetch curl assets / git pull clones
slatewave update --all                    # bulk update — live dashboard on a TTY, streaming otherwise
slatewave update --category=editor        # bulk update scoped to a category
slatewave update --all --no-interactive   # force streaming output

slatewave uninstall <theme>               # reverse files, restore backups, remove appended lines
slatewave uninstall --all                 # uninstall every installed theme
slatewave uninstall --category=editor     # uninstall every installed theme in a category
slatewave uninstall <theme> --dry-run     # preview the reversal

slatewave info <theme>                    # preview what `install <theme>` would do (paths, URLs, post-hook)
slatewave status [theme]                  # show install footprint + paths
slatewave doctor                          # diagnose drift across installed themes (read-only)
slatewave doctor --fix                    # interactively remediate stale / missing-tool / orphan rows
```

`list`, `status`, `doctor`, and `info` accept `--json` for machine-readable output — the schemas live in `internal/jsonout/` and are pinned by golden tests so scripts wiring slatewave into a dotfiles bootstrap or CI gate can rely on the shape.

First time? Run `slatewave init` — it detects which Slatewave-supported
tools are on this machine, multi-selects what's worth installing, and
runs the install through a live dashboard.

Already have themes installed and want to spelunk the catalog? Run
`slatewave browse` — `↑/↓` to navigate, `/` to filter, `i` / `u` to
install or uninstall the focused row.

## Platform support

Every manifest declares which OSes it works on, and the CLI filters
its surface area to match — so you only ever see themes you can
actually install.

- **macOS** — every theme except `windows-terminal`
- **Linux** — every theme except `windows-terminal`
- **Windows** — `vscode`, `cursor`, `vscodium`, `antigravity`, `starship`, `oh-my-posh`, and `windows-terminal`. Other themes are hidden from `slatewave list`, `browse`, and shell completion. Trying to install one explicitly errors out with a clean *"<theme> is not supported on windows"* message — no detect runs, no files are written.

The Windows-supported set is intentionally small. Each manifest needs its config paths and detect commands explicitly verified under `cmd.exe` and `PowerShell` before opting in to `supported_os = ["windows"]`. Adding a new tool to that list is a manifest-level change, not a CLI release.

## Shell completions

Theme names and category values autocomplete for every command. Drop into your shell init once:

```sh
# zsh
slatewave completion zsh > "${fpath[1]}/_slatewave"

# bash (Linux)
slatewave completion bash | sudo tee /etc/bash_completion.d/slatewave > /dev/null

# fish
slatewave completion fish > ~/.config/fish/completions/slatewave.fish
```

Then `slatewave install <TAB>` lists every theme, `slatewave uninstall <TAB>` lists only what's installed, `--category=<TAB>` lists just the categories that have at least one theme.

## Terminal theme

slatewave's output auto-adapts to your terminal background — slate text goes dark on a light terminal and light on a dark one, accent stops deepen on light backgrounds so the brand colors stay readable on either side. Detection runs once per process via the OSC 11 escape sequence and is cached.

If your terminal doesn't respond to OSC 11 (some SSH paths, older Windows consoles, anything tunneled through a multiplexer that swallows the response) or you'd rather pin a specific look, set `SLATEWAVE_THEME`:

```sh
export SLATEWAVE_THEME=light   # force the light-bg palette (deep accents, dark text)
export SLATEWAVE_THEME=dark    # force the dark-bg palette (bright accents, light text)
export SLATEWAVE_THEME=auto    # default — let slatewave detect (same as unset)
```

Values are case-insensitive. Anything other than `light` / `dark` falls through to auto-detection. Handy in CI logs where you want deterministic colors, or when your preference inverts what your terminal advertises.

## How it works

Each Slatewave theme repo ships a `slatewave.toml` describing how the CLI should install it:

```toml
[theme]
slug = "btop"
name = "Slatewave for btop"
category = "terminal"
detect_command = "btop --version"

[install]
type = "curl"
url  = "https://raw.githubusercontent.com/kevinlangleyjr/btop-slatewave/main/slatewave.theme"
dest = "$HOME/.config/btop/themes/slatewave.theme"

[activate]
type  = "ini-key"
file  = "$HOME/.config/btop/btop.conf"
key   = "color_theme"
value = "slatewave"
```

The CLI dispatches on `[install].type` (`curl` / `clone` / `vscode-ext` / `marketplace` / `gui-import` / `manual`) and `[activate].type` (`ini-key` / `gitconfig-include` / `shell-rc` / `toml-import` / `yaml-set` / `none`).

Manifests can opt in to specific operating systems with `supported_os = ["darwin", "linux", "windows"]` (defaults to `["darwin", "linux"]` when unset). Per-OS overrides exist for paths and commands too — `clone_dest_windows`, `detect_command_windows`, `verify.command_windows`, and `files_windows` / `line_windows` for shell-rc activates targeting PowerShell. The `vscode-ext` install type also accepts a `cli` field (defaults to `code`, set to `cursor` / `codium` / etc. for VSCode forks).

## Uninstall safety

Every install action is recorded in `~/.config/slatewave/installed.toml` so `slatewave uninstall` can reverse cleanly:

- **Files the CLI created** are deleted
- **Config files the CLI edited** (e.g. `btop.conf`) are restored from a `.bak` made before the edit
- **Lines appended to shell rc** are removed by exact match, plus the `# slatewave` marker
- **Gitconfig includes** are unset by path with `git config --global --unset-all`
- **VSCode-family extensions** (VSCode, Cursor, etc.) are removed via the matching editor CLI: `code --uninstall-extension`, `cursor --uninstall-extension`, etc.

`--dry-run` walks the same plan without writing.

## Design rules

1. **The CLI does not install the underlying tool.** If `bat` isn't on `$PATH`, `slatewave install bat` errors — install via `brew` / your package manager first. (One job at a time: this is a *theme* installer.)
2. **Detection before action.** Every manifest declares a `detect_command` (`bat --version`, `btop --version`); failing detection bails before any filesystem changes.
3. **Idempotent activates.** `shell-rc` lines and `ini-key` edits no-op if already in place. Re-running `slatewave install bat` after a clean install changes nothing.
4. **Backups before edits.** Any manifest whose activate type *replaces* an existing config line (`ini-key`) writes a `.slatewave.<timestamp>.bak` first.
5. **Hide what won't work.** The CLI is OS-aware — themes that don't support the current platform are filtered out of `list`, `browse`, `init`, and tab-completion before the user ever sees them. An explicit `install <slug>` for an unsupported theme errors out before any side-effects.

## Slatewave family

One palette. Every tool. See [getslatewave.com](https://getslatewave.com) for the full list.

## License

WTFPL — Do What The Fuck You Want To Public License. See [LICENSE](LICENSE).
