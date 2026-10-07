package logagent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"

	"github.com/caltechlibrary/logagent/internal/check"
	"github.com/caltechlibrary/logagent/internal/tui"
)

// isTerminal reports whether both standard input and output are terminals,
// which is when running with no arguments opens the interface. It is a
// variable so tests can say yes or no.
var isTerminal = func() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

// runTUI runs the interface; a variable so tests do not need a terminal.
var runTUI = func(cfg tui.Config) error { return tui.Run(context.Background(), cfg) }

// runInterface opens the interface for a person at a terminal. The check it
// offers is the command's own, with the host configuration found the usual way.
func runInterface(appName string, stderr io.Writer) int {
	err := runTUI(tui.Config{
		AppName:      appName,
		ColorEnabled: os.Getenv("NO_COLOR") == "",
		Check: func() (string, error) {
			report, err := buildReport(newCheckOptions())
			if err != nil {
				return "", err
			}
			var b bytes.Buffer
			if err := check.WriteText(&b, report); err != nil {
				return "", err
			}
			return b.String(), nil
		},
	})
	if err != nil {
		_, code := classify(err)
		fmt.Fprintf(stderr, "%s: %s\n", appName, err)
		return code
	}
	return ExitOK
}
