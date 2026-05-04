package cmd

import (
	"strings"
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
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
