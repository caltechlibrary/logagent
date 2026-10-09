package report

import (
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caltechlibrary/logagent/internal/classify"
	"github.com/caltechlibrary/logagent/internal/event"
	"github.com/caltechlibrary/logagent/internal/logread"
	"github.com/caltechlibrary/logagent/internal/sample"
)

var d1 = time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)

// ev is one event on day offset d at hour h.
func ev(d, h int, family, class string, status int, urt float64, who string) event.Event {
	return event.Event{
		V: event.Version, T: d1.AddDate(0, 0, d).Add(time.Duration(h) * time.Hour), Method: "GET",
		Class: class, Status: status, RT: urt, URT: urt, Timed: true, Family: family, Who: who,
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func newAgg(o Options) *Aggregator {
	if o.Top == 0 {
		o.Top = 15
	}
	return New(o)
}

func TestSectionOneCountsTheSourcesAndSaysWhatIsNotConfigured(t *testing.T) {
	a := newAgg(Options{})
	a.AddInternal(netip.MustParsePrefix("131.215.0.0/16"))
	a.AddInternal(netip.MustParsePrefix("131.215.0.0/16"))
	a.AddInternal(netip.MustParsePrefix("10.0.0.0/8"))
	a.Add(ev(0, 1, "x", "c", 200, 0.1, "a"))
	r := a.Report(logread.Stats{Files: []string{"/l/access.log.1", "/l/access.log"}, Lines: 1, Skipped: 4, Outside: 7})
	if r.Lines != 1 || r.Skipped != 4 || r.Outside != 7 || len(r.Files) != 2 {
		t.Errorf("report = %+v", r)
	}
	if len(r.Internal) != 2 || r.Internal[0].Range != "131.215.0.0/16" || r.Internal[0].Requests != 2 || r.Internal[1].Requests != 1 {
		t.Errorf("Internal = %+v", r.Internal)
	}
	joined := strings.Join(r.Notes, "\n")
	if strings.Contains(joined, "no internal ranges") {
		t.Errorf("a configured internal range was reported as missing: %q", joined)
	}
	none := newAgg(Options{}).Report(logread.Stats{})
	if !strings.Contains(strings.Join(none.Notes, "\n"), "no internal ranges") {
		t.Errorf("no note about missing internal ranges: %q", none.Notes)
	}
}

func TestNotesSaySoWhenClassesAreNotConfiguredOrTimingIsMissing(t *testing.T) {
	a := newAgg(Options{NoClasses: true})
	e := ev(0, 1, "x", "other", 200, 0, "a")
	e.Timed = false
	a.Add(e)
	notes := strings.Join(a.Report(logread.Stats{Lines: 1}).Notes, "\n")
	for _, want := range []string{"no path classes", "no request time"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes %q lack %q", notes, want)
		}
	}
	// Some timed and some not: say so, and that per-request figures use the timed ones.
	a = newAgg(Options{})
	a.Add(ev(0, 1, "x", "c", 200, 0.5, "a"))
	e2 := ev(0, 1, "x", "c", 200, 0, "b")
	e2.Timed = false
	a.Add(e2)
	if notes := strings.Join(a.Report(logread.Stats{Lines: 2}).Notes, "\n"); !strings.Contains(notes, "1 of 2") {
		t.Errorf("notes %q do not say 1 of 2 requests were timed", notes)
	}
}

func TestPerDayAndPerHourForAllTrafficAndAFamily(t *testing.T) {
	a := newAgg(Options{Match: "exa"})
	a.Add(ev(0, 1, "Exa", "c", 200, 0.1, "a"))
	a.Add(ev(0, 1, "Exa", "c", 200, 0.1, "a")) // same client again
	a.Add(ev(0, 1, "Exa", "c", 200, 0.1, "b"))
	a.Add(ev(0, 2, "SemrushBot", "c", 200, 0.1, "c"))
	a.Add(ev(1, 5, "Exa", "c", 200, 0.1, "a"))
	r := a.Report(logread.Stats{Lines: 5})
	if len(r.Days) != 2 {
		t.Fatalf("Days = %+v", r.Days)
	}
	want := []DayRow{{Day: "2026-10-07", All: 4, Match: 3, Clients: 2}, {Day: "2026-10-08", All: 1, Match: 1, Clients: 1}}
	for i, w := range want {
		if r.Days[i] != w {
			t.Errorf("Days[%d] = %+v, want %+v", i, r.Days[i], w)
		}
	}
	if len(r.Hours) != 3 || r.Hours[0].Hour != "2026-10-07 01" || r.Hours[0].All != 3 || r.Hours[0].Match != 3 || r.Hours[1].All != 1 || r.Hours[1].Match != 0 {
		t.Errorf("Hours = %+v", r.Hours)
	}
	// Without a family there is no match column and no client count.
	r = newAgg(Options{}).Report(logread.Stats{})
	if len(r.Days) != 0 {
		t.Errorf("empty Days = %+v", r.Days)
	}
}

func TestOnlyTheLast48HoursAreListed(t *testing.T) {
	a := newAgg(Options{})
	for h := 0; h < 100; h++ {
		a.Add(ev(0, h, "x", "c", 200, 0.1, "a"))
	}
	r := a.Report(logread.Stats{Lines: 100})
	if len(r.Hours) != 48 || r.Hours[47].Hour != "2026-10-11 03" || r.Hours[0].Hour != "2026-10-09 04" {
		t.Errorf("%d hours, first %v last %v", len(r.Hours), r.Hours[0], r.Hours[len(r.Hours)-1])
	}
}

func TestStatusMixAndTheErrorStatusesPerDay(t *testing.T) {
	a := newAgg(Options{})
	for i := 0; i < 5; i++ {
		a.Add(ev(0, 1, "x", "c", 200, 0.1, "a"))
	}
	a.Add(ev(0, 1, "x", "c", 429, 0, "a"))
	a.Add(ev(0, 1, "x", "c", 429, 0, "a"))
	a.Add(ev(0, 2, "x", "c", 504, 9, "a"))
	a.Add(ev(1, 1, "x", "c", 499, 0, "a"))
	a.Add(ev(1, 1, "x", "c", 502, 0, "a"))
	a.Add(ev(1, 1, "x", "c", 404, 0, "a"))
	r := a.Report(logread.Stats{Lines: 11})
	if r.Status[0] != (StatusRow{Status: 200, Count: 5}) || r.Status[1] != (StatusRow{Status: 429, Count: 2}) {
		t.Errorf("Status = %+v", r.Status)
	}
	want := []ErrorDay{{Day: "2026-10-07", S429: 2, S504: 1}, {Day: "2026-10-08", S499: 1, S502: 1}}
	if len(r.Errors) != 2 || r.Errors[0] != want[0] || r.Errors[1] != want[1] {
		t.Errorf("Errors = %+v, want %+v", r.Errors, want)
	}
}

func TestByClassDividesUpstreamSecondsByTheTimedRequests(t *testing.T) {
	a := newAgg(Options{})
	a.Add(ev(0, 1, "x", "api-iiif", 200, 3.0, "a"))
	a.Add(ev(0, 1, "x", "api-iiif", 200, 1.0, "b"))
	untimed := ev(0, 1, "x", "api-iiif", 200, 0, "c")
	untimed.Timed = false
	a.Add(untimed)
	a.Add(ev(0, 1, "x", "static", 200, 0.0, "a"))
	r := a.Report(logread.Stats{Lines: 4})
	if len(r.Classes) != 2 || r.Classes[0].Class != "api-iiif" {
		t.Fatalf("Classes = %+v (the most upstream seconds comes first)", r.Classes)
	}
	c := r.Classes[0]
	if c.Requests != 3 || c.Timed != 2 || !near(c.UpstreamSeconds, 4) || !near(c.PerRequest, 2) {
		t.Errorf("api-iiif = %+v, want 3 requests, 2 timed, 4 s, 2 s per timed request", c)
	}
}

func TestByFamilyShowsAPIShareCostAndTheLimitedRate(t *testing.T) {
	a := newAgg(Options{Declared: map[string]bool{"Exa": true}})
	a.Add(ev(0, 1, "Exa", "api-record", 200, 0.5, "a"))
	a.Add(ev(0, 1, "Exa", "api-versions", 429, 0, "a"))
	a.Add(ev(0, 1, "Exa", "ui-record", 200, 0.1, "a"))
	a.Add(ev(0, 1, "Exa", "static", 200, 0.0, "a"))
	r := a.Report(logread.Stats{Lines: 4})
	if len(r.Families) != 1 {
		t.Fatalf("Families = %+v", r.Families)
	}
	f := r.Families[0]
	if f.Family != "Exa" || f.Requests != 4 || f.APIRequests != 2 || !near(f.APIShare, 0.5) ||
		f.Timed != 4 || !near(f.UpstreamSeconds, 0.6) || !near(f.APIUpstream, 0.5) || !near(f.PerRequest, 0.15) ||
		f.Limited != 1 || !near(f.LimitedRate, 0.25) {
		t.Errorf("Exa = %+v", f)
	}
}

// DR-0002: a cohort with fewer than 10 distinct clients is folded so the
// report cannot point at one reader. A declared automated agent names itself,
// so it stays.
func TestSmallUndeclaredFamiliesAreFoldedAndDeclaredOnesAreKept(t *testing.T) {
	a := newAgg(Options{Declared: map[string]bool{"Exa": true}})
	for i := 0; i < 12; i++ {
		a.Add(ev(0, 1, "undeclared Windows", "ui-record", 200, 0.1, fmt.Sprintf("w%d", i)))
	}
	a.Add(ev(0, 1, "undeclared Mac", "ui-record", 200, 0.1, "m1"))
	a.Add(ev(0, 1, "undeclared Linux", "ui-record", 200, 0.1, "l1"))
	a.Add(ev(0, 1, "Exa", "ui-record", 200, 0.1, "e1"))
	r := a.Report(logread.Stats{Lines: 15})
	byName := map[string]FamilyRow{}
	for _, f := range r.Families {
		byName[f.Family] = f
	}
	if _, ok := byName["undeclared Mac"]; ok {
		t.Errorf("a family of one client was shown: %+v", byName["undeclared Mac"])
	}
	if o := byName["other (fewer than 10 clients)"]; o.Requests != 2 {
		t.Errorf("folded row = %+v, want the Mac and Linux requests", o)
	}
	if byName["Exa"].Requests != 1 || byName["undeclared Windows"].Requests != 12 {
		t.Errorf("Exa and the big family must stay: %+v", byName)
	}
}

func TestJSONHasStableKeysAndNoAddressesPathsOrQueries(t *testing.T) {
	a := newAgg(Options{Match: "exa"})
	e := ev(0, 1, "Exa", "api-record", 200, 0.5, "abcdef0123456789")
	e.Net, e.UA = "203.0.113.0/24", "ExaSearchBot/1.0"
	a.Add(e)
	r := a.Report(logread.Stats{Lines: 1, Files: []string{"/var/log/nginx/access.log"}})
	var buf strings.Builder
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &back); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"window", "lines", "skipped", "outside", "files", "internal", "notes", "days", "hours", "status", "errors", "classes", "families"} {
		if _, ok := back[key]; !ok {
			t.Errorf("JSON lacks %q: %s", key, buf.String())
		}
	}
	for _, leak := range []string{"203.0.113", "abcdef0123456789", "ExaSearchBot"} {
		if strings.Contains(buf.String(), leak) {
			t.Errorf("JSON contains %q", leak)
		}
	}
}

