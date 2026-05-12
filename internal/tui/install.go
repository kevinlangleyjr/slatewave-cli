package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kevinlangleyjr/slatewave-cli/internal/activator"
	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// rowState is the lifecycle of one theme inside the dashboard. Pending themes haven't started yet, the running states (detecting / installing / activating) reflect which step the install pipeline is on, and done / failed are terminal.
type rowState int

const (
	rowPending rowState = iota
	rowDetecting
	rowInstalling
	rowActivating
	rowDone
	rowFailed
)

// installRow is one row in the dashboard — a theme + its current state. Pointers live in the Model so the install goroutine and the bubbletea Update loop both see the same instance.
type installRow struct {
	slug  string
	name  string
	state rowState
	step  string
	err   error
}

// progressMsg is sent from the install goroutine via tea.Program.Send to update one row's state. installCompleteMsg signals the goroutine has finished all themes — the model quits in response.
type progressMsg struct {
	slug  string
	state rowState
	step  string
	err   error
}

type installCompleteMsg struct{}

// installModel is the bubbletea Model. The install goroutine runs in
// the background and pushes progressMsg via tea.Program.Send; Update
// applies them to the matching row, View re-renders.
//
// Concurrency contract: the goroutine in RunInstall only ever
// communicates with this model by sending progressMsg through
// p.Send — it never writes to rows / rowMap / row fields directly.
// Update is the sole writer of row state and runs on bubbletea's
// event loop. tea.Program.Send synchronizes through bubbletea's
// internal channel, so the messages happen-before the corresponding
// Update call and there's no race on the shared *installRow pointers.
// Future contributors: don't "optimize" by writing rowMap[slug].state
// from the goroutine — that's the failure mode this contract exists
// to forbid.
//
// cancel is the CancelFunc paired with the context that flows into
// runInstallPipeline. KeyCtrlC calls it before returning tea.Quit so
// the in-flight subprocess (git clone, post-hook) is killed instead of
// orphaned. nil when no parent has registered one — Update no-ops in
// that case, preserving the old "just quit the dashboard" behavior for
// callers that don't construct via RunInstall.
type installModel struct {
	rows    []*installRow
	rowMap  map[string]*installRow
	spinner spinner.Model
	cancel  context.CancelFunc
	done    bool
}

func newInstallModel(themes []manifest.Theme) installModel {
	rows := make([]*installRow, len(themes))
	rowMap := make(map[string]*installRow, len(themes))
	for i, th := range themes {
		rows[i] = &installRow{
			slug:  th.Theme.Slug,
			name:  th.Theme.Name,
			state: rowPending,
			step:  "queued",
		}
		rowMap[th.Theme.Slug] = rows[i]
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ui.Teal300)
	return installModel{rows: rows, rowMap: rowMap, spinner: sp}
}

func (m installModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m installModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progressMsg:
		if row, ok := m.rowMap[msg.slug]; ok {
			row.state = msg.state
			row.step = msg.step
			row.err = msg.err
		}
		return m, nil
	case installCompleteMsg:
		m.done = true
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m installModel) View() string {
	var b strings.Builder

	header := lipgloss.NewStyle().Bold(true).Foreground(ui.Slate200).Render("Installing")
	b.WriteString(header)
	b.WriteString("\n\n")

	for _, row := range m.rows {
		b.WriteString(m.renderRow(row))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.done {
		b.WriteString(m.summary())
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(ui.Slate500).Render("  ctrl-c to cancel"))
	}
	return b.String()
}

func (m installModel) renderRow(row *installRow) string {
	const nameWidth = 22
	name := lipgloss.NewStyle().Width(nameWidth).Foreground(ui.Slate200).Render(row.name)

	var marker, status string
	switch row.state {
	case rowPending:
		marker = lipgloss.NewStyle().Foreground(ui.Slate500).Render("·")
		status = lipgloss.NewStyle().Foreground(ui.Slate500).Render("queued")
	case rowDetecting, rowInstalling, rowActivating:
		marker = m.spinner.View()
		status = lipgloss.NewStyle().Foreground(ui.Slate400).Render(row.step)
	case rowDone:
		marker = lipgloss.NewStyle().Foreground(ui.Teal300).Render("✓")
		status = lipgloss.NewStyle().Foreground(ui.Teal300).Render("done")
	case rowFailed:
		marker = lipgloss.NewStyle().Foreground(ui.Rose400).Render("✗")
		detail := "failed"
		if row.err != nil {
			detail = "failed: " + truncate(row.err.Error(), 60)
		}
		status = lipgloss.NewStyle().Foreground(ui.Rose400).Render(detail)
	}
	return "  " + marker + "  " + name + "  " + status
}

