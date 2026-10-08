package logagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/help"
)

// reportFormat is the 19-field format both Caltech hosts write, as nginx -T prints it.
const reportFormat = `$remote_addr - $remote_user [$time_local] "$request" ` +
	`$status $body_bytes_sent "$http_referer" "$http_user_agent" ` +
	`rt=$request_time urt="$upstream_response_time" ` +
	`cf_ray="$http_cf_ray" cf_country="$http_cf_ipcountry" ` +
	`peer=$realip_remote_addr ` +
	`lang="$http_accept_language" ch_ua="$http_sec_ch_ua" ` +
	`ch_plat="$http_sec_ch_ua_platform" ` +
	`bot_score="$http_cf_bot_score" ja3="$http_cf_ja3_hash" ja4="$http_cf_ja4" cache="$upstream_cache_status"`

func reportLine(addr, when, req string, status int, ua, rt, urt string) string {
	return fmt.Sprintf(`%s - - [%s] "%s" %d 100 "https://example.org/secret-ref" "%s" rt=%s urt="%s" cf_ray="9ab" cf_country="US" peer=198.41.128.1 lang="en-US" ch_ua="-" ch_plat="-" bot_score="-" ja3="-" ja4="-" cache="-"`,
		addr, when, req, status, ua, rt, urt)
}

// reportLines are three requests: two on 2026-10-07 and one on 2026-10-08.
func reportLines() []string {
	return []string{
		reportLine("203.0.113.5", "07/Oct/2026:01:00:00 +0000", "GET /search?q=private HTTP/1.1", 200, "Mozilla/5.0 (Windows NT 10.0) Chrome/148", "0.100", "0.200"),
		reportLine("203.0.113.6", "07/Oct/2026:13:00:00 +0000", "GET /api/records/abc12-34 HTTP/1.1", 200, "ExaSearchBot/1.0", "0.300", "0.250"),
		reportLine("203.0.113.6", "08/Oct/2026:09:00:00 +0000", "GET /api/records/abc12-34/versions HTTP/1.1", 429, "ExaSearchBot/1.0", "0.000", "-"),
	}
}

