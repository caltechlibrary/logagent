// Package check reads a web server's configuration and says whether the data
// the detection tiers need is being logged. It reads configuration and counts
// supplied by the caller, changes nothing, and holds no log content: its
// output is made to be pasted into an issue (DR-0003, DR-0002).
//
// This version checks nginx: the log formats each access log uses, the
// include order that decides whether nginx accepts them, access_log off, and
// the real-IP setup behind a proxy. Apache is reported as unsupported.
package check

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/errorlog"
	"github.com/caltechlibrary/logagent/internal/fields"
	"github.com/caltechlibrary/logagent/internal/logrotate"
	"github.com/caltechlibrary/logagent/internal/nginxconf"
	"github.com/caltechlibrary/logagent/internal/proxyranges"
)

var (
	// ErrUnsupported means the configured server is not one check can read yet
	// (Apache, until the host survey exists).
	ErrUnsupported = errors.New("unsupported server")

	// ErrNoServers means the configuration holds no server block under http.
	ErrNoServers = errors.New("no server blocks in the configuration")
)

// Severities of a Finding.
const (
	// Gap means the data the tiers need is not being logged, or the
	// configuration is wrong. A gap makes the exit status 1.
	Gap = "gap"
	// Warn means something worth a person's attention that loses no required data.
	Warn = "warn"
	// Note means an optional improvement.
	Note = "note"
)

// builtinCombined is nginx's built-in combined log format.
const builtinCombined = `$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"`

// Place is a position in a configuration file.
type Place struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// Sample holds counts taken from a log, never its content.
type Sample struct {
	// Lines is how many log lines were counted.
	Lines int `json:"lines"`
	// Skipped is how many lines did not match the log format.
	Skipped int `json:"skipped"`
	// Err says why a sample could not be taken; the counts are then not used.
	Err string `json:"error"`
	// Present maps a field name to the number of lines in which it held a
	// value other than a dash. A field with no entry is not judged.
	Present map[string]int `json:"present"`
}

// Input is everything Run needs.
type Input struct {
	// Config is the host's logagent configuration.
	Config *config.Config
	// Dump is the parsed `nginx -T` text.
	Dump *nginxconf.Dump
	// Table is the field table; nil means fields.Default().
	Table *fields.Table
	// Samples holds optional counts, keyed by log path.
	Samples map[string]Sample
	// Sampler, if set, is asked for the counts of each log that has no entry in
	// Samples. It is given the log's path and its format text, the quoted
	// pieces joined with nothing between them as nginx does.
	Sampler func(path, format string) Sample
	// Ranges is the proxy's list of published ranges; nil means the list built
	// into the program.
	Ranges *proxyranges.Snapshot
	// RangesErr is why a fresh list could not be fetched, if that was tried.
	RangesErr error
	// Now is the time to judge ages by; zero means the real clock.
	Now time.Time
	// ErrorSampler, if set, is asked for the counts of each error log file the
	// nginx configuration names. A result with nothing in it means the log is
	// not on this machine.
	ErrorSampler func(path string) errorlog.Counts
	// Logrotate holds the parsed logrotate files: the first governs the logs,
	// the others supply default options only. Empty means no retention check.
	Logrotate []*logrotate.File
	// LogrotateErr is why the logrotate file could not be used, if it was tried.
	LogrotateErr error
}

// FieldStatus is how one field fares in one log. Status is ok, missing or
// empty.
type FieldStatus struct {
	Name   string `json:"name"`
	Level  string `json:"level"`
	Status string `json:"status"`
}

// LogReport is the verdict on one access log.
type LogReport struct {
	// Path and Format come from the access_log directive.
	Path   string `json:"path"`
	Format string `json:"format"`
	// Servers names the servers (and locations) that write to it.
	Servers []string `json:"servers"`
	// At is the access_log directive.
	At     Place         `json:"at"`
	Fields []FieldStatus `json:"fields"`
	// Suggested is text to add when a field is missing: the format to use and
	// where it must go. It is empty when nothing is missing.
	Suggested string `json:"suggested"`
}

// Finding is one thing the check found.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Field    string `json:"field"`
	Log      string `json:"log"`
	At       Place  `json:"at"`
	// Also lists the other places a merged finding was found, when the same
	// problem was met in several places.
	Also       []Place `json:"also"`
	Suggestion string  `json:"suggestion"`
}

// Report is the result of a check.
type Report struct {
	Logs      []LogReport
	ErrorLogs []ErrorLogReport
	Findings  []Finding
}

// ErrorLogReport is what an error log held, as counts.
type ErrorLogReport struct {
	Path    string `json:"path"`
	Lines   int    `json:"lines"`
	Skipped int    `json:"skipped"`
	First   string `json:"first"`
	Last    string `json:"last"`
	// Levels counts lines by level.
	Levels map[string]int `json:"levels"`
	// Categories lists the categories that have lines, in the table's order
	// and then "other".
	Categories []CategoryCount `json:"categories"`
}

// CategoryCount is the number of lines in one category and, for limit_conn and
// limit_req, in each zone, the busiest first.
type CategoryCount struct {
	Name  string      `json:"name"`
	Count int         `json:"count"`
	Zones []ZoneCount `json:"zones"`
}

// ZoneCount is the number of lines for one zone.
type ZoneCount struct {
	Zone  string `json:"zone"`
	Count int    `json:"count"`
}

// ExitCode returns the exit status for the report: 1 if any finding is a gap,
// otherwise 0.
//
// @returns {int} 0 when there are no gaps, 1 when there are
// @example
//
//	os.Exit(report.ExitCode())
func (r *Report) ExitCode() int {
	for _, f := range r.Findings {
		if f.Severity == Gap {
			return 1
		}
	}
	return 0
}

