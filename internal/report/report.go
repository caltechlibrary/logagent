// Package report reads an access log once, reduces each request to an event,
// counts the events and prints what they show: how much traffic there is, what
// it costs the application, and who it claims to be. It stores nothing
// (DR-0006). The report holds no address, path, query string or referer: an
// event never has them, and the counts are by class, family, status and day.
package report

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/caltechlibrary/logagent/internal/event"
	"github.com/caltechlibrary/logagent/internal/logread"
	"github.com/caltechlibrary/logagent/internal/sample"
)

// apiPrefix is what a path class's name begins with when it is application
// traffic. It is a naming convention of the host's class rules (the example
// file's api-iiif, api-record and so on), not a built-in list of classes.
const apiPrefix = "api-"

// defaultMinClients is DR-0002's threshold: a family with fewer distinct
// clients than this, and no declared name, is folded into one row.
const defaultMinClients = 10

// Options says what to read and how to name what is read.
type Options struct {
	// Window is the span of event time to read.
	Window logread.Window
	// Top is how many rows the ranked tables print; 0 prints them all.
	Top int
	// Match is a case-folded part of a family name; section 2 counts those requests
	// and their distinct clients beside all traffic. Empty means no match column.
	Match string
	// Class names a path, whose query string is already removed. Nil leaves every class empty.
	Class func(path string) string
	// Lookup names a user agent's family and says whether it is a declared automated agent.
	Lookup func(userAgent string) (string, bool)
	// Declared lists the families that are declared automated agents, kept even when small.
	// Collect fills it from Lookup as families are seen.
	Declared map[string]bool
	// Internal lists the address ranges whose requests are counted and nothing more.
	Internal []netip.Prefix
	// NoClasses is true when the host configures no path classes.
	NoClasses bool
	// MinClients overrides the small-cohort threshold; 0 means 10.
	MinClients int
	// Key is the secret for the keyed hash of an address; empty means a random one that
	// is used for this run and never written.
	Key []byte
}

type dayAgg struct {
	all, match int
	clients    map[string]struct{}
}

type hourAgg struct{ all, match int }

type classAgg struct {
	requests, timed int
	up              float64
}

type famAgg struct {
	requests, api, timed, limited int
	up, apiUp                     float64
	clients                       map[string]struct{} // at most the threshold: only "fewer than" is asked
}

// Aggregator counts events. It keeps counts, never events.
type Aggregator struct {
	o        Options
	min      int
	events   int
	timed    int
	days     map[string]*dayAgg
	hours    map[string]*hourAgg
	status   map[int]int
	errs     map[string]*ErrorDay
	classes  map[string]*classAgg
	fams     map[string]*famAgg
	internal map[string]int
	seenInt  bool
}

// New returns an Aggregator for the options.
//
// @param o {Options} what to count and how
// @returns {*Aggregator} an empty aggregator
// @example
//
//	a := report.New(report.Options{Top: 15})
//	a.Add(ev)
func New(o Options) *Aggregator {
	min := o.MinClients
	if min <= 0 {
		min = defaultMinClients
	}
	return &Aggregator{
		o: o, min: min, days: map[string]*dayAgg{}, hours: map[string]*hourAgg{}, status: map[int]int{},
		errs: map[string]*ErrorDay{}, classes: map[string]*classAgg{}, fams: map[string]*famAgg{}, internal: map[string]int{},
	}
}

// AddInternal counts one request from an internal range: it is counted and is not an event.
//
// @param r {netip.Prefix} the range that matched
// @example
//
//	a.AddInternal(netip.MustParsePrefix("131.215.0.0/16"))
func (a *Aggregator) AddInternal(r netip.Prefix) {
	a.internal[r.String()]++
	a.seenInt = true
}

