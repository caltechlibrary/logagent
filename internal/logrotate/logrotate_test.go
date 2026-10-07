package logrotate

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// ubuntuNginx is the stanza the Ubuntu nginx package installs.
const ubuntuNginx = `/var/log/nginx/*.log {
	daily
	missingok
	rotate 14
	compress
	delaycompress
	notifempty
	create 0640 www-data adm
	sharedscripts
	prerotate
		if [ -d /etc/logrotate.d/httpd-prerotate ]; then \
			run-parts /etc/logrotate.d/httpd-prerotate; \
		fi \
	endscript
	postrotate
		invoke-rc.d nginx rotate >/dev/null 2>&1
	endscript
}
`

func mustParse(t *testing.T, text string) *File {
	t.Helper()
	f, err := Parse("/etc/logrotate.d/nginx", text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return f
}

func TestParseTheStanzaTheNginxPackageInstalls(t *testing.T) {
	f := mustParse(t, ubuntuNginx)
	if len(f.Stanzas) != 1 {
		t.Fatalf("%d stanzas", len(f.Stanzas))
	}
	s := f.Stanzas[0]
	if strings.Join(s.Paths, "|") != "/var/log/nginx/*.log" || s.File != "/etc/logrotate.d/nginx" || s.Line != 1 {
		t.Errorf("stanza = %+v", s)
	}
	if s.Options.Interval != "daily" || !s.Options.RotateSet || s.Options.Rotate != 14 {
		t.Errorf("options = %+v", s.Options)
	}
	if f.Global.IntervalSet() || f.Global.RotateSet {
		t.Errorf("global = %+v", f.Global)
	}
}

func TestParseGlobalOptionsIncludesAndSeveralStanzas(t *testing.T) {
	f := mustParse(t, `# see "man logrotate" for details
weekly
rotate 4
create

include /etc/logrotate.d

/var/log/wtmp {
    missingok
    monthly
    rotate 1
}

/var/log/btmp /var/log/other.log
{
    rotate 2
}
`)
	if f.Global.Interval != "weekly" || f.Global.Rotate != 4 || !f.Global.RotateSet {
		t.Errorf("global = %+v", f.Global)
	}
	if strings.Join(f.Includes, "|") != "/etc/logrotate.d" {
		t.Errorf("includes = %v", f.Includes)
	}
	if len(f.Stanzas) != 2 || f.Stanzas[0].Options.Interval != "monthly" || f.Stanzas[0].Options.Rotate != 1 {
		t.Fatalf("stanzas = %+v", f.Stanzas)
	}
	if strings.Join(f.Stanzas[1].Paths, "|") != "/var/log/btmp|/var/log/other.log" || f.Stanzas[1].Options.Rotate != 2 || f.Stanzas[1].Line != 14 {
		t.Errorf("second stanza = %+v", f.Stanzas[1])
	}
}

func TestParseQuotedPathsAndPathsOnSeveralLines(t *testing.T) {
	f := mustParse(t, "\"/var/log/my app/*.log\" '/var/log/x y.log'\n/var/log/z.log {\n daily\n}\n")
	if got := strings.Join(f.Stanzas[0].Paths, "|"); got != "/var/log/my app/*.log|/var/log/x y.log|/var/log/z.log" {
		t.Errorf("paths = %q", got)
	}
}

func TestScriptsAreSkippedWhateverTheyContain(t *testing.T) {
	f := mustParse(t, "/var/log/a.log {\n daily\n rotate 5\n postrotate\n  echo rotate 99 }\n  weekly\n }\n endscript\n}\n")
	if len(f.Stanzas) != 1 || f.Stanzas[0].Options.Rotate != 5 || f.Stanzas[0].Options.Interval != "daily" {
		t.Errorf("stanzas = %+v", f.Stanzas)
	}
}

func TestUnknownDirectivesAreIgnored(t *testing.T) {
	f := mustParse(t, "/var/log/a.log {\n daily\n rotate 5\n somefutureoption yes\n}\n")
	if f.Stanzas[0].Options.Rotate != 5 {
		t.Errorf("stanza = %+v", f.Stanzas[0])
	}
}

func TestParseErrorsNameTheFileAndLine(t *testing.T) {
	cases := []struct {
		name, text, mention string
		line                int
	}{
		{"unterminated stanza", "/var/log/a.log {\n daily\n", "never closed", 1},
		{"stray brace", "daily\n}\n", "}", 2},
		{"rotate not a number", "/var/log/a.log {\n rotate lots\n}\n", "rotate", 2},
		{"maxage not a number", "/var/log/a.log {\n maxage soon\n}\n", "maxage", 2},
		{"unterminated script", "/var/log/a.log {\n postrotate\n echo hi\n}\n", "endscript", 2},
		{"brace in a path position", "{ daily }\n", "{", 1},
	}
	for _, c := range cases {
		_, err := Parse("/etc/logrotate.d/x", c.text)
		var se *SyntaxError
		if !errors.Is(err, ErrSyntax) || !errors.As(err, &se) {
			t.Errorf("%s: error = %v", c.name, err)
			continue
		}
		if se.Line != c.line || se.File != "/etc/logrotate.d/x" || !strings.Contains(err.Error(), c.mention) || !strings.Contains(err.Error(), "/etc/logrotate.d/x:") {
			t.Errorf("%s: %v (line %d)", c.name, err, se.Line)
		}
	}
}

func TestForFindsTheStanzaThatGovernsALog(t *testing.T) {
	f := mustParse(t, "/var/log/nginx/*.log {\n daily\n}\n/var/log/nginx/access.log {\n weekly\n}\n/var/log/other/a.log {\n monthly\n}\n")
	cases := map[string]string{
		"/var/log/nginx/access.log": "daily", // the first stanza that matches wins
		"/var/log/nginx/error.log":  "daily",
		"/var/log/other/a.log":      "monthly",
	}
	for path, want := range cases {
		s, ok := f.For(path)
		if !ok || s.Options.Interval != want {
			t.Errorf("For(%s) = %+v, %v; want %s", path, s, ok, want)
		}
	}
	for _, path := range []string{"/var/log/nginx/sub/access.log", "/var/log/nginx/access.log.1", "/var/log/mysql/a.log", ""} {
		if _, ok := f.For(path); ok {
			t.Errorf("For(%s) matched", path)
		}
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.001 }

func TestRetention(t *testing.T) {
	cases := []struct {
		name, text    string
		low, high     float64
		unlimited     bool
		indeterminate string
	}{
		{"daily 14", "daily\nrotate 14", 14, 15, false, ""},
		{"weekly 4", "weekly\nrotate 4", 28, 35, false, ""},
		{"monthly 6", "monthly\nrotate 6", 168, 217, false, ""},
		{"yearly 1", "yearly\nrotate 1", 365, 730, false, ""},
		{"hourly 48", "hourly\nrotate 48", 2, 49.0 / 24, false, ""},
		{"rotate 0 keeps only the live file", "daily\nrotate 0", 0, 1, false, ""},
		{"no rotate count means none are kept", "daily", 0, 1, false, ""},
		{"maxage shortens a long rotation", "daily\nrotate 14\nmaxage 7", 7, 8, false, ""},
		{"maxage longer than the rotation changes nothing", "daily\nrotate 14\nmaxage 90", 14, 15, false, ""},
		{"rotate -1 with maxage", "daily\nrotate -1\nmaxage 30", 30, 31, false, ""},
		{"rotate -1 with no limit", "daily\nrotate -1", 0, 0, true, ""},
		{"size alone has no time", "size 100M\nrotate 14", 0, 0, false, "size"},
		{"no interval", "rotate 14", 0, 0, false, "interval"},
	}
	for _, c := range cases {
		f := mustParse(t, "/var/log/a.log {\n"+c.text+"\n}\n")
		r := f.Stanzas[0].Retention(f.Global)
		if c.indeterminate != "" {
			if !strings.Contains(r.Indeterminate, c.indeterminate) {
				t.Errorf("%s: Indeterminate = %q, want it to mention %q", c.name, r.Indeterminate, c.indeterminate)
			}
			continue
		}
		if r.Indeterminate != "" || r.Unlimited != c.unlimited || (!c.unlimited && (!near(r.LowDays, c.low) || !near(r.HighDays, c.high))) {
			t.Errorf("%s: %+v, want low %v high %v unlimited %v", c.name, r, c.low, c.high, c.unlimited)
		}
	}
}

func TestAStanzaFallsBackOnTheGlobalOptionsAndOverridesThem(t *testing.T) {
	f := mustParse(t, "weekly\nrotate 4\n/var/log/a.log {\n daily\n}\n/var/log/b.log {\n rotate 2\n}\n/var/log/c.log {\n}\n")
	a := f.Stanzas[0].Retention(f.Global)
	if !near(a.LowDays, 4) || a.Interval != "daily" || a.Rotate != 4 {
		t.Errorf("a = %+v, want daily from the stanza and rotate 4 from the globals", a)
	}
	b := f.Stanzas[1].Retention(f.Global)
	if !near(b.LowDays, 14) || b.Interval != "weekly" || b.Rotate != 2 {
		t.Errorf("b = %+v", b)
	}
	c := f.Stanzas[2].Retention(f.Global)
	if !near(c.LowDays, 28) {
		t.Errorf("c = %+v", c)
	}
	// The stanza's own options win over a second file's globals, which are only defaults.
	other := mustParse(t, "monthly\nrotate 12\n")
	if r := f.Stanzas[0].Retention(f.Global, other.Global); r.Interval != "daily" || r.Rotate != 4 {
		t.Errorf("with a second defaults file: %+v", r)
	}
	if r := (&Stanza{}).Retention(Options{}, other.Global); r.Interval != "monthly" || r.Rotate != 12 {
		t.Errorf("only the second defaults file sets it: %+v", r)
	}
}

func TestMaxSizeIsReportedBecauseItCanRotateEarlier(t *testing.T) {
	f := mustParse(t, "/var/log/a.log {\n daily\n rotate 14\n maxsize 100M\n}\n")
	r := f.Stanzas[0].Retention(f.Global)
	if r.MaxSize != "100M" || !near(r.LowDays, 14) {
		t.Errorf("r = %+v", r)
	}
	if r := mustParse(t, "/var/log/a.log {\n daily\n rotate 14\n}\n").Stanzas[0].Retention(Options{}); r.MaxSize != "" {
		t.Errorf("MaxSize = %q", r.MaxSize)
	}
}
