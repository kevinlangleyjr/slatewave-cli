package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/jsonout"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
)

// Fixture pair: one manifest claims windows, the other defaults to
// darwin+linux. With currentGOOS = windows, only the windows-claiming
// one should appear in `slatewave list`. Detect/verify use `true` so
// the reconcile pass doesn't try to find a real binary.
const (
	manifestWindowsOK = `[theme]
slug = "winnable"
name = "Winnable"
category = "editor"
detect_command = "true"
supported_os = ["windows"]

[install]
type = "manual"

[activate]
type = "none"

[verify]
command = "true"
`

	manifestUnixOnly = `[theme]
slug = "uniqsonly"
name = "Unix Only"
category = "editor"
detect_command = "true"

[install]
type = "manual"

[activate]
type = "none"

[verify]
command = "true"
`
)

// TestListCmd_HidesUnsupportedOnCurrentOS asserts the user-facing
// promise: on Windows, `slatewave list` shows only the manifests that
// claim Windows in supported_os. The unix-only fixture must not appear
// — not as "available", not as "not detected", not at all.
func TestListCmd_HidesUnsupportedOnCurrentOS(t *testing.T) {
	defer manifest.SetGOOSForTest("windows")()

	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{
		"winnable":  manifestWindowsOK,
		"uniqsonly": manifestUnixOnly,
	})

	if err := listCmd.RunE(listCmd, nil); err != nil {
		t.Fatalf("listCmd.RunE: %v", err)
	}

	out := env.out.String()
	if !strings.Contains(out, "winnable") {
		t.Errorf("windows-supported slug missing from output: %q", out)
	}
	if strings.Contains(out, "uniqsonly") {
		t.Errorf("unix-only slug leaked into output on windows: %q", out)
	}
}

// TestListCmd_ShowsBothOnDarwin is the inverse: when the current OS
// is darwin, the unix-only manifest is supported (default) and the
// windows-only one is hidden. Locks the filter direction in both ways.
func TestListCmd_ShowsBothOnDarwin(t *testing.T) {
	defer manifest.SetGOOSForTest("darwin")()

	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{
		"winnable":  manifestWindowsOK,
		"uniqsonly": manifestUnixOnly,
	})

	if err := listCmd.RunE(listCmd, nil); err != nil {
		t.Fatalf("listCmd.RunE: %v", err)
	}

	out := env.out.String()
	if !strings.Contains(out, "uniqsonly") {
		t.Errorf("default-os slug missing from output on darwin: %q", out)
	}
	if strings.Contains(out, "winnable") {
		t.Errorf("windows-only slug leaked into output on darwin: %q", out)
	}
}

// --json output is what scripts and CI dotfiles bootstraps depend on.
// Asserts the wire shape: themes array carries every supported slug
// from the registry, installed flag flips for state-recorded entries,
// counts.installed reflects state, counts.total = themes length.
func TestListCmd_JSONOutputShape(t *testing.T) {
	defer manifest.SetGOOSForTest("darwin")()

	env := setupCmdEnv(t)
	env.useManifestDir(t, map[string]string{
		"alpha": strings.Replace(manifestUnixOnly, `slug = "uniqsonly"`, `slug = "alpha"`, 1),
		"beta":  strings.Replace(manifestUnixOnly, `slug = "uniqsonly"`, `slug = "beta"`, 1),
	})

	// Mark "alpha" as installed; "beta" stays uninstalled.
	env.putRecord(t, state.Record{
		Slug:         "alpha",
		InstallType:  "manual",
		ActivateType: "none",
	})

	t.Cleanup(func() { listJSON = false })
	listJSON = true
	if err := listCmd.RunE(listCmd, nil); err != nil {
		t.Fatalf("listCmd.RunE: %v", err)
	}

	var got jsonout.ListOutput
	if err := json.Unmarshal(env.out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\nraw: %s", err, env.out.String())
	}

	if len(got.Themes) != 2 {
		t.Errorf("themes count = %d, want 2", len(got.Themes))
	}
	if got.Counts.Total != 2 {
		t.Errorf("counts.total = %d, want 2", got.Counts.Total)
	}
	if got.Counts.Installed != 1 {
		t.Errorf("counts.installed = %d, want 1", got.Counts.Installed)
	}

	// alpha must be installed; beta must not.
	for _, row := range got.Themes {
		switch row.Slug {
		case "alpha":
			if !row.Installed {
				t.Errorf("alpha row not marked installed: %+v", row)
			}
			if row.InstallType != "manual" {
				t.Errorf("alpha install_type = %q, want manual", row.InstallType)
			}
		case "uniqsonly", "beta":
			// fixture's slug is "uniqsonly" until the Replace; either name fine
			if row.Installed {
				t.Errorf("non-alpha row %q wrongly marked installed: %+v", row.Slug, row)
			}
		}
	}
}