// MarshalJSON writes the report with its logs, findings and exit status.
//
// @returns {[]byte, error} the JSON text
// @example
//
//	data, err := json.Marshal(report)
func (r *Report) MarshalJSON() ([]byte, error) {
	out := struct {
		Logs       []LogReport      `json:"logs"`
		ErrorLogs  []ErrorLogReport `json:"error_logs"`
		Findings   []Finding        `json:"findings"`
		ExitStatus int              `json:"exit_status"`
	}{r.Logs, r.ErrorLogs, r.Findings, r.ExitCode()}
	if out.ErrorLogs == nil {
		out.ErrorLogs = []ErrorLogReport{}
	}
	if out.Logs == nil {
		out.Logs = []LogReport{}
	}
	if out.Findings == nil {
		out.Findings = []Finding{}
	}
	out.Findings = append([]Finding(nil), out.Findings...)
	for i := range out.Findings {
		if out.Findings[i].Also == nil {
			out.Findings[i].Also = []Place{}
		}
	}
	return json.Marshal(out)
}

func placeOf(d *nginxconf.Directive) Place { return Place{File: d.File, Line: d.Line} }

func (p Place) String() string { return fmt.Sprintf("%s:%d", p.File, p.Line) }

// Run checks a host's nginx configuration.
//
// @param in {Input} the configuration, the dump and optional counts
// @returns {*Report, error} the findings, or ErrUnsupported, ErrNoServers or a nginxconf error
// @example
//
//	report, err := check.Run(check.Input{Config: cfg, Dump: dump})
func Run(in Input) (*Report, error) {
	c := in.Config
	if c.Server != "nginx" {
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, c.Server)
	}
	tab := in.Table
	if tab == nil {
		tab = fields.Default()
	}
	reqs, err := tab.Effective("nginx", c.Proxy.Behind, c.Fields)
	if err != nil {
		return nil, err
	}
	tree, err := in.Dump.Resolve()
	if err != nil {
		return nil, err
	}
	servers := nginxconf.Find(tree, "http", "server")
	if len(servers) == 0 {
		return nil, ErrNoServers
	}

	order := map[*nginxconf.Directive]int{}
	nginxconf.Walk(tree, func(d *nginxconf.Directive) { order[d] = len(order) })
	formats := map[string]*nginxconf.Directive{}
	for _, d := range nginxconf.Find(tree, "http", "log_format") {
		if len(d.Args) > 0 {
			formats[d.Args[0]] = d
		}
	}

	r := &Report{Findings: []Finding{}}
	add := func(f Finding) { r.Findings = append(r.Findings, f) }

	// Which access logs does each server write, and who shares them?
	type group struct {
		al      *nginxconf.Directive
		servers []string
	}
	var groups []*group
	byDirective := map[*nginxconf.Directive]*group{}
	join := func(al *nginxconf.Directive, who string) {
		g := byDirective[al]
		if g == nil {
			g = &group{al: al}
			byDirective[al] = g
			groups = append(groups, g)
		}
		g.servers = append(g.servers, who)
	}
	for _, srv := range servers {
		label := serverLabel(srv)
		logs := nginxconf.Effective(srv, "access_log")
		if len(logs) == 0 {
			add(Finding{Code: "access-log-default", Severity: Warn, At: placeOf(srv),
				Message:    fmt.Sprintf("server %s sets no access_log, so nginx uses its compiled-in default and the combined format", label),
				Suggestion: "Set an access_log with the extended format for this server or in the http block."})
		}
		for _, al := range logs {
			if isOff(al) {
				add(Finding{Code: "access-log-off", Severity: Gap, At: placeOf(al),
					Message: fmt.Sprintf("server %s has access_log off, so its traffic is not recorded", label)})
				continue
			}
			join(al, label)
		}
		nginxconf.Walk(srv.Block, func(l *nginxconf.Directive) {
			if l.Name != "location" || !l.HasBlock {
				return
			}
			where := label + " " + strings.Join(l.Args, " ")
			for _, al := range l.Block {
				if al.Name != "access_log" {
					continue
				}
				if isOff(al) {
					add(Finding{Code: "access-log-off", Severity: Warn, At: placeOf(al),
						Message: fmt.Sprintf("location %s has access_log off, so requests to it are not recorded", where)})
					continue
				}
				join(al, where)
			}
		})
	}

	for _, g := range groups {
		lr := LogReport{Path: g.al.Args[0], Format: "combined", Servers: g.servers, At: placeOf(g.al), Fields: []FieldStatus{}}
		if len(g.al.Args) > 1 {
			lr.Format = g.al.Args[1]
		}
		text, raw, known := builtinCombined, builtinCombined, true
		sampleErr, structured := "", false
		if d, ok := formats[lr.Format]; ok {
			pieces := d.Args[1:]
			if len(pieces) > 0 && strings.HasPrefix(pieces[0], "escape=") {
				if pieces[0] == "escape=json" {
					structured = true
					sampleErr = "the log_format uses escape=json, which the sampler does not read"
				}
				pieces = pieces[1:]
			}
			text, raw = strings.Join(pieces, " "), strings.Join(pieces, "")
			if order[d] > order[g.al] {
				add(Finding{Code: "format-defined-after-use", Severity: Gap, At: placeOf(d), Log: lr.Path,
					Message: fmt.Sprintf("log_format %q at %s is defined after the access_log that uses it at %s; nginx -t rejects this as an unknown log format",
						lr.Format, placeOf(d), placeOf(g.al)),
					Suggestion: fmt.Sprintf("Move the log_format into the file that holds the access_log, or into one that is included before %s.", placeOf(g.al))})
			}
		} else if lr.Format != "combined" {
			known = false
			add(Finding{Code: "format-undefined", Severity: Gap, At: placeOf(g.al), Log: lr.Path,
				Message:    fmt.Sprintf("access_log %s names log format %q, which is not defined", lr.Path, lr.Format),
				Suggestion: "Define the log_format before this access_log, or name an existing format."})
		}
		if known {
			sample, haveSample := in.Samples[lr.Path]
			if !haveSample && in.Sampler != nil && sampleErr == "" {
				sample, haveSample = in.Sampler(lr.Path, raw), true
			}
			if sampleErr != "" {
				sample, haveSample = Sample{Err: sampleErr}, true
			}
			switch {
			case haveSample && sample.Err != "":
				add(Finding{Code: "sample-unavailable", Severity: Note, Log: lr.Path, At: lr.At,
					Message: fmt.Sprintf("could not count fields in %s: %s", lr.Path, sample.Err)})
				haveSample = false
			case haveSample && sample.Skipped > sample.Lines:
				add(Finding{Code: "sample-mismatch", Severity: Warn, Log: lr.Path, At: lr.At,
					Message: fmt.Sprintf("%d of %d sampled lines in %s do not match log format %s, so the counts are not used; the log may have been written with another format",
						sample.Skipped, sample.Skipped+sample.Lines, lr.Path, lr.Format),
					Suggestion: "Check that the log is the one this access_log writes and that the format has not changed since the lines were written."})
				haveSample = false
			}
			haveSample = haveSample && sample.Lines > 0
			anyMissing := false
			for _, q := range reqs {
				f := q.Field
				if structured && compatibilityOnly(f) {
					continue
				}
				st := FieldStatus{Name: f.Name, Level: q.Level, Status: "ok"}
				sev := Note
				if q.Level == fields.Required {
					sev = Gap
				}
				switch {
				case !hasField(text, f):
					st.Status = "missing"
					anyMissing = true
					add(Finding{Code: "field-missing", Severity: sev, Field: f.Name, Log: lr.Path, At: lr.At,
						Message:    fmt.Sprintf("%s is not in log format %s: %s", f.Name, lr.Format, f.Description),
						Suggestion: f.IfMissing})
				case haveSample && !compatibilityOnly(f) && sample.Present != nil && hasKey(sample.Present, f.Name) && sample.Present[f.Name] == 0:
					st.Status = "empty"
					add(Finding{Code: "field-empty", Severity: sev, Field: f.Name, Log: lr.Path, At: lr.At,
						Message: fmt.Sprintf("%s is in log format %s but none of %d sampled lines carried a value", f.Name, lr.Format, sample.Lines),
						Suggestion: "The log format already names this field, so the header is not arriving. Look at what sends it, not at this server. " +
							f.Description + " See logagent-fields(5)."})
				}
				lr.Fields = append(lr.Fields, st)
			}
			if anyMissing {
				lr.Suggested = suggestFormat(tab, lr, structured)
			}
		}
		r.Logs = append(r.Logs, lr)
	}

	if c.Proxy.Behind != "none" {
		ranges := in.Ranges
		if ranges == nil {
			ranges = proxyranges.Cloudflare()
		}
		now := in.Now
		if now.IsZero() {
			now = time.Now()
		}
		if in.RangesErr != nil {
			add(Finding{Code: "ranges-refresh-failed", Severity: Note,
				Message: fmt.Sprintf("could not fetch the live list of the proxy's ranges: %v; the list built into logagent, retrieved %s, was used",
					in.RangesErr, ranges.Retrieved.Format("2006-01-02"))})
		}
		r.checkRealIP(c, servers, ranges, add)
		if len(configuredRanges(c)) == 0 {
			if limit := maxAge(c); ranges.Stale(now, limit) {
				add(Finding{Code: "ranges-snapshot-old", Severity: Warn,
					Message: fmt.Sprintf("the list of %s ranges is %d days old (retrieved %s from %s); it is warned about after %d days",
						ranges.Provider, ranges.AgeDays(now), ranges.Retrieved.Format("2006-01-02"), ranges.Source, limit),
					Suggestion: "Run check with --refresh-ranges to compare with the live list, or set proxy.trusted_ranges in the host configuration to your own list. A newer release of logagent carries a newer list."})
			}
		}
	}

	checkCaches(tree, formats, add)
	checkErrorLogs(tree, servers, add)
	if in.ErrorSampler != nil {
		r.sampleErrorLogs(tree, in.ErrorSampler, add)
	}
	if len(in.Logrotate) > 0 || in.LogrotateErr != nil {
		checkLogrotate(c, in, errorLogPaths(tree), add)
	}

	have := map[string]bool{}
	for _, l := range r.Logs {
		have[l.Path] = true
	}
	for _, l := range c.Logs {
		if !have[l.Path] {
			add(Finding{Code: "log-not-found", Severity: Gap, Log: l.Path,
				Message:    fmt.Sprintf("the configuration lists %s, but no access_log in the nginx configuration writes it", l.Path),
				Suggestion: "Check the path in logagent.yaml, or that the dump is the right host's."})
		}
	}
	return r, nil
}

