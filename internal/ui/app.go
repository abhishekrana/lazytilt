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
	modeConfirm
)

// topBarHeight / footerHeight are the rendered heights of the header and footer:
// each is a content row plus an accent rule line, framing the body symmetrically.
const (
	topBarHeight = 2
	footerHeight = 2
)

// Model is the root Bubble Tea model.
type Model struct {
	width, height int
	token         string
	fallbackHost  string
	fallbackPort  int

	// events is the channel the Hub pushes viewMsg/instancesMsg onto. Init starts
	// the listen loop and each hub-sourced message re-arms it (see listenCmd).
	events chan tea.Msg

	instances []discovery.Instance
	active    int

	view    *tilt.View
	loadErr error

	// views/viewErrs cache the latest fetch for every discovered instance,
	// keyed by port, so the top-bar badges and the overview screen can show
	// cross-instance health without switching. m.view aliases views[currentPort].
	views    map[int]*tilt.View
	viewErrs map[int]error

	// overview is the cross-instance dashboard (the ‹1› screen); overviewSel is
	// the selected row within it and onlyFailing hides all-healthy instances.
	overview    bool
	overviewSel int
	onlyFailing bool

	selected int
	focus    focusArea

	vp     viewport.Model
	follow bool
	level  logLevel

	// renderedSegs is the active view's log-segment count at the last setLogs, so
	// a status-only delta (segment count unchanged) can skip the full re-assembly.
	renderedSegs int

	mode              inputMode
	typing            string
	logFilter         string
	pendingResource   string          // resource awaiting action confirmation
	pendingAction     tilt.ActionKind // action awaiting confirmation
	pendingTriggerAll bool            // a "trigger all" is awaiting confirmation

	showHelp bool

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
		views:        map[int]*tilt.View{},
		viewErrs:     map[int]error{},
		overview:     true, // land on the cross-instance overview; esc/digit drills in
		// Buffered so the Hub's watcher goroutines don't block on each other between
		// the UI consuming one message and re-arming the listen.
		events: make(chan tea.Msg, 128),
	}
}

// Events is the channel the UI listens on; hand it to a Hub so its websocket
// deltas reach the Bubble Tea loop. The channel is shared even though the Model
// is copied by value (a channel header aliases the same underlying channel).
func (m Model) Events() chan tea.Msg { return m.events }

func (m Model) Init() tea.Cmd {
	return listenCmd(m.events)
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

func (m Model) currentLabel() string {
	if m.active >= 0 && m.active < len(m.instances) {
		return m.instances[m.active].Label
	}
	return m.fallbackHost
}

// triggerAllTargets is the set of resource names a "trigger all" would update in
// the active instance: every enabled resource except the (Tiltfile) pseudo-
// resource. Disabled resources are skipped (triggering one would just error).
func (m Model) triggerAllTargets() []string {
	if m.view == nil {
		return nil
	}
	var names []string
	for _, r := range m.view.Resources() {
		if r.IsDisabled() || r.Name() == "(Tiltfile)" {
			continue
		}
		names = append(names, r.Name())
	}
	return names
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.vp.Width = max(m.width-sidebarWidth-1, 10)
		m.setLogs() // setLogs owns vp.Height (it varies with the detail strip)
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeLogFilter:
			return m.updateFilterInput(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		}
		return m.updateKeys(msg)

	case viewMsg:
		// Cache every instance's view by port for the badges/overview; only the
		// active instance's snapshot updates the focused pane and log viewport.
		// Re-arm the listen so the next hub message is delivered.
		if msg.err != nil {
			m.viewErrs[msg.port] = msg.err
			if msg.port == m.currentPort() {
				m.loadErr = msg.err
			}
			return m, listenCmd(m.events)
		}
		delete(m.viewErrs, msg.port)
		m.views[msg.port] = msg.view
		if msg.port == m.currentPort() {
			m.loadErr = nil
			m.view = msg.view
			m.clampSelection()
			// Only re-assemble the log viewport when the log pane is visible and the
			// log actually grew. Status-only deltas (and every delta while on the
			// overview screen) skip the full O(total-logs) rebuild — this is what
			// keeps idle CPU near zero on chatty stacks.
			if !m.overview && len(m.view.LogList.Segments) != m.renderedSegs {
				m.setLogs()
			}
		}
		return m, listenCmd(m.events)

	case instancesMsg:
		m = m.handleInstances(msg)
		return m, listenCmd(m.events)

	case actionResultMsg:
		// No refetch needed: the websocket stream reflects the resulting status
		// change on its own.
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("%s %s failed: %v", msg.kind, msg.resource, msg.err)
			m.statusErr = true
		} else {
			m.statusMsg = fmt.Sprintf("%s %s ✓", msg.kind, msg.resource)
			m.statusErr = false
		}
		return m, nil

	case notifyMsg:
		m.statusMsg = msg.text
		m.statusErr = msg.err
		return m, nil
	}
	return m, nil
}

