package errorlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Synthetic lines in the shapes nginx writes; none is from a real log.
const (
	ts    = "2026/10/07 12:00:01 [%s] 1234#1234: "
	req   = `, client: 203.0.113.9, server: a.example, request: "GET /x HTTP/1.1", host: "a.example"`
	reqUp = `, client: 203.0.113.9, server: a.example, request: "GET /x HTTP/1.1", upstream: "http://127.0.0.1:8000/x", host: "a.example"`
)

func line(level, conn, msg, tail string) string {
	return fmt.Sprintf(ts, level) + conn + msg + tail
}

func TestTheDefaultTableIsWellFormed(t *testing.T) {
	tab := Default()
	if len(tab.Names()) < 10 {
		t.Fatalf("only %d categories", len(tab.Names()))
	}
	name := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	seen := map[string]bool{}
	for _, n := range tab.Names() {
		if !name.MatchString(n) || seen[n] {
			t.Errorf("category name %q is invalid or repeated", n)
		}
		seen[n] = true
	}
	for _, want := range []string{"limit-conn", "limit-conn-dry-run", "limit-req", "limit-req-dry-run", "limit-req-delay",
		"upstream-timeout", "upstream-refused", "upstream-closed", "upstream-reset", "upstream-no-live",
		"body-too-large", "buffered-response-to-disk", "buffered-request-to-disk", "tls-handshake", "file-not-found", "worker-crash"} {
		if !seen[want] {
			t.Errorf("default table lacks %q", want)
		}
	}
}

const small = `{"version":1,"categories":[
 {"name":"timeout","description":"d","pattern":"upstream timed out"},
 {"name":"cap","description":"d","pattern":"limiting connections by zone \"([^\"]+)\"","zone":true}]}`

func TestParseRejectsBadTables(t *testing.T) {
	cases := []struct{ name, from, to, mention string }{
		{"version", `"version":1`, `"version":2`, "version"},
		{"unknown key", `"name":"timeout"`, `"bogus":1,"name":"timeout"`, "bogus"},
		{"bad name", `"name":"timeout"`, `"name":"Time Out"`, "name"},
		{"duplicate", `"name":"cap"`, `"name":"timeout"`, "duplicate"},
		{"no description", `"description":"d","pattern":"upstream`, `"description":"","pattern":"upstream`, "description"},
		{"bad pattern", `"pattern":"upstream timed out"`, `"pattern":"upstream (timed"`, "pattern"},
		{"empty pattern", `"pattern":"upstream timed out"`, `"pattern":""`, "pattern"},
		{"zone without a group", `"pattern":"limiting connections by zone \"([^\"]+)\"","zone":true`, `"pattern":"limiting connections","zone":true`, "zone"},
		{"a group without zone", `"pattern":"upstream timed out"`, `"pattern":"upstream (timed) out"`, "group"},
	}
	for _, c := range cases {
		if !strings.Contains(small, c.from) {
			t.Fatalf("%s: fixture lacks %q", c.name, c.from)
		}
		_, err := Parse([]byte(strings.Replace(small, c.from, c.to, 1)))
		if !errors.Is(err, ErrMalformed) || !strings.Contains(err.Error(), c.mention) {
			t.Errorf("%s: error = %v, want ErrMalformed mentioning %q", c.name, err, c.mention)
		}
	}
	if _, err := Parse([]byte(small)); err != nil {
		t.Errorf("the small table must parse: %v", err)
	}
}

