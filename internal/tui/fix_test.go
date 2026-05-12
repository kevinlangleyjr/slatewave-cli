package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFixKindLabel(t *testing.T) {
	cases := map[FixKind]string{
		FixUpdate:     "update",
		FixUninstall:  "uninstall",
		FixDropOrphan: "drop",
		FixKind(99):   "", // unknown — defensive
	}
	for k, want := range cases {
		if got := fixKindLabel(k); got != want {
			t.Errorf("fixKindLabel(%v) = %q, want %q", k, got, want)
		}
	}
}

func TestFixModel_ProgressUpdatesRowState(t *testing.T) {
	m := newFixModel([]Fix{
		{Slug: "bat", Name: "Slatewave for bat", Kind: FixUpdate},
		{Slug: "btop", Name: "Slatewave for btop", Kind: FixDropOrphan},
	}, "Fixing")
	updated, _ := m.Update(fixProgressMsg{slug: "bat", state: fixRunning, step: "refreshing"})
	m = updated.(fixModel)
	if m.rowMap["bat"].state != fixRunning {
		t.Errorf("bat state = %v, want fixRunning", m.rowMap["bat"].state)
	}
	if m.rowMap["btop"].state != fixPending {
		t.Errorf("btop state mutated to %v, expected fixPending", m.rowMap["btop"].state)
	}
}

func TestFixModel_FailureCarriesError(t *testing.T) {
	m := newFixModel([]Fix{{Slug: "bat", Name: "bat", Kind: FixUpdate}}, "Fixing")
	wantErr := errors.New("post-hook bombed")
	updated, _ := m.Update(fixProgressMsg{slug: "bat", state: fixFailed, err: wantErr})
	m = updated.(fixModel)
	if m.rowMap["bat"].err != wantErr {
		t.Errorf("err didn't propagate: got %v, want %v", m.rowMap["bat"].err, wantErr)
	}
}

func TestFixModel_UnknownSlugIsNoOp(t *testing.T) {
	m := newFixModel([]Fix{{Slug: "bat", Name: "bat", Kind: FixUpdate}}, "Fixing")
	updated, _ := m.Update(fixProgressMsg{slug: "ghost", state: fixDone})
	m = updated.(fixModel)
	if m.rowMap["bat"].state != fixPending {
		t.Errorf("unrelated row mutated: bat state = %v", m.rowMap["bat"].state)
	}
}

func TestFixModel_CompleteMsgQuits(t *testing.T) {
	m := newFixModel([]Fix{{Slug: "bat", Name: "bat", Kind: FixUpdate}}, "Fixing")
	updated, cmd := m.Update(fixCompleteMsg{})
	m = updated.(fixModel)
	if !m.done {
		t.Error("fixCompleteMsg should set done=true")
	}
	if cmd == nil {
		t.Error("fixCompleteMsg should return tea.Quit cmd")
	}
}

func TestFixModel_CtrlCQuits(t *testing.T) {
	m := newFixModel([]Fix{{Slug: "bat", Name: "bat", Kind: FixUpdate}}, "Fixing")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("ctrl-c should quit")
	}
}

func TestFixModel_ViewRendersRows(t *testing.T) {
	m := newFixModel([]Fix{
		{Slug: "bat", Name: "Slatewave for bat", Kind: FixUpdate},
		{Slug: "old", Name: "Slatewave for old", Kind: FixDropOrphan},
	}, "Fixing")
	out := m.View()
	if !strings.Contains(out, "Fixing") {
		t.Error("view missing Fixing header")
	}
	if !strings.Contains(out, "Slatewave for bat") {
		t.Error("view missing bat row")
	}
	if !strings.Contains(out, "update") {
		t.Error("view missing update kind label")
	}
	if !strings.Contains(out, "drop") {
		t.Error("view missing drop kind label")
	}
}

func TestFixModel_DoneViewShowsSummary(t *testing.T) {
	m := newFixModel([]Fix{
		{Slug: "a", Name: "a", Kind: FixUpdate},
		{Slug: "b", Name: "b", Kind: FixUpdate},
	}, "Fixing")
	updated, _ := m.Update(fixProgressMsg{slug: "a", state: fixDone})
	m = updated.(fixModel)
	updated, _ = m.Update(fixProgressMsg{slug: "b", state: fixFailed, err: errors.New("x")})
	m = updated.(fixModel)
	updated, _ = m.Update(fixCompleteMsg{})
	m = updated.(fixModel)

	out := m.View()
	if !strings.Contains(out, "1 fixed") {
		t.Errorf("summary missing `1 fixed`: %s", out)
	}
	if !strings.Contains(out, "1 failed") {
		t.Errorf("summary missing `1 failed`: %s", out)
	}
}

func TestRunFix_EmptySliceReturnsNil(t *testing.T) {
	if err := RunFix(t.Context(), nil, FixOptions{}); err != nil {
		t.Errorf("RunFix(nil) = %v, want nil", err)
	}
}

func TestFixModel_CustomTitleAppearsInView(t *testing.T) {
	// `slatewave update --interactive` reuses the fix dashboard but passes
	// "Updating" via FixOptions.Title. Pin that the title actually flows
	// into the rendered View — otherwise update would silently say
	// "Fixing" which is the wrong verb for a refresh.
	m := newFixModel([]Fix{{Slug: "bat", Name: "bat", Kind: FixUpdate}}, "Updating")
	out := m.View()
	if !strings.Contains(out, "Updating") {
		t.Errorf("custom title not in View: %q", out)
	}
	if strings.Contains(out, "Fixing") {
		t.Error("default title leaked through when custom title was set")
	}
}
