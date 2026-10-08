package report

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/caltechlibrary/logagent/internal/event"
	"github.com/caltechlibrary/logagent/internal/logread"
)

// at returns the instant s seconds after midnight UTC on the test day.
func sec(s int) time.Time { return d1.Add(time.Duration(s) * time.Second) }

// slot is a request that ended at second end after running rt seconds.
func slot(class string, end int, rt float64, status int) event.Event {
	return event.Event{
		V: event.Version, T: sec(end), Method: "GET", Class: class, Status: status, RT: rt, URT: rt, Timed: true,
		Family: "undeclared Windows", Declared: "none", Who: fmt.Sprintf("c%d", end),
	}
}

// A request is in flight from the second it began to the second it ended, and
// every second in between counts, idle ones included, the way
// bot-concurrency.py counts them.
func TestConcurrencyIsCountedPerSecondFromEndTimeAndRequestTime(t *testing.T) {
	a := newAgg(Options{})
	a.Add(slot("api-record", 10, 3.5, 200)) // starts 6.5, so seconds 6 to 10
	a.Add(slot("api-record", 9, 1.0, 200))  // 8 to 9
	a.Add(slot("api-record", 10, 2.0, 200)) // 8 to 10
	a.Add(slot("api-record", 10, 0.0, 429)) // rejected before it held a slot
	untimed := slot("api-record", 10, 0, 200)
	untimed.Timed = false
	a.Add(untimed)
	a.Add(slot("ui-record", 10, 5.0, 200)) // not an API class
	r := a.Report(logread.Stats{Lines: 6})
	c := r.Concurrency
	if c.Seconds != 5 || c.Requests != 4 || c.Limited != 1 {
		t.Fatalf("Concurrency = %+v, want 5 seconds, 4 timed API requests (the 429 included, as bot-concurrency.py counts them), 1 limited", c)
	}
	// In flight at seconds 6..10: 1, 1, 3, 3, 2.
	all := c.Rows[0]
	if all.Class != "api (all)" || all.P50 != 2 || all.P90 != 3 || all.P99 != 3 || all.Max != 3 {
		t.Errorf("all = %+v, want p50 2, p90 3, p99 3, max 3", all)
	}
	for _, s := range all.Shares {
		want := map[int]int{2: 3, 4: 0}
		if n, ok := want[s.AtLeast]; ok && s.Seconds != n {
			t.Errorf(">= %d: %d seconds, want %d", s.AtLeast, s.Seconds, n)
		}
	}
	if all.Shares[0].AtLeast != 2 || !near(all.Shares[0].Percent, 60) {
		t.Errorf("first share = %+v, want >= 2 at 60%%", all.Shares[0])
	}
}

func TestEachClassIsCountedOverTheSameSpanAsAllOfThem(t *testing.T) {
	a := newAgg(Options{})
	a.Add(slot("api-record", 10, 3.5, 200))
	a.Add(slot("api-record", 9, 1.0, 200))
	a.Add(slot("api-record", 10, 2.0, 200))
	a.Add(slot("api-iiif", 10, 1.0, 200)) // seconds 9 and 10
	c := a.Report(logread.Stats{Lines: 4}).Concurrency
	if c.Seconds != 5 {
		t.Fatalf("Seconds = %d, want 5", c.Seconds)
	}
	by := map[string]ConcurrencyRow{}
	for _, row := range c.Rows {
		by[row.Class] = row
	}
	// All: 1, 1, 3, 4, 3.
	if r := by["api (all)"]; r.P50 != 3 || r.P90 != 4 || r.Max != 4 {
		t.Errorf("all = %+v", r)
	}
	// api-iiif alone: 0, 0, 0, 1, 1 over the same five seconds, so the idle ones count.
	if r := by["api-iiif"]; r.P50 != 0 || r.P90 != 1 || r.Max != 1 || r.Shares[0].Seconds != 0 {
		t.Errorf("api-iiif = %+v", r)
	}
	if _, ok := by["ui-record"]; ok {
		t.Error("a class that is not an API class has a concurrency row")
	}
}

func TestNoSlotHoldingRequestsMeansNoConcurrencyRows(t *testing.T) {
	a := newAgg(Options{})
	a.Add(slot("ui-record", 10, 1, 200))
	if c := a.Report(logread.Stats{Lines: 1}).Concurrency; len(c.Rows) != 0 || c.Seconds != 0 {
		t.Errorf("Concurrency = %+v", c)
	}
}

