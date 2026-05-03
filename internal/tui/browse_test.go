package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
)

func TestBrowseItem_TitleHasMarkerForInstalledAndNot(t *testing.T) {
	installed := browseItem{
		th:        stubTheme("bat", "editor", "true"),
		installed: true,
		detected:  true,
	}
	notInstalled := browseItem{
		th:        stubTheme("btop", "terminal", "true"),
		installed: false,
		detected:  true,
	}

	if !strings.HasPrefix(installed.Title(), "●") {
		t.Errorf("installed Title should start with ●, got %q", installed.Title())
	}
	if !strings.HasPrefix(notInstalled.Title(), "○") {
		t.Errorf("not-installed Title should start with ○, got %q", notInstalled.Title())
	}
	if !strings.Contains(installed.Title(), "Slatewave for bat") {
		t.Error("installed Title missing theme name")
	}
}

func TestBrowseItem_DescriptionShowsInstalledHint(t *testing.T) {
	got := browseItem{
		th:        stubTheme("bat", "editor", "true"),
		installed: true,
		detected:  true,
	}.Description()
	if !strings.Contains(got, "installed") {
		t.Errorf("installed Description should mention `installed`: %q", got)
	}
	if !strings.Contains(got, "editor") {
		t.Errorf("Description missing category: %q", got)
	}
}

func TestBrowseItem_DescriptionShowsToolNotDetected(t *testing.T) {
	got := browseItem{
		th:        stubTheme("ghostty", "terminal", "true"),
		installed: false,
		detected:  false,
	}.Description()
	if !strings.Contains(got, "tool not detected") {
		t.Errorf("missing-tool Description should mention `tool not detected`: %q", got)
	}
}

func TestBrowseItem_DescriptionPlainWhenAvailable(t *testing.T) {
	// detected + not installed: no extra hint, just slug · category.
	got := browseItem{
		th:        stubTheme("alacritty", "terminal", "true"),
		installed: false,
		detected:  true,
	}.Description()
	if strings.Contains(got, "installed") {
		t.Errorf("available theme Description shouldn't say installed: %q", got)
	}
	if strings.Contains(got, "tool not detected") {
		t.Errorf("available theme Description shouldn't say tool not detected: %q", got)
	}
}

func TestBrowseItem_FilterValueIncludesNameSlugCategory(t *testing.T) {
	got := browseItem{th: stubTheme("bat", "editor", "true")}.FilterValue()
	for _, want := range []string{"Slatewave for bat", "bat", "editor"} {
		if !strings.Contains(got, want) {
			t.Errorf("FilterValue missing %q: %q", want, got)
		}
	}
}

func TestBrowseModel_InstallKeyOnNotInstalledQueuesAction(t *testing.T) {
	m := newBrowseModel([]manifest.Theme{
		stubTheme("bat", "editor", "true"),
	}, nil, map[string]bool{"bat": true})
	// list.New starts focus at index 0, so SelectedItem is bat.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = updated.(browseModel)
	if m.action.Kind != BrowseInstall {
		t.Errorf("action.Kind = %v, want BrowseInstall", m.action.Kind)
	}
	if m.action.Slug != "bat" {
		t.Errorf("action.Slug = %q, want bat", m.action.Slug)
	}
	if cmd == nil {
		t.Error("install action should also issue tea.Quit cmd")
	}
}

func TestBrowseModel_InstallKeyIgnoredWhenAlreadyInstalled(t *testing.T) {
	m := newBrowseModel([]manifest.Theme{
		stubTheme("bat", "editor", "true"),
	}, map[string]bool{"bat": true}, map[string]bool{"bat": true})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = updated.(browseModel)
	if m.action.Kind != BrowseNone {
		t.Errorf("install on already-installed theme set action.Kind = %v, want BrowseNone", m.action.Kind)
	}
}

func TestBrowseModel_UninstallKeyOnInstalledQueuesAction(t *testing.T) {
	m := newBrowseModel([]manifest.Theme{
		stubTheme("bat", "editor", "true"),
	}, map[string]bool{"bat": true}, map[string]bool{"bat": true})
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = updated.(browseModel)
	if m.action.Kind != BrowseUninstall {
		t.Errorf("action.Kind = %v, want BrowseUninstall", m.action.Kind)
	}
	if m.action.Slug != "bat" {
		t.Errorf("action.Slug = %q, want bat", m.action.Slug)
	}
	if cmd == nil {
		t.Error("uninstall action should issue tea.Quit cmd")
	}
}

func TestBrowseModel_UninstallKeyIgnoredWhenNotInstalled(t *testing.T) {
	m := newBrowseModel([]manifest.Theme{
		stubTheme("bat", "editor", "true"),
	}, nil, map[string]bool{"bat": true})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = updated.(browseModel)
	if m.action.Kind != BrowseNone {
		t.Errorf("uninstall on not-installed theme set action = %v, want BrowseNone", m.action.Kind)
	}
}

func TestBrowseModel_FilterStateSuppressesActionKeys(t *testing.T) {
	// When the / filter is active, i and u are filter input — they must not
	// trigger install/uninstall. Drive the list into Filtering state and
	// then send 'i' to confirm.
	m := newBrowseModel([]manifest.Theme{
		stubTheme("bat", "editor", "true"),
	}, nil, map[string]bool{"bat": true})

	// Open the filter (default keybind in bubbles/list is '/').
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(browseModel)
	if m.list.FilterState() != list.Filtering {
		t.Skipf("could not drive list into Filtering state (got %v) — bubbles default may have changed", m.list.FilterState())
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = updated.(browseModel)
	if m.action.Kind != BrowseNone {
		t.Errorf("`i` while filtering triggered install action = %v, want BrowseNone", m.action.Kind)
	}
}

func TestBrowseModel_WindowSizeMsgResizesList(t *testing.T) {
	m := newBrowseModel([]manifest.Theme{stubTheme("bat", "editor", "true")}, nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(browseModel)
	if m.list.Width() != 80 || m.list.Height() != 24 {
		t.Errorf("list size = %dx%d, want 80x24", m.list.Width(), m.list.Height())
	}
}

func TestRunBrowse_EmptyThemesReturnsNoneNoError(t *testing.T) {
	got, err := RunBrowse(nil, nil, nil)
	if err != nil {
		t.Errorf("RunBrowse(nil) error = %v, want nil", err)
	}
	if got.Kind != BrowseNone {
		t.Errorf("RunBrowse(nil) action = %v, want BrowseNone", got.Kind)
	}
}

func TestNewBrowseKeys_HelpStringsArePresent(t *testing.T) {
	// The additional-help-keys callback registers `i` and `u` so they show
	// up in the bubbles/list help footer. If the help strings ever drift
	// to "" the help row would render as just "i  u" which is useless.
	keys := newBrowseKeys()
	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}, keys.install) {
		t.Error("install binding should match `i`")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}, keys.uninstall) {
		t.Error("uninstall binding should match `u`")
	}
	if keys.install.Help().Desc == "" || keys.uninstall.Help().Desc == "" {
		t.Error("install/uninstall key bindings missing help description")
	}
}