// checkCaches looks at every location that uses proxy_cache. It warns when the
// cache is bypassed or not stored on a cookie, because a site that gives every
// anonymous visitor a session cookie then caches almost nothing (CaltechAUTHORS,
// 2026-10-08: two hits in 31 hours), and when no access log in force records
// $upstream_cache_status, because a cache that never hits looks fine in every
// other view of the log.
func checkCaches(tree []*nginxconf.Directive, formats map[string]*nginxconf.Directive, add func(Finding)) {
	for _, loc := range nginxconf.Locations(tree) {
		pc := nginxconf.Effective(loc, "proxy_cache")
		if len(pc) == 0 || len(pc[0].Args) == 0 || pc[0].Args[0] == "off" {
			continue
		}
		where := "location " + strings.Join(loc.Args, " ")
		for _, name := range []string{"proxy_cache_bypass", "proxy_no_cache"} {
			for _, d := range nginxconf.Effective(loc, name) {
				if !mentionsCookie(d.Args) {
					continue
				}
				add(Finding{Code: "cache-bypass-on-cookie", Severity: Warn, At: placeOf(d),
					Message:    fmt.Sprintf("%s caches, but %s names a cookie, so a request that carries any cookie skips the cache; sites that give every anonymous visitor a session cookie then cache almost nothing", where, name),
					Suggestion: "Do not key the cache on a cookie. Bypass on an Authorization header or a token argument, and, where the application marks authenticated responses with a header (Invenio sends X-User-ID), add $upstream_http_x_user_id to proxy_no_cache so an authenticated response is never stored. Confirm with a logged-in request that the header is sent on this path before relying on it."})
			}
		}
		var logs, with int
		for _, al := range nginxconf.Effective(loc, "access_log") {
			if isOff(al) || len(al.Args) == 0 {
				continue
			}
			logs++
			format := ""
			if len(al.Args) > 1 {
				if d, ok := formats[al.Args[1]]; ok {
					format = strings.Join(d.Args[1:], " ")
				}
			}
			if hasVariable(format, "$upstream_cache_status") {
				with++
			}
		}
		if logs > 0 && with == 0 {
			add(Finding{Code: "cache-status-not-logged", Severity: Warn, At: placeOf(loc),
				Message:    fmt.Sprintf("%s caches, but no access log in force there records $upstream_cache_status, so hits, misses and bypasses cannot be counted", where),
				Suggestion: "Add cache=\"$upstream_cache_status\" to the log_format. Without it a cache that never hits looks the same as one that works."})
		}
	}
}

