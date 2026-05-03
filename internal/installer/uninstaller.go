package installer

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	// know before deleting any installed files.
	for _, b := range rec.Backups {
		if opts.DryRun {
			continue
		}
		data, err := os.ReadFile(b.Path)
		if err != nil {
			return fmt.Errorf("read backup %s: %w", b.Path, err)
		}
		if err := os.WriteFile(b.Original, data, 0o644); err != nil {
			return fmt.Errorf("restore %s from %s: %w", b.Original, b.Path, err)
		}
		_ = os.Remove(b.Path)
	}

	// Remove the appended line / git include.
	if rec.AppendedLine != nil {
		if rec.AppendedLine.File == "git-config-include" {
			if !opts.DryRun {
				cmd := exec.Command("git", "config", "--global", "--unset-all", "include.path", rec.AppendedLine.Line)
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("git config --unset-all include.path: %w\n%s", err, out)
				}
			}
		} else {
			if err := removeShellRCLine(rec.AppendedLine.File, rec.AppendedLine.Line, opts); err != nil {
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
		cmd := exec.Command("code", "--uninstall-extension", t.Install.Identifier)
		out, err := cmd.CombinedOutput()
		if err != nil {
			// If the extension was already removed externally (via VSCode's
			// own UI, for example), `code --uninstall-extension` errors with
			// "Extension '<id>' is not installed." Treat that as success so
			// `slatewave uninstall` cleanly reconciles state with reality.
			if strings.Contains(string(out), "is not installed") {
				return nil
			}
			return fmt.Errorf("code --uninstall-extension %s: %w\n%s", t.Install.Identifier, err, out)
		}
	}

	return nil
}

// removeShellRCLine removes exactly one occurrence of `line` from
// `file`. Idempotent — silently no-ops if the line isn't there.
func removeShellRCLine(file, line string, opts Options) error {
	data, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", file, err)
	}
	target := strings.TrimSpace(line)
	out := make([]string, 0)
	skipNext := false
	dropped := false
	for _, l := range strings.Split(string(data), "\n") {
		// Also drop the "# slatewave" marker comment that precedes
		// our appended line, if it's right above.
		if !dropped && strings.TrimSpace(l) == "# slatewave" {
			skipNext = true
			continue
		}
		if !dropped && strings.TrimSpace(l) == target {
			dropped = true
			skipNext = false
			continue
		}
		if skipNext {
			// the line right after "# slatewave" wasn't ours —
			// re-insert the marker we just skipped
			out = append(out, "# slatewave")
			skipNext = false
		}
		out = append(out, l)
	}
	if !dropped {
		return nil // nothing to do
	}
	if opts.DryRun {
		return nil
	}
	return os.WriteFile(file, []byte(strings.Join(out, "\n")), 0o644)
}