// reportSetup writes a log, an nginx -T dump that names it, and a configuration
// with path classes, and returns the configuration's path and the log's.
func reportSetup(t *testing.T, lines []string, dumpFormat, extraYAML string) (cfgPath, logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "access.log")
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if dumpFormat == "" {
		dumpFormat = "log_format real '" + reportFormat + "';\naccess_log " + logPath + " real;"
	}
	dumpFormat = strings.ReplaceAll(dumpFormat, "{LOG}", logPath)
	dumpPath := filepath.Join(dir, "nginx-T.txt")
	if err := os.WriteFile(dumpPath, []byte("http {\n"+dumpFormat+"\nserver { server_name a.example; }\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	yaml := "version: 1\nserver: nginx\nconfig:\n  dump: " + dumpPath + "\nlogs:\n  - path: " + logPath + "\n" +
		"classes:\n  rules:\n    - {name: api-record, pattern: '/api/records/[^/]+$'}\n    - {name: api-versions, pattern: '/api/records/[^/]+/versions'}\n    - {name: ui-search, prefix: /search}\n" + extraYAML
	cfgPath = filepath.Join(dir, "logagent.yaml")
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath, logPath
}

// at fixes the clock the command uses, to noon on 2026-10-08 UTC.
func at(t *testing.T) {
	t.Helper()
	saved := reportNow
	reportNow = func() time.Time { return time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { reportNow = saved })
}

type reportJSON struct {
	Lines    int `json:"lines"`
	Events   int `json:"events"`
	Skipped  int `json:"skipped"`
	Outside  int `json:"outside"`
	Internal []struct {
		Range    string `json:"range"`
		Requests int    `json:"requests"`
	} `json:"internal"`
	Days []struct {
		Day     string `json:"day"`
		All     int    `json:"all"`
		Match   int    `json:"match"`
		Clients int    `json:"clients"`
	} `json:"days"`
	Notes []string `json:"notes"`
}

func jsonReport(t *testing.T, args ...string) reportJSON {
	t.Helper()
	code, out, errOut := run(append([]string{"report", "--json"}, args...)...)
	if code != ExitOK || errOut != "" {
		t.Fatalf("report %v: exit %d, stderr %q\n%s", args, code, errOut, out)
	}
	var r reportJSON
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	return r
}

func TestReportPrintsTheTenSectionsAndNothingAPatronDid(t *testing.T) {
	at(t)
	cfg, logPath := reportSetup(t, reportLines(), "", "")
	code, out, errOut := run("report", "--config", cfg, "--day", "2026-10-07")
	if code != ExitOK || errOut != "" {
		t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
	}
	for _, want := range []string{"1. SOURCES", "2. REQUESTS PER DAY", "3. STATUS", "4. BY PATH CLASS", "5. BY FAMILY",
		"6. UNDECLARED COHORTS", "7. CONCURRENCY", "8. CACHE", "9. THE 429s", "10. SELF-DESCRIBED AGENTS", logPath} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	for _, leak := range []string{"203.0.113", "private", "abc12-34", "secret-ref", "example.org", "ExaSearchBot", "Mozilla"} {
		if strings.Contains(out, leak) {
			t.Errorf("the report contains %q", leak)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("output contains terminal escape codes")
	}
}

func TestReportWindowOptions(t *testing.T) {
	at(t)
	cfg, _ := reportSetup(t, reportLines(), "", "")
	for name, c := range map[string]struct {
		args   []string
		events int
	}{
		"default is yesterday":     {nil, 2},
		"--day":                    {[]string{"--day", "2026-10-08"}, 1},
		"--last 24h":               {[]string{"--last", "24h"}, 2},
		"--last 2d":                {[]string{"--last", "2d"}, 3},
		"--since and --until":      {[]string{"--since", "2026-10-07T12:00:00Z", "--until", "2026-10-08T00:00:00Z"}, 1},
		"dates in --since/--until": {[]string{"--since", "2026-10-07", "--until", "2026-10-08"}, 2},
		"--since alone":            {[]string{"--since", "2026-10-07T12:00:00Z"}, 2},
	} {
		r := jsonReport(t, append([]string{"--config", cfg}, c.args...)...)
		if r.Events != c.events {
			t.Errorf("%s: %d events, want %d", name, r.Events, c.events)
		}
	}
	r := jsonReport(t, "--config", cfg, "--day", "2026-10-07")
	if r.Outside != 1 {
		t.Errorf("Outside = %d, want 1 (the 10-08 request)", r.Outside)
	}
}

func TestReportFamilyFilterAndTop(t *testing.T) {
	at(t)
	cfg, _ := reportSetup(t, reportLines(), "", "")
	r := jsonReport(t, "--config", cfg, "--day", "2026-10-07", "--family", "exa")
	if len(r.Days) != 1 || r.Days[0].All != 2 || r.Days[0].Match != 1 || r.Days[0].Clients != 1 {
		t.Errorf("Days = %+v", r.Days)
	}
	for _, flag := range []string{"--top", "-t"} {
		code, out, _ := run("report", "--config", cfg, "--day", "2026-10-07", flag, "1")
		if code != ExitOK || !strings.Contains(out, "1 more") {
			t.Errorf("%s 1: exit %d, no \"1 more\":\n%s", flag, code, out)
		}
	}
}

func TestReportSaysWhenNoInternalRangesAreConfiguredAndCountsThemWhenTheyAre(t *testing.T) {
	at(t)
	cfg, _ := reportSetup(t, reportLines(), "", "")
	r := jsonReport(t, "--config", cfg, "--day", "2026-10-07")
	if !strings.Contains(strings.Join(r.Notes, "\n"), "no internal ranges") {
		t.Errorf("Notes = %q", r.Notes)
	}
	cfg, _ = reportSetup(t, reportLines(), "", "internal_ranges:\n  - \"203.0.113.0/24\"\n")
	r = jsonReport(t, "--config", cfg, "--day", "2026-10-07")
	if r.Events != 0 || len(r.Internal) != 1 || r.Internal[0].Requests != 2 {
		t.Errorf("Events %d Internal %+v; every request was from the internal range", r.Events, r.Internal)
	}
}

func TestReportExitCodes(t *testing.T) {
	at(t)
	cfg, _ := reportSetup(t, reportLines(), "", "")
	dir := t.TempDir()
	badCfg := filepath.Join(dir, "bad.yaml")
	os.WriteFile(badCfg, []byte("version: 1\nserver: nginx\nbogus: 1\nlogs:\n  - path: /x\n"), 0o600)
	absentLog, absentPath := reportSetup(t, reportLines(), "", "")
	os.Remove(absentPath)
	wrongFormat := func() string {
		var lines []string
		for i := 0; i < 30; i++ {
			lines = append(lines, fmt.Sprintf(`203.0.113.7 - - [07/Oct/2026:10:00:00 +0000] "GET /r/%d HTTP/1.1" 200 12 "-" "agent"`, i))
		}
		c, _ := reportSetup(t, lines, "", "")
		return c
	}()
	notConfigured, _ := reportSetup(t, reportLines(), "access_log /var/log/other.log;", "")
	escapeJSON, _ := reportSetup(t, reportLines(), "log_format j escape=json '{\"t\":\"$time_local\"}';\naccess_log {LOG} j;", "")
	savedDefault := config.DefaultPath
	defer func() { config.DefaultPath = savedDefault }()
	config.DefaultPath = filepath.Join(dir, "absent.yaml")
	t.Setenv(config.EnvVar, "")

	cases := []struct {
		name string
		args []string
		code int
	}{
		{"unknown flag", []string{"report", "--no-such-flag"}, 2},
		{"surplus argument", []string{"report", "--config", cfg, "extra"}, 2},
		{"flag missing its value", []string{"report", "--day"}, 2},
		{"bad day", []string{"report", "--config", cfg, "--day", "2026-13-45"}, 2},
		{"bad top", []string{"report", "--config", cfg, "--top", "many"}, 2},
		{"negative top", []string{"report", "--config", cfg, "--top", "-1"}, 2},
		{"bad last", []string{"report", "--config", cfg, "--last", "soon"}, 2},
		{"day with last", []string{"report", "--config", cfg, "--day", "2026-10-07", "--last", "24h"}, 2},
		{"day with since", []string{"report", "--config", cfg, "--day", "2026-10-07", "--since", "2026-10-07"}, 2},
		{"since after until", []string{"report", "--config", cfg, "--since", "2026-10-08", "--until", "2026-10-07"}, 2},
		{"no configuration anywhere", []string{"report"}, 66},
		{"named configuration absent", []string{"report", "--config", filepath.Join(dir, "nope.yaml")}, 66},
		{"the log is not on this machine", []string{"report", "--config", absentLog}, 66},
		{"configuration is wrong", []string{"report", "--config", badCfg}, 78},
		{"log is not in the named format", []string{"report", "--config", wrongFormat, "--day", "2026-10-07"}, 65},
		{"log is not in the nginx configuration", []string{"report", "--config", notConfigured}, 78},
		{"escape=json is unsupported", []string{"report", "--config", escapeJSON}, 1},
	}
	for _, c := range cases {
		code, out, errOut := run(c.args...)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d (stderr %q)", c.name, code, c.code, errOut)
		}
		if c.code >= 2 && out != "" {
			t.Errorf("%s: a failure wrote to stdout: %q", c.name, out)
		}
		if c.code >= 1 && errOut == "" {
			t.Errorf("%s: a failure wrote nothing to stderr", c.name)
		}
	}
}

// --dump and --log choose what the command reads instead of the configuration's choices.
func TestReportDumpAndLogOptionsOverrideTheConfiguration(t *testing.T) {
	at(t)
	cfg, logPath := reportSetup(t, reportLines(), "", "")
	otherDump := filepath.Join(t.TempDir(), "other.txt")
	if err := os.WriteFile(otherDump, []byte("http {\naccess_log /var/log/other.log;\nserver { server_name a.example; }\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("report", "--config", cfg, "--dump", otherDump); code != 78 {
		t.Errorf("--dump with a configuration that does not name the log: exit %d, want 78 (stderr %q)", code, errOut)
	}
	if code, _, errOut := run("report", "--config", cfg, "--log", "/nope/access.log"); code != 78 {
		t.Errorf("--log with a log the configuration does not name: exit %d, want 78 (stderr %q)", code, errOut)
	}
	if code, _, errOut := run("report", "--config", cfg, "--log", logPath, "--day", "2026-10-07"); code != ExitOK {
		t.Errorf("--log naming the configured log: exit %d (stderr %q)", code, errOut)
	}
}

func TestReportErrorsAreJSONOnStderrInJSONMode(t *testing.T) {
	code, out, errOut := run("report", "--json", "--config", filepath.Join(t.TempDir(), "nope.yaml"))
	if code != 66 || out != "" {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
	var e struct {
		Error struct {
			Class string `json:"class"`
			Code  int    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errOut), &e); err != nil || e.Error.Class != "no_input" || e.Error.Code != 66 {
		t.Errorf("stderr %q: %+v, %v", errOut, e, err)
	}
}

func TestReportShortOptionsCluster(t *testing.T) {
	at(t)
	cfg, _ := reportSetup(t, reportLines(), "", "")
	code, out, errOut := run("report", "-jc", cfg)
	if code != ExitOK || errOut != "" || !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("-jc: exit %d, stderr %q\n%s", code, errOut, out)
	}
}

func TestReportHelpAndItsExitStatusSection(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		code, out, _ := run("report", flag)
		if code != ExitOK || !strings.Contains(out, "# EXIT STATUS") {
			t.Errorf("report %s: exit %d\n%s", flag, code, out)
		}
	}
	p, err := help.Lookup("report")
	if err != nil {
		t.Fatal(err)
	}
	section := p.Text[strings.Index(p.Text, "# EXIT STATUS"):]
	for _, code := range []string{"0", "1", "2", "65", "66", "70", "74", "77", "78"} {
		if !strings.Contains(section, "\n"+code+"\n:") {
			t.Errorf("EXIT STATUS lacks code %s", code)
		}
	}
	for _, opt := range []string{"--config", "--dump", "--json", "-j", "--help", "--day", "--last", "--since", "--until", "--family", "--top", "-t", "--log"} {
		if !strings.Contains(p.Text, opt) {
			t.Errorf("page does not describe %s", opt)
		}
	}
	if strings.Contains(p.Text, "planned and is not implemented") {
		t.Error("the page still says the command is not implemented")
	}
}