// mentionsCookie reports whether any argument of a cache condition is a cookie
// variable, $http_cookie or $cookie_NAME.
func mentionsCookie(args []string) bool {
	for _, a := range args {
		if hasVariable(a, "$http_cookie") || strings.HasPrefix(a, "$cookie_") {
			return true
		}
	}
	return false
}

// checkLogrotate compares how long logrotate keeps each configured log with
// the floor the tiers need and the ceiling policy allows (DR-0002).
func checkLogrotate(c *config.Config, in Input, errorLogs []string, add func(Finding)) {
	if in.LogrotateErr != nil {
		add(Finding{Code: "logrotate-unreadable", Severity: Warn,
			Message:    fmt.Sprintf("the logrotate file named by retention.logrotate could not be used, so retention was not checked: %v", in.LogrotateErr),
			Suggestion: "Check retention.logrotate in the host configuration, and that the file is readable."})
		return
	}
	main := in.Logrotate[0]
	defaults := []logrotate.Options{main.Global}
	for _, f := range in.Logrotate[1:] {
		defaults = append(defaults, f.Global)
	}
	floor, ceiling := c.Retention.Layer1MinDays, c.Retention.Layer1MaxDays
	if floor < 1 {
		floor = 14
	}
	if ceiling < 1 {
		ceiling = 90
	}
	type group struct {
		s    *logrotate.Stanza
		logs []string
	}
	var groups []*group
	byStanza := map[*logrotate.Stanza]*group{}
	paths := make([]string, 0, len(c.Logs)+len(errorLogs))
	have := map[string]bool{}
	for _, l := range c.Logs {
		paths = append(paths, l.Path)
		have[l.Path] = true
	}
	for _, p := range errorLogs {
		if !have[p] {
			paths = append(paths, p)
		}
	}
	for _, path := range paths {
		l := struct{ Path string }{path}
		s, ok := main.For(l.Path)
		if !ok {
			add(Finding{Code: "logrotate-no-match", Severity: Warn, Log: l.Path,
				Message:    fmt.Sprintf("no block in %s governs %s, so its retention is unknown", main.Path, l.Path),
				Suggestion: "Check that retention.logrotate names the file that rotates this log."})
			continue
		}
		g := byStanza[s]
		if g == nil {
			g = &group{s: s}
			byStanza[s] = g
			groups = append(groups, g)
		}
		g.logs = append(g.logs, l.Path)
	}
	for _, g := range groups {
		ret := g.s.Retention(defaults...)
		at := Place{File: g.s.File, Line: g.s.Line}
		logs := strings.Join(g.logs, ", ")
		first := g.logs[0]
		switch {
		case ret.Indeterminate != "":
			add(Finding{Code: "logrotate-indeterminate", Severity: Warn, At: at, Log: first,
				Message:    fmt.Sprintf("cannot count how many days of %s logrotate keeps: %s", logs, ret.Indeterminate),
				Suggestion: "Rotate by time (daily, weekly) with a rotate count, so retention can be checked against policy."})
		case ret.Unlimited:
			add(Finding{Code: "logrotate-retention-long", Severity: Gap, At: at, Log: first,
				Message:    fmt.Sprintf("logrotate keeps rotated copies of %s with no limit (rotate -1 and no maxage), above the %d-day ceiling", logs, ceiling),
				Suggestion: fmt.Sprintf("Set rotate to a count that stays within %d days, or add maxage %d.", ceiling, ceiling-1)})
		case ret.LowDays < float64(floor):
			need := int(math.Ceil(float64(floor) / intervalLow(ret.Interval)))
			add(Finding{Code: "logrotate-retention-short", Severity: Gap, At: at, Log: first,
				Message: fmt.Sprintf("logrotate keeps %s days of %s, below the %d days the tiers need (%s rotate %d)",
					days(ret.LowDays), logs, floor, ret.Interval, ret.Rotate),
				Suggestion: fmt.Sprintf("Set rotate %d in the block at %s%s.", need, at, maxAgeHint(ret, floor))})
		case ret.HighDays > float64(ceiling):
			keep := int(math.Floor(float64(ceiling)/intervalHigh(ret.Interval))) - 1
			if keep < 0 {
				keep = 0
			}
			add(Finding{Code: "logrotate-retention-long", Severity: Gap, At: at, Log: first,
				Message: fmt.Sprintf("logrotate keeps up to %s days of %s, above the %d-day ceiling (%s rotate %d)",
					days(ret.HighDays), logs, ceiling, ret.Interval, ret.Rotate),
				Suggestion: fmt.Sprintf("Set rotate %d (or add maxage %d) in the block at %s.", keep, ceiling-1, at)})
		}
		if ret.MaxSize != "" && ret.Indeterminate == "" {
			add(Finding{Code: "logrotate-maxsize", Severity: Note, At: at, Log: first,
				Message:    fmt.Sprintf("the block rotates %s early when it reaches %s, so on a busy day it keeps fewer days than the count says", logs, ret.MaxSize),
				Suggestion: "Check that a day of this log is smaller than maxsize, or that fewer days are enough."})
		}
	}
}