// Add counts one event.
//
// @param ev {event.Event} the event
// @example
//
//	a.Add(ev)
func (a *Aggregator) Add(ev event.Event) {
	a.events++
	if ev.Timed {
		a.timed++
	}
	day, hour := ev.T.Format("2006-01-02"), ev.T.Format("2006-01-02 15")
	matched := a.o.Match != "" && strings.Contains(strings.ToLower(ev.Family), strings.ToLower(a.o.Match))
	d := a.days[day]
	if d == nil {
		d = &dayAgg{clients: map[string]struct{}{}}
		a.days[day] = d
	}
	d.all++
	h := a.hours[hour]
	if h == nil {
		h = &hourAgg{}
		a.hours[hour] = h
	}
	h.all++
	if matched {
		d.match++
		h.match++
		d.clients[ev.Who] = struct{}{}
	}
	a.status[ev.Status]++
	e := a.errs[day]
	if e == nil {
		e = &ErrorDay{Day: day}
		a.errs[day] = e
	}
	switch ev.Status {
	case 429:
		e.S429++
	case 499:
		e.S499++
	case 502:
		e.S502++
	case 504:
		e.S504++
	}
	c := a.classes[ev.Class]
	if c == nil {
		c = &classAgg{}
		a.classes[ev.Class] = c
	}
	c.requests++
	f := a.fams[ev.Family]
	if f == nil {
		f = &famAgg{clients: map[string]struct{}{}}
		a.fams[ev.Family] = f
	}
	f.requests++
	if len(f.clients) < a.min {
		f.clients[ev.Who] = struct{}{}
	}
	isAPI := strings.HasPrefix(ev.Class, apiPrefix)
	if isAPI {
		f.api++
	}
	if ev.Status == 429 {
		f.limited++
	}
	if ev.Timed {
		c.timed++
		c.up += ev.URT
		f.timed++
		f.up += ev.URT
		if isAPI {
			f.apiUp += ev.URT
		}
	}
}

// DayRow is section 2: one UTC day.
type DayRow struct {
	Day string `json:"day"`
	All int    `json:"all"`
	// Match and Clients are for the requests whose family matched Options.Match.
	Match   int `json:"match"`
	Clients int `json:"clients"`
}

// HourRow is section 2: one UTC hour.
type HourRow struct {
	Hour  string `json:"hour"`
	All   int    `json:"all"`
	Match int    `json:"match"`
}

// StatusRow is section 3: one response status.
type StatusRow struct {
	Status int `json:"status"`
	Count  int `json:"count"`
}

// ErrorDay is section 3: the statuses that mean our caps (429), a client that
// gave up (499) and the upstream failing (502, 504), for one UTC day.
type ErrorDay struct {
	Day  string `json:"day"`
	S429 int    `json:"429"`
	S499 int    `json:"499"`
	S502 int    `json:"502"`
	S504 int    `json:"504"`
}

// ClassRow is section 4: one path class. PerRequest is upstream seconds over
// the timed requests only.
type ClassRow struct {
	Class           string  `json:"class"`
	Requests        int     `json:"requests"`
	Timed           int     `json:"timed"`
	UpstreamSeconds float64 `json:"upstream_seconds"`
	PerRequest      float64 `json:"seconds_per_request"`
}

// FamilyRow is section 5: one family of client.
type FamilyRow struct {
	Family          string  `json:"family"`
	Declared        bool    `json:"declared"`
	Requests        int     `json:"requests"`
	APIRequests     int     `json:"api_requests"`
	APIShare        float64 `json:"api_share"`
	Timed           int     `json:"timed"`
	UpstreamSeconds float64 `json:"upstream_seconds"`
	APIUpstream     float64 `json:"api_upstream_seconds"`
	PerRequest      float64 `json:"seconds_per_request"`
	Limited         int     `json:"limited"`
	LimitedRate     float64 `json:"limited_rate"`
}

// InternalRow is a range whose requests were counted and not stored.
type InternalRow struct {
	Range    string `json:"range"`
	Requests int    `json:"requests"`
}

// Window is the span of time the delivered lines cover.
type Window struct {
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
}

