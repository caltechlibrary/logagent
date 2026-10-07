package check

import (
	"strings"
	"testing"
)

// nginxConf builds a configuration with the given main-context lines, http-level
// lines and server-block lines. The access log is the clean one the other tests use.
func nginxConf(main, httpLines, server string) string {
	return main + "\nhttp {\n" + full + "\naccess_log " + logPath + " full;\n" + httpLines + "\nserver { server_name a.example;\n" + server + "\n}\n}\n"
}

const zones = "limit_conn_zone $binary_remote_addr zone=api_conc:64k;\nlimit_req_zone $binary_remote_addr zone=req_zone:64k rate=5r/s;\n"

func errorCheck(t *testing.T, text string) *Report {
	t.Helper()
	return run(t, cfg("none", nil), text)
}

// This is CaltechAUTHORS as it was on 2026-10-07: caps logged at warn and an
// error_log at its default level, error.
func TestCapsLoggedBelowTheErrorLogLevelAreReported(t *testing.T) {
	text := nginxConf("error_log /var/log/nginx/error.log;", zones+"limit_conn_log_level warn;", "limit_conn api_conc 24;")
	r := errorCheck(t, text)
	got := find(r, "error-log-level-hides-limits")
	if len(got) != 1 || got[0].Severity != Warn {
		t.Fatalf("findings = %+v", r.Findings)
	}
	for _, want := range []string{"limit_conn", "api_conc", "warn", "error"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("message lacks %q: %s", want, got[0].Message)
		}
	}
	for _, want := range []string{"error_log /var/log/nginx/error.log warn", "limit_conn_log_level error"} {
		if !strings.Contains(got[0].Suggestion, want) {
			t.Errorf("suggestion lacks %q: %s", want, got[0].Suggestion)
		}
	}
	if got[0].At.Line == 0 {
		t.Errorf("At = %+v, want the limit_conn line", got[0].At)
	}
	if r.ExitCode() != 0 {
		t.Errorf("ExitCode = %d; a hidden cap is a warning", r.ExitCode())
	}
}

func TestErrorLogLevelsAgainstTheLevelCapsAreLoggedAt(t *testing.T) {
	cases := []struct {
		name, errorLog, level string // errorLog is the error_log directive; level the limit_conn_log_level
		hidden                bool
	}{
		{"warn log shows warn caps", "error_log /var/log/nginx/error.log warn;", "warn", false},
		{"debug log shows everything", "error_log /var/log/nginx/error.log debug;", "warn", false},
		{"notice log shows warn caps", "error_log /var/log/nginx/error.log notice;", "warn", false},
		{"error log hides warn caps", "error_log /var/log/nginx/error.log error;", "warn", true},
		{"default level is error", "error_log /var/log/nginx/error.log;", "warn", true},
		{"crit log hides warn caps", "error_log /var/log/nginx/error.log crit;", "warn", true},
		{"error log shows error caps", "error_log /var/log/nginx/error.log error;", "error", false},
		{"the default cap level is error", "error_log /var/log/nginx/error.log;", "", false},
		{"crit log hides default caps", "error_log /var/log/nginx/error.log crit;", "", true},
		{"info caps need an info log", "error_log /var/log/nginx/error.log notice;", "info", true},
	}
	for _, c := range cases {
		lines := zones
		if c.level != "" {
			lines += "limit_conn_log_level " + c.level + ";\n"
		}
		r := errorCheck(t, nginxConf(c.errorLog, lines, "limit_conn api_conc 24;"))
		if got := len(find(r, "error-log-level-hides-limits")) == 1; got != c.hidden {
			t.Errorf("%s: hidden = %v, want %v (%+v)", c.name, got, c.hidden, r.Findings)
		}
	}
}

