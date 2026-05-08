package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// BrowseActionKind is what the user asked the browser to do on quit. None means they exited without picking an action — the cmd layer should render nothing further.
type BrowseActionKind int

const (
	BrowseNone BrowseActionKind = iota
	BrowseInstall
	BrowseUninstall
)

// BrowseAction is the browser's return value: a kind + the slug the user was focused on when they triggered it. The cmd layer dispatches: install routes through tui.RunInstall, uninstall through the existing uninstall pipeline.
type BrowseAction struct {
	Kind BrowseActionKind
	Slug string
}

// browseItem implements list.Item + list.DefaultItem. Title gets the install marker prepended so the focused-vs-unfocused distinction in bubbles/list's default delegate still works visually. Description carries category + tool-detected note.
type browseItem struct {
	th        manifest.Theme
	installed bool
	detected  bool
}

func (i browseItem) Title() string {
	const (
		dotInstalled    = "●"
		dotNotInstalled = "○"
	)
	marker := dotNotInstalled
	if i.installed {
		marker = dotInstalled
	}
	return marker + "  " + i.th.Theme.Name
}

func (i browseItem) Description() string {
	parts := i.th.Theme.Slug + " · " + i.th.Theme.Category
	switch {
	case i.installed:
		parts += " · installed"
	case !i.detected:
		parts += " · tool not detected"
	}
	return parts
}

// FilterValue feeds bubbles/list's built-in / filter. Including category lets users type "term" to find every terminal theme — useful when the registry grows.
func (i browseItem) FilterValue() string {
	return i.th.Theme.Name + " " + i.th.Theme.Slug + " " + i.th.Theme.Category
}

type browseKeys struct {
	install   key.Binding
	uninstall key.Binding
}

func newBrowseKeys() browseKeys {
	return browseKeys{
		install: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "install"),
		),
		uninstall: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "uninstall"),
		),
	}
}

type browseModel struct {
	list   list.Model
	keys   browseKeys
	action BrowseAction
}

func newBrowseModel(themes []manifest.Theme, installed, detected map[string]bool) browseModel {
	items := make([]list.Item, len(themes))
	for i, th := range themes {
		items[i] = browseItem{
			th:        th,
			installed: installed[th.Theme.Slug],
			detected:  detected[th.Theme.Slug],
		}
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(ui.Teal300).BorderForeground(ui.Teal300)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(ui.Slate400).BorderForeground(ui.Teal300)

	l := list.New(items, delegate, 0, 0)
	l.Title = "Slatewave themes"
	l.Styles.Title = l.Styles.Title.Background(ui.Teal300).Foreground(lipgloss.Color("#0b1220"))
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)

	keys := newBrowseKeys()
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{keys.install, keys.uninstall}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{keys.install, keys.uninstall}
	}

	return browseModel{list: l, keys: keys}
}

func (m browseModel) Init() tea.Cmd { return nil }

func (m browseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve rows for the embedded banner header. View() prepends
		// ui.Banner() + a blank-line separator, so the list gets
		// whatever's left under that.
		listHeight := max(msg.Height-ui.BannerHeight-1, 1)
		m.list.SetSize(msg.Width, listHeight)
		return m, nil
	case tea.KeyMsg:
		// While the user is typing into the / filter, every key is for the filter — don't intercept i/u.
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch {
		case key.Matches(msg, m.keys.install):
			if it, ok := m.list.SelectedItem().(browseItem); ok && !it.installed {
				m.action = BrowseAction{Kind: BrowseInstall, Slug: it.th.Theme.Slug}
				return m, tea.Quit
			}
		case key.Matches(msg, m.keys.uninstall):
			if it, ok := m.list.SelectedItem().(browseItem); ok && it.installed {
				m.action = BrowseAction{Kind: BrowseUninstall, Slug: it.th.Theme.Slug}
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m browseModel) View() string {
	return ui.Banner() + "\n\n" + m.list.View()
}

// RunBrowse opens the interactive theme browser. Returns the action the user picked, or BrowseNone if they quit without acting. detected maps slug → tool present? — pass an empty map to skip the tool-not-detected hint (e.g., in tests).
func RunBrowse(themes []manifest.Theme, installed, detected map[string]bool) (BrowseAction, error) {
	if len(themes) == 0 {
		return BrowseAction{}, nil
	}
	m := newBrowseModel(themes, installed, detected)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return BrowseAction{}, err
	}
	return final.(browseModel).action, nil
}
