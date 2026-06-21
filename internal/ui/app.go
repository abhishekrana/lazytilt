// Package ui implements the lazytilt terminal UI with Bubble Tea.
package ui

import (
	"fmt"
	"strings"

	"github.com/abhishekrana/lazytilt/internal/discovery"
	"github.com/abhishekrana/lazytilt/internal/tilt"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type focusArea int

const (
	focusSidebar focusArea = iota
	focusLogs
)

type inputMode int

const (
	modeNormal inputMode = iota
	modeLogFilter
)

// topBarHeight is the rendered height of the header (title/instances row plus an
// accent underline rule).
const topBarHeight = 2

// Model is the root Bubble Tea model.
type Model struct {
	width, height int
	token         string
	fallbackHost  string
	fallbackPort  int

	instances []discovery.Instance
	active    int
	tickN     int

	view    *tilt.View
	loadErr error

	selected int
	focus    focusArea

	vp     viewport.Model
	follow bool
	level  logLevel

	mode      inputMode
	typing    string
	logFilter string

	showDisabled bool
	showHelp     bool

	statusMsg string
	statusErr bool

	theme Theme
}

// New builds the initial model. host/port are the fallback instance used when
// discovery finds nothing; themeName selects the palette (empty = default).
func New(token, host string, port int, themeName string) Model {
	th := resolveTheme(themeName)
	vp := viewport.New(80, 20)
	return Model{
		token:        token,
		fallbackHost: host,
		fallbackPort: port,
		follow:       true,
		level:        levelAll,
		vp:           vp,
		focus:        focusSidebar,
		theme:        th,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(discoverCmd(), tickCmd())
}

func (m Model) currentPort() int {
	if m.active >= 0 && m.active < len(m.instances) {
		return m.instances[m.active].Port
	}
	return m.fallbackPort
}

func (m Model) currentHost() string {
	if m.active >= 0 && m.active < len(m.instances) {
		return m.instances[m.active].Host
	}
	return m.fallbackHost
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		bodyH := max(m.height-topBarHeight-1, 3)
		m.vp.Width = max(m.width-sidebarWidth-1, 10)
		m.vp.Height = max(bodyH-3, 1)
		m.setLogs()
		return m, nil

	case tea.KeyMsg:
		if m.mode != modeNormal {
			return m.updateFilterInput(msg)
		}
		return m.updateKeys(msg)

	case tickMsg:
		m.tickN++
		cmds := []tea.Cmd{tickCmd()}
		if m.currentPort() != 0 {
			cmds = append(cmds, fetchCmd(m.currentHost(), m.currentPort(), m.token))
		}
		if m.tickN%5 == 0 {
			cmds = append(cmds, discoverCmd())
		}
		return m, tea.Batch(cmds...)

	case viewMsg:
		if msg.port != m.currentPort() {
			return m, nil // stale response from a previous instance
		}
		if msg.err != nil {
			m.loadErr = msg.err
		} else {
			m.loadErr = nil
			m.view = msg.view
			m.clampSelection()
			m.setLogs()
		}
		return m, nil

	case instancesMsg:
		return m.handleInstances(msg)

	case actionResultMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("%s %s failed: %v", msg.kind, msg.resource, msg.err)
			m.statusErr = true
		} else {
			m.statusMsg = fmt.Sprintf("%s %s ✓", msg.kind, msg.resource)
			m.statusErr = false
		}
		return m, fetchCmd(m.currentHost(), m.currentPort(), m.token)
	}
	return m, nil
}

func (m Model) handleInstances(msg instancesMsg) (tea.Model, tea.Cmd) {
	insts := msg.instances
	if len(insts) == 0 {
		insts = []discovery.Instance{{Host: m.fallbackHost, Port: m.fallbackPort, Label: m.fallbackHost}}
	}
	prevPort := m.currentPort()
	m.instances = insts
	m.active = 0
	for i, in := range insts {
		if in.Port == prevPort {
			m.active = i
		}
	}
	if m.view == nil || m.currentPort() != prevPort {
		return m, fetchCmd(m.currentHost(), m.currentPort(), m.token)
	}
	return m, nil
}