func intervalLow(interval string) float64 {
	switch interval {
	case "hourly":
		return 1.0 / 24
	case "weekly":
		return 7
	case "monthly":
		return 28
	case "yearly":
		return 365
	}
	return 1
}

func intervalHigh(interval string) float64 {
	if interval == "monthly" {
		return 31
	}
	return intervalLow(interval)
}

func maxAgeHint(r logrotate.Retention, floor int) string {
	if r.MaxAgeDays > 0 && float64(r.MaxAgeDays) < float64(floor) {
		return fmt.Sprintf(" (maxage %d also removes copies sooner than that; raise or remove it)", r.MaxAgeDays)
	}
	return ""
}

// days formats a number of days without a needless decimal.
func days(d float64) string {
	if d == math.Trunc(d) {
		return fmt.Sprintf("%d", int(d))
	}
	return fmt.Sprintf("%.1f", d)
}

// suggestFormat builds the text for a log that lacks fields: the format and
// where it must go.
func suggestFormat(tab *fields.Table, lr LogReport, structured bool) string {
	const name = "logagent_extended"
	if structured {
		var missing []string
		for _, f := range lr.Fields {
			if f.Status == "missing" {
				missing = append(missing, f.Name)
			}
		}
		return fmt.Sprintf("Add these pairs to the escape=json log_format %s that the access_log at %s uses, before its closing brace. Pairs go inside the braces, separated by commas; drop the trailing comma on the last pair of the format.\n\n%s\n",
			lr.Format, lr.At, tab.NginxJSONPairs(missing))
	}
	return fmt.Sprintf("Define this log_format in %s before line %d, or in a file included before it: nginx rejects an access_log that names a format defined later. Then change the access_log at %s to use it (access_log %s %s;).\n\n%s\n",
		lr.At.File, lr.At.Line, lr.At, lr.Path, name, tab.NginxLogFormat(name))
}

// checkRealIP checks the real-IP setup of every server behind a proxy, merging
// findings that point at the same place into one that names every server.
func (r *Report) checkRealIP(c *config.Config, servers []*nginxconf.Directive, snapshot *proxyranges.Snapshot, add func(Finding)) {
	expected, expectedName := expectedRanges(c, snapshot)
	merged := newMerger()
	put := merged.put
	want := c.Proxy.RealIPHeader
	for _, srv := range servers {
		label := serverLabel(srv)
		header := nginxconf.Effective(srv, "real_ip_header")
		trust := nginxconf.Effective(srv, "set_real_ip_from")
		if len(header) == 0 {
			put(Finding{Code: "real-ip-missing", Severity: Gap, At: placeOf(srv),
				Message:    "real_ip_header is not set, so every request is logged with the address of the proxy and not the visitor",
				Suggestion: realIPSuggestion(c, expected)}, label)
		} else if want != "" && !strings.EqualFold(header[0].Args[0], want) {
			put(Finding{Code: "real-ip-header-mismatch", Severity: Gap, At: placeOf(header[0]),
				Message:    fmt.Sprintf("real_ip_header is %s but the proxy sends the visitor's address in %s", header[0].Args[0], want),
				Suggestion: realIPSuggestion(c, expected)}, label)
		}
		if len(header) > 0 && len(trust) == 0 {
			put(Finding{Code: "real-ip-no-trusted-proxies", Severity: Gap, At: placeOf(header[0]),
				Message:    "real_ip_header is set but no set_real_ip_from names a proxy to trust, so nginx ignores the header",
				Suggestion: realIPSuggestion(c, expected)}, label)
		}
		tooWide := false
		var trusted []netip.Prefix
		for _, t := range trust {
			if t.Args[0] == "0.0.0.0/0" || t.Args[0] == "::/0" {
				tooWide = true
				put(Finding{Code: "real-ip-trust-too-wide", Severity: Gap, At: placeOf(t),
					Message:    fmt.Sprintf("set_real_ip_from %s trusts every address, so any visitor can forge the address that is logged", t.Args[0]),
					Suggestion: "Trust only the proxy's published ranges. " + realIPSuggestion(c, expected)}, label)
			}
			if p, ok := parseTrusted(t.Args[0]); ok {
				trusted = append(trusted, p)
			}
		}
		if len(trusted) == 0 || tooWide {
			continue
		}
		if missing := expected.Missing(trusted); len(missing) > 0 {
			put(Finding{Code: "real-ip-ranges-missing", Severity: Warn, At: placeOf(trust[0]),
				Message: fmt.Sprintf("set_real_ip_from does not cover %d of the %d %s ranges (%s); requests that reach this server through them are logged with the proxy's address",
					len(missing), len(expected.Prefixes), expectedName, joinPrefixes(missing)),
				Suggestion: "Add these next to the existing set_real_ip_from lines:\n" + setRealIPLines(missing)}, label)
		}
		if extra := expected.Extra(trusted); len(extra) > 0 {
			put(Finding{Code: "real-ip-trusts-other-ranges", Severity: Note, At: placeOf(trust[0]),
				Message: fmt.Sprintf("set_real_ip_from also trusts %s, which are not inside the %s ranges; any address in them can set the address that is logged",
					joinPrefixes(extra), expectedName),
				Suggestion: "Trust only the proxy's published ranges unless these are proxies of yours, such as an internal load balancer."}, label)
		}
	}
	merged.flush(add, "servers")
}

