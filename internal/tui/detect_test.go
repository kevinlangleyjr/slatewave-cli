package tui

import (
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
)

func TestDetectAll_ParallelDetectsTrueAndFalseExits(t *testing.T) {
	themes := []manifest.Theme{
		stubTheme("present", "editor", "true"),   // exit 0 → present
		stubTheme("absent", "terminal", "false"), // exit 1 → absent
		stubTheme("alsopresent", "notes", "true"),
	}
	installed := map[string]bool{"alsopresent": true}

	got := DetectAll(themes, installed)
	if len(got) != 3 {
		t.Fatalf("DetectAll returned %d results, want 3", len(got))
	}
	// Order must match input — callers (init wizard) rely on it.
	if got[0].Theme.Theme.Slug != "present" {
		t.Errorf("got[0] slug = %q, want present (input order must be preserved)", got[0].Theme.Theme.Slug)
	}
	if !got[0].Present {
		t.Error("expected `present` to be Present (detect_command=true exits 0)")
	}
	if got[1].Present {
		t.Error("expected `absent` to NOT be Present (detect_command=false exits 1)")
	}
	if got[2].Present != true || got[2].Installed != true {
		t.Errorf("alsopresent: Present=%v Installed=%v, want both true", got[2].Present, got[2].Installed)
	}
}

func TestDetectAll_EmptyDetectCommandIsNotPresent(t *testing.T) {
	// detect_command="" must short-circuit to NOT present — otherwise sh -c ""
	// would exit 0 and we'd falsely mark the tool as installed.
	themes := []manifest.Theme{stubTheme("noisy", "editor", "")}
	got := DetectAll(themes, nil)
	if got[0].Present {
		t.Error("expected empty detect_command to mark Present=false")
	}
}

func TestDetectAll_NilInstalledMapTreatedAsNoneInstalled(t *testing.T) {
	themes := []manifest.Theme{stubTheme("bat", "editor", "true")}
	got := DetectAll(themes, nil)
	if got[0].Installed {
		t.Error("nil installed map should leave Installed=false")
	}
}
