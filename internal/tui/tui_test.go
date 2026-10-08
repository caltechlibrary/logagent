package tui

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// app builds a model on an 80x12 terminal that has already seen its size.
func app(cfg Config) *App {
	if cfg.AppName == "" {
		cfg.AppName = "logagent"
	}
	m := NewApp(cfg)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	return m
}

// press sends keys in turn and returns the model, which is a pointer.
func press(m *App, keys ...string) *App {
	for _, k := range keys {
		m.Update(key(k))
	}
	return m
}

func lineStarting(view, prefix string) string {
	for _, l := range strings.Split(view, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

func TestInitClearsTheScreen(t *testing.T) {
	cmd := NewApp(Config{AppName: "logagent"}).Init()
	if cmd == nil {
		t.Fatal("Init() = nil, want a command that clears the screen")
	}
	if got := cmd(); got != tea.ClearScreen() {
		t.Errorf("Init()() = %#v, want tea.ClearScreen()", got)
	}
}

func TestVerbsAreTheCommandsInOrderWithCheckAndReportReal(t *testing.T) {
	var names []string
	var planned []string
	for _, v := range Verbs() {
		names = append(names, v.Name)
		if v.Description == "" {
			t.Errorf("%s has no description", v.Name)
		}
		if v.Planned {
			planned = append(planned, v.Name)
		}
	}
	if got := strings.Join(names, " "); got != "check report watch respond analyze" {
		t.Errorf("verbs = %s", got)
	}
	if got := strings.Join(planned, " "); got != "watch respond analyze" {
		t.Errorf("planned = %s, want everything but check and report", got)
	}
}

func TestMenuShowsEveryVerbAndMarksThePlannedOnes(t *testing.T) {
	v := app(Config{}).View()
	for _, want := range []string{"logagent", "check", "report", "watch", "respond", "analyze"} {
		if !strings.Contains(v, want) {
			t.Errorf("menu lacks %q:\n%s", want, v)
		}
	}
	if n := strings.Count(v, "(planned)"); n != 3 {
		t.Errorf("%d verbs marked planned, want 3:\n%s", n, v)
	}
	if strings.Contains(lineStarting(v, "> "), "(planned)") {
		t.Errorf("check, the first verb, is marked planned:\n%s", v)
	}
}

// The cursor is a character, not only a colour, so it shows without colour.
func TestTheCursorIsAMarkerAndMovesWithTheKeys(t *testing.T) {
	m := app(Config{})
	if l := lineStarting(m.View(), "> "); !strings.Contains(l, "check") {
		t.Fatalf("cursor line = %q, want check", l)
	}
	press(m, "down")
	if l := lineStarting(m.View(), "> "); !strings.Contains(l, "report") {
		t.Errorf("after down: %q", l)
	}
	press(m, "j", "j")
	if l := lineStarting(m.View(), "> "); !strings.Contains(l, "respond") {
		t.Errorf("after j j: %q", l)
	}
	press(m, "k", "up")
	if l := lineStarting(m.View(), "> "); !strings.Contains(l, "report") {
		t.Errorf("after k up: %q", l)
	}
	n := 0
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.HasPrefix(l, "> ") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d cursor lines, want 1", n)
	}
}

func TestTheCursorStopsAtBothEnds(t *testing.T) {
	m := app(Config{})
	press(m, "up", "up")
	if l := lineStarting(m.View(), "> "); !strings.Contains(l, "check") {
		t.Errorf("above the top: %q", l)
	}
	for i := 0; i < 10; i++ {
		press(m, "down")
	}
	if l := lineStarting(m.View(), "> "); !strings.Contains(l, "analyze") {
		t.Errorf("below the bottom: %q", l)
	}
}

func TestAPlannedVerbShowsAPlaceholderAndGoesBack(t *testing.T) {
	m := app(Config{AppName: "logagent"})
	press(m, "down", "down", "enter")
	v := m.View()
	for _, want := range []string{"watch", "not implemented yet", "logagent help watch"} {
		if !strings.Contains(v, want) {
			t.Errorf("placeholder lacks %q:\n%s", want, v)
		}
	}
	press(m, "esc")
	if v := m.View(); !strings.Contains(v, "analyze") || strings.Contains(v, "not implemented yet") {
		t.Errorf("did not return to the menu:\n%s", v)
	}
	// The cursor is where it was.
	if l := lineStarting(m.View(), "> "); !strings.Contains(l, "watch") {
		t.Errorf("cursor after returning: %q", l)
	}
	for _, back := range []string{"q", "enter", "esc"} {
		press(m, "enter")
		press(m, back)
		if strings.Contains(m.View(), "not implemented yet") {
			t.Errorf("%s did not leave the placeholder", back)
		}
	}
}

func TestCheckRunsTheCheckAndShowsItsReport(t *testing.T) {
	calls := 0
	m := app(Config{Check: func() (string, error) {
		calls++
		return "LOG /var/log/nginx/access.log\n[gap] field-missing rt: rt is not in the format\nFound 1 gap.", nil
	}})
	press(m, "enter")
	v := m.View()
	for _, want := range []string{"LOG /var/log/nginx/access.log", "[gap] field-missing rt", "Found 1 gap."} {
		if !strings.Contains(v, want) {
			t.Errorf("report lacks %q:\n%s", want, v)
		}
	}
	if calls != 1 {
		t.Errorf("check ran %d times, want 1", calls)
	}
	press(m, "esc")
	if strings.Contains(m.View(), "[gap]") || calls != 1 {
		t.Error("esc did not return to the menu")
	}
	press(m, "enter")
	if calls != 2 {
		t.Errorf("a second Enter must run the check again: %d runs", calls)
	}
}

func TestReportRunsAndShowsItsReportAndFailureIsShownNotHidden(t *testing.T) {
	calls := 0
	m := app(Config{Report: func() (string, error) {
		calls++
		return "1. SOURCES\n  lines     12 read\n5. BY FAMILY", nil
	}})
	press(m, "down", "enter")
	v := m.View()
	for _, want := range []string{"1. SOURCES", "12 read", "5. BY FAMILY"} {
		if !strings.Contains(v, want) {
			t.Errorf("report lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "not implemented") || calls != 1 {
		t.Errorf("report is still a placeholder, or ran %d times:\n%s", calls, v)
	}
	press(m, "esc", "enter")
	if calls != 2 {
		t.Errorf("a second Enter must run the report again: %d runs", calls)
	}
	m = app(Config{Report: func() (string, error) { return "", errors.New("the log is not on this machine") }})
	press(m, "down", "enter")
	if v := m.View(); !strings.Contains(v, "report failed") || !strings.Contains(v, "the log is not on this machine") {
		t.Errorf("failure not shown:\n%s", v)
	}
	// Without a report function the interface says so, as for check.
	m = app(Config{})
	press(m, "down", "enter")
	if v := m.View(); !strings.Contains(v, "not available from this interface") {
		t.Errorf("no function configured:\n%s", v)
	}
}

func TestCheckFailureIsShownNotHidden(t *testing.T) {
	m := app(Config{Check: func() (string, error) {
		return "", errors.New("no configuration file: looked at --config (not given)")
	}})
	press(m, "enter")
	v := m.View()
	if !strings.Contains(v, "check failed") || !strings.Contains(v, "no configuration file") {
		t.Errorf("failure not shown:\n%s", v)
	}
	press(m, "q")
	if !strings.Contains(m.View(), "analyze") {
		t.Error("q did not return to the menu from a failure")
	}
}

func TestCheckWithNoCheckFunctionSaysSo(t *testing.T) {
	m := app(Config{})
	press(m, "enter")
	if v := m.View(); !strings.Contains(v, "not available") {
		t.Errorf("view:\n%s", v)
	}
}

func TestALongReportScrollsAndStaysInsideTheTerminal(t *testing.T) {
	var lines []string
	for i := 1; i <= 100; i++ {
		lines = append(lines, fmt.Sprintf("report line %03d", i))
	}
	text := strings.Join(lines, "\n")
	m := app(Config{Check: func() (string, error) { return text, nil }})
	press(m, "enter")
	v := m.View()
	if !strings.Contains(v, "report line 001") || strings.Contains(v, "report line 100") {
		t.Errorf("first screen should hold the top of the report only:\n%s", v)
	}
	if n := strings.Count(v, "\n") + 1; n > 12 {
		t.Errorf("view is %d lines, the terminal has 12", n)
	}
	press(m, "pgdown")
	if strings.Contains(m.View(), "report line 001") {
		t.Error("page down did not move")
	}
	press(m, "G")
	if !strings.Contains(m.View(), "report line 100") {
		t.Errorf("G did not reach the end:\n%s", m.View())
	}
	press(m, "g")
	if !strings.Contains(m.View(), "report line 001") {
		t.Errorf("g did not return to the top:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "esc") {
		t.Error("the screen does not say how to go back")
	}
}

func TestQuitKeys(t *testing.T) {
	isQuit := func(cmd tea.Cmd) bool {
		if cmd == nil {
			return false
		}
		_, ok := cmd().(tea.QuitMsg)
		return ok
	}
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		m := app(Config{})
		if _, cmd := m.Update(key(k)); !isQuit(cmd) {
			t.Errorf("%s at the menu does not quit", k)
		}
	}
	// In a screen q and esc go back, but ctrl+c always quits.
	m := app(Config{Check: func() (string, error) { return "x", nil }})
	press(m, "enter")
	if _, cmd := m.Update(key("q")); isQuit(cmd) {
		t.Error("q in the report quit the program instead of going back")
	}
	press(m, "enter")
	if _, cmd := m.Update(key("ctrl+c")); !isQuit(cmd) {
		t.Error("ctrl+c in the report does not quit")
	}
}

func TestColorIsOptionalAndNeverTheOnlySignal(t *testing.T) {
	plain := app(Config{ColorEnabled: false}).View()
	if strings.Contains(plain, "\x1b[") {
		t.Errorf("colour disabled but the view has escape codes: %q", plain)
	}
	colored := app(Config{ColorEnabled: true}).View()
	if !strings.Contains(colored, "\x1b[7m") {
		t.Error("colour enabled but the cursor row is not highlighted")
	}
	// Both still mark the cursor with the character.
	for _, v := range []string{plain, colored} {
		if !strings.Contains(v, "> ") {
			t.Error("no cursor marker")
		}
	}
}

func TestTheViewBeforeTheTerminalSizeIsKnownStillRenders(t *testing.T) {
	m := NewApp(Config{AppName: "logagent"})
	if v := m.View(); !strings.Contains(v, "check") {
		t.Errorf("view:\n%s", v)
	}
}

// One real program run through teatest: choose check, read it, go back, quit.
func TestRunThroughABubbleTeaProgram(t *testing.T) {
	m := NewApp(Config{AppName: "logagent", Check: func() (string, error) { return "REPORT BODY", nil }})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	wait := func(want string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return bytes.Contains(b, []byte(want)) }, teatest.WithDuration(2*time.Second))
	}
	wait("analyze")
	tm.Send(key("enter"))
	wait("REPORT BODY")
	tm.Send(key("esc"))
	tm.Send(key("q"))
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestAReportOpenedAgainStartsAtTheTop(t *testing.T) {
	var lines []string
	for i := 1; i <= 100; i++ {
		lines = append(lines, fmt.Sprintf("report line %03d", i))
	}
	text := strings.Join(lines, "\n")
	m := app(Config{Check: func() (string, error) { return text, nil }})
	press(m, "enter", "G", "esc", "enter")
	if v := m.View(); !strings.Contains(v, "report line 001") || strings.Contains(v, "report line 100") {
		t.Errorf("second opening did not start at the top:\n%s", v)
	}
}
