package sample

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caltechlibrary/logagent/internal/fields"
)

// deployedRaw is the CaltechAUTHORS log_format as nginx concatenates its quoted
// pieces: no separator is added between them.
const deployedRaw = `$remote_addr - $remote_user [$time_local] "$request" ` +
	`$status $body_bytes_sent "$http_referer" "$http_user_agent" ` +
	`rt=$request_time urt="$upstream_response_time" ` +
	`cf_ray="$http_cf_ray" cf_country="$http_cf_ipcountry" ` +
	`peer=$realip_remote_addr ` +
	`lang="$http_accept_language" ch_ua="$http_sec_ch_ua" ` +
	`ch_plat="$http_sec_ch_ua_platform" ` +
	`bot_score="$http_cf_bot_score" ja3="$http_cf_ja3_hash" ja4="$http_cf_ja4"`

const combinedRaw = `$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"`

// line builds a log line in the deployed format. A value of "" is written as nginx
// writes a missing header: a dash.
func line(botScore, urt string) string {
	if botScore == "" {
		botScore = "-"
	}
	if urt == "" {
		urt = "-"
	}
	return `203.0.113.9 - - [07/Oct/2026:12:00:01 -0700] "GET /api/records?q=x y HTTP/1.1" 200 5123 "https://example.org/" "Mozilla/5.0 (X11; Linux)" ` +
		`rt=0.123 urt="` + urt + `" cf_ray="8abc-LAX" cf_country="US" peer=172.71.1.1 lang="en-US,en;q=0.9" ` +
		`ch_ua="\x22Chromium\x22;v=\x22120\x22" ch_plat="\x22macOS\x22" bot_score="` + botScore + `" ja3="-" ja4="-"`
}

func TestParseTheDeployedFormat(t *testing.T) {
	p, err := Compile(deployedRaw)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := p.Parse(line("12", "0.120"))
	if !ok {
		t.Fatal("the line does not match its own format")
	}
	want := map[string]string{
		"remote_addr": "203.0.113.9", "remote_user": "-", "time_local": "07/Oct/2026:12:00:01 -0700",
		"request": "GET /api/records?q=x y HTTP/1.1", "status": "200", "body_bytes_sent": "5123",
		"http_user_agent": "Mozilla/5.0 (X11; Linux)", "request_time": "0.123", "upstream_response_time": "0.120",
		"realip_remote_addr": "172.71.1.1", "http_cf_bot_score": "12", "http_cf_ja4": "-",
		"http_sec_ch_ua": `\x22Chromium\x22;v=\x22120\x22`,
	}
	for k, w := range want {
		if v[k] != w {
			t.Errorf("%s = %q, want %q", k, v[k], w)
		}
	}
}

func TestParseTheCombinedFormat(t *testing.T) {
	p, err := Compile(combinedRaw)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := p.Parse(`198.51.100.7 - bob [01/Jan/2026:00:00:00 +0000] "POST /x HTTP/2.0" 404 0 "-" "curl/8.4.0"`)
	if !ok || v["remote_user"] != "bob" || v["status"] != "404" || v["http_user_agent"] != "curl/8.4.0" {
		t.Errorf("ok=%v values=%v", ok, v)
	}
}

func TestParseRejectsLinesThatDoNotMatch(t *testing.T) {
	p, _ := Compile(combinedRaw)
	for _, l := range []string{"", "garbage", `198.51.100.7 - - [x] "GET /" 200`, line("", "") + " trailing"} {
		if _, ok := p.Parse(l); ok {
			t.Errorf("accepted %q", l)
		}
	}
	// A trailing newline or carriage return is not part of the line.
	if _, ok := p.Parse(`1.2.3.4 - - [t] "GET / HTTP/1.1" 200 1 "-" "ua"` + "\r"); !ok {
		t.Error("rejected a line ending in a carriage return")
	}
}

func TestCompileTreatsLiteralTextAsLiteral(t *testing.T) {
	p, err := Compile(`(a|b) [$status] {$bytes} .* $remote_addr`)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := p.Parse(`(a|b) [200] {12} .* 10.0.0.1`)
	if !ok || v["status"] != "200" || v["bytes"] != "12" || v["remote_addr"] != "10.0.0.1" {
		t.Errorf("ok=%v values=%v", ok, v)
	}
	if _, ok := p.Parse(`aXb [200] {12} .* 10.0.0.1`); ok {
		t.Error("a regular expression metacharacter in the format acted as one")
	}
}

func TestCompileAcceptsBracedVariablesAndRejectsWhatItCannotRead(t *testing.T) {
	p, err := Compile(`${remote_addr} "${request}"`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := p.Parse(`10.0.0.1 "GET / HTTP/1.1"`); !ok || v["remote_addr"] != "10.0.0.1" {
		t.Errorf("braced variables: ok=%v values=%v", ok, v)
	}
	for name, f := range map[string]string{
		"adjacent variables": `$remote_addr$remote_user`,
		"no variables":       `just text`,
		"empty":              ``,
		"unterminated brace": `${remote_addr`,
	} {
		if _, err := Compile(f); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: error = %v, want ErrFormat", name, err)
		}
	}
}

func TestCompileReportsTheVariablesItKnows(t *testing.T) {
	p, _ := Compile(`$remote_addr "$http_cf_ray" $request_time $remote_addr`)
	got := strings.Join(p.Variables(), " ")
	if got != "remote_addr http_cf_ray request_time" {
		t.Errorf("Variables = %q, in order of first use without repeats", got)
	}
}

