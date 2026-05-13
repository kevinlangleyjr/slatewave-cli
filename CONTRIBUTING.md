# Contributing to slatewave

Thanks for being here. The two most common contributions are **a new theme manifest** (you want slatewave to install Slatewave for one more tool) and **a bug fix in the install pipeline**. The notes below cover both, plus the conventions that keep PRs landing cleanly.

## Getting set up

Requirements: Go 1.24+ and a working `git`. No other toolchain dependencies — the entire CLI builds from `go build .`.

```sh
git clone https://github.com/kevinlangleyjr/slatewave-cli
cd slatewave-cli
make build              # produces ./slatewave with the version baked in
./slatewave init        # exercise the binary
```

`make` (no args) prints every target with a short description. The ones you'll use most:

| Target                  | What it does                                                 |
| ----------------------- | ------------------------------------------------------------ |
| `make check`            | fmt + vet + race tests. **This is the gate before every PR.** |
| `make smoke`            | Dry-run-installs every embedded manifest. Catches schema / dispatch typos. |
| `make validate-manifests` | Round-trips every embedded manifest through the JSON Schema. |
| `make test-race`        | Race detector across the whole tree. |
| `make build`            | Local binary with `git describe` version baked in. |
| `make run ARGS="list"`  | Run without building. |

CI runs the same gate plus `make smoke` and `make validate-manifests` on Linux, macOS, and Windows.

## Adding a theme

Most contributors want slatewave to support one more tool. The flow:

1. **Pick a slug.** Lowercase, hyphen-separated, matches the tool's name (`bat`, `btop`, `oh-my-posh`). Use the same slug everywhere — manifest filename, the `slug` field, getslatewave.com.

2. **Write the manifest.** Drop a TOML file at `internal/manifest/embedded/<slug>.toml`. The minimum shape:

   ```toml
   [theme]
   slug = "yourtool"
   name = "Slatewave for yourtool"
   category = "editor"   # one of: editor, terminal, notes, browser, chat, productivity, other
   detect_command = "yourtool --version"   # exits 0 when yourtool is installed

   [install]
   type = "curl"   # or clone, vscode-ext, marketplace, gui-import, manual
   url  = "https://raw.githubusercontent.com/.../slatewave.theme"
   dest = "$HOME/.config/yourtool/themes/slatewave.theme"

   [activate]
   type = "ini-key"   # or shell-rc, toml-import, yaml-set, gitconfig-include, none
   file = "$HOME/.config/yourtool/config"
   key  = "theme"
   value = "slatewave"

   [verify]
   command = "yourtool --list-themes"
   expect  = "Slatewave"
   ```

   The full schema with every optional field (post-hooks, per-OS overrides, version-aware variants, `supported_os`, `done_message`, etc.) lives in `schemas/manifest.schema.json`.

3. **Run `make validate-manifests`.** Catches missing required fields, wrong enums, or typo'd field names before they bite at runtime.

4. **Run `make smoke`.** Dry-run-installs every manifest, including yours. Catches dispatch issues — wrong `install.type`, an `activate` step that the registry doesn't recognize, etc.

5. **Add a test if the manifest does anything unusual** — version variants, OS-specific overrides, post-hooks. The pattern lives in `internal/manifest/manifest_test.go` and `internal/installer/install_test.go`.

6. **End-to-end smoke once.** On your own machine, install the underlying tool, then `./slatewave install yourtool && ./slatewave uninstall yourtool`. Confirm the activation actually applies (the theme renders) and uninstall actually reverses (your config file is back to where it started).

A manifest-only PR doesn't need any Go code changes if the existing install / activate types cover your tool. If they don't, see the next section.

## Adding a new install or activate type

Less common but bigger contributions. Both follow a registry pattern:

