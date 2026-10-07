package check

import (
	"bytes"
	"strings"
	"testing"
)

func render(t *testing.T, r *Report) string {
	t.Helper()
	var b bytes.Buffer
	if err := WriteText(&b, r); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestWriteTextCleanReport(t *testing.T) {
	out := render(t, run(t, cfg("none", nil), host(full, "")))
	for _, want := range []string{logPath, "format full", "a.example", "No gaps found"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[gap]") {
		t.Errorf("a clean report shows a gap:\n%s", out)
	}
}

func TestWriteTextListsFindingsBySeverityWithPlaceAndSuggestion(t *testing.T) {
	text := "# configuration file /etc/nginx/nginx.conf:\nhttp {\n    access_log " + logPath + " combined;\n    server { server_name a.example; }\n}\n"
	out := render(t, run(t, cfg("none", nil), text))
	for _, want := range []string{
		"[gap] field-missing rt", "[note] field-missing lang", "/etc/nginx/nginx.conf:2",
		"log_format", "2 gaps", "3 notes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	// Gaps come before warnings before notes.
	if strings.Index(out, "[gap]") > strings.Index(out, "[note]") {
		t.Errorf("a note is listed before a gap:\n%s", out)
	}
	// The suggestion text is indented so it reads as part of its finding.
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "log_format") {
			t.Errorf("suggestion not indented: %q", l)
		}
	}
}

func TestWriteTextIsRepeatableAndHasNoEscapes(t *testing.T) {
	r := run(t, cfg("cloudflare", nil), host("", ""))
	a, b := render(t, r), render(t, r)
	if a != b || strings.Contains(a, "\x1b[") {
		t.Error("output differs between calls or holds escape codes")
	}
}

func TestWriteTextSingularAndPlural(t *testing.T) {
	r := run(t, cfg("none", map[string]string{"urt": "not_applicable", "lang": "not_applicable", "ch_ua": "not_applicable", "ch_plat": "not_applicable"}), host("", ""))
	out := render(t, r)
	if !strings.Contains(out, "1 gap") || strings.Contains(out, "1 gaps") {
		t.Errorf("summary:\n%s", out)
	}
}

// The report lists findings from gaps down, whatever order they were found in.
func TestWriteTextPutsGapsBeforeWarnings(t *testing.T) {
	text := "http {\naccess_log " + logPath + " combined;\nserver { server_name a.example; location /static/ { access_log off; } }\n}\n"
	r := run(t, cfg("none", nil), text)
	if r.Findings[0].Severity != Warn {
		t.Fatalf("fixture: the warning is not found first: %+v", r.Findings[0])
	}
	out := render(t, r)
	if strings.Index(out, "[gap]") > strings.Index(out, "[warn]") {
		t.Errorf("a warning is listed before a gap:\n%s", out)
	}
}

func TestWriteTextShowsTheOtherPlacesOfAMergedFinding(t *testing.T) {
	text := "# configuration file /etc/nginx/sites.conf:\nhttp {\n" + full + "\naccess_log " + logPath + " full;\n" +
		"server { server_name a.example; listen 443; }\nserver { server_name b.example; listen 80; }\n}\n"
	out := render(t, run(t, cfg("cloudflare", nil), text))
	if !strings.Contains(out, "also at /etc/nginx/sites.conf:") || strings.Count(out, "real-ip-missing") != 1 {
		t.Errorf("output:\n%s", out)
	}
}
