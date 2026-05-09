package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// cmdEnv isolates HOME (for state.Load), redirects ui.W into a buffer
// for output assertions, and lets the caller swap the manifest set via
// SLATEWAVE_MANIFESTS_DIR. The returned buffer captures everything the
// cmd/ helpers print to ui.W during the test. All env tweaks are scoped
// via t.Setenv so they auto-restore.
type cmdEnv struct {
	home string
	out  *bytes.Buffer
}

func setupCmdEnv(t *testing.T) *cmdEnv {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // os.UserHomeDir reads this on Windows
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	buf := &bytes.Buffer{}
	prev := ui.W
	ui.W = buf
	t.Cleanup(func() { ui.W = prev })

	return &cmdEnv{home: dir, out: buf}
}

// useManifestDir writes the provided slug → toml-body fixtures into a
// new temp dir and points manifest.LocalDir at it for the duration of
// the test. Lets a test exercise diagnose/reconcileWithReality against
// a known manifest set rather than the embedded one.
func (e *cmdEnv) useManifestDir(t *testing.T, fixtures map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for slug, body := range fixtures {
		path := filepath.Join(dir, slug+".toml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prev := manifest.LocalDir
	manifest.LocalDir = dir
	t.Cleanup(func() { manifest.LocalDir = prev })
}

// putRecord persists one state.Record for the test. Mirrors what an
// install would have left behind.
func (e *cmdEnv) putRecord(t *testing.T, rec state.Record) {
	t.Helper()
	s, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load in setup: %v", err)
	}
	s.Put(rec)
	if err := s.Save(); err != nil {
		t.Fatalf("state.Save in setup: %v", err)
	}
}
