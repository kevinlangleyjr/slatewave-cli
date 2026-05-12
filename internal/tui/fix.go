package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/kevinlangleyjr/slatewave-cli/internal/installer"
	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/shell"
	"github.com/kevinlangleyjr/slatewave-cli/internal/state"
	"github.com/kevinlangleyjr/slatewave-cli/internal/ui"
)

// FixKind is the remedy a doctor row maps to. The set is closed: doctor's diagnose() only emits stale / missing-tool / orphan as fixable, and each one corresponds to exactly one remedy.
type FixKind int

const (
	// FixUpdate re-runs the install pipeline's asset refresh (installer.Update + post-hook). Used for stale rows where verify failed but the tool itself is still present.
	FixUpdate FixKind = iota
	// FixUninstall reverses the install footprint and drops the state record. Used for missing-tool rows where the underlying tool is gone but our config edits / state record may still linger.
	FixUninstall
	// FixDropOrphan drops the state record without touching the filesystem. Used for orphan rows where there's no manifest left to drive a real uninstall — the install footprint is unrecoverable from here, but the lingering state record is.
	FixDropOrphan
)

// Fix is one remediation the dashboard will execute. Theme is the loaded manifest for FixUpdate / FixUninstall. Orphan fixes carry a zero Theme since the manifest is what's missing — the dashboard only needs Slug + Name to render and act.
type Fix struct {
	Slug  string
	Name  string
	Kind  FixKind
	Theme manifest.Theme
}

// FixOptions configures a TUI fix run. DryRun threads through to installer / state writes so users can preview without disk side effects. Title overrides the dashboard header — defaults to "Fixing" but `slatewave update --interactive` passes "Updating" since the same pipeline backs both flows but the verb the user expects differs.
type FixOptions struct {
	DryRun bool
	Title  string
}

type fixRowState int

const (
	fixPending fixRowState = iota
	fixRunning
	fixDone
	fixFailed
)

type fixRow struct {
	slug  string
	name  string
	kind  FixKind
	state fixRowState
	step  string
	err   error
}

type fixProgressMsg struct {
	slug  string
	state fixRowState
	step  string
	err   error
}

type fixCompleteMsg struct{}

type fixModel struct {
	rows    []*fixRow
	rowMap  map[string]*fixRow
	spinner spinner.Model
	title   string
	// cancel is the CancelFunc paired with the context that flows into
	// runFixPipeline. KeyCtrlC calls it before quitting the dashboard so
	// the in-flight subprocess (git pull, post-hook, code --uninstall-
	// extension) is killed instead of orphaned. nil when no parent has
	// registered one — matches the v0.0.19 behavior for direct callers.
	cancel context.CancelFunc
	done   bool
}

func newFixModel(fixes []Fix, title string) fixModel {
	rows := make([]*fixRow, len(fixes))
	rowMap := make(map[string]*fixRow, len(fixes))
	for i, f := range fixes {
		rows[i] = &fixRow{
			slug:  f.Slug,
			name:  f.Name,
			kind:  f.Kind,
			state: fixPending,
			step:  "queued",
		}
		rowMap[f.Slug] = rows[i]
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ui.Teal300)
	return fixModel{rows: rows, rowMap: rowMap, spinner: sp, title: title}
}

func (m fixModel) Init() tea.Cmd { return m.spinner.Tick }

func (m fixModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fixProgressMsg:
		if row, ok := m.rowMap[msg.slug]; ok {
			row.state = msg.state
			row.step = msg.step
			row.err = msg.err
		}
		return m, nil
	case fixCompleteMsg:
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

func (m fixModel) View() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ui.Slate200).Render(m.title))
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