func (m Model) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While the help popup is open it is modal: only close/quit keys act.
	if m.showHelp {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?", "esc":
			m.showHelp = false
		}
		return m, nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "esc":
		if m.showHelp {
			m.showHelp = false
		} else {
			m.focus = focusSidebar
		}
		return m, nil
	case "tab":
		if m.focus == focusSidebar {
			m.focus = focusLogs
		} else {
			m.focus = focusSidebar
		}
		return m, nil
	case "enter":
		m.focus = focusLogs
		return m, nil
	case "[":
		return m.switchInstance(-1)
	case "]":
		return m.switchInstance(1)
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return m.gotoInstance(int(msg.String()[0] - '1'))
	case "r":
		return m.runAction(tilt.ActionTrigger)
	case "e":
		return m.runAction(tilt.ActionEnable)
	case "d":
		return m.runAction(tilt.ActionDisable)
	case "f":
		m.follow = !m.follow
		m.setLogs()
		return m, nil
	case "L":
		m.level = (m.level + 1) % 3
		m.setLogs()
		return m, nil
	case "c":
		m.logFilter = ""
		m.setLogs()
		return m, nil
	case "s":
		m.showDisabled = !m.showDisabled
		m.clampSelection()
		m.setLogs()
		return m, nil
	case "T":
		m.theme = m.theme.next()
		return m, nil
	case "/":
		m.mode = modeLogFilter
		m.typing = m.logFilter
		return m, nil
	case "up", "k":
		if m.focus == focusSidebar {
			m.moveSelection(-1)
			return m, nil
		}
	case "down", "j":
		if m.focus == focusSidebar {
			m.moveSelection(1)
			return m, nil
		}
	case "g":
		if m.focus == focusLogs {
			m.vp.GotoTop()
			return m, nil
		}
	case "G":
		if m.focus == focusLogs {
			m.vp.GotoBottom()
			return m, nil
		}
	}
	if m.focus == focusLogs {
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateFilterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.applyTyping()
		m.mode = modeNormal
	case "esc":
		m.typing = ""
		m.applyTyping()
		m.mode = modeNormal
	case "backspace":
		if m.typing != "" {
			m.typing = m.typing[:len(m.typing)-1]
		}
		m.applyTyping()
	default:
		if len(msg.Runes) > 0 {
			m.typing += string(msg.Runes)
			m.applyTyping()
		}
	}
	return m, nil
}

func (m *Model) applyTyping() {
	if m.mode == modeLogFilter {
		m.logFilter = m.typing
		m.setLogs()
	}
}

func (m Model) switchInstance(d int) (tea.Model, tea.Cmd) {
	if len(m.instances) < 2 {
		return m, nil
	}
	return m.gotoInstance((m.active + d + len(m.instances)) % len(m.instances))
}

// gotoInstance switches to the instance at idx (0-based) and refetches without a
// restart; a no-op if idx is out of range or already active.
func (m Model) gotoInstance(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.instances) || idx == m.active {
		return m, nil
	}
	m.active = idx
	m.view = nil
	m.loadErr = nil
	m.selected = 0
	m.statusMsg = ""
	return m, fetchCmd(m.currentHost(), m.currentPort(), m.token)
}

func (m Model) runAction(kind tilt.ActionKind) (tea.Model, tea.Cmd) {
	r, ok := m.selectedResource()
	if !ok {
		return m, nil
	}
	m.statusMsg = fmt.Sprintf("%s %s…", kind, r.Name())
	m.statusErr = false
	return m, actionCmd(kind, r.Name(), m.currentPort())
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	bodyH := max(m.height-topBarHeight-1, 3)
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderSidebar(bodyH),
		m.renderRightPane(max(m.width-sidebarWidth-1, 10), bodyH),
	)
	frame := lipgloss.JoinVertical(lipgloss.Left, m.renderTopBar(), body, m.renderFooter())
	if m.showHelp {
		return overlayCenter(frame, m.helpBox(), m.width, m.height)
	}
	return frame
}

