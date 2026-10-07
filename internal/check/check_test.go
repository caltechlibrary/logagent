package check

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/fields"
	"github.com/caltechlibrary/logagent/internal/nginxconf"
)

const logPath = "/var/log/nginx/access.log"

// cfg is a configuration for an nginx host with one log.
func cfg(behind string, overrides map[string]string, logs ...string) *config.Config {
	if len(logs) == 0 {
		logs = []string{logPath}
	}
	c := &config.Config{Version: 1, Server: "nginx", Proxy: config.Proxy{Behind: behind}, Fields: overrides}
	if behind == "cloudflare" {
		c.Proxy.RealIPHeader = "CF-Connecting-IP"
	}
	for _, l := range logs {
		c.Logs = append(c.Logs, config.Log{Path: l})
	}
	return c
}

// dump parses configuration text, with or without file headers.
func dump(t *testing.T, text string) *nginxconf.Dump {
	t.Helper()
	d, err := nginxconf.Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return d
}

// run checks text against c and fails the test on an error.
func run(t *testing.T, c *config.Config, text string) *Report {
	t.Helper()
	r, err := Run(Input{Config: c, Dump: dump(t, text)})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return r
}

// host is a configuration whose one server logs with the given log_format text
// (empty for the built-in combined format) and has the given extra directives
// at http level.
func host(format, extra string) string {
	use := "combined"
	if format != "" {
		use = "full"
	}
	return "http {\n" + format + "\naccess_log " + logPath + " " + use + ";\n" + extra + "\nserver { server_name a.example; }\n}\n"
}

const realIP = `
set_real_ip_from 173.245.48.0/20;
set_real_ip_from 2400:cb00::/32;
real_ip_header CF-Connecting-IP;
`

// full is the format the field table generates: every field present.
var full = fields.Default().NginxLogFormat("full")

// find returns the findings with the given code.
func find(r *Report, code string) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

// codes lists "code/field" for every finding, to compare as a set.
func codes(r *Report) map[string]bool {
	out := map[string]bool{}
	for _, f := range r.Findings {
		out[f.Code+"/"+f.Field+"/"+f.Severity] = true
	}
	return out
}

func status(l LogReport, name string) string {
	for _, f := range l.Fields {
		if f.Name == name {
			return f.Status
		}
	}
	return "(absent)"
}

func TestAHostThatLogsEverythingIsClean(t *testing.T) {
	r := run(t, cfg("cloudflare", nil), host(full, realIP))
	if len(r.Findings) != 0 {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if r.ExitCode() != 0 {
		t.Errorf("ExitCode = %d, want 0", r.ExitCode())
	}
	if len(r.Logs) != 1 || r.Logs[0].Path != logPath || r.Logs[0].Format != "full" {
		t.Fatalf("logs = %+v", r.Logs)
	}
	for _, f := range r.Logs[0].Fields {
		if f.Status != "ok" {
			t.Errorf("%s: status %s, want ok", f.Name, f.Status)
		}
	}
}

func TestTheStockCombinedFormatLacksTheExtendedFields(t *testing.T) {
	r := run(t, cfg("none", nil), host("", ""))
	want := map[string]bool{
		"field-missing/rt/gap": true, "field-missing/urt/gap": true,
		"field-missing/lang/note": true, "field-missing/ch_ua/note": true, "field-missing/ch_plat/note": true,
	}
	if got := codes(r); !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v\nwant     %v", got, want)
	}
	if r.ExitCode() != 1 {
		t.Errorf("ExitCode = %d, want 1 (a required field is missing)", r.ExitCode())
	}
	l := r.Logs[0]
	if l.Format != "combined" {
		t.Errorf("Format = %q, want combined", l.Format)
	}
	for name, want := range map[string]string{"ua": "ok", "client": "ok", "status": "ok", "rt": "missing", "lang": "missing"} {
		if got := status(l, name); got != want {
			t.Errorf("%s status = %s, want %s", name, got, want)
		}
	}
	// A host with no proxy is not asked for the proxy's fields at all.
	for _, n := range []string{"peer", "cf_ray", "bot_score"} {
		if got := status(l, n); got != "(absent)" {
			t.Errorf("%s status = %s, want it left out", n, got)
		}
	}
}