// Report is what was counted. Its JSON form has stable keys.
type Report struct {
	Window   Window        `json:"window"`
	Files    []string      `json:"files"`
	Lines    int           `json:"lines"`
	Events   int           `json:"events"`
	Skipped  int           `json:"skipped"`
	Outside  int           `json:"outside"`
	Internal []InternalRow `json:"internal"`
	Notes    []string      `json:"notes"`
	Days     []DayRow      `json:"days"`
	Hours    []HourRow     `json:"hours"`
	Status   []StatusRow   `json:"status"`
	Errors   []ErrorDay    `json:"errors"`
	Classes  []ClassRow    `json:"classes"`
	Families []FamilyRow   `json:"families"`
}

func per(total float64, n int) float64 {
	if n == 0 {
		return 0
	}
	return total / float64(n)
}

// Report returns what has been counted. Families with fewer distinct clients
// than the threshold and no declared name are folded into one row (DR-0002).
//
// @param st {logread.Stats} what the reader covered, for the sources section
// @returns {*Report} the counts
// @example
//
//	r := a.Report(stats)
//	fmt.Println(r.Events)
func (a *Aggregator) Report(st logread.Stats) *Report {
	r := &Report{
		Window: Window{First: st.First, Last: st.Last}, Files: append([]string{}, st.Files...),
		Lines: st.Lines, Events: a.events, Skipped: st.Skipped, Outside: st.Outside,
		Internal: []InternalRow{}, Notes: []string{}, Days: []DayRow{}, Hours: []HourRow{},
		Status: []StatusRow{}, Errors: []ErrorDay{}, Classes: []ClassRow{}, Families: []FamilyRow{},
	}
	counted := map[string]bool{}
	for _, p := range a.o.Internal {
		counted[p.String()] = true
		r.Internal = append(r.Internal, InternalRow{Range: p.String(), Requests: a.internal[p.String()]})
	}
	for k, n := range a.internal {
		if !counted[k] {
			r.Internal = append(r.Internal, InternalRow{Range: k, Requests: n})
		}
	}
	sort.SliceStable(r.Internal, func(i, j int) bool {
		if r.Internal[i].Requests != r.Internal[j].Requests {
			return r.Internal[i].Requests > r.Internal[j].Requests
		}
		return r.Internal[i].Range < r.Internal[j].Range
	})
	if len(a.o.Internal) == 0 && !a.seenInt {
		r.Notes = append(r.Notes, "no internal ranges are configured (internal_ranges), so campus and other internal traffic is counted with everything else")
	}
	if a.o.NoClasses {
		r.Notes = append(r.Notes, "no path classes are configured (classes), so every request is one class")
	}
	switch {
	case a.events > 0 && a.timed == 0:
		r.Notes = append(r.Notes, "the log records no request time, so upstream seconds are not available")
	case a.timed > 0 && a.timed < a.events:
		r.Notes = append(r.Notes, fmt.Sprintf("%d of %d requests have a request time; per-request figures use those only", a.timed, a.events))
	}

	for day, d := range a.days {
		r.Days = append(r.Days, DayRow{Day: day, All: d.all, Match: d.match, Clients: len(d.clients)})
	}
	sort.Slice(r.Days, func(i, j int) bool { return r.Days[i].Day < r.Days[j].Day })
	for hour, h := range a.hours {
		r.Hours = append(r.Hours, HourRow{Hour: hour, All: h.all, Match: h.match})
	}
	sort.Slice(r.Hours, func(i, j int) bool { return r.Hours[i].Hour < r.Hours[j].Hour })
	if len(r.Hours) > 48 {
		r.Hours = r.Hours[len(r.Hours)-48:]
	}
	for s, n := range a.status {
		r.Status = append(r.Status, StatusRow{Status: s, Count: n})
	}
	sort.Slice(r.Status, func(i, j int) bool {
		if r.Status[i].Count != r.Status[j].Count {
			return r.Status[i].Count > r.Status[j].Count
		}
		return r.Status[i].Status < r.Status[j].Status
	})
	for _, e := range a.errs {
		r.Errors = append(r.Errors, *e)
	}
	sort.Slice(r.Errors, func(i, j int) bool { return r.Errors[i].Day < r.Errors[j].Day })
	for name, c := range a.classes {
		r.Classes = append(r.Classes, ClassRow{Class: name, Requests: c.requests, Timed: c.timed, UpstreamSeconds: c.up, PerRequest: per(c.up, c.timed)})
	}
	sort.Slice(r.Classes, func(i, j int) bool {
		x, y := r.Classes[i], r.Classes[j]
		if x.UpstreamSeconds != y.UpstreamSeconds {
			return x.UpstreamSeconds > y.UpstreamSeconds
		}
		if x.Requests != y.Requests {
			return x.Requests > y.Requests
		}
		return x.Class < y.Class
	})

	fold := &famAgg{}
	foldName := fmt.Sprintf("other (fewer than %d clients)", a.min)
	for name, f := range a.fams {
		if !a.o.Declared[name] && len(f.clients) < a.min {
			fold.requests += f.requests
			fold.api += f.api
			fold.timed += f.timed
			fold.limited += f.limited
			fold.up += f.up
			fold.apiUp += f.apiUp
			continue
		}
		r.Families = append(r.Families, famRow(name, a.o.Declared[name], f))
	}
	if fold.requests > 0 {
		r.Families = append(r.Families, famRow(foldName, false, fold))
	}
	sort.Slice(r.Families, func(i, j int) bool {
		x, y := r.Families[i], r.Families[j]
		if x.UpstreamSeconds != y.UpstreamSeconds {
			return x.UpstreamSeconds > y.UpstreamSeconds
		}
		if x.Requests != y.Requests {
			return x.Requests > y.Requests
		}
		return x.Family < y.Family
	})
	return r
}