func TestEachCategoryMatchesItsMessage(t *testing.T) {
	cases := []struct {
		category string
		line     string
	}{
		{"limit-conn", line("warn", "*5 ", `limiting connections by zone "api_conc"`, req)},
		{"limit-conn-dry-run", line("warn", "*5 ", `limiting connections, dry run, by zone "api_conc"`, req)},
		{"limit-req", line("error", "*5 ", `limiting requests, excess: 5.340 by zone "req_zone"`, req)},
		{"limit-req-dry-run", line("error", "*5 ", `limiting requests, dry run, excess: 5.340 by zone "req_zone"`, req)},
		{"limit-req-delay", line("warn", "*5 ", `delaying request, excess: 0.730, by zone "req_zone"`, req)},
		{"upstream-timeout", line("error", "*6 ", `upstream timed out (110: Connection timed out) while reading response header from upstream`, reqUp)},
		{"upstream-refused", line("error", "*7 ", `connect() failed (111: Connection refused) while connecting to upstream`, reqUp)},
		{"upstream-closed", line("error", "*8 ", `upstream prematurely closed connection while reading response header from upstream`, reqUp)},
		{"upstream-reset", line("error", "*9 ", `recv() failed (104: Connection reset by peer) while reading response header from upstream`, reqUp)},
		{"upstream-no-live", line("error", "*10 ", `no live upstreams while connecting to upstream`, reqUp)},
		{"body-too-large", line("error", "*11 ", `client intended to send too large body: 2097153 bytes`, req)},
		{"buffered-response-to-disk", line("warn", "*12 ", `an upstream response is buffered to a temporary file /var/cache/nginx/proxy_temp/1/00/0000000001 while reading upstream`, reqUp)},
		{"buffered-request-to-disk", line("warn", "*13 ", `a client request body is buffered to a temporary file /var/cache/nginx/client_temp/0000000002`, req)},
		{"tls-handshake", line("crit", "*14 ", `SSL_do_handshake() failed (SSL: error:0A000126:SSL routines::unexpected eof while reading) while SSL handshaking`, `, client: 203.0.113.9, server: 0.0.0.0:443`)},
		{"file-not-found", line("error", "*15 ", `open() "/Sites/x/htdocs/missing.png" failed (2: No such file or directory)`, req)},
		{"file-not-found", line("error", "*16 ", `open() "/Sites/x/htdocs/index.html/extra" failed (20: Not a directory)`, req)},
		{"worker-crash", line("alert", "", `worker process 1234 exited on signal 11`, "")},
	}
	tab := Default()
	for _, c := range cases {
		got := Count(strings.NewReader(c.line+"\n"), tab)
		if got.Lines != 1 || got.Categories[c.category] != 1 {
			t.Errorf("%s: Lines=%d Categories=%v for %q", c.category, got.Lines, got.Categories, c.line)
		}
		if n := len(got.Categories); n != 1 {
			t.Errorf("%s: the line landed in %d categories: %v", c.category, n, got.Categories)
		}
	}
}

func TestLinesNoCategoryKnowsAreCountedByLevelOnly(t *testing.T) {
	text := strings.Join([]string{
		line("notice", "", "signal process started", ""),
		line("error", "*20 ", `something nobody has seen before at "/secret/path"`, req),
		line("emerg", "", "bind() to 0.0.0.0:80 failed (98: Address already in use)", ""),
	}, "\n") + "\n"
	got := Count(strings.NewReader(text), Default())
	if got.Categories["other"] != 3 || got.Levels["notice"] != 1 || got.Levels["error"] != 1 || got.Levels["emerg"] != 1 {
		t.Errorf("Categories = %v Levels = %v", got.Categories, got.Levels)
	}
}

func TestLevelsAreCountedForEveryLine(t *testing.T) {
	text := line("warn", "*5 ", `limiting connections by zone "api_conc"`, req) + "\n" +
		line("crit", "*14 ", `SSL_do_handshake() failed (SSL: x) while SSL handshaking`, "") + "\n" +
		line("error", "*6 ", `upstream timed out (110: Connection timed out) while reading response header from upstream`, reqUp) + "\n"
	got := Count(strings.NewReader(text), Default())
	if got.Levels["warn"] != 1 || got.Levels["crit"] != 1 || got.Levels["error"] != 1 {
		t.Errorf("Levels = %v", got.Levels)
	}
}

func TestZonesAreCountedByName(t *testing.T) {
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines, line("warn", "*5 ", `limiting connections by zone "api_conc"`, req))
	}
	for i := 0; i < 2; i++ {
		lines = append(lines, line("warn", "*6 ", `limiting connections by zone "iiif_conc"`, req))
	}
	lines = append(lines, line("warn", "*7 ", `limiting connections, dry run, by zone "api_conc"`, req))
	got := Count(strings.NewReader(strings.Join(lines, "\n")+"\n"), Default())
	if got.Zones["limit-conn"]["api_conc"] != 5 || got.Zones["limit-conn"]["iiif_conc"] != 2 || got.Zones["limit-conn-dry-run"]["api_conc"] != 1 {
		t.Errorf("Zones = %v", got.Zones)
	}
	if got.Categories["limit-conn"] != 7 {
		t.Errorf("Categories = %v", got.Categories)
	}
	if _, ok := got.Zones["upstream-timeout"]; ok {
		t.Errorf("a category with no zone has zones: %v", got.Zones)
	}
}