func (m Model) handleInstances(msg instancesMsg) Model {
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
	// Drop cached views for instances that have gone away, so stale health never
	// lingers in the badges/overview. (The Hub closes their websockets in step.)
	valid := make(map[int]bool, len(insts))
	for _, in := range insts {
		valid[in.Port] = true
	}
	for p := range m.views {
		if !valid[p] {
			delete(m.views, p)
			delete(m.viewErrs, p)
		}
	}
	// If the active instance changed (the previously-active one went away, or this
	// is the first discovery), point the focused pane at the new instance's cached
	// view. The Hub streams every instance continuously, so the snapshot is already
	// warm (or arrives momentarily) — no refetch needed.
	if m.currentPort() != prevPort {
		port := m.currentPort()
		m.view = m.views[port]
		m.loadErr = m.viewErrs[port]
		m.selected = 0
		m.clampSelection()
		m.setLogs()
	}
	return m
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
	if m.overview {
		return m.updateOverviewKeys(msg)
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "esc":
		m.focus = focusSidebar
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
	case "1":
		m.overview = true
		m.clampOverview()
		return m, nil
	case "2", "3", "4", "5", "6", "7", "8", "9":
		return m.gotoInstance(int(msg.String()[0] - '2'))
	case "r":
		if r, ok := m.selectedResource(); ok {
			m.mode = modeConfirm
			m.pendingResource = r.Name()
			m.pendingAction = tilt.ActionTrigger
		}
		return m, nil
	case "R":
		if len(m.triggerAllTargets()) > 0 {
			m.mode = modeConfirm
			m.pendingTriggerAll = true
		}
		return m, nil
	case "d":
		// One toggle: enable a disabled resource, disable an enabled one.
		if r, ok := m.selectedResource(); ok {
			m.mode = modeConfirm
			m.pendingResource = r.Name()
			m.pendingAction = tilt.ActionDisable
			if r.IsDisabled() {
				m.pendingAction = tilt.ActionEnable
			}
		}
		return m, nil
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
	case "o":
		if r, ok := m.selectedResource(); ok && m.view != nil {
			return m, openLogsCmd(r.Name(), m.resourceLogText(r))
		}
		return m, nil
	case "s":
		if r, ok := m.selectedResource(); ok && m.view != nil {
			return m, saveLogsCmd(r.Name(), m.resourceLogText(r))
		}
		return m, nil
	case "S":
		// Instance-level: snapshot the whole active instance (no resource needed).
		m.statusMsg = "creating snapshot…"
		m.statusErr = false
		return m, snapshotCmd(m.currentHost(), m.currentPort(), m.currentLabel())
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

// updateConfirm handles the action confirmation prompt (trigger / enable /
// disable): y/enter confirms, ctrl+c quits, anything else cancels.
func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "Y", "enter":
		if m.pendingTriggerAll {
			targets := m.triggerAllTargets()
			m.mode = modeNormal
			m.pendingTriggerAll = false
			m.statusMsg = fmt.Sprintf("triggering %d resources…", len(targets))
			m.statusErr = false
			return m, triggerAllCmd(targets, m.currentPort())
		}
		res, act := m.pendingResource, m.pendingAction
		m.mode = modeNormal
		m.pendingResource = ""
		m.statusMsg = fmt.Sprintf("%s %s…", act, res)
		m.statusErr = false
		return m, actionCmd(act, res, m.currentPort())
	default:
		m.mode = modeNormal
		m.pendingResource = ""
		m.pendingTriggerAll = false
		return m, nil
	}
}

func (m Model) switchInstance(d int) (tea.Model, tea.Cmd) {
	if len(m.instances) < 2 {
		return m, nil
	}
	return m.gotoInstance((m.active + d + len(m.instances)) % len(m.instances))
}

// gotoInstance switches to the instance at idx (0-based) without a restart; a
// no-op if idx is out of range or already active. The switch is instant: the
// Hub streams every instance, so the target's snapshot is already cached (a
// brief blank pane until the first snapshot arrives if it isn't yet).
func (m Model) gotoInstance(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.instances) || idx == m.active {
		return m, nil
	}
	m.active = idx
	port := m.currentPort()
	m.view = m.views[port]
	m.loadErr = m.viewErrs[port]
	m.selected = 0
	m.statusMsg = ""
	m.clampSelection()
	m.setLogs()
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	bodyH := max(m.height-topBarHeight-footerHeight, 3)
	var body string
	if m.overview {
		body = m.renderOverview(bodyH)
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderSidebar(bodyH),
			m.renderRightPane(max(m.width-sidebarWidth-1, 10), bodyH),
		)
	}
	frame := lipgloss.JoinVertical(lipgloss.Left, m.renderTopBar(), body, m.renderFooter())
	switch {
	case m.showHelp:
		return overlayCenter(frame, m.helpBox(), m.width, m.height)
	case m.mode == modeConfirm:
		return overlayCenter(frame, m.confirmBox(), m.width, m.height)
	}
	return frame
}