func TestTextShowsTheFiveSectionsAndTopLimitsTheRankedTables(t *testing.T) {
	a := newAgg(Options{Top: 2, Match: "exa"})
	for i, c := range []string{"c1", "c2", "c3", "c4"} {
		a.Add(ev(0, 1, "Exa", c, 200, float64(i+1), fmt.Sprintf("w%d", i)))
	}
	r := a.Report(logread.Stats{Lines: 4, Files: []string{"/l/access.log"}})
	var buf strings.Builder
	if err := r.WriteText(&buf, 2); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	for _, h := range []string{"1. SOURCES", "2. REQUESTS PER DAY", "3. STATUS", "4. BY PATH CLASS", "5. BY FAMILY"} {
		if !strings.Contains(text, h) {
			t.Errorf("text lacks the heading %q:\n%s", h, text)
		}
	}
	if strings.Contains(text, "c1 ") || !strings.Contains(text, "c4") {
		t.Errorf("--top 2 should show the two costliest classes (c4, c3) and not c1:\n%s", text)
	}
	if !strings.Contains(text, "2 more") {
		t.Errorf("the text should say how many rows were left out:\n%s", text)
	}
}

const realFormat = `$remote_addr - $remote_user [$time_local] "$request" ` +
	`$status $body_bytes_sent "$http_referer" "$http_user_agent" ` +
	`rt=$request_time urt="$upstream_response_time" ` +
	`cf_ray="$http_cf_ray" cf_country="$http_cf_ipcountry" ` +
	`peer=$realip_remote_addr ` +
	`lang="$http_accept_language" ch_ua="$http_sec_ch_ua" ` +
	`ch_plat="$http_sec_ch_ua_platform" ` +
	`bot_score="$http_cf_bot_score" ja3="$http_cf_ja3_hash" ja4="$http_cf_ja4" cache="$upstream_cache_status"`

