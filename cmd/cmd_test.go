package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// setFlags sets the named cobra flags to the given string values and
// returns a reset closure that restores them to their default zero
// value. Used by tests that need to drive a command through its flag
// surface — replaces the older pattern of mutating package-level
// vars directly. Pass the closure to defer/cleanup.
//
// Default value for restoration is "false" for bools and "" for
// strings; this matches how every flag is currently registered
// (BoolVar / StringVar with a false / "" default).
func setFlags(t *testing.T, cmd *cobra.Command, values map[string]string) func() {
	t.Helper()
	prev := map[string]string{}
	for name := range values {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("flag %q not registered on %q", name, cmd.Name())
		}
		prev[name] = f.Value.String()
	}
	for name, val := range values {
		if err := cmd.Flags().Set(name, val); err != nil {
			t.Fatalf("set flag %q=%q: %v", name, val, err)
		}
	}
	return func() {
		for name, val := range prev {
			_ = cmd.Flags().Set(name, val)
		}
	}
}

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