// merger collects findings and merges those that say the same thing, as when
// several servers or places have the same problem. The merged finding points at
// the first place, lists the other places in Also, and names every label.
type merger struct {
	order []mergeKey
	seen  map[mergeKey]*mergePending
}

type mergeKey struct{ code, message string }

type mergePending struct {
	f      Finding
	labels []string
}

func newMerger() *merger { return &merger{seen: map[mergeKey]*mergePending{}} }

// put records a finding found at the place the label names.
func (m *merger) put(f Finding, label string) {
	k := mergeKey{f.Code, f.Message}
	p := m.seen[k]
	if p == nil {
		p = &mergePending{f: f}
		m.seen[k] = p
		m.order = append(m.order, k)
	} else if f.At != p.f.At && !containsPlace(p.f.Also, f.At) {
		p.f.Also = append(p.f.Also, f.At)
	}
	for _, l := range p.labels {
		if l == label {
			return
		}
	}
	p.labels = append(p.labels, label)
}

// flush hands the merged findings on, in the order first found, each with
// "(word: label, label)" added to its message.
func (m *merger) flush(add func(Finding), word string) {
	for _, k := range m.order {
		p := m.seen[k]
		p.f.Message += " (" + word + ": " + strings.Join(p.labels, ", ") + ")"
		add(p.f)
	}
}

// expectedRanges returns the ranges a server behind the proxy should trust and
// the name to call them: the host's own list if it gave one, otherwise the
// snapshot.
func expectedRanges(c *config.Config, snapshot *proxyranges.Snapshot) (*proxyranges.Snapshot, string) {
	if own := configuredRanges(c); len(own) > 0 {
		return &proxyranges.Snapshot{Provider: "configured", Source: "proxy.trusted_ranges", Prefixes: own}, "configured"
	}
	return snapshot, "Cloudflare"
}

// configuredRanges returns the ranges the host's configuration lists as the
// proxy's. The configuration loader has already checked them.
func configuredRanges(c *config.Config) []netip.Prefix {
	var out []netip.Prefix
	for _, s := range c.Proxy.TrustedRanges {
		if p, ok := parseTrusted(s); ok {
			out = append(out, p)
		}
	}
	return out
}

func maxAge(c *config.Config) int {
	if c.Proxy.RangesMaxAgeDays < 1 {
		return 90
	}
	return c.Proxy.RangesMaxAgeDays
}

// parseTrusted reads a set_real_ip_from argument: a range or a single address.
func parseTrusted(arg string) (netip.Prefix, bool) {
	if p, err := netip.ParsePrefix(arg); err == nil {
		return p.Masked(), true
	}
	if a, err := netip.ParseAddr(arg); err == nil {
		return netip.PrefixFrom(a, a.BitLen()), true
	}
	return netip.Prefix{}, false
}

func joinPrefixes(ps []netip.Prefix) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.String()
	}
	return strings.Join(parts, ", ")
}

func setRealIPLines(ps []netip.Prefix) string {
	var b strings.Builder
	for _, p := range ps {
		fmt.Fprintf(&b, "set_real_ip_from %s;\n", p)
	}
	return b.String()
}

// realIPSuggestion is the text for a missing or wrong real-IP setup, listing
// the ranges to trust.
func realIPSuggestion(c *config.Config, expected *proxyranges.Snapshot) string {
	var b strings.Builder
	b.WriteString("In the http block, before the server blocks, in a file nginx.conf includes, trust the proxy and read the visitor's address from its header")
	if expected.Provider == "configured" {
		b.WriteString(" (ranges from proxy.trusted_ranges):\n")
	} else {
		fmt.Fprintf(&b, " (ranges as published at %s, retrieved %s):\n", expected.Source, expected.Retrieved.Format("2006-01-02"))
	}
	b.WriteString(setRealIPLines(expected.Prefixes))
	h := c.Proxy.RealIPHeader
	if h == "" {
		h = "CF-Connecting-IP"
	}
	fmt.Fprintf(&b, "real_ip_header %s;\n", h)
	return b.String()
}

func containsPlace(ps []Place, p Place) bool {
	for _, q := range ps {
		if q == p {
			return true
		}
	}
	return false
}

