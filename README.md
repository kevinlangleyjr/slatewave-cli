<div align="center">

<img src="https://getslatewave.com/brand/icon.png" alt="" height="64" align="middle">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://getslatewave.com/brand/wordmark-light.png">
  <img alt="Slatewave" src="https://getslatewave.com/brand/wordmark.png" height="64" align="middle">
</picture>

# slatewave

The family installer for the [Slatewave](https://getslatewave.com) themes — one CLI to install, activate, and uninstall every theme in the family.

> _Slate below, teal above._

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
slatewave list                  # every theme, with ● / ○ install markers
slatewave install <theme>       # install + activate one theme
slatewave install <theme> --dry-run    # preview the plan
slatewave uninstall <theme>     # reverse files, restore backups, remove appended lines
slatewave status [theme]        # show install footprint + paths
```

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

The CLI dispatches on `[install].type` (`curl` / `clone` / `vscode-ext` / `marketplace` / `gui-import` / `manual`) and `[activate].type` (`ini-key` / `gitconfig-include` / `shell-rc` / `none`).

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
