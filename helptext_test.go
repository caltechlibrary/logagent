package logagent

import (
	"testing"

	"github.com/caltechlibrary/logagent/internal/help"
)

// helpVars maps every embedded page to the exported constant that holds its
// text, so a page added without its constant fails this test.
var helpVars = map[string]string{
	"logagent.1":         LogagentHelpText,
	"logagent-analyze.1": LogagentAnalyzeHelpText,
	"logagent-check.1":   LogagentCheckHelpText,
	"logagent-report.1":  LogagentReportHelpText,
	"logagent-respond.1": LogagentRespondHelpText,
	"logagent-watch.1":   LogagentWatchHelpText,
	"logagent-config.5":  LogagentConfigHelpText,
	"logagent-jsonl.5":   LogagentJSONLHelpText,
	"logagent-privacy.7": LogagentPrivacyHelpText,
	"logagent-tiers.7":   LogagentTiersHelpText,
}

func TestEveryHelpPageHasAnExportedText(t *testing.T) {
	for _, p := range help.Pages() {
		got, ok := helpVars[p.File()]
		if !ok {
			t.Errorf("%s has no exported help text in helptext.go", p.File())
			continue
		}
		if got != p.Text {
			t.Errorf("%s: exported text differs from the page", p.File())
		}
	}
	if len(helpVars) != len(help.Pages()) {
		t.Errorf("helpVars lists %d pages, %d are embedded", len(helpVars), len(help.Pages()))
	}
}