func TestLimitReqHasItsOwnLevel(t *testing.T) {
	r := errorCheck(t, nginxConf("error_log /var/log/nginx/error.log;", zones+"limit_req_log_level warn;", "limit_req zone=req_zone burst=10;"))
	got := find(r, "error-log-level-hides-limits")
	if len(got) != 1 || !strings.Contains(got[0].Message, "limit_req") || !strings.Contains(got[0].Message, "req_zone") {
		t.Errorf("findings = %+v", r.Findings)
	}
	// limit_conn_log_level does not govern limit_req.
	r = errorCheck(t, nginxConf("error_log /var/log/nginx/error.log;", zones+"limit_conn_log_level warn;", "limit_req zone=req_zone burst=10;"))
	if len(find(r, "error-log-level-hides-limits")) != 0 {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestNoCapsMeansNothingToHide(t *testing.T) {
	r := errorCheck(t, nginxConf("error_log /var/log/nginx/error.log crit;", zones+"limit_conn_log_level warn;", ""))
	if len(find(r, "error-log-level-hides-limits")) != 0 {
		t.Errorf("a zone with no limit_conn in use: %+v", r.Findings)
	}
}

func TestSeveralErrorLogsAndTheOneThatDoesNotCount(t *testing.T) {
	two := "error_log /var/log/nginx/error.log error;\nerror_log /var/log/nginx/caps.log warn;"
	if r := errorCheck(t, nginxConf(two, zones+"limit_conn_log_level warn;", "limit_conn api_conc 24;")); len(find(r, "error-log-level-hides-limits")) != 0 {
		t.Errorf("a second log at warn shows the caps: %+v", r.Findings)
	}
	devnull := "error_log /var/log/nginx/error.log error;\nerror_log /dev/null warn;"
	if r := errorCheck(t, nginxConf(devnull, zones+"limit_conn_log_level warn;", "limit_conn api_conc 24;")); len(find(r, "error-log-level-hides-limits")) != 1 {
		t.Errorf("/dev/null records nothing: %+v", r.Findings)
	}
}

// error_log in a server or location replaces the inherited ones, as add_header does.
func TestAnErrorLogInAServerReplacesTheMainOne(t *testing.T) {
	main := "error_log /var/log/nginx/error.log warn;"
	text := nginxConf(main, zones+"limit_conn_log_level warn;", "error_log /var/log/nginx/site.log error;\nlimit_conn api_conc 24;")
	if r := errorCheck(t, text); len(find(r, "error-log-level-hides-limits")) != 1 {
		t.Errorf("the server's error log at error hides the caps: %+v", r.Findings)
	}
	text = nginxConf("error_log /var/log/nginx/error.log;", zones+"limit_conn_log_level warn;", "error_log /var/log/nginx/site.log warn;\nlimit_conn api_conc 24;")
	if r := errorCheck(t, text); len(find(r, "error-log-level-hides-limits")) != 0 {
		t.Errorf("the server's error log at warn shows the caps: %+v", r.Findings)
	}
}

func TestACapInALocationIsJudgedWithTheLevelAtThatLocation(t *testing.T) {
	text := nginxConf("error_log /var/log/nginx/error.log;", zones, "location /api/ {\nlimit_conn api_conc 24;\nlimit_conn_log_level warn;\n}")
	r := errorCheck(t, text)
	got := find(r, "error-log-level-hides-limits")
	if len(got) != 1 || !strings.Contains(got[0].Message, "/api/") {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestTheSameHiddenCapInManyPlacesIsOneFinding(t *testing.T) {
	text := "error_log /var/log/nginx/error.log;\nhttp {\n" + full + "\naccess_log " + logPath + " full;\n" + zones + "limit_conn_log_level warn;\n" +
		"server { server_name a.example;\nlimit_conn api_conc 24; }\nserver { server_name b.example;\nlimit_conn api_conc 24; }\n}\n"
	r := errorCheck(t, text)
	got := find(r, "error-log-level-hides-limits")
	if len(got) != 1 || len(got[0].Also) != 1 || !strings.Contains(got[0].Message, "a.example") || !strings.Contains(got[0].Message, "b.example") {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestAnErrorLogThatIsDevNullIsDisabled(t *testing.T) {
	r := errorCheck(t, nginxConf("error_log /dev/null;", "", ""))
	got := find(r, "error-log-disabled")
	if len(got) != 1 || got[0].Severity != Warn || !strings.Contains(got[0].Message, "a.example") {
		t.Fatalf("findings = %+v", r.Findings)
	}
	// Any real file keeps it enabled; so does stderr or syslog.
	for _, main := range []string{"error_log /dev/null;\nerror_log /var/log/nginx/error.log;", "error_log stderr;", "error_log syslog:server=unix:/dev/log;"} {
		if r := errorCheck(t, nginxConf(main, "", "")); len(find(r, "error-log-disabled")) != 0 || len(find(r, "error-log-default")) != 0 {
			t.Errorf("%q: %+v", main, r.Findings)
		}
	}
}

func TestNoErrorLogAnywhereIsANote(t *testing.T) {
	r := errorCheck(t, nginxConf("", "", ""))
	got := find(r, "error-log-default")
	if len(got) != 1 || got[0].Severity != Note || !strings.Contains(got[0].Message, "a.example") {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestACleanErrorLogSetupHasNoFindings(t *testing.T) {
	r := errorCheck(t, nginxConf("error_log /var/log/nginx/error.log warn;", zones+"limit_conn_log_level warn;", "limit_conn api_conc 24;"))
	if len(r.Findings) != 0 {
		t.Errorf("findings = %+v", r.Findings)
	}
}

// The error log holds client addresses too, so it is held to the same retention.
func TestTheErrorLogIsPartOfTheRetentionCheck(t *testing.T) {
	text := nginxConf("error_log /var/log/nginx/error.log warn;", "", "")
	r := runWith(t, cfg("none", nil), text, func(in *Input) { in.Logrotate = rotation(t, stanza("daily\nrotate 7")) })
	got := find(r, "logrotate-retention-short")
	if len(got) != 1 || !strings.Contains(got[0].Message, logPath) || !strings.Contains(got[0].Message, "/var/log/nginx/error.log") {
		t.Fatalf("findings = %+v", r.Findings)
	}
}

func TestAnErrorLogNoStanzaGovernsIsAWarning(t *testing.T) {
	text := nginxConf("error_log /var/log/other/error.log warn;", "", "")
	r := runWith(t, cfg("none", nil), text, func(in *Input) { in.Logrotate = rotation(t, stanza("daily\nrotate 14")) })
	got := find(r, "logrotate-no-match")
	if len(got) != 1 || got[0].Log != "/var/log/other/error.log" {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestOnlyFileErrorLogsAreCheckedForRetention(t *testing.T) {
	for _, main := range []string{"error_log /dev/null;", "error_log stderr;", "error_log syslog:server=unix:/dev/log;", "error_log memory:32m debug;"} {
		text := nginxConf(main, "", "")
		r := runWith(t, cfg("none", nil), text, func(in *Input) { in.Logrotate = rotation(t, stanza("daily\nrotate 14")) })
		for _, f := range r.Findings {
			if strings.HasPrefix(f.Code, "logrotate") {
				t.Errorf("%q: %+v", main, f)
			}
		}
	}
}

func TestAnErrorLogNamedTwiceIsCheckedOnce(t *testing.T) {
	text := nginxConf("error_log /var/log/nginx/error.log warn;", "error_log /var/log/nginx/error.log warn;", "error_log /var/log/nginx/error.log error;")
	r := runWith(t, cfg("none", nil), text, func(in *Input) { in.Logrotate = rotation(t, stanza("daily\nrotate 7")) })
	got := find(r, "logrotate-retention-short")
	if len(got) != 1 || strings.Count(got[0].Message, "/var/log/nginx/error.log") != 1 {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestSeveralZonesInOneContextAreOneFinding(t *testing.T) {
	text := nginxConf("error_log /var/log/nginx/error.log;", zones+"limit_conn_zone $binary_remote_addr zone=iiif_conc:64k;\nlimit_conn_log_level warn;", "limit_conn api_conc 24;\nlimit_conn iiif_conc 6;")
	r := errorCheck(t, text)
	got := find(r, "error-log-level-hides-limits")
	if len(got) != 1 || len(got[0].Also) != 0 || !strings.Contains(got[0].Message, "api_conc, iiif_conc") {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestAnErrorLogAlsoListedAsAConfiguredLogIsCheckedOnce(t *testing.T) {
	c := cfg("none", nil, logPath, "/var/log/nginx/error.log")
	text := nginxConf("error_log /var/log/nginx/error.log warn;", "", "")
	r := runWith(t, c, text, func(in *Input) { in.Logrotate = rotation(t, stanza("daily\nrotate 7")) })
	got := find(r, "logrotate-retention-short")
	if len(got) != 1 || strings.Count(got[0].Message, "/var/log/nginx/error.log") != 1 {
		t.Errorf("findings = %+v", r.Findings)
	}
}