func (m installModel) summary() string {
	var done, failed int
	for _, row := range m.rows {
		switch row.state {
		case rowDone:
			done++
		case rowFailed:
			failed++
		}
	}
	parts := []string{
		lipgloss.NewStyle().Foreground(ui.Teal300).Render(fmt.Sprintf("%d done", done)),
	}
	if failed > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(ui.Rose400).Render(fmt.Sprintf("%d failed", failed)))
	}
	return "  " + strings.Join(parts, lipgloss.NewStyle().Foreground(ui.Slate400).Render(" · "))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// InstallOptions configures a TUI install run. DryRun threads through to installer / activator so the dashboard can preview without touching disk.
type InstallOptions struct {
	DryRun bool
}

// RunInstall executes the install pipeline for every theme in the slice and renders progress as a live TUI. Returns nil if every theme installed successfully, or a non-nil error reflecting the *number* of failures (not the first error) so the caller can summarize without losing context.
//
// ctx flows from the caller (cobra's cmd.Context()) into runInstallPipeline so a SIGINT at the CLI layer cancels the in-flight subprocess. RunInstall wraps it in a CancelFunc stashed on the model so the bubbletea KeyCtrlC handler can cancel locally — bubbletea grabs raw stdin, so Ctrl-C inside the dashboard arrives as a KeyMsg, not as SIGINT, and that signal-aware ctx alone isn't enough.
func RunInstall(ctx context.Context, themes []manifest.Theme, opts InstallOptions) error {
	if len(themes) == 0 {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := newInstallModel(themes)
	m.cancel = cancel
	p := tea.NewProgram(m)

	go func() {
		// Track which slug is in-flight so a panic deep in the install
		// pipeline surfaces in the dashboard row instead of silently
		// hanging the program. Without this, a nil-pointer deref in any
		// dispatcher would kill the goroutine before installCompleteMsg
		// fires — Update never sees a quit, the user is stuck.
		var currentSlug string
		defer func() {
			if r := recover(); r != nil {
				if currentSlug != "" {
					p.Send(progressMsg{slug: currentSlug, state: rowFailed, err: fmt.Errorf("panic: %v", r)})
				}
				p.Send(installCompleteMsg{})
			}
		}()
		for _, th := range themes {
			currentSlug = th.Theme.Slug
			runInstallPipeline(runCtx, p, th, opts)
		}
		p.Send(installCompleteMsg{})
	}()

	final, err := p.Run()
	if err != nil {
		return err
	}
	finalModel, ok := final.(installModel)
	if !ok {
		return fmt.Errorf("internal: bubbletea returned unexpected model type %T", final)
	}
	var failed int
	for _, row := range finalModel.rows {
		if row.state == rowFailed {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d themes failed to install", failed, len(themes))
	}
	return nil
}

// runInstallPipeline mirrors cmd/install.go's installOne but emits progress to a bubbletea program rather than printing static step lines. Side effects (state writes, file changes) are identical so a TUI install and a plain install land the same observable result.
//
// Bails fast on a cancelled ctx so post-Ctrl-C the goroutine doesn't waste a detect timeout per remaining theme before quietly exiting.
func runInstallPipeline(ctx context.Context, p *tea.Program, th manifest.Theme, opts InstallOptions) {
	if ctx.Err() != nil {
		return
	}
	slug := th.Theme.Slug

	if th.Install.Type != "marketplace" && th.Install.Type != "manual" {
		p.Send(progressMsg{slug: slug, state: rowDetecting, step: "detecting"})
		if err := installer.Detect(th); err != nil {
			p.Send(progressMsg{slug: slug, state: rowFailed, err: err})
			return
		}
	}

	p.Send(progressMsg{slug: slug, state: rowInstalling, step: installStepLabel(th)})
	rec, err := installer.Install(ctx, th, installer.Options{DryRun: opts.DryRun})
	if err != nil {
		p.Send(progressMsg{slug: slug, state: rowFailed, err: err})
		return
	}

	if th.Activate.Type != "" && th.Activate.Type != "none" {
		p.Send(progressMsg{slug: slug, state: rowActivating, step: "activating"})
		if err := activator.Activate(th, &rec, activator.Options{DryRun: opts.DryRun}); err != nil {
			p.Send(progressMsg{slug: slug, state: rowFailed, err: err})
			return
		}
	}

	if !opts.DryRun {
		if err := state.Update(func(s *state.Store) error {
			s.Put(rec)
			return nil
		}); err != nil {
			p.Send(progressMsg{slug: slug, state: rowFailed, err: err})
			return
		}
	}

	p.Send(progressMsg{slug: slug, state: rowDone})
}

func installStepLabel(th manifest.Theme) string {
	switch th.Install.Type {
	case "curl", "gui-import":
		return "fetching"
	case "clone":
		return "cloning"
	case "vscode-ext":
		return "installing extension"
	case "marketplace":
		return "opening marketplace"
	default:
		return "installing"
	}
}