func cached(class, status string, n int) []event.Event {
	var out []event.Event
	for i := 0; i < n; i++ {
		e := slot(class, 10+i, 0.1, 200)
		e.Cache = status
		out = append(out, e)
	}
	return out
}

func TestTheCacheTallyIsPerClassAndOnlyForClassesThatHaveACacheStatus(t *testing.T) {
	a := newAgg(Options{})
	for _, e := range append(append(cached("api-iiif", "HIT", 2), cached("api-iiif", "MISS", 1)...), cached("api-iiif", "BYPASS", 7)...) {
		a.Add(e)
	}
	a.Add(slot("ui-record", 50, 0.1, 200)) // no cache status at all
	r := a.Report(logread.Stats{Lines: 11})
	if len(r.Cache) != 1 {
		t.Fatalf("Cache = %+v, want one class", r.Cache)
	}
	c := r.Cache[0]
	if c.Class != "api-iiif" || c.Hit != 2 || c.Miss != 1 || c.Bypass != 7 || c.Requests != 10 || !near(c.HitRate, 0.2) {
		t.Errorf("api-iiif = %+v", c)
	}
	if strings.Contains(strings.Join(r.Notes, "\n"), "no cache status") {
		t.Errorf("a note says there is no cache status: %q", r.Notes)
	}
	none := newAgg(Options{})
	none.Add(slot("ui-record", 50, 0.1, 200))
	if n := strings.Join(none.Report(logread.Stats{Lines: 1}).Notes, "\n"); !strings.Contains(n, "no cache status") {
		t.Errorf("notes %q should say the log records no cache status", n)
	}
}

func cohortEvent(i int, plat string, ch bool, country string, declared string, up float64) event.Event {
	return event.Event{
		V: event.Version, T: sec(i), Method: "GET", Class: "ui-record", Status: 200, RT: up, URT: up, Timed: true,
		Family: "undeclared Windows", Declared: declared, Plat: plat, CH: ch, Country: country, Who: fmt.Sprintf("%s-%s-%d", plat, country, i),
	}
}

// DR-0002: a cohort of fewer than 10 distinct clients is folded so the report
// cannot point at one reader.
func TestUndeclaredCohortsAreByPlatformHintAndCountryAndSmallOnesAreFolded(t *testing.T) {
	a := newAgg(Options{})
	for i := 0; i < 12; i++ {
		a.Add(cohortEvent(i, "Windows", true, "US", "none", 2))
	}
	for i := 0; i < 3; i++ {
		a.Add(cohortEvent(100+i, "", false, "CN", "none", 1))
	}
	a.Add(cohortEvent(200, "Linux", true, "US", "unverified", 9)) // declared: not in this section
	r := a.Report(logread.Stats{Lines: 16})
	one := func(rows []CohortRow, name string) CohortRow {
		for _, row := range rows {
			if row.Name == name {
				return row
			}
		}
		return CohortRow{Name: "(missing)"}
	}
	if w := one(r.Cohorts.Platform, "Windows"); w.Requests != 12 || w.Clients != 12 || !near(w.UpstreamSeconds, 24) {
		t.Errorf("Windows = %+v", w)
	}
	if w := one(r.Cohorts.Platform, "other (fewer than 10 clients)"); w.Requests != 3 || w.Clients != 3 {
		t.Errorf("folded platform = %+v", w)
	}
	if got := one(r.Cohorts.Platform, "Linux"); got.Name == "Linux" {
		t.Errorf("a declared agent is in the undeclared cohorts: %+v", got)
	}
	if h := one(r.Cohorts.Hints, "sec-ch-ua sent"); h.Requests != 12 {
		t.Errorf("hints = %+v", r.Cohorts.Hints)
	}
	if h := one(r.Cohorts.Hints, "other (fewer than 10 clients)"); h.Requests != 3 {
		t.Errorf("hints = %+v", r.Cohorts.Hints)
	}
	if c := one(r.Cohorts.Country, "US"); c.Requests != 12 {
		t.Errorf("country = %+v", r.Cohorts.Country)
	}
	if c := one(r.Cohorts.Country, "CN"); c.Name == "CN" {
		t.Errorf("a country of 3 clients was shown by name: %+v", c)
	}
}

func limited(i int, family, class, country string, ch bool, who string) event.Event {
	e := slot(class, i, 0, 429)
	e.Family, e.Country, e.CH, e.Who = family, country, ch, who
	return e
}

