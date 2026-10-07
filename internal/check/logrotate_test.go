package check

import (
	"strings"
	"testing"

	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/logrotate"
)

// rotation parses logrotate files; the first governs the logs, the rest only
// supply defaults.
func rotation(t *testing.T, texts ...string) []*logrotate.File {
	t.Helper()
	var out []*logrotate.File
	for i, text := range texts {
		path := "/etc/logrotate.d/nginx"
		if i > 0 {
			path = "/etc/logrotate.conf"
		}
		f, err := logrotate.Parse(path, text)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, f)
	}
	return out
}

func stanza(opts string) string { return "/var/log/nginx/*.log {\n" + opts + "\n}\n" }

func rotated(t *testing.T, c *config.Config, files []*logrotate.File) *Report {
	t.Helper()
	return runWith(t, c, host(full, ""), func(in *Input) { in.Logrotate = files })
}

func TestRetentionWithinPolicyIsClean(t *testing.T) {
	for _, opts := range []string{"daily\nrotate 14", "daily\nrotate 30", "weekly\nrotate 2", "daily\nrotate 89"} {
		r := rotated(t, cfg("none", nil), rotation(t, stanza(opts)))
		if len(r.Findings) != 0 {
			t.Errorf("%q: findings = %+v", opts, r.Findings)
		}
	}
}

func TestRetentionBelowTheFloorIsAGap(t *testing.T) {
	r := rotated(t, cfg("none", nil), rotation(t, stanza("daily\nrotate 7")))
	got := find(r, "logrotate-retention-short")
	if len(got) != 1 || got[0].Severity != Gap || got[0].Log != logPath {
		t.Fatalf("findings = %+v", r.Findings)
	}
	for _, want := range []string{"7 days", "14", logPath} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("message lacks %q: %s", want, got[0].Message)
		}
	}
	if got[0].At.File != "/etc/logrotate.d/nginx" || got[0].At.Line != 1 {
		t.Errorf("At = %+v, want the stanza", got[0].At)
	}
	if !strings.Contains(got[0].Suggestion, "rotate 14") {
		t.Errorf("suggestion = %s", got[0].Suggestion)
	}
	if r.ExitCode() != 1 {
		t.Errorf("ExitCode = %d", r.ExitCode())
	}
}

