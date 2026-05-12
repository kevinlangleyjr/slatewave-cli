package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
)

func TestInstallStepLabel(t *testing.T) {
	cases := []struct {
		installType string
		want        string
	}{
		{"curl", "fetching"},
		{"gui-import", "fetching"},
		{"clone", "cloning"},
		{"vscode-ext", "installing extension"},
		{"marketplace", "opening marketplace"},
		{"manual", "installing"},       // default branch
		{"made-up-type", "installing"}, // unknown also falls through
	}
	for _, c := range cases {
		th := manifest.Theme{Install: manifest.Install{Type: c.installType}}
		if got := installStepLabel(th); got != c.want {
			t.Errorf("installStepLabel(%q) = %q, want %q", c.installType, got, c.want)
		}
	}
}

func TestTruncate_ShortStringUntouched(t *testing.T) {
	if got := truncate("hi", 10); got != "hi" {
		t.Errorf("truncate(short, 10) = %q, want %q", got, "hi")
	}
}

func TestTruncate_LongStringClipped(t *testing.T) {
	long := strings.Repeat("x", 100)
	got := truncate(long, 10)
	if len([]rune(got)) != 10 {
		t.Errorf("truncate to 10 returned %d runes: %q", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncate did not end with ellipsis: %q", got)
	}
}

func TestInstallModel_ProgressUpdatesRowState(t *testing.T) {
	m := newInstallModel([]manifest.Theme{
		stubTheme("bat", "editor", "true"),
		stubTheme("btop", "terminal", "true"),
	})

	// Pending → Detecting via progressMsg.
	updated, _ := m.Update(progressMsg{slug: "bat", state: rowDetecting, step: "detecting"})
	m = updated.(installModel)
	if m.rowMap["bat"].state != rowDetecting {
		t.Errorf("bat state = %v, want rowDetecting", m.rowMap["bat"].state)
	}
	if m.rowMap["bat"].step != "detecting" {
		t.Errorf("bat step = %q, want %q", m.rowMap["bat"].step, "detecting")
	}
	// btop must remain untouched — the progressMsg only targeted bat.
	if m.rowMap["btop"].state != rowPending {
		t.Errorf("btop state changed to %v, expected rowPending", m.rowMap["btop"].state)
	}
}

func TestInstallModel_FailureCarriesError(t *testing.T) {
	m := newInstallModel([]manifest.Theme{stubTheme("bat", "editor", "true")})
	wantErr := errors.New("boom")
	updated, _ := m.Update(progressMsg{slug: "bat", state: rowFailed, err: wantErr})
	m = updated.(installModel)
	if m.rowMap["bat"].err != wantErr {
		t.Errorf("rowFailed didn't propagate err: got %v, want %v", m.rowMap["bat"].err, wantErr)
	}
}

func TestInstallModel_UnknownSlugIsNoOp(t *testing.T) {
	// progressMsg for a slug that's not in the model shouldn't panic — the
	// install goroutine and the model are decoupled and a stray send must
	// not crash the program.
	m := newInstallModel([]manifest.Theme{stubTheme("bat", "editor", "true")})
	updated, _ := m.Update(progressMsg{slug: "ghost", state: rowDone})
	m = updated.(installModel)
	if m.rowMap["bat"].state != rowPending {
		t.Errorf("unrelated row mutated: bat state = %v", m.rowMap["bat"].state)
	}
}

func TestInstallModel_CompleteMsgQuits(t *testing.T) {
	m := newInstallModel([]manifest.Theme{stubTheme("bat", "editor", "true")})
	updated, cmd := m.Update(installCompleteMsg{})
	m = updated.(installModel)
	if !m.done {
		t.Error("installCompleteMsg should set done=true")
	}
	if cmd == nil {
		t.Error("installCompleteMsg should return tea.Quit cmd, got nil")
	}
}

func TestInstallModel_CtrlCQuits(t *testing.T) {
	m := newInstallModel([]manifest.Theme{stubTheme("bat", "editor", "true")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("ctrl-c should return tea.Quit cmd")
	}
}

func TestInstallModel_ViewRendersAllRows(t *testing.T) {
	m := newInstallModel([]manifest.Theme{
		stubTheme("bat", "editor", "true"),
		stubTheme("btop", "terminal", "true"),
	})
	out := m.View()
	if !strings.Contains(out, "Slatewave for bat") {
		t.Error("view missing bat row")
	}
	if !strings.Contains(out, "Slatewave for btop") {
		t.Error("view missing btop row")
	}
	if !strings.Contains(out, "Installing") {
		t.Error("view missing Installing header")
	}
}

func TestInstallModel_DoneViewShowsSummary(t *testing.T) {
	m := newInstallModel([]manifest.Theme{
		stubTheme("bat", "editor", "true"),
		stubTheme("btop", "terminal", "true"),
	})
	updated, _ := m.Update(progressMsg{slug: "bat", state: rowDone})
	m = updated.(installModel)
	updated, _ = m.Update(progressMsg{slug: "btop", state: rowFailed, err: errors.New("nope")})
	m = updated.(installModel)
	updated, _ = m.Update(installCompleteMsg{})
	m = updated.(installModel)

	out := m.View()
	if !strings.Contains(out, "1 done") {
		t.Errorf("done summary missing `1 done`: %s", out)
	}
	if !strings.Contains(out, "1 failed") {
		t.Errorf("done summary missing `1 failed`: %s", out)
	}
}

func TestRunInstall_EmptySliceReturnsNil(t *testing.T) {
	// The fast-exit path: no themes means no tea.Program startup, which is
	// what we want for callers that filter to nothing.
	if err := RunInstall(t.Context(), nil, InstallOptions{}); err != nil {
		t.Errorf("RunInstall(nil) = %v, want nil", err)
	}
}
