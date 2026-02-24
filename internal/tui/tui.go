package tui

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fabiant7t/jeltz/internal/logstream"
)

type Config struct {
	ListenAddr string
	Events     <-chan logstream.Event
	Dropped    func() uint64
	Stop       func()
}

type eventMsg struct{ ev logstream.Event }

type streamClosedMsg struct{}

func waitEvent(ch <-chan logstream.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return streamClosedMsg{}
		}
		return eventMsg{ev: ev}
	}
}

type row struct {
	level slog.Level
	text  string
}

type model struct {
	cfg Config

	vp viewport.Model

	events     []logstream.Event
	rows       []row
	filter     slog.Level
	visible    map[string]bool
	keyCatalog map[string]struct{}

	searchMode  bool
	searchInput string
	searchQuery string

	visMode   bool
	visKeys   []string
	visCursor int

	total int
}

func newModel(cfg Config) model {
	vp := viewport.New(0, 0)
	return model{
		cfg:        cfg,
		vp:         vp,
		filter:     -1000, // show all
		visible:    make(map[string]bool),
		keyCatalog: make(map[string]struct{}),
	}
}

func (m model) Init() tea.Cmd { return waitEvent(m.cfg.Events) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		top := 2
		bottom := 1
		h := msg.Height - top - bottom
		if h < 1 {
			h = 1
		}
		m.vp.Width = msg.Width
		m.vp.Height = h
		m.rebuildViewport(true)
		return m, nil
	case tea.KeyMsg:
		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.searchInput = ""
				m.searchQuery = ""
				m.filter = -1000
				m.rebuildViewport(false)
				return m, nil
			case "enter":
				m.searchMode = false
				m.searchQuery = strings.TrimSpace(m.searchInput)
				m.rebuildViewport(false)
				return m, nil
			case "backspace":
				if len(m.searchInput) > 0 {
					m.searchInput = m.searchInput[:len(m.searchInput)-1]
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.searchInput += msg.String()
				}
				return m, nil
			}
		}
		if m.visMode {
			switch msg.String() {
			case "esc", "v":
				m.visMode = false
				m.searchQuery = ""
				m.filter = -1000
				m.rebuildViewport(false)
				return m, nil
			case "j", "down":
				if m.visCursor < len(m.visKeys)-1 {
					m.visCursor++
				}
				return m, nil
			case "k", "up":
				if m.visCursor > 0 {
					m.visCursor--
				}
				return m, nil
			case " ":
				if len(m.visKeys) > 0 {
					k := m.visKeys[m.visCursor]
					m.visible[k] = !m.visible[k]
				}
				return m, nil
			case "a":
				for _, k := range m.visKeys {
					m.visible[k] = true
				}
				return m, nil
			case "n":
				for _, k := range m.visKeys {
					m.visible[k] = false
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			if m.cfg.Stop != nil {
				m.cfg.Stop()
			}
			return m, tea.Quit
		case "esc":
			m.searchMode = false
			m.searchInput = ""
			m.searchQuery = ""
			m.filter = -1000
			m.rebuildViewport(false)
			return m, nil
		case "/":
			m.searchMode = true
			m.searchInput = ""
			return m, nil
		case "f":
			m.cycleFilter()
			m.rebuildViewport(false)
			return m, nil
		case "v":
			m.visMode = true
			m.syncVisKeys()
			return m, nil
		case "c":
			m.events = nil
			m.rows = nil
			m.total = 0
			m.searchQuery = ""
			m.visible = make(map[string]bool)
			m.keyCatalog = make(map[string]struct{})
			m.rebuildViewport(false)
			return m, nil
		case "j", "down":
			m.vp.LineDown(1)
			return m, nil
		case "k", "up":
			m.vp.LineUp(1)
			return m, nil
		case "ctrl+d":
			m.vp.HalfViewDown()
			return m, nil
		case "ctrl+u":
			m.vp.HalfViewUp()
			return m, nil
		case "g", "home":
			m.vp.GotoTop()
			return m, nil
		case "G", "end":
			m.vp.GotoBottom()
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	case eventMsg:
		m.total++
		m.events = append(m.events, msg.ev)
		for k := range msg.ev.Attrs {
			m.keyCatalog[k] = struct{}{}
			if _, ok := m.visible[k]; !ok {
				m.visible[k] = true
			}
		}
		if len(m.events) > 2000 {
			m.events = m.events[len(m.events)-2000:]
		}
		m.rebuildViewport(true)
		return m, waitEvent(m.cfg.Events)
	case streamClosedMsg:
		return m, nil
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m *model) cycleFilter() {
	switch m.filter {
	case -1000:
		m.filter = slog.LevelInfo
	case slog.LevelInfo:
		m.filter = slog.LevelWarn
	case slog.LevelWarn:
		m.filter = slog.LevelError
	default:
		m.filter = -1000
	}
}

func (m *model) rebuildViewport(stickBottom bool) {
	m.rows = m.rows[:0]
	lines := make([]string, 0, len(m.events))
	for _, ev := range m.events {
		if m.searchQuery != "" && !eventMatchesSearch(ev, m.searchQuery) {
			continue
		}
		r := row{
			level: ev.Level,
			text:  renderEvent(ev, m.visible),
		}
		if m.filter != -1000 && r.level < m.filter {
			continue
		}
		m.rows = append(m.rows, r)
		lines = append(lines, r.text)
	}
	m.vp.SetContent(strings.Join(lines, "\n"))
	if stickBottom {
		m.vp.GotoBottom()
	}
}

func (m model) View() string {
	if m.searchMode {
		return topStyle.Render(fmt.Sprintf("search: /%s", m.searchInput)) + "\n" + m.vp.View() + "\n" + m.status()
	}
	top := topStyle.Render(fmt.Sprintf(
		"jeltz UI  |  listen=%s  |  filter=%s  |  / search  v visibility  vim: j/k  ctrl-d/u  g/G  f filter  c clear  q quit",
		m.cfg.ListenAddr, filterLabel(m.filter),
	))
	view := top + "\n" + m.vp.View() + "\n" + m.status()
	if m.visMode {
		view += "\n" + m.renderVisibilityDialog()
	}
	return view
}

func (m model) status() string {
	return statusStyle.Render(fmt.Sprintf(
		"logs=%d  shown=%d  dropped=%d  search=%q",
		m.total, len(m.rows), dropped(m.cfg.Dropped), m.searchQuery,
	))
}

func (m *model) syncVisKeys() {
	m.visKeys = m.visKeys[:0]
	for k := range m.keyCatalog {
		m.visKeys = append(m.visKeys, k)
	}
	sort.Strings(m.visKeys)
	if m.visCursor >= len(m.visKeys) {
		m.visCursor = len(m.visKeys) - 1
	}
	if m.visCursor < 0 {
		m.visCursor = 0
	}
}

func (m model) renderVisibilityDialog() string {
	if len(m.visKeys) == 0 {
		return dialogStyle.Render("Visibility settings\n(no keys yet)\n\nesc/v: close")
	}
	lines := []string{"Visibility settings (space toggle, a all-on, n all-off, esc close)", ""}
	for i, k := range m.visKeys {
		check := "[x]"
		if !m.visible[k] {
			check = "[ ]"
		}
		cursor := "  "
		if i == m.visCursor {
			cursor = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s %s", cursor, check, k))
	}
	return dialogStyle.Render(strings.Join(lines, "\n"))
}

func dropped(fn func() uint64) uint64 {
	if fn == nil {
		return 0
	}
	return fn()
}

func eventMatchesSearch(ev logstream.Event, q string) bool {
	query := strings.ToLower(strings.TrimSpace(q))
	if query == "" {
		return true
	}
	if strings.Contains(strings.ToLower(ev.Message), query) {
		return true
	}
	for k, v := range ev.Attrs {
		if strings.Contains(strings.ToLower(k), query) || strings.Contains(strings.ToLower(v), query) {
			return true
		}
	}
	return false
}

func filterLabel(filter slog.Level) string {
	switch filter {
	case -1000:
		return "all"
	case slog.LevelInfo:
		return "info+"
	case slog.LevelWarn:
		return "warn+"
	case slog.LevelError:
		return "error"
	default:
		return "all"
	}
}

func renderEvent(ev logstream.Event, visible map[string]bool) string {
	ts := ev.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	level := levelStyle(ev.Level).Render(strings.ToUpper(ev.Level.String()))
	comp := ev.Component
	if comp == "" {
		comp = "-"
	}
	var kv []string
	if len(ev.Attrs) > 0 {
		keys := make([]string, 0, len(ev.Attrs))
		for k := range ev.Attrs {
			if v, ok := visible[k]; ok && !v {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		kv = make([]string, 0, len(keys))
		for _, k := range keys {
			kv = append(kv, fmt.Sprintf("%s=%q", k, ev.Attrs[k]))
		}
	}

	line := fmt.Sprintf("%s %s [%s] %s",
		timeStyle.Render(ts.Format("15:04:05.000")),
		level,
		componentStyle.Render(comp),
		ev.Message,
	)
	if len(kv) > 0 {
		line += "  " + strings.Join(kv, " ")
	}
	return line
}

var (
	topStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("62")).Padding(0, 1)
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Background(lipgloss.Color("236")).Padding(0, 1)
	dialogStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("69")).Padding(0, 1)
	timeStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	componentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
)

func levelStyle(level slog.Level) lipgloss.Style {
	switch {
	case level >= slog.LevelError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	case level >= slog.LevelWarn:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	case level >= slog.LevelInfo:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	}
}

func Run(ctx context.Context, cfg Config) error {
	p := tea.NewProgram(newModel(cfg), tea.WithAltScreen())
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			p.Quit()
		case <-done:
		}
	}()
	_, err := p.Run()
	close(done)
	return err
}
