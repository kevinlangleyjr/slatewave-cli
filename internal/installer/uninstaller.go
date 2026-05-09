package installer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
)

// Uninstall reverses the install footprint recorded in rec:
//   - restore every backup over its original
//   - remove every created path (file or directory tree)
//   - remove the appended shell-rc line, OR `git config --global
//     --unset-all include.path <path>` if rec marked the include
//   - dispatch type-specific reversals (e.g., `code --uninstall-extension`)
//
// The Theme manifest is passed so the installer can derive type-specific
// reversal info (the vscode-ext identifier, etc.) — install records
// don't store every install field redundantly.
//
// On --dry-run, walks through what would happen but mutates nothing.
func Uninstall(rec state.Record, t manifest.Theme, opts Options) error {
	// Restore backups first — if a backup restore fails, we want to
	// know before deleting any installed files. Routed through
	// writeAtomic so a mid-write failure (disk full, kernel signal)
	// can't corrupt the user's original config: either the rename
	// completes and they're back to the pre-install state, or it
	// doesn't and the activated file is still in place + the .bak
	// is still there for them to recover manually.
	for _, b := range rec.Backups {
		if opts.DryRun {
			continue
		}
		src, err := os.Open(b.Path)
		if err != nil {
			return fmt.Errorf("read backup %s: %w", b.Path, err)
		}
		// Restore at the backup file's mode — backup was captured at the
		// original file's mode, so restoring at backup's mode round-trips
		// to the user's pre-install permissions (0o600 stays 0o600, etc.).
		err = writeAtomic(b.Original, preservedMode(b.Path), func(w io.Writer) error {
			_, err := io.Copy(w, src)
			return err
		})
		_ = src.Close()
		if err != nil {
			return fmt.Errorf("restore %s from %s: %w", b.Original, b.Path, err)
		}
		_ = os.Remove(b.Path)
	}

	// Remove the appended line / git include.
	if rec.AppendedLine != nil {
		if rec.AppendedLine.File == "git-config-include" {
			if !opts.DryRun {
				// `git config --unset-all <name> <value-pattern>` treats
				// the value as a regex — paths contain `.` which would
				// match any character, risking removal of an unrelated
				// include. Anchor and escape so we only match the exact
				// path we recorded.
				pattern := "^" + regexp.QuoteMeta(rec.AppendedLine.Line) + "$"
				cmd := exec.Command("git", "config", "--global", "--unset-all", "include.path", pattern)
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("git config --unset-all include.path: %w\n%s", err, out)
				}
			}
		} else {
			marker := "# slatewave"
			if t.Activate.CommentPrefix != "" {
				marker = t.Activate.CommentPrefix + " slatewave"
			}
			if err := removeShellRCLine(rec.AppendedLine.File, rec.AppendedLine.Line, marker, opts); err != nil {
				return err
			}
		}
	}

	// Delete created files / directories.
	for _, p := range rec.CreatedPaths {
		if opts.DryRun {
			continue
		}
		if err := os.RemoveAll(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}

	// Type-specific reversals — for installs that didn't write to disk
	// directly (e.g., vscode-ext shells out to `code --install-extension`).
	switch t.Install.Type {
	case "vscode-ext":
		if t.Install.Identifier == "" || opts.DryRun {
			break
		}
		cli := VSCodeExtCLI(t)
		cmd := exec.Command(cli, "--uninstall-extension", t.Install.Identifier)
		out, err := cmd.CombinedOutput()
		if err != nil {
			// If the extension was already removed externally (via the
			// editor's own UI, for example), `--uninstall-extension`
			// errors with "Extension '<id>' is not installed." Treat
			// that as success so `slatewave uninstall` cleanly reconciles
			// state with reality.
			if strings.Contains(string(out), "is not installed") {
				return nil
			}
			return fmt.Errorf("%s --uninstall-extension %s: %w\n%s", cli, t.Install.Identifier, err, out)
		}
	}

	return nil
}

// removeShellRCLine removes every occurrence of `line` from `file`.
// Idempotent — silently no-ops if the line isn't there.
//
// `marker` is the exact trimmed text of the comment we wrote above the
// activation line at install time (e.g. "# slatewave" or "-- slatewave",
// derived from the manifest's Activate.CommentPrefix). It's stripped
// when adjacent to the target — but only when adjacent, so unrelated
// comments matching the same style are preserved.
//
// Loops over every match rather than stopping at the first. A buggy
// older version (or a manual user paste) could have landed our line
// in the rc file twice; stopping after the first match would leave
// the duplicate sourced forever.
func removeShellRCLine(file, line, marker string, opts Options) error {
	data, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", file, err)
	}
	target := strings.TrimSpace(line)
	inputLines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(inputLines))
	dropped := false
	i := 0
	for i < len(inputLines) {
		l := inputLines[i]
		trimmed := strings.TrimSpace(l)

		// Marker followed by our exact line → drop both. Marker followed
		// by anything else → keep (it's the user's annotation that
		// happens to match our marker style).
		if trimmed == marker && i+1 < len(inputLines) && strings.TrimSpace(inputLines[i+1]) == target {
			i += 2
			dropped = true
			continue
		}
		// Naked occurrence (line without a preceding marker — shouldn't
		// happen for installs we wrote, but covers manual pastes and
		// older formats).
		if trimmed == target {
			i++
			dropped = true
			continue
		}

		out = append(out, l)
		i++
	}
	if !dropped {
		return nil // nothing to do
	}
	if opts.DryRun {
		return nil
	}
	return os.WriteFile(file, []byte(strings.Join(out, "\n")), preservedMode(file))
}