func serverLabel(srv *nginxconf.Directive) string {
	name, port := "(default)", ""
	for _, d := range srv.Block {
		switch {
		case d.Name == "server_name" && len(d.Args) > 0 && name == "(default)":
			name = d.Args[0]
		case d.Name == "listen" && len(d.Args) > 0 && port == "":
			port = d.Args[0]
			if i := strings.LastIndex(port, ":"); i >= 0 {
				port = port[i+1:]
			}
		}
	}
	if port != "" {
		return name + ":" + port
	}
	return name
}

// compatibilityOnly reports whether nothing but the stock format needs a field,
// which sites often leave as a dash, so an empty count says nothing.
func compatibilityOnly(f fields.Field) bool {
	for _, n := range f.NeededBy {
		if n != "compatibility" {
			return false
		}
	}
	return true
}

func isOff(al *nginxconf.Directive) bool { return len(al.Args) == 1 && al.Args[0] == "off" }

func hasKey(m map[string]int, k string) bool { _, ok := m[k]; return ok }

// hasVariable reports whether expr (such as $http_cf_ray) appears in a log
// format as that whole variable, as $name or ${name}.
func hasField(format string, f fields.Field) bool {
	if hasVariable(format, f.Nginx.Expr) {
		return true
	}
	for _, alt := range f.Nginx.Also {
		all := true
		for _, v := range strings.Fields(alt) {
			all = all && hasVariable(format, v)
		}
		if all {
			return true
		}
	}
	return false
}

func hasVariable(format, expr string) bool {
	name := strings.TrimPrefix(expr, "$")
	if strings.Contains(format, "${"+name+"}") {
		return true
	}
	rest := format
	for {
		i := strings.Index(rest, expr)
		if i < 0 {
			return false
		}
		end := i + len(expr)
		if end >= len(rest) || !isNameChar(rest[end]) {
			return true
		}
		rest = rest[end:]
	}
}

