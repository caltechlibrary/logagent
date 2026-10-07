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
	"strings"

	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/fields"
	"github.com/caltechlibrary/logagent/internal/nginxconf"
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
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Field      string `json:"field"`
	Log        string `json:"log"`
	At         Place  `json:"at"`
	Suggestion string `json:"suggestion"`
}

// Report is the result of a check.
type Report struct {
	Logs     []LogReport
	Findings []Finding
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
		Logs       []LogReport `json:"logs"`
		Findings   []Finding   `json:"findings"`
		ExitStatus int         `json:"exit_status"`
	}{r.Logs, r.Findings, r.ExitCode()}
	if out.Logs == nil {
		out.Logs = []LogReport{}
	}
	if out.Findings == nil {
		out.Findings = []Finding{}
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
		sampleErr := ""
		if d, ok := formats[lr.Format]; ok {
			pieces := d.Args[1:]
			if len(pieces) > 0 && strings.HasPrefix(pieces[0], "escape=") {
				if pieces[0] == "escape=json" {
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
				st := FieldStatus{Name: f.Name, Level: q.Level, Status: "ok"}
				sev := Note
				if q.Level == fields.Required {
					sev = Gap
				}
				switch {
				case !hasVariable(text, f.Nginx.Expr):
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
				lr.Suggested = suggestFormat(tab, lr)
			}
		}
		r.Logs = append(r.Logs, lr)
	}

	if c.Proxy.Behind != "none" {
		r.checkRealIP(c, servers, add)
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

// suggestFormat builds the text for a log that lacks fields: the format and
// where it must go.
func suggestFormat(tab *fields.Table, lr LogReport) string {
	const name = "logagent_extended"
	return fmt.Sprintf("Define this log_format in %s before line %d, or in a file included before it: nginx rejects an access_log that names a format defined later. Then change the access_log at %s to use it (access_log %s %s;).\n\n%s\n",
		lr.At.File, lr.At.Line, lr.At, lr.Path, name, tab.NginxLogFormat(name))
}

// checkRealIP checks the real-IP setup of every server behind a proxy, merging
// findings that point at the same place into one that names every server.
func (r *Report) checkRealIP(c *config.Config, servers []*nginxconf.Directive, add func(Finding)) {
	type key struct {
		code string
		at   Place
	}
	type pending struct {
		f       Finding
		servers []string
	}
	var order []key
	seen := map[key]*pending{}
	put := func(f Finding, server string) {
		k := key{f.Code, f.At}
		p := seen[k]
		if p == nil {
			p = &pending{f: f}
			seen[k] = p
			order = append(order, k)
		}
		p.servers = append(p.servers, server)
	}
	want := c.Proxy.RealIPHeader
	for _, srv := range servers {
		label := serverLabel(srv)
		header := nginxconf.Effective(srv, "real_ip_header")
		trust := nginxconf.Effective(srv, "set_real_ip_from")
		if len(header) == 0 {
			put(Finding{Code: "real-ip-missing", Severity: Gap, At: placeOf(srv),
				Message:    "real_ip_header is not set, so every request is logged with the address of the proxy and not the visitor",
				Suggestion: realIPSuggestion(c)}, label)
		} else if want != "" && !strings.EqualFold(header[0].Args[0], want) {
			put(Finding{Code: "real-ip-header-mismatch", Severity: Gap, At: placeOf(header[0]),
				Message:    fmt.Sprintf("real_ip_header is %s but the proxy sends the visitor's address in %s", header[0].Args[0], want),
				Suggestion: realIPSuggestion(c)}, label)
		}
		if len(header) > 0 && len(trust) == 0 {
			put(Finding{Code: "real-ip-no-trusted-proxies", Severity: Gap, At: placeOf(header[0]),
				Message:    "real_ip_header is set but no set_real_ip_from names a proxy to trust, so nginx ignores the header",
				Suggestion: realIPSuggestion(c)}, label)
		}
		for _, t := range trust {
			if t.Args[0] == "0.0.0.0/0" || t.Args[0] == "::/0" {
				put(Finding{Code: "real-ip-trust-too-wide", Severity: Gap, At: placeOf(t),
					Message:    fmt.Sprintf("set_real_ip_from %s trusts every address, so any visitor can forge the address that is logged", t.Args[0]),
					Suggestion: "Trust only the proxy's published ranges. " + realIPSuggestion(c)}, label)
			}
		}
	}
	for _, k := range order {
		p := seen[k]
		p.f.Message += " (servers: " + strings.Join(p.servers, ", ") + ")"
		add(p.f)
	}
}

// realIPSuggestion is the text for a missing or wrong real-IP setup.
func realIPSuggestion(c *config.Config) string {
	var b strings.Builder
	b.WriteString("In the http block, before the server blocks, in a file nginx.conf includes, trust the proxy and read the visitor's address from its header:\n")
	if ranges := c.TrustedRanges(); len(ranges) > 0 {
		for _, p := range ranges {
			fmt.Fprintf(&b, "set_real_ip_from %s;\n", p)
		}
	} else {
		b.WriteString("set_real_ip_from <each range the proxy publishes>;\n")
	}
	h := c.Proxy.RealIPHeader
	if h == "" {
		h = "CF-Connecting-IP"
	}
	fmt.Fprintf(&b, "real_ip_header %s;\n", h)
	return b.String()
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