func (m fixModel) renderRow(row *fixRow) string {
	const nameWidth = 22
	name := lipgloss.NewStyle().Width(nameWidth).Foreground(ui.Slate200).Render(row.name)
	kind := lipgloss.NewStyle().Width(10).Foreground(ui.Slate500).Render(fixKindLabel(row.kind))

	var marker, status string
	switch row.state {
	case fixPending:
		marker = lipgloss.NewStyle().Foreground(ui.Slate500).Render("·")
		status = lipgloss.NewStyle().Foreground(ui.Slate500).Render("queued")
	case fixRunning:
		marker = m.spinner.View()
		status = lipgloss.NewStyle().Foreground(ui.Slate400).Render(row.step)
	case fixDone:
		marker = lipgloss.NewStyle().Foreground(ui.Teal300).Render("✓")
		status = lipgloss.NewStyle().Foreground(ui.Teal300).Render("fixed")
	case fixFailed:
		marker = lipgloss.NewStyle().Foreground(ui.Rose400).Render("✗")
		detail := "failed"
		if row.err != nil {
			detail = "failed: " + truncate(row.err.Error(), 60)
		}
		status = lipgloss.NewStyle().Foreground(ui.Rose400).Render(detail)
	}
	return "  " + marker + "  " + name + "  " + kind + "  " + status
}

func (m fixModel) summary() string {
	var done, failed int
	for _, row := range m.rows {
		switch row.state {
		case fixDone:
			done++
		case fixFailed:
			failed++
		}
	}
	parts := []string{
		lipgloss.NewStyle().Foreground(ui.Teal300).Render(fmt.Sprintf("%d fixed", done)),
	}
	if failed > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(ui.Rose400).Render(fmt.Sprintf("%d failed", failed)))
	}
	return "  " + strings.Join(parts, lipgloss.NewStyle().Foreground(ui.Slate400).Render(" · "))
}

func fixKindLabel(k FixKind) string {
	switch k {
	case FixUpdate:
		return "update"
	case FixUninstall:
		return "uninstall"
	case FixDropOrphan:
		return "drop"
	}
	return ""
}

// PickFixes shows a multi-select of fixable doctor rows so the user can confirm which remedies to run. Every fix is pre-selected — doctor --fix is a "yes please clean up the obvious stuff" action; deselecting is the exception. Returns the chosen fixes in input order. Returns (nil, ErrAborted) on Ctrl+C.
func PickFixes(fixes []Fix) ([]Fix, error) {
	if len(fixes) == 0 {
		return nil, nil
	}
	options := make([]huh.Option[string], len(fixes))
	preselected := make([]string, len(fixes))
	for i, f := range fixes {
		label := fmt.Sprintf("%s — %s", f.Name, fixKindLabel(f.Kind))
		options[i] = huh.NewOption(label, f.Slug)
		preselected[i] = f.Slug
	}

	selected := preselected
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Fix which issues?").
				Description("Spacebar toggles, enter confirms.").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ErrAborted
		}
		return nil, err
	}

	keep := map[string]bool{}
	for _, s := range selected {
		keep[s] = true
	}
	out := make([]Fix, 0, len(selected))
	for _, f := range fixes {
		if keep[f.Slug] {
			out = append(out, f)
		}
	}
	return out, nil
}

