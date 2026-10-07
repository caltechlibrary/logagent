package logagent

import (
	"errors"
	"strings"
	"testing"

	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/tui"
)

// stubTerminal makes Run believe it is (or is not) on a terminal and replaces
// the interface with a recorder.
func stubTerminal(t *testing.T, terminal bool, run func(tui.Config) error) *[]tui.Config {
	t.Helper()
	savedTerm, savedRun := isTerminal, runTUI
	t.Cleanup(func() { isTerminal, runTUI = savedTerm, savedRun })
	var got []tui.Config
	isTerminal = func() bool { return terminal }
	runTUI = func(cfg tui.Config) error {
		got = append(got, cfg)
		if run != nil {
			return run(cfg)
		}
		return nil
	}
	return &got
}

func TestNoArgumentsOnATerminalOpensTheInterface(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	got := stubTerminal(t, true, nil)
	code, out, errOut := run()
	if code != ExitOK || out != "" || errOut != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if len(*got) != 1 || (*got)[0].AppName != "logagent" || !(*got)[0].ColorEnabled || (*got)[0].Check == nil {
		t.Errorf("interface config = %+v", *got)
	}
}

func TestNoColorTurnsTheInterfaceColourOff(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	got := stubTerminal(t, true, nil)
	run()
	if len(*got) != 1 || (*got)[0].ColorEnabled {
		t.Errorf("config = %+v", *got)
	}
}

func TestNoArgumentsWithoutATerminalIsStillAUsageError(t *testing.T) {
	got := stubTerminal(t, false, nil)
	code, _, errOut := run()
	if code != ExitUsage || errOut == "" || len(*got) != 0 {
		t.Errorf("exit %d, stderr %q, interface runs %d", code, errOut, len(*got))
	}
}

func TestArgumentsNeverOpenTheInterface(t *testing.T) {
	got := stubTerminal(t, true, nil)
	run("--version")
	run("nosuchverb")
	if len(*got) != 0 {
		t.Errorf("interface opened %d times", len(*got))
	}
}

func TestAnInterfaceFailureIsClassified(t *testing.T) {
	stubTerminal(t, true, func(tui.Config) error { return errors.New("no tty") })
	code, _, errOut := run()
	if code != 70 || !strings.Contains(errOut, "no tty") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

// The check the interface runs is the command's own, using the host
// configuration found the usual way.
func TestTheInterfaceCheckUsesTheHostConfiguration(t *testing.T) {
	got := stubTerminal(t, true, nil)
	run()
	check := (*got)[0].Check

	cfg, _ := setup(t, goodDump, "")
	t.Setenv(config.EnvVar, cfg)
	text, err := check()
	if err != nil || !strings.Contains(text, "No gaps found") {
		t.Errorf("clean host: %v\n%s", err, text)
	}
	cfg, _ = setup(t, gapDump, "")
	t.Setenv(config.EnvVar, cfg)
	text, err = check()
	if err != nil || !strings.Contains(text, "[gap]") {
		t.Errorf("host with gaps: %v\n%s", err, text)
	}
	t.Setenv(config.EnvVar, "/nonexistent/logagent.yaml")
	if _, err := check(); !errors.Is(err, config.ErrNoConfig) {
		t.Errorf("no configuration: error = %v", err)
	}
}