func TestRetentionAboveTheCeilingIsAGap(t *testing.T) {
	r := rotated(t, cfg("none", nil), rotation(t, stanza("daily\nrotate 120")))
	got := find(r, "logrotate-retention-long")
	if len(got) != 1 || got[0].Severity != Gap || !strings.Contains(got[0].Message, "90") || !strings.Contains(got[0].Message, "121") {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if !strings.Contains(got[0].Suggestion, "rotate 89") && !strings.Contains(got[0].Suggestion, "maxage") {
		t.Errorf("suggestion = %s", got[0].Suggestion)
	}
	// A log that is never removed is over any ceiling.
	r = rotated(t, cfg("none", nil), rotation(t, stanza("daily\nrotate -1")))
	if got := find(r, "logrotate-retention-long"); len(got) != 1 || !strings.Contains(got[0].Message, "no limit") {
		t.Errorf("rotate -1: %+v", r.Findings)
	}
	// With maxage it has one.
	r = rotated(t, cfg("none", nil), rotation(t, stanza("daily\nrotate -1\nmaxage 30")))
	if len(r.Findings) != 0 {
		t.Errorf("rotate -1 with maxage 30: %+v", r.Findings)
	}
}

func TestThePolicyComesFromTheConfiguration(t *testing.T) {
	c := cfg("none", nil)
	c.Retention.Layer1MinDays, c.Retention.Layer1MaxDays = 30, 60
	for _, tc := range []struct{ opts, code string }{
		{"daily\nrotate 14", "logrotate-retention-short"},
		{"daily\nrotate 30", ""},
		{"daily\nrotate 59", ""},
		{"daily\nrotate 60", "logrotate-retention-long"}, // 61 days at its fullest
	} {
		r := rotated(t, c, rotation(t, stanza(tc.opts)))
		if tc.code == "" && len(r.Findings) != 0 || tc.code != "" && len(find(r, tc.code)) != 1 {
			t.Errorf("%q: findings = %+v", tc.opts, r.Findings)
		}
	}
}

func TestARotationThatCannotBeCountedInDaysIsAWarning(t *testing.T) {
	r := rotated(t, cfg("none", nil), rotation(t, stanza("size 100M\nrotate 14")))
	got := find(r, "logrotate-indeterminate")
	if len(got) != 1 || got[0].Severity != Warn || !strings.Contains(got[0].Message, "size") {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if r.ExitCode() != 0 || len(find(r, "logrotate-retention-short")) != 0 {
		t.Errorf("exit %d, findings %+v", r.ExitCode(), r.Findings)
	}
}

func TestMaxSizeIsANoteBecauseItCanRotateEarly(t *testing.T) {
	r := rotated(t, cfg("none", nil), rotation(t, stanza("daily\nrotate 14\nmaxsize 100M")))
	got := find(r, "logrotate-maxsize")
	if len(got) != 1 || got[0].Severity != Note || !strings.Contains(got[0].Message, "100M") {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if r.ExitCode() != 0 {
		t.Errorf("ExitCode = %d", r.ExitCode())
	}
}

func TestALogNoStanzaGovernsIsAWarning(t *testing.T) {
	r := rotated(t, cfg("none", nil), rotation(t, "/var/log/other/*.log {\n daily\n rotate 14\n}\n"))
	got := find(r, "logrotate-no-match")
	if len(got) != 1 || got[0].Severity != Warn || got[0].Log != logPath || !strings.Contains(got[0].Message, "/etc/logrotate.d/nginx") {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestDefaultsComeFromTheSecondFile(t *testing.T) {
	files := rotation(t, stanza("daily"), "weekly\nrotate 14\n")
	r := rotated(t, cfg("none", nil), files)
	if len(r.Findings) != 0 {
		t.Errorf("findings = %+v", r.Findings)
	}
	files = rotation(t, stanza("daily"), "weekly\nrotate 3\n")
	if r := rotated(t, cfg("none", nil), files); len(find(r, "logrotate-retention-short")) != 1 {
		t.Errorf("rotate 3 from the defaults file: %+v", r.Findings)
	}
}

func TestLogsSharingAStanzaAreOneFinding(t *testing.T) {
	c := cfg("none", nil, logPath, "/var/log/nginx/error.log")
	r := rotated(t, c, rotation(t, stanza("daily\nrotate 7")))
	got := find(r, "logrotate-retention-short")
	if len(got) != 1 || !strings.Contains(got[0].Message, logPath) || !strings.Contains(got[0].Message, "/var/log/nginx/error.log") {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestEachLogIsJudgedByItsOwnStanza(t *testing.T) {
	c := cfg("none", nil, logPath, "/var/log/other/a.log")
	files := rotation(t, "/var/log/nginx/*.log {\n daily\n rotate 14\n}\n/var/log/other/*.log {\n daily\n rotate 3\n}\n")
	r := rotated(t, c, files)
	got := find(r, "logrotate-retention-short")
	if len(got) != 1 || got[0].Log != "/var/log/other/a.log" {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestNoLogrotateFileMeansNoCheck(t *testing.T) {
	r := rotated(t, cfg("none", nil), nil)
	for _, f := range r.Findings {
		if strings.HasPrefix(f.Code, "logrotate") {
			t.Errorf("unexpected %+v", f)
		}
	}
}

func TestAnUnreadableLogrotateFileIsAWarning(t *testing.T) {
	r := runWith(t, cfg("none", nil), host(full, ""), func(in *Input) { in.LogrotateErr = errString("/etc/logrotate.d/nginx:3: rotate needs a number") })
	got := find(r, "logrotate-unreadable")
	if len(got) != 1 || got[0].Severity != Warn || !strings.Contains(got[0].Message, "rotate needs a number") {
		t.Errorf("findings = %+v", r.Findings)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