func TestBehindCloudflareAlsoNeedsThePeerAndCloudflareFields(t *testing.T) {
	r := run(t, cfg("cloudflare", nil), host("", realIP))
	got := codes(r)
	for _, want := range []string{"field-missing/peer/gap", "field-missing/cf_ray/gap", "field-missing/cf_country/gap", "field-missing/bot_score/note", "field-missing/ja4/note"} {
		if !got[want] {
			t.Errorf("no finding %s in %v", want, got)
		}
	}
}

func TestOverridesChangeWhatIsAskedFor(t *testing.T) {
	r := run(t, cfg("none", map[string]string{"urt": "not_applicable", "lang": "required", "ch_ua": "not_applicable"}), host("", ""))
	got := codes(r)
	if got["field-missing/urt/gap"] || got["field-missing/ch_ua/note"] {
		t.Errorf("a not_applicable field was reported: %v", got)
	}
	if !got["field-missing/lang/gap"] {
		t.Errorf("lang was made required but is not a gap: %v", got)
	}
	if st := status(r.Logs[0], "urt"); st != "(absent)" {
		t.Errorf("urt status = %s, want it left out", st)
	}
}

func TestAFieldIsPresentOnlyAsAWholeVariable(t *testing.T) {
	format := `log_format full '$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent" ' 'rt=${request_time} urt="$upstream_response_time_extra"';`
	r := run(t, cfg("none", nil), host(format, ""))
	l := r.Logs[0]
	if got := status(l, "rt"); got != "ok" {
		t.Errorf("rt written as ${request_time}: status %s, want ok", got)
	}
	if got := status(l, "urt"); got != "missing" {
		t.Errorf("urt: $upstream_response_time_extra is another variable, status %s, want missing", got)
	}
}

func TestAMissingFieldComesWithTheFormatToUseAndWhereItGoes(t *testing.T) {
	text := "# configuration file /etc/nginx/nginx.conf:\nhttp {\n    access_log " + logPath + " combined;\n    server { server_name a.example; }\n}\n"
	r := run(t, cfg("none", nil), text)
	l := r.Logs[0]
	if l.At != (Place{File: "/etc/nginx/nginx.conf", Line: 2}) {
		t.Errorf("At = %+v, want the access_log line", l.At)
	}
	for _, want := range []string{"log_format", "$request_time", "$upstream_response_time", "/etc/nginx/nginx.conf:2"} {
		if !strings.Contains(l.Suggested, want) {
			t.Errorf("Suggested lacks %q:\n%s", want, l.Suggested)
		}
	}
	for _, f := range find(r, "field-missing") {
		if f.Suggestion == "" || f.At != l.At {
			t.Errorf("%s: suggestion %q at %+v", f.Field, f.Suggestion, f.At)
		}
	}
	// A clean log has nothing to suggest.
	if clean := run(t, cfg("none", nil), host(full, "")); clean.Logs[0].Suggested != "" {
		t.Errorf("Suggested = %q for a clean log", clean.Logs[0].Suggested)
	}
}

