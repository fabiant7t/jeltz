package tui

import (
	"context"
	"fmt"
	"log/slog"
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

	rows   []row
	filter slog.Level

	total int
}

func newModel(cfg Config) model {
	vp := viewport.New(0, 0)
	return model{
		cfg:    cfg,
		vp:     vp,
		filter: -1000, // show all
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
		switch msg.String() {
		case "q", "ctrl+c":
			if m.cfg.Stop != nil {
				m.cfg.Stop()
			}
			return m, tea.Quit
		case "l":
			m.cycleFilter()
			m.rebuildViewport(false)
			return m, nil
		case "c":
			m.rows = nil
			m.total = 0
			m.rebuildViewport(false)
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	case eventMsg:
		m.total++
		m.rows = append(m.rows, row{
			level: msg.ev.Level,
			text:  renderEvent(msg.ev),
		})
		if len(m.rows) > 2000 {
			m.rows = m.rows[len(m.rows)-2000:]
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
	lines := make([]string, 0, len(m.rows))
	for _, r := range m.rows {
		if m.filter != -1000 && r.level < m.filter {
			continue
		}
		lines = append(lines, r.text)
	}
	m.vp.SetContent(strings.Join(lines, "\n"))
	if stickBottom {
		m.vp.GotoBottom()
	}
}

func (m model) View() string {
	top := topStyle.Render(fmt.Sprintf(
		"jeltz UI  |  listen=%s  |  filter=%s  |  commands: l=cycle filter  c=clear  q=quit",
		m.cfg.ListenAddr, filterLabel(m.filter),
	))
	status := statusStyle.Render(fmt.Sprintf(
		"logs=%d  shown=%d  dropped=%d",
		m.total, countShown(m.rows, m.filter), dropped(m.cfg.Dropped),
	))
	return top + "\n" + m.vp.View() + "\n" + status
}

func dropped(fn func() uint64) uint64 {
	if fn == nil {
		return 0
	}
	return fn()
}

func countShown(rows []row, filter slog.Level) int {
	if filter == -1000 {
		return len(rows)
	}
	n := 0
	for _, r := range rows {
		if r.level >= filter {
			n++
		}
	}
	return n
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

func renderEvent(ev logstream.Event) string {
	ts := ev.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	level := levelStyle(ev.Level).Render(strings.ToUpper(ev.Level.String()))
	comp := ev.Component
	if comp == "" {
		comp = "-"
	}
	return fmt.Sprintf("%s %s [%s] %s",
		timeStyle.Render(ts.Format("15:04:05.000")),
		level,
		componentStyle.Render(comp),
		ev.Message,
	)
}

var (
	topStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("62")).Padding(0, 1)
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Background(lipgloss.Color("236")).Padding(0, 1)
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
