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

```sh
brew tap kevinlangleyjr/slatewave
brew install slatewave
```

Or from a release binary on the [Releases page](https://github.com/kevinlangleyjr/slatewave-cli/releases).

## Commands

```
slatewave init                          # interactive setup wizard — detect + pick + install
slatewave browse                        # interactive TUI list with filter, install, uninstall
slatewave list                          # every theme, with ● / ○ install markers
slatewave install <theme>               # install + activate one theme
slatewave install --all                 # install every shipping theme
slatewave install --category=editor     # install every theme in a category
slatewave install --interactive <flags> # live progress dashboard instead of streamed steps
slatewave install <theme> --dry-run     # preview the plan
slatewave update <theme>                # re-fetch curl assets / git pull clones
slatewave update --all                  # update every installed theme
slatewave uninstall <theme>             # reverse files, restore backups, remove appended lines
slatewave status [theme]                # show install footprint + paths
slatewave doctor                        # diagnose drift across installed themes (read-only)
slatewave doctor --fix                  # interactively remediate stale / missing-tool / orphan rows
```

First time? Run `slatewave init` — it detects which Slatewave-supported
tools are on this machine, multi-selects what's worth installing, and
runs the install through a live dashboard.

Already have themes installed and want to spelunk the catalog? Run
`slatewave browse` — `↑/↓` to navigate, `/` to filter, `i` / `u` to
install or uninstall the focused row.

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

The CLI dispatches on `[install].type` (`curl` / `clone` / `vscode-ext` / `marketplace` / `gui-import` / `manual`) and `[activate].type` (`ini-key` / `gitconfig-include` / `shell-rc` / `toml-import` / `none`).

## Uninstall safety

Every install action is recorded in `~/.config/slatewave/installed.toml` so `slatewave uninstall` can reverse cleanly:

- **Files the CLI created** are deleted
- **Config files the CLI edited** (e.g. `btop.conf`) are restored from a `.bak` made before the edit
- **Lines appended to shell rc** are removed by exact match, plus the `# slatewave` marker
- **Gitconfig includes** are unset by path with `git config --global --unset-all`
- **VSCode extensions** are removed via `code --uninstall-extension`

`--dry-run` walks the same plan without writing.

## Design rules

1. **The CLI does not install the underlying tool.** If `bat` isn't on `$PATH`, `slatewave install bat` errors — install via `brew` / your package manager first. (One job at a time: this is a *theme* installer.)
2. **Detection before action.** Every manifest declares a `detect_command` (`bat --version`, `btop --version`); failing detection bails before any filesystem changes.
3. **Idempotent activates.** `shell-rc` lines and `ini-key` edits no-op if already in place. Re-running `slatewave install bat` after a clean install changes nothing.
4. **Backups before edits.** Any manifest whose activate type *replaces* an existing config line (`ini-key`) writes a `.slatewave.<timestamp>.bak` first.

## Slatewave family

One palette. Every tool. See [getslatewave.com](https://getslatewave.com) for the full list.

## License

WTFPL — Do What The Fuck You Want To Public License. See [LICENSE](LICENSE).
