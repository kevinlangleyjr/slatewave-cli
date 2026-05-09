// Package state persists what the CLI has done so uninstall can
// reverse it cleanly. Lives at ~/.config/slatewave/installed.toml.
//
// Each entry records:
//   - which theme was installed
//   - paths the CLI created (delete on uninstall)
//   - config-file backups the CLI made (restore on uninstall)
//   - shell-rc lines the CLI appended (remove on uninstall)
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/BurntSushi/toml"
)

// Record is one theme's install footprint.
type Record struct {
	Slug         string    `toml:"slug"`
	InstalledAt  time.Time `toml:"installed_at"`
	InstallType  string    `toml:"install_type"`
	ActivateType string    `toml:"activate_type"`

	// Reversal payload — populated as the install runs.
	CreatedPaths []string  `toml:"created_paths,omitempty"` // delete on uninstall
	Backups      []Backup  `toml:"backups,omitempty"`       // restore on uninstall
	AppendedLine *Appended `toml:"appended_line,omitempty"` // remove on uninstall
}

// Backup is a copy of a user config file made before the CLI edited
// it. On uninstall the original is restored from Path.
type Backup struct {
	Original string `toml:"original"` // user file edited
	Path     string `toml:"path"`     // backup location
}

// Appended tracks an `export FOO=bar` style line appended to a shell
// rc. Reverting means removing exactly that line.
type Appended struct {
	File string `toml:"file"`
	Line string `toml:"line"`
}

// Store is the on-disk record set.
type Store struct {
	Records map[string]Record `toml:"records"`
}

// File returns the path to the state file (~/.config/slatewave/installed.toml).
func File() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "slatewave", "installed.toml"), nil
}

// Load reads the state file. Returns an empty store if the file does
// not exist yet.
func Load() (*Store, error) {
	path, err := File()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Store{Records: map[string]Record{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s Store
	if _, err := toml.Decode(string(data), &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if s.Records == nil {
		s.Records = map[string]Record{}
	}
	return &s, nil
}

// Update brackets a Load → mutate → Save cycle with an exclusive
// advisory lock on a sibling .lock file. Two concurrent slatewave
// invocations (e.g., `install bat` from one terminal, `install btop`
// from another) would otherwise both Load → mutate → Save with the
// later writer clobbering the earlier — Update serializes them.
//
// fn receives the loaded store; mutate and return nil to save, or
// return an error to abort without saving. The returned error is the
// first of: lock acquisition, Load, fn, Save.
//
// Lock hold time is dominated by the in-memory mutate plus a
// temp-file rename — typically sub-millisecond. Callers should keep
// long-running work (network fetch, git clone, file writes) outside
// the callback so Update doesn't block other invocations.
func Update(fn func(*Store) error) error {
	lock, err := acquireLock()
	if err != nil {
		return err
	}
	defer func() { _ = lock.release() }()

	s, err := Load()
	if err != nil {
		return err
	}
	if err := fn(s); err != nil {
		return err
	}
	return s.Save()
}

type stateLock struct {
	f *os.File
}

func (l *stateLock) release() error {
	err := unlockFile(l.f.Fd())
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	return err
}

// acquireLock opens a sibling .lock file next to the state TOML and
// takes an exclusive flock / LockFileEx on it. The lock file is
// stable across runs (we don't remove it after release) so the
// kernel keeps the inode warm and lock acquisition stays cheap.
func acquireLock() (*stateLock, error) {
	path, err := File()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	if err := lockFile(f.Fd()); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	return &stateLock{f: f}, nil
}

// Save writes the state file, creating the parent directory if needed.
func (s *Store) Save() error {
	path, err := File()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".installed-*.toml")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := toml.NewEncoder(tmp).Encode(s); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encode state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename tmp: %w", err)
	}
	return nil
}

// Get returns the record for slug, or zero + false if not installed.
func (s *Store) Get(slug string) (Record, bool) {
	r, ok := s.Records[slug]
	return r, ok
}

// Put inserts or updates a record.
func (s *Store) Put(r Record) {
	if s.Records == nil {
		s.Records = map[string]Record{}
	}
	s.Records[r.Slug] = r
}

// Remove deletes a record by slug.
func (s *Store) Remove(slug string) {
	delete(s.Records, slug)
}

// AllSlugs returns the slugs of every installed theme, sorted ascending. Callers in cmd/list, cmd/update, and cmd/doctor render slugs in this order so successive runs produce identical output (helpful for diffs and screen-reader users).
func (s *Store) AllSlugs() []string {
	out := make([]string, 0, len(s.Records))
	for k := range s.Records {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