func TestThe429sAreBrokenDownByFamilyClassHourCountryAndHints(t *testing.T) {
	a := newAgg(Options{Declared: map[string]bool{"Exa": true}})
	for i := 0; i < 12; i++ {
		a.Add(slot("ui-record", 5+i, 0.1, 200))                                           // clients that are never limited, so countries have 10+ clients
		a.Add(limited(3600*3+i, "Exa", "api-record", "US", false, fmt.Sprintf("u%d", i))) // hour 3
	}
	a.Add(limited(3600*5, "Exa", "api-iiif", "US", true, "u0")) // hour 5
	r := a.Report(logread.Stats{Lines: 25})
	l := r.Limited
	if l.Total != 13 {
		t.Fatalf("Total = %d, want 13", l.Total)
	}
	top := func(rows []CountRow) CountRow { return rows[0] }
	if got := top(l.Family); got.Name != "Exa" || got.Count != 13 {
		t.Errorf("Family = %+v", l.Family)
	}
	if got := top(l.Class); got.Name != "api-record" || got.Count != 12 {
		t.Errorf("Class = %+v", l.Class)
	}
	if got := top(l.Hour); got.Name != "03" || got.Count != 12 {
		t.Errorf("Hour = %+v", l.Hour)
	}
	if got := top(l.Country); got.Name != "US" || got.Count != 13 {
		t.Errorf("Country = %+v", l.Country)
	}
	if got := top(l.Hints); got.Name != "no sec-ch-ua" || got.Count != 12 {
		t.Errorf("Hints = %+v", l.Hints)
	}
}

func TestACountryOfFewerThanTenClientsIsNotNamedAmongThe429s(t *testing.T) {
	a := newAgg(Options{})
	a.Add(limited(10, "undeclared Windows", "api-record", "CN", false, "one-client"))
	r := a.Report(logread.Stats{Lines: 1})
	if got := r.Limited.Country; len(got) != 1 || got[0].Name != "other (fewer than 10 clients)" {
		t.Errorf("Country = %+v, want the one client folded", got)
	}
}

func agentEvent(i int, ua, declared string) event.Event {
	e := slot("ui-record", i, 0.1, 200)
	e.UA, e.Declared = ua, declared
	return e
}

// A user agent is text the client chose. The report prints the ones that name
// an agent and are not in the declared list, quoted, cut, and with any contact
// address reduced to its domain.
func TestSelfDescribedAgentsAreListedCutAndWithTheirContactAddressesReduced(t *testing.T) {
	a := newAgg(Options{})
	for i := 0; i < 3; i++ {
		a.Add(agentEvent(i, "citation-weekend-agent/2.0", "none"))
	}
	for i := 0; i < 2; i++ {
		a.Add(agentEvent(10+i, "x-agent mailto:jane.doe@example.edu", "none"))
	}
	for i := 0; i < 5; i++ {
		a.Add(agentEvent(20+i, "Mozilla/5.0 (Windows NT 10.0) Chrome/148 Safari/537.36", "none")) // a browser: no marker
	}
	a.Add(agentEvent(30, "SomeCrawler/3.0", "unverified")) // declared: not this section
	a.Add(agentEvent(31, "crawler "+strings.Repeat("é", 100), "none"))
	r := a.Report(logread.Stats{Lines: 12})
	got := map[string]int{}
	for _, row := range r.Agents {
		got[row.UserAgent] = row.Requests
	}
	if got["citation-weekend-agent/2.0"] != 3 {
		t.Errorf("Agents = %+v", r.Agents)
	}
	if got["x-agent mailto:<email>@example.edu"] != 2 {
		t.Errorf("the contact address was not reduced: %+v", r.Agents)
	}
	for ua := range got {
		if strings.Contains(ua, "jane.doe") || strings.Contains(ua, "Mozilla") || strings.Contains(ua, "SomeCrawler") {
			t.Errorf("%q should not be listed", ua)
		}
		if len(ua) > 80 {
			t.Errorf("%q is %d bytes, want at most 80", ua, len(ua))
		}
	}
	if r.Agents[0].UserAgent != "citation-weekend-agent/2.0" {
		t.Errorf("the commonest comes first: %+v", r.Agents)
	}
}

