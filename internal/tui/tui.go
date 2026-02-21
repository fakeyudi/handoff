// Package tui provides a Bubble Tea TUI for viewing handoff bundles.
package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fakeyudi/handoff/internal/bundle"
)

// ── Styles ────────────

var (
	// Title bar at the very top
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 2)

	// Active tab: bright, underlined
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	// Inactive tab: muted
	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Background(lipgloss.Color("235")).
				Padding(0, 1)

	// Separator between tabs
	tabSepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238")).
			Background(lipgloss.Color("235"))

	// Section heading inside a tab
	sectionHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86"))

	// Key=value label
	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	timeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("178"))

	bulletStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205"))

	kindAnnotationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	kindFileEditStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	kindCommandStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("245")).
			Padding(0, 1)

	// Diff rendering
	diffAddStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	diffDelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	diffMetaStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	// Selected row in File Edits list
	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("237"))
)

// ── Tab definitions ─────────────────

type tabID int

const (
	tabSummary tabID = iota
	tabAnnotations
	tabFileEdits
	tabGit
	tabCommands
	tabEditorTabs
	tabTimeline
	tabCount
)

var tabNames = [tabCount]string{
	"Summary", "Annotations", "File Edits", "Git", "Commands", "Editor Tabs", "Timeline",
}

// ── Timeline event ───────────────────

type eventKind string

const (
	kindNote    eventKind = "NOTE"
	kindSummary eventKind = "SUMMARY"
	kindEdit    eventKind = "EDIT"
	kindCmd     eventKind = "CMD"
)

type timelineEvent struct {
	ts   time.Time
	kind eventKind
	text string
}

// ── Model ────────────────────

// Model is the root Bubble Tea model for the TUI.
type Model struct {
	bundle        *bundle.ContextBundle
	filename      string
	activeTab     tabID
	viewports     [tabCount]viewport.Model
	width         int
	height        int
	ready         bool
	sortAsc       bool
	timeline      []timelineEvent
	// File Edits tab: cursor position and expanded set
	editCursor    int
	expandedEdits map[int]bool
}

// New creates a new TUI model for the given bundle and source filename.
func New(b *bundle.ContextBundle, filename string) Model {
	m := Model{
		bundle:        b,
		filename:      filepath.Base(filename),
		sortAsc:       false,
		expandedEdits: make(map[int]bool),
	}
	m.timeline = buildTimeline(b)
	return m
}

// ── Bubble Tea interface ───────────────

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "l", "right":
			m.activeTab = (m.activeTab + 1) % tabCount
		case "shift+tab", "h", "left":
			m.activeTab = (m.activeTab - 1 + tabCount) % tabCount
		case "1", "2", "3", "4", "5", "6", "7":
			m.activeTab = tabID(msg.String()[0] - '1')
		case "s":
			if m.activeTab == tabTimeline {
				m.sortAsc = !m.sortAsc
				m.rebuildTimelineViewport()
			}
		case "up", "k":
			if m.activeTab == tabFileEdits && m.editCursor > 0 {
				m.editCursor--
				m.rebuildFileEditsViewport()
				return m, nil
			}
		case "down", "j":
			if m.activeTab == tabFileEdits && m.editCursor < len(m.bundle.FileEdits)-1 {
				m.editCursor++
				m.rebuildFileEditsViewport()
				return m, nil
			}
		case "enter", " ":
			if m.activeTab == tabFileEdits && len(m.bundle.FileEdits) > 0 {
				fe := m.bundle.FileEdits[m.editCursor]
				if fe.Diff != "" { // only expandable when diff exists
					if m.expandedEdits[m.editCursor] {
						delete(m.expandedEdits, m.editCursor)
					} else {
						m.expandedEdits[m.editCursor] = true
					}
					m.rebuildFileEditsViewport()
				}
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.viewports[m.activeTab], cmd = m.viewports[m.activeTab].Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.initViewports()
		return m, nil
	}
	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return "Loading…"
	}

	// ── Row 1: title bar ──────────────────────────────────────────────────────
	title := titleStyle.Width(m.width).Render("  handoff  " + m.filename)

	// ── Row 2: tab bar ────────────────────────────────────────────────────────
	var tabParts []string
	for i := tabID(0); i < tabCount; i++ {
		label := fmt.Sprintf(" %d %s ", i+1, tabNames[i])
		if i == m.activeTab {
			tabParts = append(tabParts, activeTabStyle.Render(label))
		} else {
			tabParts = append(tabParts, inactiveTabStyle.Render(label))
		}
		if i < tabCount-1 {
			tabParts = append(tabParts, tabSepStyle.Render("│"))
		}
	}
	tabRow := lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		Width(m.width).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, tabParts...))

	// ── Row 3…N-1: scrollable content ────────────────────────────────────────
	content := m.viewports[m.activeTab].View()

	// ── Row N: status / hint bar ──────────────────────────────────────────────
	hint := "  ←/→ tab  ↑/↓ scroll  1-7 jump  q quit"
	if m.activeTab == tabTimeline {
		dir := "newest first"
		if m.sortAsc {
			dir = "oldest first"
		}
		hint += "  s sort (" + dir + ")"
	}
	if m.activeTab == tabFileEdits {
		hint += "  ↑/↓ select  enter expand/collapse"
	}
	// show scroll % on the right
	pct := fmt.Sprintf("%3.0f%%", m.viewports[m.activeTab].ScrollPercent()*100)
	pad := m.width - lipgloss.Width(hint) - len(pct) - 2
	if pad < 1 {
		pad = 1
	}
	statusBar := statusBarStyle.Width(m.width).Render(
		hint + strings.Repeat(" ", pad) + pct,
	)

	return lipgloss.JoinVertical(lipgloss.Left, title, tabRow, content, statusBar)
}

