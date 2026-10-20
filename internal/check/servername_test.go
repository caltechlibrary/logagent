package check

import (
	"strings"
	"testing"
)

// serverNameHost is a host whose server block holds the given text.
func serverNameHost(body string) string {
	return "error_log /var/log/nginx/error.log warn;\nhttp {\n" + full + "\naccess_log " + logPath + " full;\n" +
		"server {\n" + body + "\n}\n}\n"
}

// A server_name with no semicolon swallows the next directive's words as extra
// server names; nginx -t passes with a warning, and the directive never applies.
// This is the CaltechDATA new-data defect of 2026-10-08.
func TestAServerNameThatSwallowsTheNextDirectiveIsAGap(t *testing.T) {
	body := "  server_name data.example.org\n  add_header Strict-Transport-Security \"max-age=15768000\";\n  listen 443;"
	r := run(t, cfg("none", nil), serverNameHost(body))
	got := find(r, "server-name-swallows-directive")
	if len(got) != 1 || got[0].Severity != Gap {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if !strings.Contains(got[0].Message, "add_header") || !strings.Contains(got[0].Suggestion, "semicolon") {
		t.Errorf("message %q suggestion %q", got[0].Message, got[0].Suggestion)
	}
	if got[0].At.Line == 0 {
		t.Errorf("no line: %+v", got[0].At)
	}
	if r.ExitCode() != 1 {
		t.Errorf("ExitCode = %d, want 1", r.ExitCode())
	}
}

// With no semicolon before a block directive, the parser reads server_name as
// a directive that has a block, which is never right.
func TestAServerNameWithABlockIsAGap(t *testing.T) {
	// The block's directive is named location (a listed word) in one case and a
	// module's own directive, unknown to the table, in the other.
	for _, opener := range []string{"location /", "some_module_zone z"} {
		body := "  server_name data.example.org\n  " + opener + " {\n    return 204;\n  }"
		r := run(t, cfg("none", nil), serverNameHost(body))
		if got := find(r, "server-name-swallows-directive"); len(got) != 1 || got[0].Severity != Gap {
			t.Errorf("%q: findings = %+v", opener, r.Findings)
		}
	}
}

// The message names the first directive swallowed, the one the fault starts at.
func TestTheMessageNamesTheFirstSwallowedDirective(t *testing.T) {
	body := "  server_name a.example.org\n  root /srv\n  index index.html;"
	r := run(t, cfg("none", nil), serverNameHost(body))
	got := find(r, "server-name-swallows-directive")
	if len(got) != 1 || !strings.Contains(got[0].Message, "(root)") {
		t.Fatalf("findings = %+v", r.Findings)
	}
}

// Real server names are never reported: several names, the catch-all, wildcards
// and a regular expression, over one line or several.
func TestRealServerNamesAreNotFlagged(t *testing.T) {
	for _, names := range []string{
		"a.example.org b.example.org",
		"_",
		"*.example.org .example.net",
		`~^www\d+\.example\.org$`,
		"a.example.org\n    b.example.org\n    c.example.org",
		"return.example.org root.example.org",
	} {
		r := run(t, cfg("none", nil), serverNameHost("  server_name "+names+";\n  listen 443;"))
		if got := find(r, "server-name-swallows-directive"); len(got) != 0 {
			t.Errorf("%q: findings = %+v", names, got)
		}
	}
}

// One finding for each server_name that swallows, not one for each swallowed word.
func TestEachSwallowingServerNameIsReportedOnce(t *testing.T) {
	body := "  server_name a.example.org\n  root /srv\n  index index.html;\n  server_name b.example.org\n  add_header X-B \"2\";"
	r := run(t, cfg("none", nil), serverNameHost(body))
	if got := find(r, "server-name-swallows-directive"); len(got) != 2 {
		t.Fatalf("findings = %+v", r.Findings)
	}
}
