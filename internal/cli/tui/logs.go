package tui

import (
	"fmt"
	"image/color"
	"net/url"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gorilla/websocket"

	"github.com/theolujay/appa/internal/hub"
)

const logViewerTitle = " Deployment Logs "

var phaseColors = map[string]color.Color{
	"prepare": lipgloss.Color("#8BE9FD"),
	"build":   lipgloss.Color("#F1FA8C"),
	"deploy":  lipgloss.Color("#50FA7B"),
	"routing": lipgloss.Color("#FF79C6"),
}

type logModel struct {
	lines      []string
	autoScroll bool
	scrollOff  int
	width      int
	height     int
	err        error
	status     string

	conn   *websocket.Conn
	connMu sync.Mutex
	done   chan struct{}

	deploymentID int64
	apiURL       string
}

func NewLogViewer(apiURL string, deploymentID int64) tea.Model {
	return &logModel{
		apiURL:       apiURL,
		deploymentID: deploymentID,
		autoScroll:   true,
		done:         make(chan struct{}),
	}
}

func (m *logModel) Init() tea.Cmd {
	return tea.Batch(m.connectWS, tea.Tick(5*time.Second, m.connTimeout))
}

func (m *logModel) connectWS() tea.Msg {
	u, err := url.Parse(m.apiURL)
	if err != nil {
		return errMsg{Err: fmt.Errorf("invalid API URL: %w", err)}
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	wsURL := fmt.Sprintf("%s://%s/v1/deployments/%d/logs", scheme, u.Host, m.deploymentID)

	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return errMsg{Err: fmt.Errorf("dial: %w", err)}
	}

	m.connMu.Lock()
	m.conn = c
	m.connMu.Unlock()

	return connectedMsg{}
}

func (m *logModel) connTimeout(t time.Time) tea.Msg {
	m.connMu.Lock()
	conn := m.conn
	m.connMu.Unlock()
	if conn == nil {
		return errMsg{Err: fmt.Errorf("timed out connecting to log stream")}
	}
	return nil
}

func (m *logModel) readPump() tea.Msg {
	m.connMu.Lock()
	conn := m.conn
	m.connMu.Unlock()
	if conn == nil {
		return nil
	}

	var evt hub.Event
	if err := conn.ReadJSON(&evt); err != nil {
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			return streamEndedMsg{}
		}
		return errMsg{Err: fmt.Errorf("read: %w", err)}
	}

	switch evt.Type {
	case hub.MessageTypeLog:
		return logLineMsg{
			line:  evt.Log.Line,
			phase: evt.Log.Phase,
		}
	case hub.MessageTypeStatus:
		line := evt.Status.Status
		if evt.Status.URL != "" {
			line = fmt.Sprintf("%s — %s", evt.Status.Status, evt.Status.URL)
		}
		return logLineMsg{line: line, phase: "status"}
	}
	return m.readPump()
}

type connectedMsg struct{}

type logLineMsg struct {
	line  string
	phase string
}

type streamEndedMsg struct{}

func (m *logModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 3
		if m.scrollOff > 0 && m.autoScroll {
			m.scrollOff = max(0, len(m.lines)-m.height)
		}

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.closeConn()
			return m, tea.Quit
		case "f":
			m.autoScroll = !m.autoScroll
			if m.autoScroll {
				m.scrollOff = max(0, len(m.lines)-m.height)
			}
		case "up", "k":
			if m.scrollOff > 0 {
				m.scrollOff--
				m.autoScroll = false
			}
		case "down", "j":
			if m.scrollOff < len(m.lines)-m.height {
				m.scrollOff++
			}
		case "pageup", "b":
			half := m.height / 2
			m.scrollOff = max(0, m.scrollOff-half)
			m.autoScroll = false
		case "pagedown", "space":
			half := m.height / 2
			m.scrollOff = min(len(m.lines)-m.height, m.scrollOff+half)
		case "g":
			m.scrollOff = 0
			m.autoScroll = false
		case "G":
			m.scrollOff = max(0, len(m.lines)-m.height)
			m.autoScroll = true
		}

	case connectedMsg:
		return m, m.readPump

	case logLineMsg:
		styled := styleLogLine(msg.line, msg.phase)
		m.lines = append(m.lines, styled)
		if m.autoScroll {
			m.scrollOff = max(0, len(m.lines)-m.height)
		}
		return m, m.readPump

	case streamEndedMsg:
		m.status = "stream ended"

	case errMsg:
		m.err = msg.Err
		m.closeConn()
		return m, tea.Quit
	}

	return m, nil
}

func (m *logModel) View() tea.View {
	view := tea.NewView(m.renderView())
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m *logModel) renderView() string {
	var b strings.Builder

	if len(m.lines) == 0 && m.err == nil && m.status == "" {
		b.WriteString("Connecting to log stream...\n")
	}

	visible := m.lines
	if len(m.lines) > m.height {
		start := len(m.lines) - m.height
		if m.scrollOff < len(m.lines)-m.height {
			start = m.scrollOff
		}
		end := start + m.height
		if end > len(m.lines) {
			end = len(m.lines)
		}
		visible = m.lines[start:end]
	}

	for _, l := range visible {
		b.WriteString(l)
		b.WriteByte('\n')
	}

	b.WriteString(m.renderFooter())

	return b.String()
}

func (m *logModel) renderFooter() string {
	var statusDot string
	switch {
	case m.err != nil:
		statusDot = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("●")
	case m.status == "stream ended":
		statusDot = lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")).Render("●")
	default:
		statusDot = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("●")
	}

	statusText := "streaming"
	if m.err != nil {
		statusText = fmt.Sprintf("error: %s", m.err)
	} else if m.status != "" {
		statusText = m.status
	}

	followDot := " "
	if m.autoScroll {
		followDot = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#50FA7B")).
			Bold(true).
			Render("●")
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6272A4")).
		Render(" [q] quit  [f] follow  [↑↓/j k] scroll  [g/G] top/bottom ")

	info := fmt.Sprintf("%s %s  %s follow\n%s", statusDot, statusText, followDot, help)

	return lipgloss.NewStyle().
		Width(m.width).
		Padding(0, 1).
		Background(lipgloss.Color("#282A36")).
		Render(info)
}

func styleLogLine(line, phase string) string {
	st := lipgloss.NewStyle()
	switch phase {
	case "prepare":
		st = st.Foreground(phaseColors["prepare"])
	case "build":
		st = st.Foreground(phaseColors["build"])
	case "deploy":
		st = st.Foreground(phaseColors["deploy"])
	case "routing":
		st = st.Foreground(phaseColors["routing"])
	case "status":
		st = st.Bold(true).Foreground(lipgloss.Color("#FFB86C"))
	default:
		st = st.Foreground(lipgloss.Color("#F8F8F2"))
	}
	return st.Render(line)
}

func (m *logModel) closeConn() {
	select {
	case <-m.done:
	default:
		close(m.done)
	}
	m.connMu.Lock()
	if m.conn != nil {
		m.conn.Close()
		m.conn = nil
	}
	m.connMu.Unlock()
}
