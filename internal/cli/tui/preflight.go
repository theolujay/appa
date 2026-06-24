package tui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
)

type checkState int

const (
	checkPending checkState = iota
	checkRunning
	checkOK
	checkFail
	checkWarn
)

type Check struct {
	Label string
	Fn    func() (ok bool, info string, warn bool)
}

type checkItem struct {
	label string
	state checkState
	info  string
	fn    func() (ok bool, info string, warn bool)
}

type PreflightModel struct {
	checks   []checkItem
	current  int
	width    int
	height   int
	done     bool
	Failures int
	Warnings int

	// protect concurrent access to fields that may be read from Render
	mu sync.RWMutex

	spring   harmonica.Spring
	progress float64
	velocity float64
	target   float64

	// cached styles to avoid allocating on every render
	titleStyle    lipgloss.Style
	subtitleStyle lipgloss.Style
	numStyle      lipgloss.Style
	labelStyle    lipgloss.Style
	pendingStyle  lipgloss.Style
	runningStyle  lipgloss.Style
	okStyle       lipgloss.Style
	failStyle     lipgloss.Style
	warnStyle     lipgloss.Style
	infoStyle     lipgloss.Style
}

func NewPreflightModel(checks []Check) *PreflightModel {
	return &PreflightModel{
		checks: func() []checkItem {
			items := make([]checkItem, len(checks))
			for i, c := range checks {
				items[i] = checkItem{label: c.Label, state: checkPending, fn: c.Fn}
			}
			return items
		}(),
		spring:        harmonica.NewSpring(harmonica.FPS(60), 8.0, 0.4),
		titleStyle:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#007fff")),
		subtitleStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#7F8FA0")),
		numStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("#7F8FA0")),
		labelStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("#F8F8F2")),
		pendingStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("#7F8FA0")),
		runningStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("#8BE9FD")),
		okStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Bold(true),
		failStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true),
		warnStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("#F1FA8C")).Bold(true),
		infoStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("#7F8FA0")),
	}
}

type checkResult struct {
	index int
	ok    bool
	info  string
	warn  bool
}

func (m *PreflightModel) Init() tea.Cmd {
	return tea.Batch(m.nextCheck(), m.springTick())
}

func (m *PreflightModel) springTick() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return springTickMsg(t)
	})
}

type springTickMsg time.Time

func (m *PreflightModel) nextCheck() tea.Cmd {
	m.mu.Lock()
	if m.current >= len(m.checks) {
		m.done = true
		m.mu.Unlock()
		return nil
	}
	m.checks[m.current].state = checkRunning
	m.target = float64(m.current + 1)

	idx := m.current
	m.mu.Unlock()
	return func() tea.Msg {
		// run check outside of locks
		ok, info, warn := m.checks[idx].fn()
		return checkResult{index: idx, ok: ok, info: info, warn: warn}
	}
}

func (m *PreflightModel) completeCheck(index int, ok bool, info string, warn bool) {
	m.mu.Lock()
	m.checks[index].state = checkOK
	m.checks[index].info = info
	if !ok {
		m.checks[index].state = checkFail
		m.Failures++
	} else if warn {
		m.checks[index].state = checkWarn
		m.Warnings++
	}
	m.current++
	m.mu.Unlock()
}

func (m *PreflightModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "enter", "esc":
			return m, tea.Quit
		}

	case springTickMsg:
		m.progress, m.velocity = m.spring.Update(m.progress, m.velocity, m.target)
		if m.progress < m.target || !m.done {
			return m, m.springTick()
		}
		return m, nil

	case checkResult:
		m.completeCheck(msg.index, msg.ok, msg.info, msg.warn)
		return m, m.nextCheck()
	}

	return m, nil
}

func (m *PreflightModel) View() tea.View {
	var b strings.Builder

	// snapshot state under read lock so rendering can proceed without holding locks
	m.mu.RLock()
	checks := make([]checkItem, len(m.checks))
	copy(checks, m.checks)
	done := m.done
	progress := m.progress
	failures := m.Failures
	warnings := m.Warnings
	// styles are immutable
	title := m.titleStyle.Render("Preflight Checks")
	subtitle := m.subtitleStyle.Render("Press q to quit")
	m.mu.RUnlock()

	b.WriteString(lipgloss.JoinVertical(lipgloss.Left, title, subtitle))
	b.WriteByte('\n')

	for i, c := range checks {
		b.WriteString(m.renderCheck(i, c, progress))
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	if done {
		// renderSummary needs failures/warnings snapshot
		b.WriteString(m.renderSummaryWith(failures, warnings))
	}

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m *PreflightModel) renderCheck(i int, c checkItem, progress float64) string {
	idx := i + 1

	var icon, status string
	switch c.state {
	case checkPending:
		icon = m.pendingStyle.Render("○")
		status = m.pendingStyle.Render("pending")
	case checkRunning:
		amount := math.Mod(progress-float64(i), 1.0)
		if amount < 0 {
			amount += 1.0
		}
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := frames[int(amount*float64(len(frames)))%len(frames)]
		icon = m.runningStyle.Render(frame)
		status = m.runningStyle.Render("checking")
	case checkOK:
		icon = m.okStyle.Render("✓")
		status = m.okStyle.Bold(false).Render("passed")
	case checkFail:
		icon = m.failStyle.Render("✗")
		status = m.failStyle.Bold(false).Render("failed")
	case checkWarn:
		icon = m.warnStyle.Render("!")
		status = m.warnStyle.Bold(false).Render("warning")
	}

	num := m.numStyle.Render(fmt.Sprintf("%02d", idx))

	label := m.labelStyle.Render(c.label)

	extra := ""
	if c.info != "" {
		extra = " " + m.infoStyle.Render(c.info)
	}

	return fmt.Sprintf("  %s %s %s%s  %s", num, icon, label, extra, status)
}

// renderSummaryWith renders summary using provided snapshots to avoid locking inside
func (m *PreflightModel) renderSummaryWith(failures, warnings int) string {
	var clr color.Color
	var label string
	switch {
	case failures > 0:
		clr = lipgloss.Color("#FF5555")
		label = fmt.Sprintf("✗ %d failure(s)", failures)
		if warnings > 0 {
			label += fmt.Sprintf(", %d warning(s)", warnings)
		}
	case warnings > 0:
		clr = lipgloss.Color("#F1FA8C")
		label = fmt.Sprintf("✓ All critical checks passed (%d warning(s))", warnings)
	default:
		clr = lipgloss.Color("#50FA7B")
		label = "✓ All checks passed"
	}

	style := lipgloss.NewStyle().Bold(true).Foreground(clr).Padding(0, 2).Border(lipgloss.RoundedBorder()).BorderForeground(clr)
	return style.Render(label)
}