func famRow(name string, declared bool, f *famAgg) FamilyRow {
	return FamilyRow{
		Family: name, Declared: declared, Requests: f.requests, APIRequests: f.api,
		APIShare: per(float64(f.api), f.requests), Timed: f.timed, UpstreamSeconds: f.up,
		APIUpstream: f.apiUp, PerRequest: per(f.up, f.timed), Limited: f.limited,
		LimitedRate: per(float64(f.limited), f.requests),
	}
}

// Collect reads the log at path through the window, reduces each line to an
// event, counts it and returns the report. A request from an internal range is
// counted and not made an event. A line that does not match the format, or has
// a status or time that cannot be read, is counted as skipped.
//
// @param path {string} the live access log; its rotated and gzipped files are read too
// @param p {*sample.Parser} the reader for the log's format
// @param o {Options} the window, lookups and ranges
// @returns {*Report} the counts, also when an error is returned after some lines were read
// @returns {error} what logread.Read returns, such as an error matching fs.ErrNotExist or logread.ErrMismatch
// @example
//
//	r, err := report.Collect("/var/log/nginx/access.log", parser, report.Options{Top: 15})
func Collect(path string, p *sample.Parser, o Options) (*Report, error) {
	key := o.Key
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
	}
	if o.Declared == nil {
		o.Declared = map[string]bool{}
	}
	eo := event.Options{Key: key, Internal: o.Internal, Class: o.Class}
	if o.Lookup != nil {
		eo.Family = func(ua string) (string, bool) {
			name, declared := o.Lookup(ua)
			if declared {
				o.Declared[name] = true
			}
			return name, declared
		}
	}
	a := New(o)
	invalid := 0
	st, err := logread.Read(path, p, o.Window, func(v sample.Values, _ time.Time) {
		ev, out, berr := event.Build(v, eo)
		switch {
		case berr != nil:
			invalid++
		case !out.Kept:
			a.AddInternal(out.Excluded)
		default:
			a.Add(ev)
		}
	})
	st.Skipped += invalid
	st.Lines -= invalid
	return a.Report(st), err
}