func TestCountTreatsADashAndNothingAsAbsent(t *testing.T) {
	p, _ := Compile(deployedRaw)
	lines := strings.Join([]string{line("12", "0.1"), line("", "0.2"), line("90", ""), "not a log line", line("", "")}, "\n") + "\n"
	s := Count(strings.NewReader(lines), p, fields.Default())
	if s.Lines != 4 || s.Skipped != 1 {
		t.Fatalf("Lines = %d, Skipped = %d, want 4 and 1", s.Lines, s.Skipped)
	}
	for name, want := range map[string]int{"bot_score": 2, "urt": 2, "ua": 4, "rt": 4, "cf_ray": 4, "ja3": 0, "ja4": 0, "client": 4, "user": 0} {
		if got, ok := s.Present[name]; !ok || got != want {
			t.Errorf("Present[%s] = %d (present=%v), want %d", name, got, ok, want)
		}
	}
}

func TestCountOnlyCountsFieldsTheFormatCarries(t *testing.T) {
	p, _ := Compile(combinedRaw)
	s := Count(strings.NewReader(`1.2.3.4 - - [t] "GET / HTTP/1.1" 200 1 "-" "ua"`+"\n"), p, fields.Default())
	if _, ok := s.Present["rt"]; ok {
		t.Error("rt counted although the format has no $request_time")
	}
	if s.Present["ua"] != 1 {
		t.Errorf("Present = %v", s.Present)
	}
}

// The counts hold no values: nothing of a line survives in the result.
func TestCountKeepsNoLogContent(t *testing.T) {
	p, _ := Compile(deployedRaw)
	s := Count(strings.NewReader(line("12", "0.1")+"\n"), p, fields.Default())
	if txt := fmt.Sprintf("%+v", s); strings.Contains(txt, "203.0.113.9") || strings.Contains(txt, "Mozilla") {
		t.Errorf("result holds log content: %s", txt)
	}
}

func writeLog(t *testing.T, lines []string, trailingNewline bool) string {
	t.Helper()
	text := strings.Join(lines, "\n")
	if trailingNewline {
		text += "\n"
	}
	path := filepath.Join(t.TempDir(), "access.log")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFileSamplesTheLastLines(t *testing.T) {
	p, _ := Compile(deployedRaw)
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, line("", "0.1")) // old lines: no bot score
	}
	for i := 0; i < 5; i++ {
		lines = append(lines, line("33", "0.1")) // recent lines: a bot score
	}
	path := writeLog(t, lines, true)
	s, err := File(path, 10, p, fields.Default())
	if err != nil {
		t.Fatal(err)
	}
	if s.Lines != 10 || s.Present["bot_score"] != 5 {
		t.Errorf("Lines = %d, bot_score = %d, want 10 and 5", s.Lines, s.Present["bot_score"])
	}
	for _, n := range []int{25, 1000} {
		s, _ := File(path, n, p, fields.Default())
		if s.Lines != 25 {
			t.Errorf("n=%d: Lines = %d, want all 25", n, s.Lines)
		}
	}
}

func TestFileWithoutATrailingNewlineAndAcrossManyBlocks(t *testing.T) {
	p, _ := Compile(deployedRaw)
	var lines []string
	for i := 0; i < 3000; i++ { // about 700 KB, far over one read block
		lines = append(lines, line("", "0.1"))
	}
	lines = append(lines, line("77", "0.1"))
	path := writeLog(t, lines, false)
	s, err := File(path, 1000, p, fields.Default())
	if err != nil {
		t.Fatal(err)
	}
	if s.Lines != 1000 || s.Skipped != 0 || s.Present["bot_score"] != 1 {
		t.Errorf("Lines = %d, Skipped = %d, bot_score = %d", s.Lines, s.Skipped, s.Present["bot_score"])
	}
}

func TestFileEdgeCases(t *testing.T) {
	p, _ := Compile(deployedRaw)
	empty := writeLog(t, nil, false)
	if s, err := File(empty, 100, p, fields.Default()); err != nil || s.Lines != 0 || s.Skipped != 0 {
		t.Errorf("empty file: %+v, %v", s, err)
	}
	if _, err := File(filepath.Join(t.TempDir(), "nope.log"), 100, p, fields.Default()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing file: error = %v, want fs.ErrNotExist", err)
	}
	if _, err := File(t.TempDir(), 100, p, fields.Default()); err == nil {
		t.Error("a directory was sampled")
	}
	if _, err := File(empty, 0, p, fields.Default()); err == nil {
		t.Error("n = 0 accepted")
	}
}

func TestFilePermissionRefusedIsReportedAsSuch(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	p, _ := Compile(deployedRaw)
	path := writeLog(t, []string{line("", "")}, true)
	os.Chmod(path, 0)
	if _, err := File(path, 10, p, fields.Default()); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("error = %v, want fs.ErrPermission", err)
	}
}

func TestAnEmptyValueIsAbsentJustAsADashIs(t *testing.T) {
	p, err := Compile(`$remote_addr "$http_cf_ray" $status`)
	if err != nil {
		t.Fatal(err)
	}
	s := Count(strings.NewReader("1.1.1.1 \"\" 200\n1.1.1.2 \"-\" 200\n1.1.1.3 \"8abc\" 200\n"), p, fields.Default())
	if s.Lines != 3 || s.Present["cf_ray"] != 1 || s.Present["client"] != 3 {
		t.Errorf("Lines = %d, Present = %v", s.Lines, s.Present)
	}
}