// ── Viewport management ───────────────────────────────────────────────────────

func (m *Model) initViewports() {
	// title(1) + tabRow(1) + statusBar(1) = 3 fixed rows
	vpHeight := m.height - 3
	if vpHeight < 1 {
		vpHeight = 1
	}
	for i := tabID(0); i < tabCount; i++ {
		vp := viewport.New(m.width, vpHeight)
		vp.SetContent(m.renderTab(i))
		m.viewports[i] = vp
	}
}

func (m *Model) rebuildTimelineViewport() {
	m.viewports[tabTimeline].SetContent(m.renderTab(tabTimeline))
	m.viewports[tabTimeline].GotoTop()
}

func (m *Model) rebuildFileEditsViewport() {
	m.viewports[tabFileEdits].SetContent(m.renderTab(tabFileEdits))
}

// ── Tab renderers ─────────────────────────────────────────────────────────────

func (m *Model) renderTab(t tabID) string {
	switch t {
	case tabSummary:
		return m.renderSummary()
	case tabAnnotations:
		return m.renderAnnotations()
	case tabFileEdits:
		return m.renderFileEdits()
	case tabGit:
		return m.renderGit()
	case tabCommands:
		return m.renderCommands()
	case tabEditorTabs:
		return m.renderEditorTabs()
	case tabTimeline:
		return m.renderTimeline()
	}
	return ""
}

func heading(s string) string {
	return "\n" + sectionHeader.Render("  "+s) + "\n\n"
}

func bullet(text string) string {
	return bulletStyle.Render("  •") + "  " + text + "\n"
}

func (m *Model) renderSummary() string {
	s := m.bundle.Session
	var sb strings.Builder
	sb.WriteString(heading("Session Summary"))

	row := func(label, value string) {
		sb.WriteString(labelStyle.Render(fmt.Sprintf("  %-14s", label)) + "  " + value + "\n")
	}
	row("Work Dir:", s.WorkDir)
	row("Started:", s.StartTime.Format("2006-01-02 15:04:05 MST"))
	row("Stopped:", s.StopTime.Format("2006-01-02 15:04:05 MST"))
	row("Duration:", s.Duration)
	if s.Author != "" {
		row("Author:", s.Author)
	}
	if m.bundle.Git != nil {
		row("Branch:", m.bundle.Git.Branch)
		row("Head Commit:", m.bundle.Git.HeadCommit)
	}

	sb.WriteString("\n")
	sb.WriteString(heading("Counts"))
	row("Annotations:", fmt.Sprintf("%d", len(m.bundle.Annotations)))
	row("File Edits:", fmt.Sprintf("%d", len(m.bundle.FileEdits)))
	row("Commands:", fmt.Sprintf("%d", len(m.bundle.Commands)))
	row("Editor Tabs:", fmt.Sprintf("%d", len(m.bundle.EditorTabs)))
	return sb.String()
}