// RunFix executes each fix serially and renders progress as a live TUI. Returns nil if every fix succeeded, or a non-nil error reflecting the *count* of failures so the caller can summarize.
//
// Wraps ctx in a CancelFunc stashed on the model so the bubbletea KeyCtrlC handler can cancel the in-flight subprocess. Bubbletea grabs raw stdin, so Ctrl-C inside the dashboard arrives as a KeyMsg, not as SIGINT — the signal-aware ctx at the CLI layer alone wouldn't reach this far.
func RunFix(ctx context.Context, fixes []Fix, opts FixOptions) error {
	if len(fixes) == 0 {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	title := opts.Title
	if title == "" {
		title = "Fixing"
	}
	m := newFixModel(fixes, title)
	m.cancel = cancel
	p := tea.NewProgram(m)

	go func() {
		// Same shape as RunInstall's worker: track the in-flight slug
		// and recover() so a panic in installer.Update / installer.Uninstall
		// surfaces on the row instead of leaving the dashboard spinning
		// forever waiting for fixCompleteMsg.
		var currentSlug string
		defer func() {
			if r := recover(); r != nil {
				if currentSlug != "" {
					p.Send(fixProgressMsg{slug: currentSlug, state: fixFailed, err: fmt.Errorf("panic: %v", r)})
				}
				p.Send(fixCompleteMsg{})
			}
		}()
		for _, f := range fixes {
			currentSlug = f.Slug
			runFixPipeline(runCtx, p, f, opts)
		}
		p.Send(fixCompleteMsg{})
	}()

	final, err := p.Run()
	if err != nil {
		return err
	}
	finalModel, ok := final.(fixModel)
	if !ok {
		return fmt.Errorf("internal: bubbletea returned unexpected model type %T", final)
	}
	var failed int
	for _, row := range finalModel.rows {
		if row.state == fixFailed {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d fixes failed", failed, len(fixes))
	}
	return nil
}

func runFixPipeline(ctx context.Context, p *tea.Program, f Fix, opts FixOptions) {
	if ctx.Err() != nil {
		return
	}
	switch f.Kind {
	case FixUpdate:
		runUpdateFix(ctx, p, f, opts)
	case FixUninstall:
		runUninstallFix(p, f, opts)
	case FixDropOrphan:
		runDropOrphanFix(p, f, opts)
	}
}

// runUpdateFix mirrors cmd/update.go's updateOne body so a stale theme gets the same asset refresh + post-hook + timestamp bump as a manual `slatewave update`.
func runUpdateFix(ctx context.Context, p *tea.Program, f Fix, opts FixOptions) {
	slug := f.Slug
	p.Send(fixProgressMsg{slug: slug, state: fixRunning, step: "refreshing assets"})
	if err := installer.Update(ctx, f.Theme, installer.Options{DryRun: opts.DryRun}); err != nil {
		p.Send(fixProgressMsg{slug: slug, state: fixFailed, err: err})
		return
	}
	if f.Theme.Install.Post != nil && !opts.DryRun {
		p.Send(fixProgressMsg{slug: slug, state: fixRunning, step: f.Theme.Install.Post.Description})
		if err := shell.RunInherit(ctx, f.Theme.Install.Post.Command); err != nil {
			p.Send(fixProgressMsg{slug: slug, state: fixFailed, err: fmt.Errorf("post-hook: %w", err)})
			return
		}
	}
	if !opts.DryRun {
		if err := state.Update(func(s *state.Store) error {
			if rec, ok := s.Get(slug); ok {
				rec.InstalledAt = time.Now().UTC()
				s.Put(rec)
			}
			return nil
		}); err != nil {
			p.Send(fixProgressMsg{slug: slug, state: fixFailed, err: err})
			return
		}
	}
	p.Send(fixProgressMsg{slug: slug, state: fixDone})
}

// runUninstallFix mirrors cmd/uninstall.go: reverse the install footprint via installer.Uninstall, then drop the state record. For missing-tool rows the underlying tool is gone, but our config edits + state record can still be cleaned up safely.
func runUninstallFix(p *tea.Program, f Fix, opts FixOptions) {
	slug := f.Slug
	p.Send(fixProgressMsg{slug: slug, state: fixRunning, step: "reversing install"})

	s, err := state.Load()
	if err != nil {
		p.Send(fixProgressMsg{slug: slug, state: fixFailed, err: err})
		return
	}
	rec, ok := s.Get(slug)
	if !ok {
		p.Send(fixProgressMsg{slug: slug, state: fixFailed, err: fmt.Errorf("no state record")})
		return
	}
	if err := installer.Uninstall(rec, f.Theme, installer.Options{DryRun: opts.DryRun}); err != nil {
		p.Send(fixProgressMsg{slug: slug, state: fixFailed, err: err})
		return
	}
	if !opts.DryRun {
		if err := state.Update(func(s *state.Store) error {
			s.Remove(slug)
			return nil
		}); err != nil {
			p.Send(fixProgressMsg{slug: slug, state: fixFailed, err: err})
			return
		}
	}
	p.Send(fixProgressMsg{slug: slug, state: fixDone})
}

// runDropOrphanFix drops a state record whose manifest no longer ships. We can't run installer.Uninstall — it needs the manifest to know what to reverse — but the lingering state record is itself the bug we're fixing.
func runDropOrphanFix(p *tea.Program, f Fix, opts FixOptions) {
	slug := f.Slug
	p.Send(fixProgressMsg{slug: slug, state: fixRunning, step: "dropping orphan record"})
	if opts.DryRun {
		p.Send(fixProgressMsg{slug: slug, state: fixDone})
		return
	}
	if err := state.Update(func(s *state.Store) error {
		s.Remove(slug)
		return nil
	}); err != nil {
		p.Send(fixProgressMsg{slug: slug, state: fixFailed, err: err})
		return
	}
	p.Send(fixProgressMsg{slug: slug, state: fixDone})
}
