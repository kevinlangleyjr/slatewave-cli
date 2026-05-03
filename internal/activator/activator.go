// Package activator dispatches the activate step of a theme manifest.
// Each [activate].type maps to a function below. All activators are
// idempotent (safe to run twice) and record reversal info into the
// passed-in state.Record.
package activator

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
)

// Options controls activator behavior.
type Options struct {
	DryRun bool
}

// Activate runs the activate step for theme t. Mutates rec in place to
// record reversal info (backups, appended lines).
func Activate(t manifest.Theme, rec *state.Record, opts Options) error {
	switch t.Activate.Type {
	case "", "none":
		return nil
	case "ini-key":
		return doIniKey(t, rec, opts)
	case "gitconfig-include":
		return doGitconfigInclude(t, rec, opts)
	case "shell-rc":
		return doShellRC(t, rec, opts)
	case "toml-import":
		return doTOMLImport(t, rec, opts)
	default:
		return fmt.Errorf("unknown activate type %q for theme %q", t.Activate.Type, t.Theme.Slug)
	}
}

// ----- type: ini-key -----

// doIniKey ensures `Key = Value` lands in an INI-ish config, handling
// each of the four shapes the file might be in:
//
//  1. file is missing entirely (e.g. ~/.config/ghostty/config before
//     the user has launched ghostty for the first time): create the
//     parent dir, write a fresh file with our line. Record the file
//     as a CreatedPath so uninstall removes it.
//
//  2. file exists but doesn't contain our key (a fresh config the user
//     hasn't set this option in yet): append `Key = Value` to the end
//     under a `# slatewave` marker. Record a Backup so uninstall can
//     restore.
//
//  3. file exists with our key set to a different value: replace the
//     value. Backup as in (2).
//
//  4. file exists with our key already set to our value: idempotent
//     no-op, no backup.
func doIniKey(t manifest.Theme, rec *state.Record, opts Options) error {
	if t.Activate.File == "" || t.Activate.Key == "" {
		return fmt.Errorf("ini-key activate for %q missing file or key", t.Theme.Slug)
	}
	file, err := expandPath(t.Activate.File)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(file)
	fileExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", file, err)
	}

	keyRe := regexp.MustCompile(`(?m)^(` + regexp.QuoteMeta(t.Activate.Key) + `\s*=).*$`)
	desiredLine := fmt.Sprintf("%s %s", t.Activate.Key+" =", quoteIfNeeded(t.Activate.Value))

	var updated string
	switch {
	case !fileExists:
		// Case 1: create file with just our line.
		updated = "# slatewave\n" + desiredLine + "\n"
	case keyRe.Match(data):
		// Cases 3 / 4: key exists — replace value (or no-op).
		updated = keyRe.ReplaceAllString(string(data), desiredLine)
		if string(data) == updated {
			return nil // already at desired value
		}
	default:
		// Case 2: file exists but no such key — append.
		prefix := ""
		if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
			prefix = "\n"
		}
		updated = string(data) + prefix + "\n# slatewave\n" + desiredLine + "\n"
	}

	if opts.DryRun {
		return nil
	}

	if !fileExists {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return fmt.Errorf("create parent dir: %w", err)
		}
		rec.CreatedPaths = append(rec.CreatedPaths, file)
	} else {
		backup, err := backupFile(file)
		if err != nil {
			return err
		}
		rec.Backups = append(rec.Backups, state.Backup{Original: file, Path: backup})
	}
	return os.WriteFile(file, []byte(updated), 0o644)
}

// quoteIfNeeded wraps a value in double-quotes if it contains spaces
// and isn't already quoted. Matches the convention btop.conf uses.
func quoteIfNeeded(v string) string {
	if strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
		return v
	}
	if !strings.ContainsAny(v, " \t") {
		return `"` + v + `"`
	}
	return `"` + v + `"`
}

// ----- type: gitconfig-include -----

// doGitconfigInclude adds an include.path entry to the user's global
// ~/.gitconfig via `git config`. Records the path so uninstall can
// remove just that one include without touching other entries.
func doGitconfigInclude(t manifest.Theme, rec *state.Record, opts Options) error {
	if t.Activate.IncludePath == "" {
		return fmt.Errorf("gitconfig-include activate for %q missing include_path", t.Theme.Slug)
	}
	path, err := expandPath(t.Activate.IncludePath)
	if err != nil {
		return err
	}
	// Idempotency check — list existing includes, bail if ours is there.
	out, _ := exec.Command("git", "config", "--global", "--get-all", "include.path").CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == path {
			return nil
		}
	}
	if opts.DryRun {
		return nil
	}
	cmd := exec.Command("git", "config", "--global", "--add", "include.path", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config --add include.path: %w\n%s", err, out)
	}
	// Use AppendedLine to record the include path so uninstall can call
	// `git config --global --unset-all include.path <path>` precisely.
	rec.AppendedLine = &state.Appended{File: "git-config-include", Line: path}
	return nil
}

// ----- type: shell-rc -----

// doShellRC appends a line to whichever of t.Activate.Files exists
// (or the one matching $SHELL if none yet exists). Idempotent: if the
// line is already there, no-op.
func doShellRC(t manifest.Theme, rec *state.Record, opts Options) error {
	if t.Activate.Line == "" {
		return fmt.Errorf("shell-rc activate for %q missing line", t.Theme.Slug)
	}
	target, err := pickShellRC(t.Activate.Files)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", target, err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == strings.TrimSpace(t.Activate.Line) {
			return nil // already there
		}
	}
	if opts.DryRun {
		return nil
	}
	f, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", target, err)
	}
	defer func() { _ = f.Close() }()
	prefix := ""
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		prefix = "\n"
	}
	if _, err := f.WriteString(prefix + "\n# slatewave\n" + t.Activate.Line + "\n"); err != nil {
		return fmt.Errorf("append %s: %w", target, err)
	}
	rec.AppendedLine = &state.Appended{File: target, Line: t.Activate.Line}
	return nil
}