func (m Model) renderTopBar() string {
	title := m.theme.accent().Bold(true).Render(" LAZYTILT ")
	segs := make([]string, 0, len(m.instances))
	for i, in := range m.instances {
		tag := fmt.Sprintf("‹%d›", i+1)
		if i == m.active {
			segs = append(segs, m.theme.header().Render(tag+" "+in.Label))
		} else {
			segs = append(segs, m.theme.muted().Render(tag+" "+in.Label))
		}
	}
	bar := " " + title + "   " + strings.Join(segs, "   ")
	// Title/instances row + a full-width accent rule, so the header reads as a
	// header without painting a (uneven) background band.
	rule := m.theme.accent().Render(strings.Repeat("─", m.width))
	return lipgloss.JoinVertical(lipgloss.Left, ansi.Truncate(bar, m.width, "…"), rule)
}

func (m Model) renderFooter() string {
	if m.mode != modeNormal {
		return m.theme.footer().Width(m.width).Render(fmt.Sprintf(" search logs: %s▏", m.typing))
	}
	inner := ""
	if m.statusMsg != "" {
		c := m.theme.OK
		if m.statusErr {
			c = m.theme.Err
		}
		inner = lipgloss.NewStyle().Foreground(c).Render(ansi.Truncate(" "+m.statusMsg, m.width, "…"))
	} else {
		keys := " ↑↓ move · r trigger · e/d enable·disable · ⏎ logs · / search logs · f follow · L level · s disabled · 1-9/[ ] instance · T theme · ? help · q quit"
		inner = ansi.Truncate(keys, m.width, "…")
	}
	return m.theme.footer().Width(m.width).Render(inner)
}

// helpBox is the floating help popup (a bordered box, centered by overlayCenter).
func (m Model) helpBox() string {
	rows := [][2]string{
		{"↑/k ↓/j", "move selection"},
		{"⏎ / tab", "focus logs / toggle pane"},
		{"[  ]", "previous / next instance"},
		{"1 … 9", "jump to instance N"},
		{"r", "trigger (rebuild)"},
		{"e  d", "enable / disable"},
		{"/", "search logs (highlights matches)"},
		{"f", "follow / tail logs"},
		{"L", "cycle log level"},
		{"c", "clear log filter"},
		{"s", "toggle disabled resources"},
		{"T", "cycle theme (" + m.theme.Name + ")"},
		{"g  G", "top / bottom of logs"},
		{"?  esc", "close this help"},
		{"q", "quit"},
	}
	lines := []string{m.theme.accent().Bold(true).Render("lazytilt — keys"), ""}
	for _, kv := range rows {
		lines = append(lines, fmt.Sprintf("  %-14s %s", kv[0], m.theme.muted().Render(kv[1])))
	}
	lines = append(lines, "", m.theme.muted().Render("  press ? or esc to close"))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Accent).
		Foreground(m.theme.Text).
		Padding(2, 5).
		Width(58).
		Render(strings.Join(lines, "\n"))
}

// overlayCenter composites box centered over bg (a width×height frame),
// replacing the cells it covers. ANSI-aware via ansi.Cut.
func overlayCenter(bg, box string, width, height int) string {
	bgLines := strings.Split(bg, "\n")
	boxLines := strings.Split(box, "\n")
	bw := lipgloss.Width(box)
	x := max((width-bw)/2, 0)
	y := max((height-len(boxLines))/2, 0)

	for i, bl := range boxLines {
		row := y + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		base := bgLines[row]
		if w := lipgloss.Width(base); w < width {
			base += strings.Repeat(" ", width-w)
		}
		left := ansi.Cut(base, 0, x)
		right := ansi.Cut(base, x+bw, width)
		bgLines[row] = left + "\x1b[0m" + bl + "\x1b[0m" + right
	}
	return strings.Join(bgLines, "\n")
}
