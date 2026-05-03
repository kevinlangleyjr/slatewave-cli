package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stateInTempHome redirects $HOME to a per-test temp dir so the real
// ~/.config/slatewave/installed.toml never gets touched by a test.
func stateInTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestLoad_MissingFileReturnsEmptyStore(t *testing.T) {
	stateInTempHome(t)

	s, err := Load()
	if err != nil {
		t.Fatalf("Load on fresh home: %v", err)
	}
	if s == nil {
		t.Fatal("Load returned nil store")
	}
	if got := len(s.Records); got != 0 {
		t.Fatalf("expected empty Records, got %d entries", got)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	stateInTempHome(t)

	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	rec := Record{
		Slug:         "btop",
		InstalledAt:  now,
		InstallType:  "curl",
		ActivateType: "ini-key",
		CreatedPaths: []string{"/tmp/foo.theme"},
		Backups:      []Backup{{Original: "/tmp/btop.conf", Path: "/tmp/btop.conf.bak"}},
	}
	s.Put(rec)

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("re-Load: %v", err)
	}
	r, ok := got.Get("btop")
	if !ok {
		t.Fatal("expected btop record after round-trip")
	}
	if r.Slug != "btop" || r.InstallType != "curl" || r.ActivateType != "ini-key" {
		t.Errorf("record fields drifted: got %+v", r)
	}
	if len(r.CreatedPaths) != 1 || r.CreatedPaths[0] != "/tmp/foo.theme" {
		t.Errorf("CreatedPaths drifted: got %+v", r.CreatedPaths)
	}
	if len(r.Backups) != 1 || r.Backups[0].Original != "/tmp/btop.conf" {
		t.Errorf("Backups drifted: got %+v", r.Backups)
	}
	// TOML encodes time at second precision after truncation; allow round-trip equality.
	if !r.InstalledAt.Equal(now) {
		t.Errorf("InstalledAt drifted: got %v want %v", r.InstalledAt, now)
	}
}

func TestSave_LeavesNoTempFilesBehind(t *testing.T) {
	dir := stateInTempHome(t)

	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s.Put(Record{Slug: "vscode", InstallType: "vscode-ext"})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Save uses os.CreateTemp("...installed-*.toml") + rename. After a
	// clean save, no .installed-*.toml temp files should remain in the
	// state directory.
	stateDir := filepath.Join(dir, ".config", "slatewave")
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".installed-") {
			t.Errorf("Save left a temp file behind: %s", e.Name())
		}
	}
	// And the canonical file should be there.
	if _, err := os.Stat(filepath.Join(stateDir, "installed.toml")); err != nil {
		t.Errorf("installed.toml not created: %v", err)
	}
}

func TestStore_PutGetRemove(t *testing.T) {
	s := &Store{Records: map[string]Record{}}

	if _, ok := s.Get("missing"); ok {
		t.Error("Get on missing slug should return ok=false")
	}

	s.Put(Record{Slug: "bat", InstallType: "curl"})
	r, ok := s.Get("bat")
	if !ok {
		t.Fatal("Get after Put should return ok=true")
	}
	if r.InstallType != "curl" {
		t.Errorf("InstallType drift: %q", r.InstallType)
	}

	s.Remove("bat")
	if _, ok := s.Get("bat"); ok {
		t.Error("Get after Remove should return ok=false")
	}
}

func TestStore_AllSlugs(t *testing.T) {
	s := &Store{Records: map[string]Record{}}
	s.Put(Record{Slug: "bat"})
	s.Put(Record{Slug: "btop"})
	s.Put(Record{Slug: "delta"})

	got := s.AllSlugs()
	if len(got) != 3 {
		t.Fatalf("AllSlugs len = %d, want 3", len(got))
	}
	want := map[string]bool{"bat": true, "btop": true, "delta": true}
	for _, slug := range got {
		if !want[slug] {
			t.Errorf("AllSlugs returned unexpected slug %q", slug)
		}
	}
}

func TestStore_PutNilMapDoesNotPanic(t *testing.T) {
	// Defensive: a Store loaded from a TOML with no [records] block
	// has Records == nil. Put must lazily initialize.
	s := &Store{}
	s.Put(Record{Slug: "foo"})
	if _, ok := s.Get("foo"); !ok {
		t.Error("Put on nil-map Store didn't initialize")
	}
}