- **Install types** live in `internal/installer/installer.go` under `var installers = map[string]installerImpl{...}`. Add one entry, write the `install` / `update` / `uninstallExtra` funcs alongside the existing `doCurl` / `doClone` / etc. Threading a `context.Context` is required so the new dispatcher honors Ctrl-C cancellation.
- **Activate types** live in `internal/activator/activator.go` under `var activators = map[string]activatorFn{...}`. Same shape. Activates **must be idempotent** — re-running `slatewave install` on a clean install changes nothing.
- **Always emit `state.Record` reversal info.** Files you create go into `CreatedPaths`, files you edit get backed up to `.slatewave.<timestamp>.bak` and the backup path goes into `Backups`, lines appended to shell rc files go into `AppendedLine`. The uninstaller walks these to reverse the install — no record, no reversal.
- **Update the schema.** `schemas/manifest.schema.json` documents the manifest grammar. New install or activate types need their fields described there + an enum entry on `install.type` / `activate.type`.
- **Add tests.** `installer/install_test.go` and `activator/activator_test.go` both have idempotency / dry-run / reversal coverage patterns to mirror.

## Running the test suite

`make check` is the canonical gate. It runs:

- `go fmt ./...` — formatting must be clean.
- `go vet ./...` — vet must pass.
- `go test ./...` — every package's tests must pass.

Beyond that:

- `make test-race` adds the race detector. Required for any change that touches goroutines (TUI dashboards, version-check, detect-all).
- Fuzz tests live in `*_fuzz_test.go` files. CI runs the seed corpus on every PR; manual fuzzing with `go test -fuzz=FuzzName -fuzztime=30s ./internal/installer/` (or wherever the fuzz lives) is a great way to stress new line-based parsers.

## Commit + PR conventions

- **Conventional commit prefix.** Subject starts with one of `feat:` / `fix:` / `docs:` / `test:` / `refactor:` / `chore:` / `ci:` / `build:`, optionally scoped (`feat(installer):`, `fix(tui):`).
- **One concern per commit.** A schema change + a new theme manifest + a refactor → three commits. Reviewers (and `git bisect`) thank you.
- **Subject line under ~72 characters.** Wrap detail into the body.
- **Body explains the why.** What user-visible behavior changed, why this approach, what alternatives were rejected. The code shows *what*; the commit explains *why*.
- **No manual line-wrapping in the body.** Let it flow; only use newlines to separate paragraphs. (`git log` and most viewers wrap for display.)
- **Reference issues or audit items if relevant.** `Closes #42` works; "follow-up to abc123" works when there's no issue.
- **CI must pass before merge.** Hooks are not skipped (`--no-verify`, `--no-gpg-sign`, etc. are off-limits unless a maintainer explicitly says otherwise).

PRs follow the same shape — title is the conventional-commit-style summary, body explains the change with a "## Summary" + "## Test plan" section. A draft PR is fine if you want early review.

## CHANGELOG entries

Every user-visible change appends a bullet under the `[Unreleased]` section of `CHANGELOG.md`, grouped into Added / Changed / Fixed / Removed. Pure refactors and internal CI tweaks don't need an entry. If your change touches the `--json` schemas, the manifest schema, or any flag surface, it definitely needs one — the audience is people scripting against slatewave.

## Code style

- gofmt is enforced. Run `make fmt` before committing.
- `golangci-lint` config lives in `.golangci.yml`. `make check` doesn't run it, but CI does — `golangci-lint run` locally if you want to surface issues before the PR.
- Comments earn their keep: explain the *why*, not the *what*. The existing code leans toward longer comments above non-obvious choices (atomic write rationale, regex-anchoring on `git config --unset-all`, why `quoteIfNeeded` always quotes). That's the style — match it where the code makes a non-obvious decision, skip it where the code reads itself.
- New shared styles go in `internal/ui/style.go` rather than inline `lipgloss.NewStyle()` at the use site. Visual vocabulary stays consistent.
- New install / activate types document their reversal contract in the comment block above the `do*` function — what's in `CreatedPaths`, what's backed up, what `uninstallExtra` cleans up.

## Where to ask

- **Manifest authoring questions** — open a Discussion or a draft PR; the schema is documented but the choice of `install.type` vs `activate.type` for a new tool sometimes wants a quick second opinion.
- **Bug reports** — issues, with a `slatewave --version` + repro.
- **Security issues** — see [SECURITY.md](SECURITY.md). Don't open a public issue.
- **Anything else** — open an issue and tag it appropriately.

Thanks for contributing. Welcome to the family.