func logLine(addr, when, req string, status int, ua string, rt, urt string) string {
	return fmt.Sprintf(`%s - - [%s] "%s" %d 100 "https://example.org/secret-ref" "%s" rt=%s urt="%s" cf_ray="9ab" cf_country="US" peer=198.41.128.1 lang="en-US" ch_ua="-" ch_plat="-" bot_score="-" ja3="-" ja4="-" cache="-"`,
		addr, when, req, status, ua, rt, urt)
}

// End to end: a log file in, a report out, and nothing in the report that a
// patron did or who they are (DR-0002).
func TestCollectReadsALogAndTheReportHoldsNoAddressPathQueryOrReferer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")
	lines := []string{
		logLine("203.0.113.5", "07/Oct/2026:01:00:00 +0000", "GET /search?q=private+topic HTTP/1.1", 200, "Mozilla/5.0 (Windows NT 10.0) Chrome/148", "0.100", "0.200"),
		logLine("203.0.113.6", "07/Oct/2026:01:00:05 +0000", "GET /api/records/abc12-34 HTTP/1.1", 200, "ExaSearchBot/1.0", "0.300", "0.250"),
		logLine("203.0.113.6", "07/Oct/2026:02:00:00 +0000", "GET /api/records/abc12-34/versions HTTP/1.1", 429, "ExaSearchBot/1.0", "0.000", "-"),
		logLine("131.215.1.2", "07/Oct/2026:02:00:01 +0000", "GET /records/zzz99-88 HTTP/1.1", 200, "Mozilla/5.0 (Macintosh) Safari", "0.050", "0.040"),
		"this line is not in the format",
		// Matches the format but its status is not a number, so it cannot become an event.
		logLine("203.0.113.9", "07/Oct/2026:03:00:00 +0000", "GET /x HTTP/1.1", 0, "curl/8", "0.1", "0.1"),
	}
	lines[len(lines)-1] = strings.Replace(lines[len(lines)-1], "\" 0 100", "\" abc 100", 1)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := sample.Compile(realFormat)
	if err != nil {
		t.Fatal(err)
	}
	cls, err := classify.NewClasses([]classify.Rule{
		{Name: "api-record", Pattern: `/api/records/[^/]+$`}, {Name: "api-versions", Pattern: `/api/records/[^/]+/versions`},
		{Name: "ui-record", Pattern: `/records/[^/]+$`}, {Name: "ui-search", Prefix: "/search"},
	}, "other")
	if err != nil {
		t.Fatal(err)
	}
	fams := classify.DefaultFamilies()
	r, err := Collect(path, p, Options{
		Top: 15, Match: "exa", Class: cls.Class, Lookup: fams.Lookup,
		Internal: []netip.Prefix{netip.MustParsePrefix("131.215.0.0/16")},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if r.Lines != 4 || r.Skipped != 2 || r.Events != 3 {
		t.Errorf("Lines %d Skipped %d Events %d, want 4, 2, 3 (the internal request is counted, not an event)", r.Lines, r.Skipped, r.Events)
	}
	if len(r.Internal) != 1 || r.Internal[0].Range != "131.215.0.0/16" || r.Internal[0].Requests != 1 {
		t.Errorf("Internal = %+v", r.Internal)
	}
	if len(r.Days) != 1 || r.Days[0].All != 3 || r.Days[0].Match != 2 || r.Days[0].Clients != 1 {
		t.Errorf("Days = %+v", r.Days)
	}
	var exa FamilyRow
	for _, f := range r.Families {
		if f.Family == "Exa" {
			exa = f
		}
	}
	if exa.Requests != 2 || exa.Limited != 1 || exa.APIRequests != 2 || !near(exa.UpstreamSeconds, 0.25) {
		t.Errorf("Exa = %+v", exa)
	}
	var j, x strings.Builder
	if err := r.WriteJSON(&j); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteText(&x, 15); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"203.0.113", "131.215.1.2", "private", "topic", "/search", "abc12-34", "zzz99-88", "secret-ref", "example.org", "ExaSearchBot", "Mozilla"} {
		if strings.Contains(j.String(), leak) || strings.Contains(x.String(), leak) {
			t.Errorf("the report contains %q", leak)
		}
	}
}