func (m *Model) renderAnnotations() string {
	var sb strings.Builder
	sb.WriteString(heading(fmt.Sprintf("Annotations (%d)", len(m.bundle.Annotations))))
	if len(m.bundle.Annotations) == 0 {
		sb.WriteString(dimStyle.Render("  (none)") + "\n")
		return sb.String()
	}
	for _, a := range m.bundle.Annotations {
		kind := "NOTE"
		if a.IsSummary {
			kind = "SUMMARY"
		}
		ts := timeStyle.Render(a.Timestamp.Format("15:04:05"))
		badge := kindAnnotationStyle.Render("[" + kind + "]")
		sb.WriteString(fmt.Sprintf("  %s  %s  %s\n\n", ts, badge, a.Message))
	}
	return sb.String()
}

func (m *Model) renderFileEdits() string {
	var sb strings.Builder
	sb.WriteString(heading(fmt.Sprintf("File Edits (%d)", len(m.bundle.FileEdits))))
	if len(m.bundle.FileEdits) == 0 {
		sb.WriteString(dimStyle.Render("  (none)") + "\n")
		return sb.String()
	}
	for i, fe := range m.bundle.FileEdits {
		ts := timeStyle.Render(fe.Timestamp.Format("15:04:05"))
		relPath := stripWorkDir(fe.Path, m.bundle.Session.WorkDir)

		// Toggle indicator and diff icon
		hasDiff := fe.Diff != ""
		isGitDiff := hasDiff && !strings.HasPrefix(fe.Diff, "--- /dev/null")
		expanded := m.expandedEdits[i]

		var icon string
		if !hasDiff {
			icon = dimStyle.Render("○ ") // no diff available
		} else if isGitDiff {
			icon = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("◈ ") // git diff
		} else {
			icon = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◇ ") // full file (non-git)
		}

		toggle := dimStyle.Render("  ▶ ")
		if expanded {
			toggle = dimStyle.Render("  ▼ ")
		}
		if !hasDiff {
			toggle = "    " // no arrow, not expandable
		}

		row := fmt.Sprintf("%s%s%s  %s", toggle, icon, ts, relPath)
		if i == m.editCursor {
			// Pad to width so the highlight fills the line
			row = selectedRowStyle.Width(m.width - 2).Render(row)
		}
		sb.WriteString(row + "\n")

		// Expanded diff block
		if expanded && hasDiff {
			sb.WriteString(renderDiff(fe.Diff, m.width))
			sb.WriteString("\n")
		} else {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// renderDiff colorises a unified diff string.
func renderDiff(diff string, width int) string {
	var sb strings.Builder
	border := dimStyle.Render("  " + strings.Repeat("─", width-4))
	sb.WriteString(border + "\n")
	for _, line := range strings.Split(diff, "\n") {
		var rendered string
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			rendered = diffMetaStyle.Render("  " + line)
		case strings.HasPrefix(line, "+"):
			rendered = diffAddStyle.Render("  " + line)
		case strings.HasPrefix(line, "-"):
			rendered = diffDelStyle.Render("  " + line)
		case strings.HasPrefix(line, "@@"):
			rendered = diffMetaStyle.Render("  " + line)
		default:
			rendered = dimStyle.Render("  " + line)
		}
		sb.WriteString(rendered + "\n")
	}
	sb.WriteString(border + "\n")
	return sb.String()
}

func (m *Model) renderGit() string {
	var sb strings.Builder
	sb.WriteString(heading("Git Changes"))
	if m.bundle.Git == nil {
		sb.WriteString(dimStyle.Render("  (not a git repository or git data unavailable)") + "\n")
		return sb.String()
	}
	g := m.bundle.Git
	row := func(label, value string) {
		sb.WriteString(labelStyle.Render(fmt.Sprintf("  %-14s", label)) + "  " + value + "\n")
	}
	row("Branch:", g.Branch)
	row("Head Commit:", g.HeadCommit)

	if len(g.RecentLog) > 0 {
		sb.WriteString(heading("Recent Commits"))
		for _, l := range g.RecentLog {
			sb.WriteString(bullet(l))
		}
	}
	if g.StagedDiff != "" {
		sb.WriteString(heading("Staged Diff"))
		sb.WriteString(dimStyle.Render(indent(g.StagedDiff, "    ")) + "\n")
	}
	if g.Diff != "" {
		sb.WriteString(heading("Unstaged Diff"))
		sb.WriteString(dimStyle.Render(indent(g.Diff, "    ")) + "\n")
	}
	return sb.String()
}

func (m *Model) renderCommands() string {
	var sb strings.Builder
	sb.WriteString(heading(fmt.Sprintf("Terminal Commands (%d)", len(m.bundle.Commands))))
	if len(m.bundle.Commands) == 0 {
		sb.WriteString(dimStyle.Render("  (none)") + "\n")
		return sb.String()
	}
	for i, c := range m.bundle.Commands {
		num := dimStyle.Render(fmt.Sprintf("  %3d.", i+1))
		if !c.Timestamp.IsZero() && c.Timestamp.Year() > 1 {
			ts := timeStyle.Render(" [" + c.Timestamp.Format("15:04:05") + "]")
			sb.WriteString(num + ts + "  " + c.Raw + "\n\n")
		} else {
			sb.WriteString(num + "  " + c.Raw + "\n\n")
		}
	}
	return sb.String()
}

func (m *Model) renderEditorTabs() string {
	var sb strings.Builder
	sb.WriteString(heading(fmt.Sprintf("Editor Tabs (%d)", len(m.bundle.EditorTabs))))
	if len(m.bundle.EditorTabs) == 0 {
		sb.WriteString(dimStyle.Render("  (none)") + "\n")
		return sb.String()
	}
	for i, tab := range m.bundle.EditorTabs {
		num := dimStyle.Render(fmt.Sprintf("  %3d.", i+1))
		sb.WriteString(num + "  " + stripWorkDir(tab, m.bundle.Session.WorkDir) + "\n\n")
	}
	return sb.String()
}

func (m *Model) renderTimeline() string {
	var sb strings.Builder

	dir := "newest first"
	if m.sortAsc {
		dir = "oldest first"
	}
	sb.WriteString(heading(fmt.Sprintf("Timeline (%s)", dir)))

	events := make([]timelineEvent, len(m.timeline))
	copy(events, m.timeline)
	if m.sortAsc {
		sort.Slice(events, func(i, j int) bool { return events[i].ts.Before(events[j].ts) })
	} else {
		sort.Slice(events, func(i, j int) bool { return events[i].ts.After(events[j].ts) })
	}

	if len(events) == 0 {
		sb.WriteString(dimStyle.Render("  (no timestamped events in this session)") + "\n")
		return sb.String()
	}

	for _, ev := range events {
		ts := timeStyle.Render(ev.ts.Format("15:04:05"))
		var badge string
		switch ev.kind {
		case kindNote, kindSummary:
			badge = kindAnnotationStyle.Render(fmt.Sprintf("  %-8s", string(ev.kind)))
		case kindEdit:
			badge = kindFileEditStyle.Render(fmt.Sprintf("  %-8s", string(ev.kind)))
		case kindCmd:
			badge = kindCommandStyle.Render(fmt.Sprintf("  %-8s", string(ev.kind)))
		}
		sb.WriteString(ts + badge + "  " + ev.text + "\n\n")
	}
	return sb.String()
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func buildTimeline(b *bundle.ContextBundle) []timelineEvent {
	var events []timelineEvent
	zero := time.Time{}
	for _, a := range b.Annotations {
		if a.Timestamp == zero {
			continue
		}
		k := kindNote
		if a.IsSummary {
			k = kindSummary
		}
		events = append(events, timelineEvent{ts: a.Timestamp, kind: k, text: a.Message})
	}
	for _, fe := range b.FileEdits {
		if fe.Timestamp == zero {
			continue
		}
		events = append(events, timelineEvent{ts: fe.Timestamp, kind: kindEdit, text: stripWorkDir(fe.Path, b.Session.WorkDir)})
	}
	for _, c := range b.Commands {
		if c.Timestamp == zero || c.Timestamp.Year() <= 1 {
			continue
		}
		events = append(events, timelineEvent{ts: c.Timestamp, kind: kindCmd, text: c.Raw})
	}
	return events
}

// stripWorkDir removes the workDir prefix from path, returning a relative path.
// If path doesn't start with workDir, it's returned unchanged.
func stripWorkDir(path, workDir string) string {
	if workDir == "" {
		return path
	}
	prefix := workDir
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if strings.HasPrefix(path, prefix) {
		return path[len(prefix):]
	}
	return path
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

// Run starts the TUI for the given bundle.
func Run(b *bundle.ContextBundle, filename string) error {
	p := tea.NewProgram(New(b, filename), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
