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

func TestLoadSupported_FiltersByCurrentOS(t *testing.T) {
	// LoadSupported on the embedded set: every manifest defaults to
	// darwin+linux, so on windows the result should be empty until
	// individual manifests opt in. This locks the filter behavior
	// without coupling the test to which manifests later opt in.
	defer SetGOOSForTest("windows")()
	got, err := LoadSupported()
	if err != nil {
		t.Fatalf("LoadSupported: %v", err)
	}
	for _, th := range got {
		if !SupportsCurrentOS(th) {
			t.Errorf("LoadSupported leaked %s on windows (SupportedOS=%v)", th.Theme.Slug, th.Theme.SupportedOS)
		}
	}
}

func TestLoadSupported_DarwinReturnsAll(t *testing.T) {
	// On darwin every existing manifest is supported (default behavior),
	// so LoadSupported should match LoadAll one-to-one.
	defer SetGOOSForTest("darwin")()
	all, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadSupported()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(all) {
		t.Errorf("on darwin: LoadSupported=%d LoadAll=%d (drift?)", len(got), len(all))
	}
}
