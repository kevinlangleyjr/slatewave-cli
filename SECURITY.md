# Security policy

## Reporting a vulnerability

Please report security issues privately — don't open a public issue or PR with details.

- **Preferred:** [GitHub private vulnerability reporting](https://github.com/kevinlangleyjr/slatewave-cli/security/advisories/new) — handled inside the repo, no extra hop.
- **Fallback:** email `me@kevinlangleyjr.dev`. Reasonable response window is one week; if you don't hear back, ping again via the GitHub channel.

Include a clear repro (manifest fragment, command, observed behavior, expected behavior), the slatewave version (`slatewave --version`), and your OS / shell.

## Supported versions

While the project is pre-1.0, only the latest tagged release receives security fixes. Once 1.0.0 ships, this section grows a supported-range table.

## Scope

In scope:

- The slatewave CLI binary and everything it does on the user's machine — file writes, subprocess execution, network fetches, state-file handling.
- The embedded manifests shipped under `internal/manifest/embedded/`.
- The published release artifacts (homebrew formula, deb / rpm / apk / pacman packages, Windows zip, Linux/macOS tarballs).

Out of scope:

- Vulnerabilities in the underlying tools slatewave themes (bat, btop, helix, alacritty, etc.) — report those upstream.
- User-authored manifests not in the embedded set.
- Theme content (visual correctness, license disputes).
- Issues that require an attacker to already have local write access to the user's `$HOME`.

## Hardening already in place

The codebase has been through two opinionated audits (`v0.0.14` and `v0.0.19`) and ships with these mitigations:

- **Bounded I/O.** Every HTTP fetch caps at 60 s and 100 MiB; every shell-out runs under a `context.Context` so Ctrl-C / SIGINT actually kills the child instead of orphaning it.
- **Atomic writes.** Theme files, backup restores, and state writes go through a temp-file + rename helper so a partial write (interrupted process, full disk) can never corrupt user config.
- **Backup before mutate.** Every activator that *replaces* a config line snapshots the original file at `.slatewave.<timestamp>.bak` first. File mode is preserved so a `0o600` `.gitconfig` doesn't get downgraded to world-readable.
- **State serialization.** The state file (`~/.config/slatewave/installed.toml`) is locked with `flock` on Unix and `LockFileEx` on Windows so concurrent `slatewave install` invocations can't clobber each other.
- **Schema-validated manifests.** Every embedded manifest is round-tripped through the JSON Schema in `schemas/manifest.schema.json` as a CI gate — a typo'd field name or wrong type fails the PR rather than landing on main.
- **Content-type guard.** HTTP responses advertising `text/html` are rejected before any write, so a misconfigured CDN serving a captive-portal page can't end up at the theme dest path.
- **Anchored regex on `git config --unset-all`.** Include-path removal is `regexp.QuoteMeta`'d + `^…$`-anchored so a user with two similar `include.path` entries can't lose the wrong one on uninstall.
- **No automatic tool install.** slatewave never installs the underlying tool itself — if `bat` isn't on `$PATH`, the install errors out. The CLI's surface area is theme files + config edits, nothing else.

## Coordinated disclosure

Once a report lands, the rough timeline is: acknowledge within 1 week, triage and fix within 4 weeks (faster for high-severity), and ship a patched release before publishing a public advisory. Reporter credit is offered by default; tell us if you'd rather stay anonymous.
