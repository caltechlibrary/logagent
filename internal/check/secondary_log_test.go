package check

import (
	"strings"
	"testing"
)

const authLog = "/var/log/nginx/auth-requests.log"

// secondaryHost is a host whose one server writes the full log and a second,
// narrow log (the shape of CaltechAUTHORS's auth-requests.log experiment).
func secondaryHost(narrow string) string {
	return "error_log /var/log/nginx/error.log warn;\nhttp {\n" + full + "\n" +
		"log_format narrow '$remote_addr $status';\n" +
		"server { server_name a.example;\n  access_log " + logPath + " full;\n  access_log " + authLog + " " + narrow + ";\n}\n}\n"
}

// An access_log the host file does not list is not one logagent reads, so its
// narrow format is a note and not a missing-field gap.
func TestASecondaryLogTheConfigDoesNotListIsANoteNotAGap(t *testing.T) {
	r := run(t, cfg("none", nil), secondaryHost("narrow"))
	for _, f := range r.Findings {
		if f.Log == authLog && f.Code == "field-missing" {
			t.Errorf("field-missing for an unlisted log: %+v", f)
		}
	}
	got := find(r, "access-log-not-configured")
	if len(got) != 1 || got[0].Severity != Note || got[0].Log != authLog || got[0].At.Line == 0 {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if !strings.Contains(got[0].Message, authLog) || !strings.Contains(got[0].Suggestion, "logs") {
		t.Errorf("message %q suggestion %q", got[0].Message, got[0].Suggestion)
	}
	if len(r.Logs) != 1 || r.Logs[0].Path != logPath {
		t.Errorf("Logs = %+v, want only %s", r.Logs, logPath)
	}
	if r.ExitCode() != 0 {
		t.Errorf("ExitCode = %d, want 0", r.ExitCode())
	}
}

// Listing the same log makes it logagent's to read, and the rule applies again.
func TestTheSameSecondaryLogListedIsHeldToTheFieldRules(t *testing.T) {
	r := run(t, cfg("none", nil, logPath, authLog), secondaryHost("narrow"))
	if got := find(r, "access-log-not-configured"); len(got) != 0 {
		t.Errorf("a listed log is reported as unlisted: %+v", got)
	}
	var gap bool
	for _, f := range find(r, "field-missing") {
		if f.Log == authLog && f.Severity == Gap {
			gap = true
		}
	}
	if !gap {
		t.Errorf("no field-missing gap for the listed narrow log: %+v", r.Findings)
	}
	if r.ExitCode() != 1 {
		t.Errorf("ExitCode = %d, want 1", r.ExitCode())
	}
}

// What nginx itself would reject is still a gap, listed or not.
func TestAnUnlistedLogWithAnUndefinedFormatIsStillAGap(t *testing.T) {
	r := run(t, cfg("none", nil), secondaryHost("nosuch"))
	if got := find(r, "format-undefined"); len(got) != 1 || got[0].Severity != Gap || got[0].Log != authLog {
		t.Fatalf("findings = %+v", r.Findings)
	}
}
