package manifest

import (
	"testing"
)

func TestSupportsCurrentOS_DefaultDarwinLinux(t *testing.T) {
	// Empty SupportedOS → defaults to darwin + linux. Locks the
	// pre-Windows behavior every existing manifest had implicitly.
	th := Theme{}
	cases := map[string]bool{
		"darwin":  true,
		"linux":   true,
		"windows": false,
	}
	for goos, want := range cases {
		restore := SetGOOSForTest(goos)
		got := SupportsCurrentOS(th)
		restore()
		if got != want {
			t.Errorf("default SupportedOS on %s: got %v want %v", goos, got, want)
		}
	}
}

func TestSupportsCurrentOS_ExplicitList(t *testing.T) {
	th := Theme{}
	th.Theme.SupportedOS = []string{"windows"}

	cases := map[string]bool{
		"darwin":  false,
		"linux":   false,
		"windows": true,
	}
	for goos, want := range cases {
		restore := SetGOOSForTest(goos)
		got := SupportsCurrentOS(th)
		restore()
		if got != want {
			t.Errorf("[\"windows\"] on %s: got %v want %v", goos, got, want)
		}
	}
}

func TestSupportsCurrentOS_AllThree(t *testing.T) {
	th := Theme{}
	th.Theme.SupportedOS = []string{"darwin", "linux", "windows"}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		restore := SetGOOSForTest(goos)
		if !SupportsCurrentOS(th) {
			t.Errorf("all-three list should support %s", goos)
		}
		restore()
	}
}

func TestDetectCommandFor_WindowsOverride(t *testing.T) {
	th := Theme{}
	th.Theme.DetectCommand = "command -v wt"
	th.Theme.DetectCommandWindows = "where wt"

	defer SetGOOSForTest("windows")()
	if got := DetectCommandFor(th); got != "where wt" {
		t.Errorf("on windows: got %q want %q", got, "where wt")
	}
}

func TestDetectCommandFor_FallbackWhenWindowsEmpty(t *testing.T) {
	th := Theme{}
	th.Theme.DetectCommand = "code --version"
	// No DetectCommandWindows set — Windows must fall back.

	defer SetGOOSForTest("windows")()
	if got := DetectCommandFor(th); got != "code --version" {
		t.Errorf("fallback on windows: got %q want %q", got, "code --version")
	}
}

func TestDetectCommandFor_UnixIgnoresWindowsOverride(t *testing.T) {
	th := Theme{}
	th.Theme.DetectCommand = "command -v wt"
	th.Theme.DetectCommandWindows = "where wt"

	defer SetGOOSForTest("darwin")()
	if got := DetectCommandFor(th); got != "command -v wt" {
		t.Errorf("on darwin: got %q want %q", got, "command -v wt")
	}
}

func TestVerifyCommandFor_WindowsOverride(t *testing.T) {
	th := Theme{}
	th.Verify.Command = "test -f ~/.config/starship.toml"
	th.Verify.CommandWindows = `if exist "%USERPROFILE%\.config\starship.toml" (exit 0) else (exit 1)`

	defer SetGOOSForTest("windows")()
	got := VerifyCommandFor(th)
	if got != th.Verify.CommandWindows {
		t.Errorf("on windows: got %q want windows variant", got)
	}
}

func TestVerifyCommandFor_FallbackWhenWindowsEmpty(t *testing.T) {
	th := Theme{}
	th.Verify.Command = "code --list-extensions"

	defer SetGOOSForTest("windows")()
	if got := VerifyCommandFor(th); got != "code --list-extensions" {
		t.Errorf("fallback on windows: got %q want unix variant", got)
	}
}

func TestLoadSupported_DarwinReturnsDarwinSupportedOnly(t *testing.T) {
	// On darwin LoadSupported must include every manifest that defaults
	// to ["darwin", "linux"] (i.e. the bulk of the embedded set) and
	// every manifest with darwin in supported_os, but exclude any
	// windows-only manifest. windows-terminal is the canonical exclude
	// here — its supported_os is ["windows"], so it ships in LoadAll
	// but must be filtered out of LoadSupported on darwin.
	defer SetGOOSForTest("darwin")()
	got, err := LoadSupported()
	if err != nil {
		t.Fatal(err)
	}
	for _, th := range got {
		if !SupportsCurrentOS(th) {
			t.Errorf("LoadSupported leaked %s on darwin (SupportedOS=%v)", th.Theme.Slug, th.Theme.SupportedOS)
		}
		if th.Theme.Slug == "windows-terminal" {
			t.Errorf("LoadSupported on darwin returned windows-only theme %q", th.Theme.Slug)
		}
	}
}

func TestLoadSupported_WindowsReturnsOptedInOnly(t *testing.T) {
	// On windows LoadSupported must include only the manifests that
	// explicitly list "windows" in supported_os. Every theme in the
	// returned slice must claim windows; the test doesn't lock the
	// exact slug list (so adding a fifth windows-supported manifest
	// later doesn't break this) but it does enforce a non-empty result
	// and the SupportsCurrentOS invariant.
	defer SetGOOSForTest("windows")()
	got, err := LoadSupported()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one windows-supported manifest in the embedded set")
	}
	for _, th := range got {
		if !SupportsCurrentOS(th) {
			t.Errorf("LoadSupported leaked %s on windows (SupportedOS=%v)", th.Theme.Slug, th.Theme.SupportedOS)
		}
	}
}
