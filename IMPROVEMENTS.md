# Slatewave CLI — improvement audit

A hard look at the codebase as of `v0.0.14`. Organized by severity. Where I cite a file, the line numbers are approximate as of `72025a7`.

The good news first: the architecture is solid. Package boundaries are clean (`cmd` / `installer` / `activator` / `manifest` / `state` / `tui` / `ui` / `shell`), the reversal model (CreatedPaths + Backups + AppendedLine on every Record) is genuinely well thought out, the manifest-driven install/activate dispatch keeps theme adds out of CLI release scope, and most packages have decent test coverage. Most of what follows is hardening, not redesign.

---

## P0 — correctness and safety

### 1. `http.Get` has no timeout
`internal/installer/installer.go:128` and `internal/installer/updater.go:72` both call `http.Get(url)` directly. That uses `http.DefaultClient`, which has **no timeout**. A hung server (or a TCP black hole on the user's flaky Wi-Fi) freezes `slatewave install` indefinitely with no way out except Ctrl-C.

**Fix:** define a package-level `*http.Client` with `Timeout: 60 * time.Second` and route both fetch paths through it.

### 2. `installer.Detect` has no timeout in non-TUI paths
The TUI's parallel `DetectAll` wraps each detect in a 3s `context.WithTimeout` (`internal/tui/detect.go:60`). But `installer.Detect` (called from `cmd/install.go`'s `installOne`, `cmd/doctor.go`'s `diagnose`, etc.) calls `shell.Run(context.Background(), cmd)` — unbounded. A misbehaving `detect_command` can hang `slatewave install` or `slatewave doctor`.

**Fix:** pipe the same `detectTimeout` constant through `installer.Detect`.

### 3. `fetchToFile` is non-atomic; `atomicRefetch` is
`updater.go` correctly uses temp-file + `os.Rename` (`atomicRefetch`, line 68). `installer.go`'s `fetchToFile` (line 124) writes directly to `dest`. If a fresh install is interrupted mid-write (Ctrl-C, OOM, network reset after the body started), the user is left with a partial file the CLI thinks is installed (state record written downstream, but the file is corrupt). Update was made atomic; install was overlooked.

**Fix:** consolidate on the atomic implementation. One `writeAtomic(url, dest) error` helper used by both paths.

### 4. Backup restore is not atomic in uninstall
`internal/installer/uninstaller.go:33-41`:

```go
data, err := os.ReadFile(b.Path)
if err := os.WriteFile(b.Original, data, 0o644); err != nil { ... }
_ = os.Remove(b.Path)
```

If `WriteFile` fails partway (disk full, kernel signal mid-syscall), the user's original config is corrupted *and* the backup is still around — but the user has to know to find it. Mirror the atomic temp-rename pattern from `state.Save`.

### 5. File mode is hardcoded `0o644`, original mode lost
Every `os.WriteFile` call in `activator.go` and `uninstaller.go` writes 0644. If the user's `~/.gitconfig` was 0600 (paranoid setup, has secrets in `[user]` block), activating a theme silently downgrades it to world-readable.

**Fix:** in `backupFile` and at every file-rewrite site, `os.Stat` the original first and pass its `mode.Perm()` to `WriteFile`. Default to 0644 only when creating a new file.

### 6. `git config --unset-all include.path <path>` treats path as regex
`internal/installer/uninstaller.go:47`:

```go
cmd := exec.Command("git", "config", "--global", "--unset-all", "include.path", rec.AppendedLine.Line)
```

Per `git config(1)`, the value argument to `--unset-all` is a *regex*. Paths contain `.` (matches any character). Today the slatewave include path is specific enough that this hasn't bitten anyone, but a user with two similar include paths could lose the wrong one on uninstall.

**Fix:** wrap with `^` and `$` and pre-escape: `"^" + regexp.QuoteMeta(rec.AppendedLine.Line) + "$"`.

### 7. `removeShellRCLine` only removes the first occurrence
`internal/installer/uninstaller.go:107`. If a buggy older version (or a manual user paste) caused the same line to land in the rc file twice, uninstall leaves the duplicate. The user thinks Slatewave is gone but the line is still being sourced.

**Fix:** loop until `dropped` stops advancing, or just continue past the first match.

### 8. State file isn't locked
`state.Load` → mutate → `state.Save` is a read-modify-write. Run `slatewave install bat` and `slatewave install btop` in two terminals at the same time and the second `Save` clobbers the first record. Edge case but trivial to repro on `--all` runs interrupted and re-launched.

**Fix:** advisory file lock (`flock` on unix, `LockFileEx` on windows) around the read-modify-write — even a 100ms hold is fine.

### 9. No HTTP body size cap or content-type sanity check
`fetchToFile` does `io.Copy(f, resp.Body)` with no max-bytes limit. A compromised CDN response (or an accidental redirect to a 50GB tarball) fills the user's disk. `io.LimitReader` with a per-asset cap (say 100MB — generous for a theme file) plus a content-type guard would close this.

---

## P1 — UX and developer experience

### 10. No `slatewave info <theme>` command
The user sees a slug in `slatewave list` and wants context: what does this theme do, what config does it edit, what URL does it fetch from, what's the manifest version. Today they have to read the embedded TOML in the repo. Surfacing those fields via `slatewave info bat` (or `--describe`) closes the gap.

### 11. Tool-specific done messages should live in manifests
`cmd/install.go:doneMessage` and `cmd/uninstall.go:uninstallDoneMessage` are slug-keyed switches in CLI code:

```go
case "ghostty", "alacritty", "wezterm", "iterm2", "kitty":
    return fmt.Sprintf("Reverted. Quit and relaunch %s to see your original colors…", t.Theme.Name[len("Slatewave for "):])
```

Two issues: (1) adding a new terminal-type theme requires editing `cmd/uninstall.go` instead of just dropping a manifest, defeating the manifest-driven design; (2) `t.Theme.Name[len("Slatewave for "):]` panics if any theme's name ever doesn't start with `"Slatewave for "`. Move to `[install].done_message` and `[uninstall].done_message` manifest fields.

### 12. No `--json` output for scripting
`slatewave list`, `slatewave status`, `slatewave doctor` all render ANSI-styled boxes. There's no machine-readable mode for users who want to wire Slatewave into a dotfiles repo or CI check. A `--json` flag on each (returning a stable shape) would unlock automation.

### 13. No `slatewave self update` / version nag
Users on Homebrew get nudged via `brew outdated`, but Linux package + manual-tarball installs have no signal that v0.0.14 → v0.1.0 happened. A once-a-day async check against the GitHub releases API (cached in `~/.config/slatewave/version-check.json`, gated behind `SLATEWAVE_NO_UPDATE_CHECK=1` for hermetic environments) would close this.

### 14. No verbose / debug mode
There's no `--verbose` flag. When something fails on a user's machine, the only signal is the wrapped error string. Adding `-v` that streams every shell command, every fetched URL, every write target to stderr (gated behind a flag so default output stays tidy) would cut diagnostic time dramatically.

### 15. Bulk install/update should default to TUI mode
`slatewave install --all` runs serial-stream output today; `--interactive` opts in to the live dashboard. For a 30-theme install on a fresh machine, the dashboard is dramatically better UX (you can see what's queued, what's running, what failed). Flip the default — `--no-interactive` or `--quiet` for the streaming mode.

### 16. Globalflag pattern makes tests fragile
Every cmd file declares `var installDryRun bool` etc. as package-level variables. Cross-test state requires manual reset. `cobra` supports per-`Cmd` flag binding via `cmd.Flags().GetBool("dry-run")` inside the RunE — moving to that style would let tests construct fresh `Cmd` instances per case.

---

## P2 — testing and CI gaps

### 17. No Windows CI matrix
`README.md` advertises `vscode`/`cursor`/`vscodium`/`antigravity`/`starship`/`oh-my-posh`/`windows-terminal` as Windows-supported. CI runs `ubuntu-latest` and `macos-latest` only (`.github/workflows/ci.yml`). Every Windows-specific code path (`shell.Run` → `cmd /C`, the PowerShell-profile shell-rc activator, `where` detect commands) is untested. Add `windows-latest` to the test matrix — even a smoke `go test ./...` would catch path-separator and exec-quote regressions.

### 18. No `internal/ui` tests
`ui/banner.go`'s slant-style ASCII is hand-laid; one stray quote breaks the visual. A golden-file test (`Banner()` → diff against `testdata/banner.txt`) would catch silent regressions.

### 19. `make smoke` not run in CI
`Makefile`'s `smoke` target dry-run-installs every embedded theme — exactly the check that should run on every PR to catch a manifest with a typo'd field name. Add it to `ci.yml` (linux-only is fine).

### 20. No JSON-Schema validation of embedded manifests
`schemas/manifest.schema.json` exists but isn't enforced. A `make validate-manifests` target that runs each `internal/manifest/embedded/*.toml` through a TOML→JSON roundtrip + schema check would catch field-name drift between the schema and the structs.

### 21. No fuzz tests for `tomlImportRewrite` / `yamlSetRewrite` / `removeShellRCLine`
These are line-based parsers operating on user-authored content. Go's `testing/fuzz` would shake out crashers (unbalanced brackets, unicode anchors, files with no trailing newline, etc.) without much effort.

---

## P3 — architecture / future-proofing

### 22. Manifests are embedded; there's no remote fallback
`internal/manifest/registry.go` has a comment: *"v0.2 will fall back to fetching per-theme manifests from each theme repo's slatewave.toml."* Right now if a theme's URL changes between CLI releases, users are stuck on the old URL until they upgrade the CLI. The remote-manifest path is the right answer; the embedded set becomes the offline fallback.

### 23. Dispatch switches duplicated across `Install` / `Update` / `Uninstall` / `Activate`
Four big switches that all branch on the same six install types or six activate types. A registry pattern (`installers["curl"] = curlImpl{install, update, uninstall}`) would make adding a new type one struct addition instead of four switch edits.

### 24. `ui.W` is a package-level mutable global
Useful for tests but classic global state. Threading the writer through `cobra.Command.Context()` is a one-time refactor that makes the data flow explicit and parallel-test-safe.

### 25. `go.mod`'s pin comment is stale
`go.mod` says:
> `// Pinned to the highest version golangci-lint v1's binary can analyze.`

But CI is on `golangci-lint-action@v9` running v2.x (see `.golangci.yml`'s comment). The constraint that drove the pin is gone. Drop the comment and bump to a current Go directive on the next dependency sweep.

### 26. `cmd/list.go`'s reconcileWithReality silently mutates state
`reconcileWithReality` runs every theme's verify command on every `list` invocation (cheap but not free) and silently drops records whose verify fails. This is a behavior the user would benefit from knowing about — at minimum a one-line "dropped N stale records" line at the bottom of `list`. Hidden state mutation in a command documented as "list" is mildly surprising.

### 27. `pickShellRC`'s SHELL match is too fuzzy
`internal/activator/activator.go:362` matches by `strings.Contains(strings.ToLower(p), filepath.Base(shell))`. With `SHELL=/bin/sh`, `filepath.Base="sh"` matches both `.bashrc` (contains "sh") and `.zshrc` (contains "sh"). The fallback chain saves us in practice — first existing file wins — but the matching algorithm is sketchy and would bite if both rc files exist and neither was the user's actual shell. Use exact suffix match: `.zshrc` ↔ `zsh`, `.bashrc` ↔ `bash`, `.config/fish/config.fish` ↔ `fish`.

---

## Execution plan — phased

The phases below sequence every numbered item above into independently shippable units. Order is roughly "highest-risk-reduction first, lowest-coupling-to-rest first." Each phase ends at a clean commit boundary so we can stop, review, commit, and resume without leaving the tree in a half-state.

Items are tagged with their numbered references from the audit above (`#1`, `#2`, …). One audit item maps to one or more atomic commits inside a phase — never split across phases.

### Phase 1 — HTTP and process timeouts
Make every network and shell call bounded so a hung server / hanging detect never freezes the CLI.

- `#1` Add a package-level `*http.Client` with `Timeout: 60s` in `internal/installer`; route `installer.go:fetchToFile` and `updater.go:atomicRefetch` through it.
- `#2` Pipe `detectTimeout` through `installer.Detect` (and any other `shell.Run(context.Background(), …)` from a non-TUI hot path).
- `#9` Wrap the body in `io.LimitReader` with a per-asset cap (~100MB) before `io.Copy`. Reject responses whose `Content-Type` is obviously wrong (HTML when we expected text/binary theme assets).

**Commits:** ~3 (one per item). All localized to `internal/installer/`. Tests: extend `install_test.go` with an httptest server that hangs and confirm the client times out.

### Phase 2 — Atomic filesystem operations
Eliminate every read-modify-write that can corrupt user config on interrupt or disk-full.

- `#3` Extract one `writeAtomic(reader io.Reader, dest string) error` helper. Replace `installer.go:fetchToFile` with it; `updater.go:atomicRefetch` already uses this shape — collapse the duplication.
- `#4` Use the same temp-rename pattern in `uninstaller.go:Uninstall`'s backup-restore loop.
- `#5` Add a `preservedMode(file)` helper: `os.Stat` the file, return `mode.Perm()` (default 0644 if missing). Use it everywhere `os.WriteFile(..., 0o644)` currently rewrites a user-owned file (`activator.go:doIniKey`, `doShellRC`, `doTOMLImport`, `doYAMLSet`, and `uninstaller.go`'s restore + `removeShellRCLine`).
- `#8` Add advisory file locking around `state.Load` → mutate → `state.Save`. `flock(LOCK_EX)` on unix; `golang.org/x/sys/windows` `LockFileEx` on Windows. Hold ≤ a few hundred ms.

**Commits:** ~4. Mostly `internal/installer/` and `internal/state/` plus a small touch to `internal/activator/`.

### Phase 3 — Activator and shell-rc correctness
Close the regex / matching footguns in places that touch the user's git config and rc files.

- `#6` `regexp.QuoteMeta` the include path before passing to `git config --unset-all`. Anchor with `^…$` so the match is literal.
- `#7` Loop `removeShellRCLine` until no occurrence remains, not just the first.
- `#27` Replace `pickShellRC`'s `strings.Contains(toLower(p), Base($SHELL))` with a small explicit map (`zsh → .zshrc`, `bash → .bashrc`, `fish → config.fish`).

**Commits:** ~3. All in `internal/installer/uninstaller.go` and `internal/activator/activator.go`. Add regression tests for each — these are the kind of bugs that come back if untested.

### Phase 4 — Manifest-driven post-install messaging
Pull the slug-keyed switches out of `cmd/` so adding a theme stays a manifest-only change.

- `#11` Add `[install].done_message` and `[uninstall].done_message` (plus the JSON Schema entries). Backfill every theme that currently has a custom message in `cmd/install.go:doneMessage` / `cmd/uninstall.go:uninstallDoneMessage`. Delete the switches. Drop the `t.Theme.Name[len("Slatewave for "):]` slice — pull `display_name` from the manifest if needed.

**Commits:** 2 — one for the schema/struct/manifest backfill, one to delete the cmd-side switches.

### Phase 5 — CI and validation hardening
Catch the next regression before it ships, not after.

- `#17` Add `windows-latest` to the test matrix in `.github/workflows/ci.yml`. Even if some tests skip on Windows initially, having the build green there closes the "advertised but untested" gap.
- `#19` Add a `make smoke` step to CI (linux-only) so a typo in any embedded manifest fails the PR.
- `#20` Add `make validate-manifests`: TOML→JSON roundtrip every embedded manifest and check it against `schemas/manifest.schema.json`. Add to CI.
- `#18` Golden-file test for `ui.Banner()` so slant changes are intentional.
- `#21` `testing/fuzz` corpora for `tomlImportRewrite`, `yamlSetRewrite`, `removeShellRCLine`. Run them in CI with a short fuzz time (a few seconds) on every PR.

**Commits:** ~4. All in `.github/`, `Makefile`, and new `*_fuzz_test.go` files.

### Phase 6 — Observability and scripting
Make the CLI usable in scripts and debuggable on the user's machine.

- `#14` `--verbose` (or `-v`) flag on the root command. When set, stream every shell command, fetched URL, and write target to stderr. Wire through `internal/ui` so it's a single check.
- `#12` `--json` output mode for `list`, `status`, `doctor`. Define stable output schemas in a small `internal/jsonout/` package so structure changes are reviewable.

**Commits:** ~3 — `--verbose` first (small, used by everything), then `--json` per command.

### Phase 7 — Discoverability and self-care
Surface the catalog and keep users on a current build.

- `#10` `slatewave info <theme>` — print manifest details (install type + URL/repo, activate type + targets, OS support, source URL on getslatewave.com).
- `#15` Flip bulk install/update default to TUI mode; add `--no-interactive` for the streaming style. Update the `--interactive` flag help to be deprecated/no-op.
- `#13` Once-a-day async version check against the GitHub releases API. Cache result in `~/.config/slatewave/version-check.json`. Gate behind `SLATEWAVE_NO_UPDATE_CHECK=1` for hermetic CI.

**Commits:** ~3.

### Phase 8 — Architecture cleanups
The lower-urgency tidying. Worth doing once the rest is shipped, in any order — none of these change observable behavior.

- `#16` Replace package-level cobra flag globals with `cmd.Flags().GetBool(...)` reads inside RunE.
- `#23` Registry pattern for install / activate dispatch — collapses four parallel switches into one map keyed by type.
- `#24` Thread `io.Writer` through `cobra.Command.Context()` instead of `ui.W` global.
- `#25` Drop the stale `go.mod` pin comment; bump Go directive to current.
- `#26` Make `list`'s silent reconcile visible — one-line "dropped N stale records" footer when it actually mutates.

**Commits:** 5 — small, independent.

### Out of scope (future, larger initiative)

- `#22` Remote manifest fallback (`v0.2` per existing TODO comment). This is a multi-phase project of its own — fetch + cache + signature verification + fallback ordering — and shouldn't be folded into the hardening pass above.

---

**Suggested rhythm:** finish a phase, run `make check`, commit each numbered item as its own atomic commit (Jira number on last line per repo convention), then pause for review before opening the next phase.

---

## Phase 8 detailed plan — #16 and #24

These two refactors touch lots of files mechanically. Splitting each into a plan-then-execute pair so the diffs stay reviewable.

### #16 — Cobra flag globals → RunE-scoped reads

**Why:** Every cmd file has package-level `var installDryRun bool` declarations bound to flags via `BoolVar`. Tests reset them via `t.Cleanup(func() { listJSON = false })`. Per-Cmd reads via `cmd.Flags().GetBool` would scope state correctly; tests could either mutate the flag through cobra or pass values directly to helpers.

**Approach (one commit):**

1. **Define a per-command flags struct.** Each command gets a small struct (`installFlags`, `updateFlags`, `listFlags`, etc.) bundling its booleans and strings. RunE constructs the struct from `cmd.Flags()` and passes it into helpers.

   ```go
   type installFlags struct {
       DryRun, All, Interactive, NoInteractive bool
       Category                                string
   }
   func parseInstallFlags(cmd *cobra.Command) installFlags { ... }
   ```

2. **Thread the struct through helpers.** Helpers like `installOne(slug string, suppressFinal bool)` grow an `opts installFlags` parameter. Internal callers (`installBulk`, `installInteractiveTUI`) pass it through.

3. **Delete package-level vars.** Remove `var installDryRun bool`, `var installAll bool`, etc. The `BoolVar(&installDryRun, ...)` calls become `Bool("dry-run", false, ...)`.

4. **Update tests.** Two patterns:
   - Cobra-driven tests (most existing): `cmd.Flags().Set("json", "true")` then `cmd.RunE(cmd, args)`. The `t.Cleanup` resets become `cmd.Flags().Set("json", "false")`.
   - Helper-direct tests: pass an `installFlags` struct literal — no flag layer.

5. **Exception:** `verboseFlag` in root.go stays a package global. PersistentPreRun fires before the subcommand's RunE constructs its flags struct, so verbose's value has to live somewhere shared. Document the exception inline.

**Files:** cmd/install.go, uninstall.go, update.go, list.go, status.go, doctor.go, info.go, plus their `_test.go` counterparts. ~250 line diff.

**Risk:** mechanical but easy to miss a call site. A focused review of each file's RunE signature ("helpers all take `opts X`?") catches drift.

### #24 — `ui.W` global → context-threaded writer

**Why:** `internal/ui` declares `var W io.Writer = os.Stdout` as a package-level mutable global. Tests swap it for a `*bytes.Buffer` and rely on no-other-test-runs-concurrently to avoid races. Threading the writer through cobra.Command.Context() is more idiomatic Go and removes the mutable-global hazard.

**Approach (one commit):**

1. **Introduce ui's context API:**

   ```go
   // ui/writer.go
   type writerKey struct{}
   func WithWriter(ctx context.Context, w io.Writer) context.Context { ... }
   func Writer(cmd *cobra.Command) io.Writer { ... fall back to os.Stdout ... }
   ```

2. **Migrate ui helpers to take an io.Writer.** `Header`, `Done`, `MutedLn`, `Errorf`, `StepStart` grow a leading `w io.Writer` parameter. `StepStart` returns a closure that closes over the same writer.

   ```go
   func Header(w io.Writer, action, themeName string)
   func Done(w io.Writer, message string)
   func StepStart(w io.Writer, message string) func(err error)
   ```

3. **Update every call site.** Each cmd file's helpers grow `*cobra.Command` (or `io.Writer`) parameter. `fmt.Fprintln(ui.W, ...)` becomes `fmt.Fprintln(out, ...)` where `out` is local.

4. **Wire in PersistentPreRun.** root.go does `cmd.SetContext(ui.WithWriter(cmd.Context(), os.Stdout))` once at startup. Tests inject a buffer the same way.

5. **Demote `ui.W`.** Once every call site reads via `ui.Writer(cmd)`, make `W` package-private (or remove it entirely if no test needs it).

**Files:** internal/ui/log.go, every cmd/*.go that uses ui or writes to ui.W, every cmd/*_test.go that does `prev := ui.W; ui.W = buf`. ~400 line diff.

**Risk:** The TUI sub-packages (internal/tui) use bubbletea's own writer, not ui.W — they shouldn't need changes. Worth eyeballing to confirm.

---

**Order:** ship #16 first (smaller, no signature changes in internal/ui). Then #24 builds on the now-clean cmd/ shape.