// The 2026-10-06 failure: a log_format in conf.d, included after the
// access_log line that names it, makes nginx -t fail.
func TestAFormatDefinedAfterTheAccessLogThatUsesItIsAGap(t *testing.T) {
	text := `# configuration file /etc/nginx/nginx.conf:
http {
    access_log /var/log/nginx/access.log full;
    include conf.d/*.conf;
    server { server_name a.example; }
}
# configuration file /etc/nginx/conf.d/format.conf:
` + full + "\n"
	r := run(t, cfg("none", nil), text)
	got := find(r, "format-defined-after-use")
	if len(got) != 1 || got[0].Severity != "gap" {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if got[0].At.File != "/etc/nginx/conf.d/format.conf" {
		t.Errorf("At = %+v, want the log_format", got[0].At)
	}
	if !strings.Contains(got[0].Message, "/etc/nginx/nginx.conf:2") || !strings.Contains(got[0].Message, "full") {
		t.Errorf("message %q must name the access_log and the format", got[0].Message)
	}
	if r.ExitCode() != 1 {
		t.Errorf("ExitCode = %d", r.ExitCode())
	}
}

func TestAnAccessLogNamingAnUndefinedFormatIsAGap(t *testing.T) {
	r := run(t, cfg("none", nil), "http {\naccess_log "+logPath+" nosuch;\nserver { server_name a.example; }\n}\n")
	got := find(r, "format-undefined")
	if len(got) != 1 || got[0].Severity != "gap" || !strings.Contains(got[0].Message, "nosuch") {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestAccessLogOff(t *testing.T) {
	// On a server: its traffic is not recorded at all, a gap.
	r := run(t, cfg("none", nil), "http {\naccess_log "+logPath+" combined;\nserver { server_name a.example; access_log off; }\n}\n")
	got := find(r, "access-log-off")
	if len(got) != 1 || got[0].Severity != "gap" || !strings.Contains(got[0].Message, "a.example") {
		t.Errorf("server level: %+v", r.Findings)
	}
	// On one location while the server logs: a warning, the rest is still seen.
	r = run(t, cfg("none", nil), "http {\naccess_log "+logPath+" combined;\nserver { server_name a.example; location /static/ { access_log off; } }\n}\n")
	got = find(r, "access-log-off")
	if len(got) != 1 || got[0].Severity != "warn" || !strings.Contains(got[0].Message, "/static/") {
		t.Errorf("location level: %+v", r.Findings)
	}
}

func TestAConfiguredLogNoServerWritesIsAGap(t *testing.T) {
	r := run(t, cfg("none", nil, logPath, "/var/log/nginx/other.log"), host(full, ""))
	got := find(r, "log-not-found")
	if len(got) != 1 || !strings.Contains(got[0].Message, "/var/log/nginx/other.log") || got[0].Severity != "gap" {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestServersSharingAnAccessLogAreOneLog(t *testing.T) {
	text := "http {\naccess_log " + logPath + " combined;\nserver { server_name a.example; }\nserver { server_name b.example; }\n}\n"
	r := run(t, cfg("none", nil), text)
	if len(r.Logs) != 1 || !reflect.DeepEqual(r.Logs[0].Servers, []string{"a.example", "b.example"}) {
		t.Fatalf("logs = %+v", r.Logs)
	}
	if n := len(find(r, "field-missing")); n != 5 {
		t.Errorf("%d field-missing findings, want 5 (not repeated per server)", n)
	}
	// Different logs per server are checked separately.
	text = "http {\n" + full + "\nserver { server_name a.example; access_log " + logPath + " full; }\nserver { server_name b.example; access_log /var/log/nginx/b.log combined; }\n}\n"
	r = run(t, cfg("none", nil, logPath, "/var/log/nginx/b.log"), text)
	if len(r.Logs) != 2 || len(find(r, "field-missing")) == 0 {
		t.Fatalf("logs = %+v findings = %+v", r.Logs, r.Findings)
	}
	for _, f := range find(r, "field-missing") {
		if f.Log != "/var/log/nginx/b.log" {
			t.Errorf("finding for %s attached to %s", f.Field, f.Log)
		}
	}
}

func TestRealIPBehindCloudflare(t *testing.T) {
	cases := []struct {
		name, extra, code string // code "" means no real-ip finding at all
	}{
		{"complete", realIP, ""},
		{"none configured", "", "real-ip-missing"},
		{"header but nothing trusted", "real_ip_header CF-Connecting-IP;", "real-ip-no-trusted-proxies"},
		{"trusted but no header", "set_real_ip_from 173.245.48.0/20;", "real-ip-missing"},
		{"wrong header", "set_real_ip_from 173.245.48.0/20;\nreal_ip_header X-Forwarded-For;", "real-ip-header-mismatch"},
		{"trusts everyone", "set_real_ip_from 0.0.0.0/0;\nreal_ip_header CF-Connecting-IP;", "real-ip-trust-too-wide"},
		{"trusts everyone over IPv6", "set_real_ip_from ::/0;\nreal_ip_header CF-Connecting-IP;", "real-ip-trust-too-wide"},
	}
	for _, c := range cases {
		r := run(t, cfg("cloudflare", nil), host(full, c.extra))
		var got []Finding
		for _, f := range r.Findings {
			if strings.HasPrefix(f.Code, "real-ip") {
				got = append(got, f)
			}
		}
		if c.code == "" {
			if len(got) != 0 {
				t.Errorf("%s: findings %+v", c.name, got)
			}
			continue
		}
		if len(got) != 1 || got[0].Code != c.code || got[0].Severity != "gap" || got[0].Suggestion == "" {
			t.Errorf("%s: findings %+v, want one %s gap with a suggestion", c.name, got, c.code)
		}
	}
}

func TestTheRealIPSuggestionNamesTheHeaderAndWhereToPutIt(t *testing.T) {
	text := "# configuration file /etc/nginx/nginx.conf:\nhttp {\n" + full + "\naccess_log " + logPath + " full;\nserver { server_name a.example; }\n}\n"
	r := run(t, cfg("cloudflare", nil), text)
	got := find(r, "real-ip-missing")
	if len(got) != 1 {
		t.Fatalf("findings = %+v", r.Findings)
	}
	for _, want := range []string{"real_ip_header CF-Connecting-IP", "set_real_ip_from", "http"} {
		if !strings.Contains(got[0].Suggestion, want) {
			t.Errorf("suggestion lacks %q:\n%s", want, got[0].Suggestion)
		}
	}
}

func TestNoProxyMeansNoRealIPFindings(t *testing.T) {
	r := run(t, cfg("none", nil), host(full, ""))
	for _, f := range r.Findings {
		if strings.HasPrefix(f.Code, "real-ip") {
			t.Errorf("unexpected %+v", f)
		}
	}
}

func TestRealIPSetAtHttpLevelCoversEveryServer(t *testing.T) {
	text := "http {\n" + full + "\n" + realIP + "\naccess_log " + logPath + " full;\nserver { server_name a.example; }\nserver { server_name b.example; set_real_ip_from 10.0.0.0/8; }\n}\n"
	r := run(t, cfg("cloudflare", nil), text)
	// b replaces the inherited trust list with its own; the header is still
	// inherited. Whether b's list covers the proxy's ranges can only be judged
	// once the range snapshot exists, so for now it is not a finding.
	if n := len(find(r, "real-ip-missing")) + len(find(r, "real-ip-no-trusted-proxies")); n != 0 {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestASampleSeparatesAnEmptyFieldFromAMissingOne(t *testing.T) {
	in := Input{
		Config: cfg("cloudflare", nil),
		Dump:   dump(t, host(full, realIP)),
		Samples: map[string]Sample{logPath: {Lines: 1000, Present: map[string]int{
			"client": 1000, "ua": 1000, "rt": 1000, "urt": 800, "cf_ray": 1000, "cf_country": 1000, "peer": 1000,
			"bot_score": 0, "ja3": 0, "ja4": 0, "lang": 990, "ch_ua": 500, "ch_plat": 500,
		}}},
	}
	r, err := Run(in)
	if err != nil {
		t.Fatal(err)
	}
	got := codes(r)
	for _, want := range []string{"field-empty/bot_score/note", "field-empty/ja3/note", "field-empty/ja4/note"} {
		if !got[want] {
			t.Errorf("no finding %s in %v", want, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("findings = %v", got)
	}
	if st := status(r.Logs[0], "bot_score"); st != "empty" {
		t.Errorf("bot_score status = %s", st)
	}
	if st := status(r.Logs[0], "ch_ua"); st != "ok" {
		t.Errorf("ch_ua is present in half the lines, status = %s, want ok", st)
	}
	// Required and empty is a gap.
	in.Config = cfg("cloudflare", map[string]string{"bot_score": "required"})
	r, _ = Run(in)
	if !codes(r)["field-empty/bot_score/gap"] || r.ExitCode() != 1 {
		t.Errorf("required but empty: %v exit %d", codes(r), r.ExitCode())
	}
	// A field a sample cannot speak for (here the empty log) is not judged.
	in.Samples = map[string]Sample{logPath: {Lines: 0}}
	r, _ = Run(in)
	if len(find(r, "field-empty")) != 0 {
		t.Errorf("an empty sample judged a field: %+v", r.Findings)
	}
}

func TestARequiredMissingFieldIsNotAlsoReportedEmpty(t *testing.T) {
	in := Input{
		Config:  cfg("none", nil),
		Dump:    dump(t, host("", "")),
		Samples: map[string]Sample{logPath: {Lines: 100, Present: map[string]int{}}},
	}
	r, _ := Run(in)
	if len(find(r, "field-empty")) != 0 || status(r.Logs[0], "rt") != "missing" {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestApacheIsUnsupported(t *testing.T) {
	c := cfg("none", nil)
	c.Server = "apache"
	_, err := Run(Input{Config: c, Dump: dump(t, host(full, ""))})
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("error = %v, want ErrUnsupported", err)
	}
}

func TestADumpWithNoServersIsAnError(t *testing.T) {
	_, err := Run(Input{Config: cfg("none", nil), Dump: dump(t, "events {}\nhttp { }\n")})
	if !errors.Is(err, ErrNoServers) {
		t.Errorf("error = %v, want ErrNoServers", err)
	}
}

func TestAnUnresolvableIncludeIsReportedAsSuch(t *testing.T) {
	_, err := Run(Input{Config: cfg("none", nil), Dump: dump(t, "http { include missing.conf; server { } }\n")})
	if !errors.Is(err, nginxconf.ErrInclude) {
		t.Errorf("error = %v, want nginxconf.ErrInclude", err)
	}
}

func TestOnlyGapsSetTheExitStatus(t *testing.T) {
	// Optional fields missing and an access_log off on a location: findings, but no gap.
	text := "http {\n" + full + "\naccess_log " + logPath + " full;\nserver { server_name a.example; location /static/ { access_log off; } }\n}\n"
	r := run(t, cfg("none", nil), text)
	if len(r.Findings) == 0 {
		t.Fatal("expected a warning")
	}
	if r.ExitCode() != 0 {
		t.Errorf("ExitCode = %d with findings %+v", r.ExitCode(), r.Findings)
	}
}

func TestReportIsOrderedAndRepeatable(t *testing.T) {
	text := host("", "")
	a, b := run(t, cfg("none", nil), text), run(t, cfg("none", nil), text)
	if !reflect.DeepEqual(a, b) {
		t.Error("two runs differ")
	}
	// Field findings follow the field table's order.
	var order []string
	for _, f := range find(a, "field-missing") {
		order = append(order, f.Field)
	}
	if !reflect.DeepEqual(order, []string{"rt", "urt", "lang", "ch_ua", "ch_plat"}) {
		t.Errorf("order = %v", order)
	}
}

func TestReportMarshalsToJSONWithStableKeys(t *testing.T) {
	r := run(t, cfg("none", nil), host("", ""))
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		Logs     []map[string]any `json:"logs"`
		Findings []map[string]any `json:"findings"`
		Exit     int              `json:"exit_status"`
	}
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Logs) != 1 || len(back.Findings) == 0 || back.Exit != 1 {
		t.Fatalf("json = %s", data)
	}
	for _, k := range []string{"code", "severity", "message", "field", "log", "at", "suggestion"} {
		if _, ok := back.Findings[0][k]; !ok {
			t.Errorf("finding has no %q key: %v", k, back.Findings[0])
		}
	}
}

// Findings never carry request content: the report is made to be pasted into
// an issue (DR-0003, DR-0002).
func TestReportHoldsNoLogContent(t *testing.T) {
	in := Input{
		Config:  cfg("none", nil),
		Dump:    dump(t, host("", "")),
		Samples: map[string]Sample{logPath: {Lines: 3, Present: map[string]int{"ua": 3}}},
	}
	r, _ := Run(in)
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "Mozilla") {
		t.Error("report contains log content")
	}
}

func TestDefaultTableIsUsedWhenNoneIsGiven(t *testing.T) {
	a, err := Run(Input{Config: cfg("none", nil), Dump: dump(t, host("", ""))})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Run(Input{Config: cfg("none", nil), Dump: dump(t, host("", "")), Table: fields.Default()})
	if !reflect.DeepEqual(a, b) {
		t.Error("nil Table differs from fields.Default()")
	}
}

func TestAProblemInheritedByManyServersIsOneFindingNamingThem(t *testing.T) {
	text := "http {\n" + full + "\nset_real_ip_from 0.0.0.0/0;\nreal_ip_header CF-Connecting-IP;\naccess_log " + logPath + " full;\n" +
		"server { server_name a.example; }\nserver { server_name b.example; }\n}\n"
	r := run(t, cfg("cloudflare", nil), text)
	got := find(r, "real-ip-trust-too-wide")
	if len(got) != 1 || !strings.Contains(got[0].Message, "a.example, b.example") {
		t.Errorf("findings = %+v", got)
	}
}