// confirmBox is the action confirmation popup, centered by overlayCenter.
func (m Model) confirmBox() string {
	var title string
	if m.pendingTriggerAll {
		title = fmt.Sprintf("Trigger all %d resources in %s?", len(m.triggerAllTargets()), m.currentLabel())
	} else {
		verb := m.pendingAction.String()
		if verb != "" {
			verb = strings.ToUpper(verb[:1]) + verb[1:]
		}
		title = verb + " " + m.pendingResource + "?"
	}
	lines := []string{
		m.theme.header().Render(title),
		"",
		m.theme.muted().Render("(y) yes        (n) no"),
	}
	// Grow the box to fit the title (a "Trigger all N resources in <label>?"
	// prompt is wider than a single-resource one).
	inner := 0
	for _, ln := range lines {
		if w := lipgloss.Width(ln); w > inner {
			inner = w
		}
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Accent).
		Foreground(m.theme.Text).
		Padding(2, 6).
		Width(max(inner+12, 46)). // +12 for the horizontal padding (6 each side)
		Align(lipgloss.Center).
		Render(strings.Join(lines, "\n"))
}

func (m Model) renderTopBar() string {
	title := m.theme.accent().Bold(true).Render(" LAZYTILT ")
	segs := make([]string, 0, len(m.instances)+1)
	// ‹1› overview leads the bar, highlighted while the overview screen is open;
	// the instances follow as ‹2›, ‹3›, …
	ovStyle := m.theme.muted()
	if m.overview {
		ovStyle = m.theme.header()
	}
	segs = append(segs, ovStyle.Render("‹1› overview"))
	for i, in := range m.instances {
		tag := fmt.Sprintf("‹%d›", i+2)
		nameStyle := m.theme.muted()
		if i == m.active && !m.overview {
			nameStyle = m.theme.header()
		}
		badge, bc := m.instanceBadge(in.Port)
		seg := nameStyle.Render(tag+" "+in.Label) + " " + lipgloss.NewStyle().Foreground(bc).Render(badge)
		segs = append(segs, seg)
	}
	bar := " " + title + "   " + strings.Join(segs, "   ")
	// Title/instances row + a full-width accent rule, so the header reads as a
	// header without painting a (uneven) background band.
	rule := m.theme.accent().Render(strings.Repeat("─", m.width))
	return lipgloss.JoinVertical(lipgloss.Left, ansi.Truncate(bar, m.width, "…"), rule)
}

func (m Model) renderFooter() string {
	// A full-width accent rule above the keys, mirroring the header rule below the
	// title so the body is framed symmetrically.
	rule := m.theme.accent().Render(strings.Repeat("─", m.width))

	var inner string
	switch {
	case m.overview:
		keys := " ↑↓ move · ⏎ open · F only-failing · 2-9 instance · 1/esc back · T theme · ? help · q quit"
		inner = ansi.Truncate(keys, m.width, "…")
	case m.mode == modeLogFilter:
		inner = fmt.Sprintf(" search logs: %s▏", m.typing)
	case m.statusMsg != "":
		c := m.theme.OK
		if m.statusErr {
			c = m.theme.Err
		}
		inner = lipgloss.NewStyle().Foreground(c).Render(ansi.Truncate(" "+m.statusMsg, m.width, "…"))
	default:
		keys := " ↑↓ move · r trigger · R trigger-all · d enable/disable · ⏎ logs · / search · f follow · L level · o open in editor · s save logs · S snapshot · 1 overview · 2-9/[ ] instance · T theme · ? help · q quit"
		inner = ansi.Truncate(keys, m.width, "…")
	}
	return lipgloss.JoinVertical(lipgloss.Left, rule, m.theme.footer().Width(m.width).Render(inner))
}

// helpBox is the floating help popup (a bordered box, centered by overlayCenter).
func (m Model) helpBox() string {
	rows := [][2]string{
		{"↑/k ↓/j", "move selection"},
		{"≡ (top row)", "All Resources — combined log stream"},
		{"⏎ / tab", "focus logs / toggle pane"},
		{"1", "overview — health of all instances"},
		{"F", "overview: show only failing"},
		{"2 … 9", "jump to instance N"},
		{"[  ]", "previous / next instance"},
		{"r", "trigger update (asks y/n)"},
		{"R", "trigger all resources (asks y/n)"},
		{"d", "enable / disable (asks y/n)"},
		{"/", "search logs (highlights matches)"},
		{"f", "follow / tail logs"},
		{"L", "cycle log level"},
		{"c", "clear log filter"},
		{"o", "open logs in $EDITOR (vim)"},
		{"s", "save logs to a temp file"},
		{"S", "snapshot the instance (tilt snapshot)"},
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