func TestTheTextHasAllTenSectionsAndMarksUserAgentsAsData(t *testing.T) {
	a := newAgg(Options{})
	a.Add(agentEvent(1, "citation-weekend-agent/2.0", "none"))
	a.Add(slot("api-record", 10, 2, 200))
	var b strings.Builder
	if err := a.Report(logread.Stats{Lines: 2}).WriteText(&b, 15); err != nil {
		t.Fatal(err)
	}
	text := b.String()
	for _, h := range []string{"6. UNDECLARED COHORTS", "7. CONCURRENCY", "8. CACHE", "9. THE 429s", "10. SELF-DESCRIBED AGENTS"} {
		if !strings.Contains(text, h) {
			t.Errorf("text lacks the heading %q:\n%s", h, text)
		}
	}
	if !strings.Contains(text, "chosen by the client") || !strings.Contains(text, `"citation-weekend-agent/2.0"`) {
		t.Errorf("the agents section should quote the user agent and say it is client-chosen text:\n%s", text)
	}
}

func TestJSONHasTheNewSectionsWithStableKeys(t *testing.T) {
	var b strings.Builder
	if err := newAgg(Options{}).Report(logread.Stats{}).WriteJSON(&b); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"cohorts"`, `"concurrency"`, `"cache"`, `"limited"`, `"agents"`} {
		if !strings.Contains(b.String(), key) {
			t.Errorf("JSON lacks %s", key)
		}
	}
}

func TestEachMarkerWordFindsAnAgentAndABrowserIsNeverListed(t *testing.T) {
	for _, ua := range []string{"somebot/1.0", "WebCrawler 2", "FooSpider/3", "research-agent/1", "tool (mailto:ops@example.org)", "harvest +http://example.org/about"} {
		a := newAgg(Options{})
		a.Add(agentEvent(1, ua, "none"))
		if rows := a.Report(logread.Stats{Lines: 1}).Agents; len(rows) != 1 {
			t.Errorf("%q: Agents = %+v, want it listed", ua, rows)
		}
	}
	for _, ua := range []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_3_1 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 Safari/604.1", "curl/8.0", "",
	} {
		a := newAgg(Options{})
		a.Add(agentEvent(1, ua, "none"))
		if rows := a.Report(logread.Stats{Lines: 1}).Agents; len(rows) != 0 {
			t.Errorf("%q: Agents = %+v, want none", ua, rows)
		}
	}
}

// bot-concurrency.py takes the first value whose cumulative count reaches p of the
// seconds. Ten seconds make the median land exactly on a whole number: five at one
// in flight, five at three.
func TestAPercentileThatFallsExactlyOnASecondCountTakesThatValue(t *testing.T) {
	a := newAgg(Options{})
	for s := 0; s < 5; s++ {
		a.Add(slot("api-record", s, 0, 200)) // one at a time, seconds 0 to 4
	}
	for s := 5; s < 10; s++ {
		for k := 0; k < 3; k++ {
			e := slot("api-record", s, 0, 200)
			e.Who = fmt.Sprintf("c%d-%d", s, k)
			a.Add(e)
		}
	}
	c := a.Report(logread.Stats{Lines: 20}).Concurrency
	if c.Seconds != 10 || c.Rows[0].P50 != 1 || c.Rows[0].P90 != 3 || c.Rows[0].Max != 3 {
		t.Errorf("Concurrency = %+v, want 10 seconds, p50 1 (5 of 10 seconds reach it), p90 3", c)
	}
}

func TestAnAgentIsCutOnACharacterBoundaryNotInTheMiddleOfOne(t *testing.T) {
	a := newAgg(Options{})
	a.Add(agentEvent(1, "crawler  "+strings.Repeat("€", 40), "none")) // byte 80 falls inside a three-byte character
	rows := a.Report(logread.Stats{Lines: 1}).Agents
	if len(rows) != 1 || len(rows[0].UserAgent) != 78 || !utf8.ValidString(rows[0].UserAgent) {
		t.Errorf("Agents = %+v, want 78 valid bytes", rows)
	}
}

func TestSmallUndeclaredFamiliesAreFoldedAmongThe429sAndDeclaredOnesAreKept(t *testing.T) {
	a := newAgg(Options{Declared: map[string]bool{"Exa": true}})
	a.Add(limited(10, "undeclared Mac", "api-record", "US", false, "one-client"))
	a.Add(limited(11, "Exa", "api-record", "US", false, "exa-1"))
	r := a.Report(logread.Stats{Lines: 2})
	names := map[string]int{}
	for _, row := range r.Limited.Family {
		names[row.Name] = row.Count
	}
	if names["Exa"] != 1 || names["other (fewer than 10 clients)"] != 1 || len(names) != 2 {
		t.Errorf("Family = %+v, want Exa and the folded one-client family", r.Limited.Family)
	}
}