// A log whose format grew: some lines are the stock combined format, which is the
// first nine fields of today's.
func TestCollectCountsLinesFromAnOlderFormAndSaysSo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")
	old := `203.0.113.5 - - [07/Oct/2026:01:00:00 +0000] "GET /api/records/abc12-34 HTTP/1.1" 200 100 "-" "Mozilla/5.0 (Windows NT 10.0) Chrome/148"`
	lines := []string{
		old, old,
		logLine("203.0.113.6", "07/Oct/2026:02:00:00 +0000", "GET /api/records/abc12-34 HTTP/1.1", 200, "Mozilla/5.0 (Windows NT 10.0) Chrome/148", "0.300", "0.250"),
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := sample.CompileTolerant(realFormat)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Collect(path, p, Options{Class: func(string) string { return "api-record" }})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if r.Lines != 3 || r.Events != 3 || r.Older != 2 || r.Skipped != 0 {
		t.Errorf("Lines %d Events %d Older %d Skipped %d, want 3, 3, 2, 0", r.Lines, r.Events, r.Older, r.Skipped)
	}
	notes := strings.Join(r.Notes, "\n")
	if !strings.Contains(notes, "2 of 3 lines") || !strings.Contains(notes, "older") {
		t.Errorf("notes %q should say 2 of 3 lines are in an older format", notes)
	}
	if !strings.Contains(notes, "1 of 3 requests have a request time") {
		t.Errorf("notes %q should also say how many requests have a time", notes)
	}
}