func isNameChar(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// logLevels orders nginx's error log levels from the most verbose.
var logLevels = map[string]int{"debug": 0, "info": 1, "notice": 2, "warn": 3, "error": 4, "crit": 5, "alert": 6, "emerg": 7}

func levelIndex(name string) int {
	if i, ok := logLevels[name]; ok {
		return i
	}
	return logLevels["error"]
}

// errorLogsAt returns the error_log directives in force at a context: the
// nearest context that sets any, up through http, then the main context's.
func errorLogsAt(tree []*nginxconf.Directive, ctx *nginxconf.Directive) []*nginxconf.Directive {
	if d := nginxconf.Effective(ctx, "error_log"); len(d) > 0 {
		return d
	}
	var out []*nginxconf.Directive
	for _, d := range tree {
		if d.Name == "error_log" {
			out = append(out, d)
		}
	}
	return out
}

func errorLogLevel(d *nginxconf.Directive) string {
	if len(d.Args) > 1 {
		return d.Args[1]
	}
	return "error"
}

// recordsAnything reports whether an error_log target keeps what it is given.
func recordsAnything(d *nginxconf.Directive) bool {
	return len(d.Args) > 0 && d.Args[0] != "/dev/null"
}

// errorLogFile returns the path of an error_log target that is an absolute file.
func errorLogFile(d *nginxconf.Directive) (string, bool) {
	if len(d.Args) == 0 || !strings.HasPrefix(d.Args[0], "/") || d.Args[0] == "/dev/null" {
		return "", false
	}
	return d.Args[0], true
}

// errorLogPaths lists the error log files named anywhere in the configuration,
// once each, in the order they appear.
func errorLogPaths(tree []*nginxconf.Directive) []string {
	var out []string
	seen := map[string]bool{}
	nginxconf.Walk(tree, func(d *nginxconf.Directive) {
		if d.Name != "error_log" {
			return
		}
		if p, ok := errorLogFile(d); ok && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	})
	return out
}

// contextLabel names a server, location or http block for a message.
func contextLabel(ctx *nginxconf.Directive) string {
	switch ctx.Name {
	case "server":
		return serverLabel(ctx)
	case "location":
		for p := ctx.Parent; p != nil; p = p.Parent {
			if p.Name == "server" {
				return serverLabel(p) + " " + strings.Join(ctx.Args, " ")
			}
		}
		return strings.Join(ctx.Args, " ")
	}
	return ctx.Name
}

// checkErrorLogs finds an error log that cannot record what the configuration
// says it will: caps logged below the error_log level, or no error log at all.
func checkErrorLogs(tree, servers []*nginxconf.Directive, add func(Finding)) {
	hidden := newMerger()
	nginxconf.Walk(tree, func(d *nginxconf.Directive) {
		if (d.Name != "limit_conn" && d.Name != "limit_req") || d.Parent == nil {
			return
		}
		ctx := d.Parent
		// One finding per context and kind, naming every zone, at the first directive.
		for _, e := range ctx.Block {
			if e.Name == d.Name && e != d {
				if e.Line < d.Line || (e.Line == d.Line && e.File < d.File) {
					return
				}
			}
		}
		level := "error"
		if l := nginxconf.Effective(ctx, d.Name+"_log_level"); len(l) > 0 && len(l[0].Args) > 0 {
			level = l[0].Args[0]
		}
		logs := errorLogsAt(tree, ctx)
		visible := false
		var described []string
		for _, l := range logs {
			described = append(described, errorLogLevel(l)+" at "+placeOf(l).String())
			if recordsAnything(l) && levelIndex(level) >= levelIndex(errorLogLevel(l)) {
				visible = true
			}
		}
		if visible {
			return
		}
		if len(logs) == 0 {
			described = []string{"error by default"}
		}
		var zones []string
		for _, e := range ctx.Block {
			if e.Name != d.Name || len(e.Args) == 0 {
				continue
			}
			z := e.Args[0]
			if d.Name == "limit_req" {
				z = strings.TrimPrefix(strings.Fields(strings.Join(e.Args, " "))[0], "zone=")
				for _, a := range e.Args {
					if strings.HasPrefix(a, "zone=") {
						z = strings.TrimPrefix(a, "zone=")
					}
				}
			}
			zones = append(zones, z)
		}
		hidden.put(Finding{Code: "error-log-level-hides-limits", Severity: Warn, At: placeOf(d),
			Message: fmt.Sprintf("%s rejections (zone %s) are logged at %s, but the effective error_log is %s, so they are not recorded",
				d.Name, strings.Join(zones, ", "), level, strings.Join(described, "; ")),
			Suggestion: fmt.Sprintf("Set the error_log to %s or more verbose (error_log /var/log/nginx/error.log %s;), or set %s_log_level error to log the rejections at the level the error_log keeps.",
				level, level, d.Name)}, contextLabel(ctx))
	})
	hidden.flush(add, "in")

	disabled, missing := newMerger(), newMerger()
	for _, srv := range servers {
		logs := errorLogsAt(tree, srv)
		recording := false
		for _, l := range logs {
			recording = recording || recordsAnything(l)
		}
		switch {
		case len(logs) == 0:
			missing.put(Finding{Code: "error-log-default", Severity: Note, At: placeOf(srv),
				Message:    "no error_log is set, so nginx uses its compiled-in default path and the error level, and this check cannot find or rotate-check that file",
				Suggestion: "Set error_log /var/log/nginx/error.log warn; in the main context so the log's place and level are written down."}, serverLabel(srv))
		case !recording:
			disabled.put(Finding{Code: "error-log-disabled", Severity: Warn, At: placeOf(logs[0]),
				Message:    "the effective error_log is /dev/null, so nginx errors, including upstream failures, are not recorded",
				Suggestion: "Point error_log at a file, for example error_log /var/log/nginx/error.log warn;."}, serverLabel(srv))
		}
	}
	disabled.flush(add, "servers")
	missing.flush(add, "servers")
}

// sampleErrorLogs asks for the counts of each error log file and adds them to
// the report, with a warning for critical lines and notes for what cannot be
// read or does not look like an error log.
func (r *Report) sampleErrorLogs(tree []*nginxconf.Directive, sampler func(string) errorlog.Counts, add func(Finding)) {
	for _, path := range errorLogPaths(tree) {
		var at Place
		for _, d := range errorLogDirectives(tree) {
			if p, ok := errorLogFile(d); ok && p == path {
				at = placeOf(d)
				break
			}
		}
		c := sampler(path)
		switch {
		case c.Err != "":
			add(Finding{Code: "error-log-unavailable", Severity: Note, Log: path, At: at,
				Message: fmt.Sprintf("could not count the lines of %s: %s", path, c.Err)})
			continue
		case c.Lines == 0 && c.Skipped == 0:
			continue
		case c.Skipped > c.Lines:
			add(Finding{Code: "error-log-mismatch", Severity: Warn, Log: path, At: at,
				Message: fmt.Sprintf("%d of %d sampled lines in %s do not look like nginx error log lines, so the counts are not used",
					c.Skipped, c.Skipped+c.Lines, path),
				Suggestion: "Check that this file is the error log the configuration writes, and not another program's."})
			continue
		}
		r.ErrorLogs = append(r.ErrorLogs, errorLogReport(path, c))
		var parts []string
		for _, level := range []string{"crit", "alert", "emerg"} {
			if n := c.Levels[level]; n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, level))
			}
		}
		if len(parts) > 0 {
			add(Finding{Code: "error-log-critical", Severity: Warn, Log: path, At: at,
				Message: fmt.Sprintf("%s lines in the last %d lines of %s (%s to %s)",
					strings.Join(parts, ", "), c.Lines, path, c.First, c.Last),
				Suggestion: "Read those lines on the host: they are the ones nginx logs when it is failing, not when it is busy."})
		}
	}
}

// errorLogDirectives returns every error_log directive in the tree in document order.
func errorLogDirectives(tree []*nginxconf.Directive) []*nginxconf.Directive {
	var out []*nginxconf.Directive
	nginxconf.Walk(tree, func(d *nginxconf.Directive) {
		if d.Name == "error_log" {
			out = append(out, d)
		}
	})
	return out
}

func errorLogReport(path string, c errorlog.Counts) ErrorLogReport {
	e := ErrorLogReport{Path: path, Lines: c.Lines, Skipped: c.Skipped, First: c.First, Last: c.Last, Levels: c.Levels, Categories: []CategoryCount{}}
	order := errorlog.Default().Names()
	known := map[string]bool{}
	for _, n := range order {
		known[n] = true
	}
	var extra []string
	for n := range c.Categories {
		if !known[n] && n != errorlog.Other {
			extra = append(extra, n)
		}
	}
	sort.Strings(extra)
	order = append(append(order, extra...), errorlog.Other)
	for _, name := range order {
		n := c.Categories[name]
		if n == 0 {
			continue
		}
		cc := CategoryCount{Name: name, Count: n, Zones: []ZoneCount{}}
		for zone, count := range c.Zones[name] {
			cc.Zones = append(cc.Zones, ZoneCount{Zone: zone, Count: count})
		}
		sort.Slice(cc.Zones, func(i, j int) bool {
			if cc.Zones[i].Count != cc.Zones[j].Count {
				return cc.Zones[i].Count > cc.Zones[j].Count
			}
			return cc.Zones[i].Zone < cc.Zones[j].Zone
		})
		e.Categories = append(e.Categories, cc)
	}
	return e
}