// pickShellRC chooses which rc file to append to. Preference order:
//  1. The first existing file in candidates.
//  2. If none exist, the file matching $SHELL.
//  3. Fallback to the first candidate.
func pickShellRC(candidates []string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("no candidate shell-rc files configured")
	}
	expanded := make([]string, len(candidates))
	for i, c := range candidates {
		p, err := expandPath(c)
		if err != nil {
			return "", err
		}
		expanded[i] = p
	}
	for _, p := range expanded {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	shell := os.Getenv("SHELL")
	for _, p := range expanded {
		if strings.Contains(strings.ToLower(p), filepath.Base(shell)) {
			return p, nil
		}
	}
	return expanded[0], nil
}

// ----- type: toml-import -----

// doTOMLImport adds a path entry to a TOML `import = [...]` array. Used
// by alacritty (under [general]) and similar tools whose config is a
// TOML file with an array of imported sub-files.
//
// The manifest declares:
//
//	[activate]
//	type      = "toml-import"
//	toml_path = "$HOME/.config/alacritty/alacritty.toml"
//	import    = "~/.config/alacritty/themes/slatewave.toml"
//
// We do a small, line-based edit rather than a full TOML parse-edit-
// emit so user comments and ordering survive. Three cases:
//
//  1. import = [...] already contains our entry  → no-op (idempotent)
//  2. import = [...] exists but doesn't contain us → add entry to that array
//  3. no import array yet → append `import = ["<our entry>"]` at file end
//
// Always backs the file up before rewriting so uninstall can restore.
func doTOMLImport(t manifest.Theme, rec *state.Record, opts Options) error {
	if t.Activate.TOMLPath == "" || t.Activate.Import == "" {
		return fmt.Errorf("toml-import activate for %q missing toml_path or import", t.Theme.Slug)
	}
	file, err := expandPath(t.Activate.TOMLPath)
	if err != nil {
		return err
	}
	entry, err := expandPath(t.Activate.Import)
	if err != nil {
		return err
	}

	// Read existing config; missing-file is a valid starting state
	// (we'll create one with just our import line).
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", file, err)
	}

	updated, changed, err := tomlImportRewrite(string(data), entry)
	if err != nil {
		return err
	}
	if !changed {
		return nil // already imports our entry; idempotent no-op
	}
	if opts.DryRun {
		return nil
	}

	// Make the parent dir if we're creating the file fresh.
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	// Back up if the file existed; nothing to back up if we're creating.
	if len(data) > 0 {
		backup, err := backupFile(file)
		if err != nil {
			return err
		}
		rec.Backups = append(rec.Backups, state.Backup{Original: file, Path: backup})
	} else {
		// File created from nothing — uninstall should delete, not restore.
		rec.CreatedPaths = append(rec.CreatedPaths, file)
	}

	return os.WriteFile(file, []byte(updated), 0o644)
}

// tomlImportRewrite returns the rewritten config text and whether
// anything changed. Pulled out for unit testing without filesystem.
func tomlImportRewrite(content, entry string) (string, bool, error) {
	// Idempotency: if entry already appears in any import = [...] array,
	// no-op. We match against the quoted form since that's how it'll
	// land in the file.
	quoted := fmt.Sprintf("%q", entry)
	if strings.Contains(content, quoted) {
		return content, false, nil
	}

	// Case 2: there's an existing single-line `import = [ ... ]` block;
	// inject our entry before the closing bracket. Multi-line arrays
	// fall through to case 3 (append a new single-line import) so we
	// don't have to parse arbitrary TOML formatting.
	importRe := regexp.MustCompile(`(?m)^(\s*import\s*=\s*\[)([^\]]*)(\])`)
	if m := importRe.FindStringSubmatchIndex(content); m != nil {
		// indices 2,3 = first capture (prefix incl. opening `[`),
		//         4,5 = second (existing entries),
		//         6,7 = third (closing `]`).
		// head must include `import = [`, so slice up to m[3].
		head := content[:m[3]]
		existing := strings.TrimSpace(content[m[4]:m[5]])
		var newBody string
		if existing == "" {
			newBody = quoted
		} else {
			newBody = existing
			if !strings.HasSuffix(newBody, ",") {
				newBody += ","
			}
			newBody += " " + quoted
		}
		closingPlus := content[m[6]:]
		return head + newBody + closingPlus, true, nil
	}

	// Case 3: no import array yet. Append one.
	suffix := "\n"
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		suffix = "\n" + suffix
	}
	out := content + suffix + "# slatewave\nimport = [" + quoted + "]\n"
	return out, true, nil
}

// ----- helpers -----

func expandPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	p = os.ExpandEnv(p)
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p, nil
}

// backupFile copies file to <file>.slatewave.<timestamp>.bak and
// returns the backup path.
func backupFile(file string) (string, error) {
	ts := time.Now().UTC().Format("20060102T150405")
	backup := fmt.Sprintf("%s.slatewave.%s.bak", file, ts)
	src, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("read for backup: %w", err)
	}
	if err := os.WriteFile(backup, src, 0o644); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}
	return backup, nil
}
