// Package tui is logagent's terminal interface: a menu of the top-level
// commands, a placeholder for each one not built yet and a real screen for
// check. It is built on Bubble Tea, as clasm is, and renders inline.
//
// The cursor is a character as well as a highlight, and colour is optional
// (NO_COLOR and non-terminal output turn it off), so the screens read the same
// without colour.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	ansiReverse = "\x1b[7m"
	ansiReset   = "\x1b[0m"

	defaultWidth  = 80
	defaultHeight = 24
)

// Verb is one top-level command shown in the menu.
type Verb struct {
	// Name is the command word.
	Name string
	// Description says what the command is for.
	Description string
	// Planned marks a command that is not implemented yet.
	Planned bool
}

// Verbs returns the commands in menu order.
//
// @returns {[]Verb} check, report, watch, respond and analyze
// @example
//
//	for _, v := range tui.Verbs() {
//		fmt.Println(v.Name, v.Planned)
//	}
func Verbs() []Verb {
	return []Verb{
		{Name: "check", Description: "validate that the web server logs the data detection needs"},
		{Name: "report", Description: "summarize traffic and what the tiers saw"},
		{Name: "watch", Description: "detect bursts and automated traffic as they happen", Planned: true},
		{Name: "respond", Description: "generate, and with approval apply, a response", Planned: true},
		{Name: "analyze", Description: "review aggregated history over weeks and months", Planned: true},
	}
}

// Config configures the interface.
type Config struct {
	// AppName is the program name shown in titles and in the hint to read a
	// command's manual page.
	AppName string
	// Check runs the check and returns the report as text. A nil Check means
	// the command is not available from this interface.
	Check func() (string, error)
	// Report runs the traffic report for the default window and returns it as
	// text. A nil Report means the command is not available from this interface.
	Report func() (string, error)
	// ColorEnabled turns on the reverse-video cursor row. It is false for
	// NO_COLOR and for output that is not a terminal.
	ColorEnabled bool
}

type screen int

const (
	menuScreen screen = iota
	placeholderScreen
	reportScreen
)

// App is the Bubble Tea model of the whole interface.
type App struct {
	cfg           Config
	screen        screen
	cursor        int
	width, height int
	verb          Verb
	report        viewport.Model
	reportTitle   string
}

// NewApp builds the model for cfg.
//
// @param cfg {Config} what to show and run
// @returns {*App} the model, ready for tea.NewProgram
// @example
//
//	p := tea.NewProgram(tui.NewApp(tui.Config{AppName: "logagent"}))
func NewApp(cfg Config) *App {
	return &App{cfg: cfg, report: viewport.New(defaultWidth, defaultHeight-2)}
}

// Run runs the interface until the person quits.
//
// @param ctx {context.Context} cancels the program
// @param cfg {Config} what to show and run
// @returns {error} an error from the program, or nil on a normal quit
// @example
//
//	err := tui.Run(ctx, tui.Config{AppName: "logagent", ColorEnabled: true})
func Run(ctx context.Context, cfg Config) error {
	_, err := tea.NewProgram(NewApp(cfg), tea.WithContext(ctx)).Run()
	return err
}

// Init clears the screen before the first render, as clasm's screens do.
func (m *App) Init() tea.Cmd { return tea.ClearScreen }

func (m *App) size() (int, int) {
	w, h := m.width, m.height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	return w, h
}

// Update handles keys and window size changes.
func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		w, h := m.size()
		m.report.Width, m.report.Height = w, h-2
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.screen {
		case menuScreen:
			return m.updateMenu(msg)
		case placeholderScreen:
			switch msg.String() {
			case "q", "esc", "enter":
				m.screen = menuScreen
			}
		case reportScreen:
			switch msg.String() {
			case "q", "esc":
				m.screen = menuScreen
			case "g", "home":
				m.report.GotoTop()
			case "G", "end":
				m.report.GotoBottom()
			default:
				m.report, _ = m.report.Update(msg)
			}
		}
	}
	return m, nil
}

func (m *App) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	verbs := Verbs()
	switch msg.String() {
	case "q", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(verbs)-1 {
			m.cursor++
		}
	case "enter":
		m.choose(verbs[m.cursor])
	}
	return m, nil
}

// choose opens the screen for a verb. The check runs here, in the update, so
// the screen shows a finished report; it is a single read of the configuration.
func (m *App) choose(v Verb) {
	m.verb = v
	if v.Planned {
		m.screen = placeholderScreen
		return
	}
	var run func() (string, error)
	switch v.Name {
	case "check":
		run = m.cfg.Check
	case "report":
		run = m.cfg.Report
	}
	var text string
	if run == nil {
		text = v.Name + " is not available from this interface."
	} else if out, err := run(); err != nil {
		text = fmt.Sprintf("%s failed: %v", v.Name, err)
	} else {
		text = out
	}
	w, h := m.size()
	m.report.Width, m.report.Height = w, h-2
	m.report.SetContent(lipgloss.NewStyle().Width(w).Render(strings.TrimRight(text, "\n")))
	m.report.GotoTop()
	m.reportTitle = m.cfg.AppName + " " + v.Name
	m.screen = reportScreen
}

// View renders the current screen.
func (m *App) View() string {
	switch m.screen {
	case placeholderScreen:
		return fmt.Sprintf("%s %s\n\n%s is not implemented yet.\nRead what it will do with:  %s help %s\n\nq, esc or enter to go back\n",
			m.cfg.AppName, m.verb.Name, m.verb.Name, m.cfg.AppName, m.verb.Name)
	case reportScreen:
		return m.reportTitle + "\n" + m.report.View() + "\nesc or q back, up/down page scroll, g/G top/bottom, ctrl+c quit"
	}
	var b strings.Builder
	b.WriteString(m.cfg.AppName + "\n\n")
	for i, v := range Verbs() {
		desc := v.Description
		if v.Planned {
			desc += " (planned)"
		}
		row := fmt.Sprintf("%-8s %s", v.Name, desc)
		if i == m.cursor {
			row = "> " + row
			if m.cfg.ColorEnabled {
				row = ansiReverse + row + ansiReset
			}
		} else {
			row = "  " + row
		}
		b.WriteString(row + "\n")
	}
	b.WriteString("\nup/down or j/k move, enter choose, q quit\n")
	return b.String()
}