func TestTheTimeSpanComesFromTheLines(t *testing.T) {
	text := "2026/10/05 21:40:12 [error] 1#1: first\n2026/10/06 03:00:00 [error] 1#1: middle\n2026/10/07 14:02:59 [error] 1#1: last\n"
	got := Count(strings.NewReader(text), Default())
	if got.First != "2026-10-05 21:40:12" || got.Last != "2026-10-07 14:02:59" || got.Lines != 3 {
		t.Errorf("First = %q Last = %q Lines = %d", got.First, got.Last, got.Lines)
	}
}

func TestLinesThatAreNotErrorLogLinesAreSkipped(t *testing.T) {
	text := "garbage\n\n   \nthe request body continues on a second line\n" +
		line("error", "*5 ", `upstream timed out (110: Connection timed out) while reading response header from upstream`, reqUp) + "\n" +
		"2026/10/07 12:00:01 [error] no process id here\n"
	got := Count(strings.NewReader(text), Default())
	if got.Lines != 1 || got.Skipped != 3 {
		t.Errorf("Lines = %d Skipped = %d, want 1 and 3 (blank lines are not counted)", got.Lines, got.Skipped)
	}
}

func TestACustomTableIsUsed(t *testing.T) {
	tab, err := Parse([]byte(small))
	if err != nil {
		t.Fatal(err)
	}
	text := line("error", "*6 ", `upstream timed out (110: Connection timed out)`, reqUp) + "\n" + line("error", "*7 ", `connect() failed (111: Connection refused) while connecting to upstream`, reqUp) + "\n"
	got := Count(strings.NewReader(text), tab)
	if got.Categories["timeout"] != 1 || got.Categories["other"] != 1 || got.Categories["upstream-refused"] != 0 {
		t.Errorf("Categories = %v", got.Categories)
	}
}

// What comes out holds names and numbers: no address, request, path or message text.
func TestTheCountsHoldNoLogContent(t *testing.T) {
	text := line("error", "*5 ", `open() "/Sites/secret/htdocs/private.pdf" failed (2: No such file or directory)`, req) + "\n" +
		line("error", "*6 ", `something with "/another/secret/path"`, req) + "\n" +
		line("warn", "*7 ", `limiting connections by zone "api_conc"`, req) + "\n"
	got := Count(strings.NewReader(text), Default())
	data, _ := json.Marshal(got)
	for _, leak := range []string{"203.0.113.9", "a.example", "secret", "private.pdf", "GET /x", "127.0.0.1", "HTTP/1.1"} {
		if strings.Contains(string(data), leak) {
			t.Errorf("the counts contain %q: %s", leak, data)
		}
	}
	if !strings.Contains(string(data), "api_conc") {
		t.Errorf("the zone name is missing: %s", data)
	}
}

func TestFileReadsTheEndOfTheLog(t *testing.T) {
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, line("error", "*5 ", `upstream timed out (110: Connection timed out) while reading response header from upstream`, reqUp))
	}
	for i := 0; i < 5; i++ {
		lines = append(lines, line("warn", "*6 ", `limiting connections by zone "api_conc"`, req))
	}
	p := filepath.Join(t.TempDir(), "error.log")
	os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	got, err := File(p, 10, Default())
	if err != nil {
		t.Fatal(err)
	}
	if got.Lines != 10 || got.Categories["limit-conn"] != 5 || got.Categories["upstream-timeout"] != 5 {
		t.Errorf("Lines = %d Categories = %v", got.Lines, got.Categories)
	}
	if _, err := File(filepath.Join(t.TempDir(), "nope"), 10, Default()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: error = %v", err)
	}
}

func TestTheFirstMatchingCategoryWins(t *testing.T) {
	tab, err := Parse([]byte(`{"version":1,"categories":[
 {"name":"first","description":"d","pattern":"upstream"},
 {"name":"second","description":"d","pattern":"upstream timed out"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	got := Count(strings.NewReader(line("error", "*5 ", "upstream timed out (110: Connection timed out)", reqUp)+"\n"), tab)
	if got.Categories["first"] != 1 || got.Categories["second"] != 0 {
		t.Errorf("Categories = %v", got.Categories)
	}
}

func TestACategoryCannotBeCalledOther(t *testing.T) {
	_, err := Parse([]byte(`{"version":1,"categories":[{"name":"other","description":"d","pattern":"x"}]}`))
	if !errors.Is(err, ErrMalformed) || !strings.Contains(err.Error(), "other") {
		t.Errorf("error = %v", err)
	}
}
