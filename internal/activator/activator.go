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
	default:
		return fmt.Errorf("unknown activate type %q for theme %q", t.Activate.Type, t.Theme.Slug)
	}
}

// ----- type: ini-key -----

// doIniKey edits a single Key=Value line in an INI-ish config (e.g.
// btop.conf's `color_theme = "slatewave"`). Backs the file up first.
func doIniKey(t manifest.Theme, rec *state.Record, opts Options) error {
	if t.Activate.File == "" || t.Activate.Key == "" {
		return fmt.Errorf("ini-key activate for %q missing file or key", t.Theme.Slug)
	}
	file, err := expandPath(t.Activate.File)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}
	keyRe := regexp.MustCompile(`(?m)^(` + regexp.QuoteMeta(t.Activate.Key) + `\s*=).*$`)
	desired := fmt.Sprintf("%s %s", t.Activate.Key+" =", quoteIfNeeded(t.Activate.Value))
	if !keyRe.Match(data) {
		return fmt.Errorf("key %q not found in %s — manifest may be stale", t.Activate.Key, file)
	}
	updated := keyRe.ReplaceAllString(string(data), desired)
	if string(data) == updated {
		return nil // already activated; idempotent no-op
	}
	if opts.DryRun {
		return nil
	}
	backup, err := backupFile(file)
	if err != nil {
		return err
	}
	rec.Backups = append(rec.Backups, state.Backup{Original: file, Path: backup})
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
	defer f.Close()
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
