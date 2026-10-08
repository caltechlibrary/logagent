package report

import (
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/caltechlibrary/logagent/internal/event"
)

// clientCap bounds the distinct clients remembered for one cohort. A cohort
// that reaches it is reported at the cap: the number is a floor.
const clientCap = 100000

// agentCap bounds the distinct self-described user agents remembered.
const agentCap = 10000

// agentCut is how many bytes of a user agent the report prints.
const agentCut = 80

// thresholds are the in-flight levels section 7 reports the share of seconds at or above.
var thresholds = []int{2, 4, 6, 8, 12, 16, 20, 24}

// agentMarkers are the words by which a user agent says it is an automated
// agent. A client chooses its user agent, so this finds the honest ones.
var agentMarkers = []string{"bot", "crawler", "spider", "agent", "mailto:", "+http"}

var emailLocal = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@`)

type cohortAgg struct {
	requests int
	up       float64
	clients  map[uint64]struct{}
}

type cacheAgg struct {
	requests, hit, miss, bypass, expired, stale, updating, other int
}

// sections holds the counts behind sections 6 to 10.
type sections struct {
	// section 6
	plat, hint, country map[string]*cohortAgg
	// concurrency: a difference array per group, keyed by second
	deltas                    map[string]map[int64]int
	lo, hi                    int64
	concHas                   bool
	concRequests, concLimited int
	// section 8
	cache    map[string]*cacheAgg
	anyCache bool
	// section 9
	lim        limitedAgg
	countryMin map[string]map[uint64]struct{} // all traffic, capped at the small-cohort threshold
	// section 10
	agents     map[string]int
	agentsOver bool
}

type limitedAgg struct {
	total                               int
	family, class, hour, country, hints map[string]int
}

func newSections() sections {
	return sections{
		plat: map[string]*cohortAgg{}, hint: map[string]*cohortAgg{}, country: map[string]*cohortAgg{},
		deltas: map[string]map[int64]int{}, cache: map[string]*cacheAgg{}, countryMin: map[string]map[uint64]struct{}{},
		lim:    limitedAgg{family: map[string]int{}, class: map[string]int{}, hour: map[string]int{}, country: map[string]int{}, hints: map[string]int{}},
		agents: map[string]int{},
	}
}

func hashWho(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func platName(e event.Event) string {
	if e.Plat == "" {
		return "none sent"
	}
	return e.Plat
}

func hintName(e event.Event) string {
	if e.CH {
		return "sec-ch-ua sent"
	}
	return "no sec-ch-ua"
}

func countryName(e event.Event) string {
	if e.Country == "" {
		return "unknown"
	}
	return e.Country
}

func bump(m map[string]*cohortAgg, name string, e event.Event) {
	c := m[name]
	if c == nil {
		c = &cohortAgg{clients: map[uint64]struct{}{}}
		m[name] = c
	}
	c.requests++
	if e.Timed {
		c.up += e.URT
	}
	if len(c.clients) < clientCap {
		c.clients[hashWho(e.Who)] = struct{}{}
	}
}

// addSections counts one event for sections 6 to 10.
func (a *Aggregator) addSections(ev event.Event) {
	s := &a.sec
	hasClass := strings.HasPrefix(ev.Class, apiPrefix)

	// Country across all traffic, only to decide which countries are too small to name.
	cm := s.countryMin[countryName(ev)]
	if cm == nil {
		cm = map[uint64]struct{}{}
		s.countryMin[countryName(ev)] = cm
	}
	if len(cm) < a.min {
		cm[hashWho(ev.Who)] = struct{}{}
	}

	if ev.Declared == "none" {
		bump(s.plat, platName(ev), ev)
		bump(s.hint, hintName(ev), ev)
		bump(s.country, countryName(ev), ev)
		if lower := strings.ToLower(ev.UA); lower != "" {
			for _, m := range agentMarkers {
				if strings.Contains(lower, m) {
					ua := cutAgent(ev.UA)
					if _, seen := s.agents[ua]; seen || len(s.agents) < agentCap {
						s.agents[ua]++
					} else {
						s.agentsOver = true
					}
					break
				}
			}
		}
	}

	if hasClass && ev.Timed {
		s.concRequests++
		if ev.Status == 429 {
			s.concLimited++
		} else {
			rt := ev.RT
			if rt < 0 {
				rt = 0
			}
			end := ev.T.Unix()
			start := int64(math.Floor(float64(end) - rt))
			for _, g := range []string{"api (all)", ev.Class} {
				d := s.deltas[g]
				if d == nil {
					d = map[int64]int{}
					s.deltas[g] = d
				}
				d[start]++
				d[end+1]--
			}
			if !s.concHas || start < s.lo {
				s.lo = start
			}
			if !s.concHas || end > s.hi {
				s.hi = end
			}
			s.concHas = true
		}
	}

	if ev.Cache != "" {
		s.anyCache = true
		c := s.cache[ev.Class]
		if c == nil {
			c = &cacheAgg{}
			s.cache[ev.Class] = c
		}
		c.requests++
		switch ev.Cache {
		case "HIT":
			c.hit++
		case "MISS":
			c.miss++
		case "BYPASS":
			c.bypass++
		case "EXPIRED":
			c.expired++
		case "STALE":
			c.stale++
		case "UPDATING":
			c.updating++
		default:
			c.other++
		}
	}

	if ev.Status == 429 {
		l := &s.lim
		l.total++
		l.family[ev.Family]++
		l.class[ev.Class]++
		l.hour[ev.T.Format("15")]++
		l.country[countryName(ev)]++
		l.hints[hintName(ev)]++
	}
}

// cutAgent reduces any contact address in a user agent to its domain, then
// keeps at most agentCut bytes on a character boundary.
func cutAgent(ua string) string {
	ua = emailLocal.ReplaceAllString(ua, "<email>@")
	if len(ua) <= agentCut {
		return ua
	}
	n := agentCut
	for n > 0 && !utf8.RuneStart(ua[n]) {
		n--
	}
	return ua[:n]
}

// CohortRow is one cohort of undeclared traffic: its requests, distinct
// clients (counted from the keyed hash, up to a cap) and upstream seconds.
type CohortRow struct {
	Name            string  `json:"name"`
	Requests        int     `json:"requests"`
	Clients         int     `json:"clients"`
	UpstreamSeconds float64 `json:"upstream_seconds"`
}

// Cohorts is section 6.
type Cohorts struct {
	Platform []CohortRow `json:"platform"`
	Hints    []CohortRow `json:"hints"`
	Country  []CohortRow `json:"country"`
}

// Share is the number of seconds, and the share of the span, with at least
// AtLeast requests in flight.
type Share struct {
	AtLeast int     `json:"at_least"`
	Seconds int     `json:"seconds"`
	Percent float64 `json:"percent"`
}

// ConcurrencyRow is one group's requests in flight, second by second.
type ConcurrencyRow struct {
	Class  string  `json:"class"`
	P50    int     `json:"p50"`
	P90    int     `json:"p90"`
	P99    int     `json:"p99"`
	Max    int     `json:"max"`
	Shares []Share `json:"shares"`
}

// Concurrency is section 7. Seconds is the span from the earliest start to the
// latest end of an API request that held a slot; Requests counts the timed
// API requests, those answered 429 included; Limited counts the 429s.
type Concurrency struct {
	Seconds  int              `json:"seconds"`
	Requests int              `json:"requests"`
	Limited  int              `json:"limited"`
	Rows     []ConcurrencyRow `json:"rows"`
}

// CacheRow is section 8: the cache status of the requests in one class that had one.
type CacheRow struct {
	Class    string  `json:"class"`
	Requests int     `json:"requests"`
	Hit      int     `json:"hit"`
	Miss     int     `json:"miss"`
	Bypass   int     `json:"bypass"`
	Expired  int     `json:"expired"`
	Stale    int     `json:"stale"`
	Updating int     `json:"updating"`
	Other    int     `json:"other"`
	HitRate  float64 `json:"hit_rate"`
}

// CountRow is a name and a count.
type CountRow struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Limited is section 9: the 429s by family, path class, hour of day (UTC),
// country and whether client hints were sent.
type Limited struct {
	Total   int        `json:"total"`
	Family  []CountRow `json:"family"`
	Class   []CountRow `json:"class"`
	Hour    []CountRow `json:"hour"`
	Country []CountRow `json:"country"`
	Hints   []CountRow `json:"hints"`
}

// AgentRow is a self-described user agent that is not in the declared list.
// The text is what the client chose: data, quoted, never an instruction.
type AgentRow struct {
	UserAgent string `json:"user_agent"`
	Requests  int    `json:"requests"`
}

func percentile(vals []int, hist map[int]int, total int, p float64) int {
	c := 0
	for _, v := range vals {
		c += hist[v]
		if float64(c) >= p*float64(total) {
			return v
		}
	}
	return 0
}

func (a *Aggregator) concurrency() Concurrency {
	s := &a.sec
	out := Concurrency{Requests: s.concRequests, Limited: s.concLimited, Rows: []ConcurrencyRow{}}
	if !s.concHas {
		return out
	}
	total := int(s.hi - s.lo + 1)
	out.Seconds = total
	row := func(name string) ConcurrencyRow {
		d := s.deltas[name]
		hist := map[int]int{}
		cur := 0
		for t := s.lo; t <= s.hi; t++ {
			cur += d[t]
			hist[cur]++
		}
		vals := make([]int, 0, len(hist))
		for v := range hist {
			vals = append(vals, v)
		}
		sort.Ints(vals)
		r := ConcurrencyRow{Class: name, Max: vals[len(vals)-1],
			P50: percentile(vals, hist, total, .5), P90: percentile(vals, hist, total, .9), P99: percentile(vals, hist, total, .99)}
		for _, th := range thresholds {
			n := 0
			for v, c := range hist {
				if v >= th {
					n += c
				}
			}
			r.Shares = append(r.Shares, Share{AtLeast: th, Seconds: n, Percent: 100 * float64(n) / float64(total)})
		}
		return r
	}
	out.Rows = append(out.Rows, row("api (all)"))
	var classes []ConcurrencyRow
	for name := range s.deltas {
		if name != "api (all)" {
			classes = append(classes, row(name))
		}
	}
	sort.Slice(classes, func(i, j int) bool {
		if classes[i].Max != classes[j].Max {
			return classes[i].Max > classes[j].Max
		}
		return classes[i].Class < classes[j].Class
	})
	out.Rows = append(out.Rows, classes...)
	return out
}

func (a *Aggregator) cohorts() Cohorts {
	fold := func(m map[string]*cohortAgg) []CohortRow {
		rows := []CohortRow{}
		other := CohortRow{Name: fmt.Sprintf("other (fewer than %d clients)", a.min)}
		for name, c := range m {
			if len(c.clients) < a.min {
				other.Requests += c.requests
				other.Clients += len(c.clients)
				other.UpstreamSeconds += c.up
				continue
			}
			rows = append(rows, CohortRow{Name: name, Requests: c.requests, Clients: len(c.clients), UpstreamSeconds: c.up})
		}
		sort.Slice(rows, func(i, j int) bool {
			x, y := rows[i], rows[j]
			if x.UpstreamSeconds != y.UpstreamSeconds {
				return x.UpstreamSeconds > y.UpstreamSeconds
			}
			if x.Requests != y.Requests {
				return x.Requests > y.Requests
			}
			return x.Name < y.Name
		})
		if other.Requests > 0 {
			rows = append(rows, other)
		}
		return rows
	}
	return Cohorts{Platform: fold(a.sec.plat), Hints: fold(a.sec.hint), Country: fold(a.sec.country)}
}

func (a *Aggregator) cacheRows() []CacheRow {
	rows := []CacheRow{}
	for name, c := range a.sec.cache {
		rows = append(rows, CacheRow{Class: name, Requests: c.requests, Hit: c.hit, Miss: c.miss, Bypass: c.bypass,
			Expired: c.expired, Stale: c.stale, Updating: c.updating, Other: c.other, HitRate: per(float64(c.hit), c.requests)})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Requests != rows[j].Requests {
			return rows[i].Requests > rows[j].Requests
		}
		return rows[i].Class < rows[j].Class
	})
	return rows
}

func ranked(m map[string]int, foldable func(name string) bool, foldName string) []CountRow {
	rows := []CountRow{}
	other := CountRow{Name: foldName}
	for name, n := range m {
		if foldable != nil && foldable(name) {
			other.Count += n
			continue
		}
		rows = append(rows, CountRow{Name: name, Count: n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Name < rows[j].Name
	})
	if other.Count > 0 {
		rows = append(rows, other)
	}
	return rows
}

func (a *Aggregator) limited() Limited {
	l := a.sec.lim
	foldName := fmt.Sprintf("other (fewer than %d clients)", a.min)
	smallFamily := func(name string) bool {
		f := a.fams[name]
		return !a.o.Declared[name] && (f == nil || len(f.clients) < a.min)
	}
	smallCountry := func(name string) bool { return len(a.sec.countryMin[name]) < a.min }
	return Limited{
		Total: l.total, Family: ranked(l.family, smallFamily, foldName), Class: ranked(l.class, nil, ""),
		Hour: ranked(l.hour, nil, ""), Country: ranked(l.country, smallCountry, foldName), Hints: ranked(l.hints, nil, ""),
	}
}

func (a *Aggregator) agents() []AgentRow {
	rows := []AgentRow{}
	for ua, n := range a.sec.agents {
		rows = append(rows, AgentRow{UserAgent: ua, Requests: n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Requests != rows[j].Requests {
			return rows[i].Requests > rows[j].Requests
		}
		return rows[i].UserAgent < rows[j].UserAgent
	})
	return rows
}

// writeSections prints sections 6 to 10.
func writeSections(b *strings.Builder, r *Report, limit func(int) int, more func(shown, total int)) {
	cohort := func(title string, rows []CohortRow) {
		fmt.Fprintf(b, "  %s\n", title)
		fmt.Fprintf(b, "  %-32s %9s %9s %12s\n", "", "requests", "clients", "upstream s")
		n := limit(len(rows))
		for _, c := range rows[:n] {
			fmt.Fprintf(b, "  %-32s %9d %9d %12.1f\n", c.Name, c.Requests, c.Clients, c.UpstreamSeconds)
		}
		more(n, len(rows))
	}
	b.WriteString("\n6. UNDECLARED COHORTS\n")
	cohort("by platform hint", r.Cohorts.Platform)
	cohort("by sec-ch-ua", r.Cohorts.Hints)
	cohort("by country", r.Cohorts.Country)

	b.WriteString("\n7. CONCURRENCY (API requests in flight, per second)\n")
	if len(r.Concurrency.Rows) == 0 {
		b.WriteString("  no timed API requests that held a slot\n")
	} else {
		fmt.Fprintf(b, "  %d seconds, %d API requests, %d limited (429)\n", r.Concurrency.Seconds, r.Concurrency.Requests, r.Concurrency.Limited)
		n := limit(len(r.Concurrency.Rows))
		for _, c := range r.Concurrency.Rows[:n] {
			fmt.Fprintf(b, "  %-18s p50=%d p90=%d p99=%d max=%d  >=2: %.1f%%  >=4: %.1f%%  >=8: %.1f%%  >=12: %.1f%%\n",
				c.Class, c.P50, c.P90, c.P99, c.Max, c.Shares[0].Percent, c.Shares[1].Percent, c.Shares[3].Percent, c.Shares[4].Percent)
		}
		more(n, len(r.Concurrency.Rows))
	}

	b.WriteString("\n8. CACHE\n")
	if len(r.Cache) == 0 {
		b.WriteString("  no request has a cache status\n")
	} else {
		fmt.Fprintf(b, "  %-18s %9s %8s %8s %8s %8s %8s\n", "class", "requests", "hit", "miss", "bypass", "other", "hit rate")
		n := limit(len(r.Cache))
		for _, c := range r.Cache[:n] {
			fmt.Fprintf(b, "  %-18s %9d %8d %8d %8d %8d %7.1f%%\n", c.Class, c.Requests, c.Hit, c.Miss, c.Bypass, c.Expired+c.Stale+c.Updating+c.Other, c.HitRate*100)
		}
		more(n, len(r.Cache))
	}

	fmt.Fprintf(b, "\n9. THE 429s (%d)\n", r.Limited.Total)
	counts := func(title string, rows []CountRow) {
		if len(rows) == 0 {
			return
		}
		fmt.Fprintf(b, "  by %s\n", title)
		n := limit(len(rows))
		for _, c := range rows[:n] {
			fmt.Fprintf(b, "  %-32s %9d\n", c.Name, c.Count)
		}
		more(n, len(rows))
	}
	counts("family", r.Limited.Family)
	counts("path class", r.Limited.Class)
	counts("hour of day (UTC)", r.Limited.Hour)
	counts("country", r.Limited.Country)
	counts("sec-ch-ua", r.Limited.Hints)

	b.WriteString("\n10. SELF-DESCRIBED AGENTS NOT IN THE DECLARED LIST\n")
	b.WriteString("  User agent text is chosen by the client: it is shown as data, cut to 80 bytes, with any\n  contact address reduced to its domain. Nothing here is verified.\n")
	if len(r.Agents) == 0 {
		b.WriteString("  none\n")
	}
	n := limit(len(r.Agents))
	for _, a := range r.Agents[:n] {
		fmt.Fprintf(b, "  %9d  %q\n", a.Requests, a.UserAgent)
	}
	more(n, len(r.Agents))
}