// WriteJSON writes the report as indented JSON with stable keys.
//
// @param w {io.Writer} where to write
// @returns {error} an error from writing
// @example
//
//	err := r.WriteJSON(os.Stdout)
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteText writes the report for a person. The ranked tables (status, path
// class, family) print their top rows and say how many were left out.
//
// @param w {io.Writer} where to write
// @param top {int} how many rows of a ranked table to print; 0 prints them all
// @returns {error} an error from writing
// @example
//
//	err := r.WriteText(os.Stdout, 15)
func (r *Report) WriteText(w io.Writer, top int) error {
	var b strings.Builder
	more := func(shown, total int) {
		if total > shown {
			fmt.Fprintf(&b, "  ... %d more\n", total-shown)
		}
	}
	limit := func(n int) int {
		if top > 0 && n > top {
			return top
		}
		return n
	}

	b.WriteString("1. SOURCES\n")
	if !r.Window.First.IsZero() {
		fmt.Fprintf(&b, "  window    %s to %s (UTC)\n", r.Window.First.UTC().Format("2006-01-02 15:04:05"), r.Window.Last.UTC().Format("2006-01-02 15:04:05"))
	}
	for _, f := range r.Files {
		fmt.Fprintf(&b, "  file      %s\n", f)
	}
	fmt.Fprintf(&b, "  lines     %d read, %d events, %d skipped, %d outside the window\n", r.Lines, r.Events, r.Skipped, r.Outside)
	for _, in := range r.Internal {
		fmt.Fprintf(&b, "  internal  %-20s %d requests counted, not stored\n", in.Range, in.Requests)
	}
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "  note      %s\n", n)
	}

	b.WriteString("\n2. REQUESTS PER DAY (UTC)\n")
	fmt.Fprintf(&b, "  %-12s %10s %10s %8s\n", "day", "all", "match", "clients")
	for _, d := range r.Days {
		fmt.Fprintf(&b, "  %-12s %10d %10d %8d\n", d.Day, d.All, d.Match, d.Clients)
	}
	b.WriteString("  per hour, the last 48 hours\n")
	for _, h := range r.Hours {
		fmt.Fprintf(&b, "  %-14s %10d %10d\n", h.Hour, h.All, h.Match)
	}

	b.WriteString("\n3. STATUS\n")
	n := limit(len(r.Status))
	for _, s := range r.Status[:n] {
		fmt.Fprintf(&b, "  %-6d %10d\n", s.Status, s.Count)
	}
	more(n, len(r.Status))
	fmt.Fprintf(&b, "  %-12s %8s %8s %8s %8s\n", "day", "429", "499", "502", "504")
	for _, e := range r.Errors {
		fmt.Fprintf(&b, "  %-12s %8d %8d %8d %8d\n", e.Day, e.S429, e.S499, e.S502, e.S504)
	}

	b.WriteString("\n4. BY PATH CLASS\n")
	fmt.Fprintf(&b, "  %-18s %10s %10s %12s %8s\n", "class", "requests", "timed", "upstream s", "s/req")
	n = limit(len(r.Classes))
	for _, c := range r.Classes[:n] {
		fmt.Fprintf(&b, "  %-18s %10d %10d %12.1f %8.3f\n", c.Class, c.Requests, c.Timed, c.UpstreamSeconds, c.PerRequest)
	}
	more(n, len(r.Classes))

	b.WriteString("\n5. BY FAMILY\n")
	fmt.Fprintf(&b, "  %-30s %9s %7s %12s %8s %7s\n", "family", "requests", "api", "upstream s", "s/req", "429")
	n = limit(len(r.Families))
	for _, f := range r.Families[:n] {
		fmt.Fprintf(&b, "  %-30s %9d %6.1f%% %12.1f %8.3f %6.1f%%\n", f.Family, f.Requests, f.APIShare*100, f.UpstreamSeconds, f.PerRequest, f.LimitedRate*100)
	}
	more(n, len(r.Families))
	_, err := io.WriteString(w, b.String())
	return err
}
