// Package fields holds the table of log fields the tiers need. One embedded
// data file drives the checker and the log_format and LogFormat snippets
// logagent publishes, so the recommended configuration and the check cannot
// disagree.
package fields

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed fields.json
var defaultJSON []byte

// Levels, servers and proxies the table and its callers accept.
const (
	Required      = "required"
	Optional      = "optional"
	NotApplicable = "not_applicable"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Server describes how one web server writes a field.
type Server struct {
	// Expr is the variable (nginx) or format directive (Apache).
	Expr string `json:"expr"`
	// Unavailable marks a field the server cannot log; Note says why.
	Unavailable bool   `json:"unavailable"`
	Note        string `json:"note"`
}

// Field is one row of the table.
type Field struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	NeededBy    []string `json:"needed_by"`
	// Default is required or optional.
	Default string `json:"default"`
	// Applies is always, behind_proxy or behind:cloudflare.
	Applies string `json:"applies"`
	// Combined marks a field of the stock combined format.
	Combined bool `json:"combined"`
	// Quoted says the generated label wraps the value in double quotes.
	Quoted    bool   `json:"quoted"`
	Nginx     Server `json:"nginx"`
	Apache    Server `json:"apache"`
	IfMissing string `json:"if_missing"`
}

// Table is a validated field table.
type Table struct {
	Version int     `json:"version"`
	Fields  []Field `json:"fields"`
}

// Requirement is one field as it applies to one host.
type Requirement struct {
	Field Field
	// Level is required or optional.
	Level string
	// Unavailable is true when the host's server cannot log the field.
	Unavailable bool
}

// Default returns the table embedded in the binary. It panics if the embedded
// data is invalid, which a test prevents.
//
// @returns {*Table} the default table
// @example
//
//	tab := fields.Default()
//	fmt.Println(tab.Has("ua")) // true
func Default() *Table {
	t, err := Parse(defaultJSON)
	if err != nil {
		panic("logagent: embedded fields.json: " + err.Error())
	}
	return t
}

// Parse strictly decodes and validates a field table.
//
// @param data {[]byte} the JSON text of a table
// @returns {*Table, error} the table, or the first problem found
// @example
//
//	tab, err := fields.Parse(data)
func Parse(data []byte) (*Table, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var t Table
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("field table: %w", err)
	}
	if t.Version != 1 {
		return nil, fmt.Errorf("field table: version must be 1 (got %d)", t.Version)
	}
	seen := map[string]bool{}
	for i, f := range t.Fields {
		if !namePattern.MatchString(f.Name) {
			return nil, fmt.Errorf("field table: fields[%d]: name %q must be lower_snake_case", i, f.Name)
		}
		if seen[f.Name] {
			return nil, fmt.Errorf("field table: duplicate field %q", f.Name)
		}
		seen[f.Name] = true
		if len(f.NeededBy) == 0 {
			return nil, fmt.Errorf("field table: %s: needed_by must name at least one consumer", f.Name)
		}
		switch f.Default {
		case Required, Optional:
		default:
			return nil, fmt.Errorf("field table: %s: default must be required or optional (got %q)", f.Name, f.Default)
		}
		switch f.Applies {
		case "always", "behind_proxy", "behind:cloudflare":
		default:
			return nil, fmt.Errorf("field table: %s: applies must be always, behind_proxy or behind:cloudflare (got %q)", f.Name, f.Applies)
		}
		if len(f.Nginx.Expr) < 2 || !strings.HasPrefix(f.Nginx.Expr, "$") {
			return nil, fmt.Errorf("field table: %s: nginx expr must be a variable such as $name (got %q)", f.Name, f.Nginx.Expr)
		}
		if f.Apache.Expr == "" && !(f.Apache.Unavailable && f.Apache.Note != "") {
			return nil, fmt.Errorf("field table: %s: apache needs an expr, or unavailable with a note", f.Name)
		}
		if f.Description == "" || f.IfMissing == "" {
			return nil, fmt.Errorf("field table: %s: description and if_missing are required", f.Name)
		}
	}
	return &t, nil
}

// Get returns the named field.
//
// @param name {string} the field name
// @returns {Field, bool} the field and whether it exists
// @example
//
//	f, ok := fields.Default().Get("ua")
func (t *Table) Get(name string) (Field, bool) {
	for _, f := range t.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// Has reports whether the table has the named field.
//
// @param name {string} the field name
// @returns {bool} true when the field exists
// @example
//
//	ok := fields.Default().Has("bot_score")
func (t *Table) Has(name string) bool {
	_, ok := t.Get(name)
	return ok
}

// Effective returns the fields that apply to one host, in table order, with
// their level after the host's overrides. A field overridden to required or
// optional is included even if its proxy condition is not met; one overridden
// to not_applicable is dropped.
//
// @param server {string} nginx or apache
// @param behind {string} none or cloudflare, the config's proxy.behind
// @param overrides {map[string]string} the config's fields: map; may be nil
// @returns {[]Requirement, error} the requirements, or an error for an unknown server, proxy, override name or level
// @example
//
//	rs, err := fields.Default().Effective("nginx", "cloudflare", nil)
func (t *Table) Effective(server, behind string, overrides map[string]string) ([]Requirement, error) {
	switch server {
	case "nginx", "apache":
	default:
		return nil, fmt.Errorf("server must be nginx or apache (got %q)", server)
	}
	switch behind {
	case "none", "cloudflare":
	default:
		return nil, fmt.Errorf("proxy must be none or cloudflare (got %q)", behind)
	}
	for name, level := range overrides {
		if !t.Has(name) {
			return nil, fmt.Errorf("fields.%s: no such field", name)
		}
		switch level {
		case Required, Optional, NotApplicable:
		default:
			return nil, fmt.Errorf("fields.%s: must be required, optional or not_applicable (got %q)", name, level)
		}
	}
	var out []Requirement
	for _, f := range t.Fields {
		level, overridden := overrides[f.Name]
		if overridden && level == NotApplicable {
			continue
		}
		if !overridden {
			if !appliesTo(f.Applies, behind) {
				continue
			}
			level = f.Default
		}
		r := Requirement{Field: f, Level: level}
		if server == "apache" {
			r.Unavailable = f.Apache.Unavailable
		}
		out = append(out, r)
	}
	return out, nil
}

func appliesTo(applies, behind string) bool {
	switch applies {
	case "behind_proxy":
		return behind != "none"
	case "behind:cloudflare":
		return behind == "cloudflare"
	}
	return true
}

// NginxLogFormat renders a log_format directive holding every field of the
// table: the stock combined format first, then one label=value pair per
// extended field.
//
// @param name {string} the log_format name
// @returns {string} the directive, ending in a semicolon
// @example
//
//	text := fields.Default().NginxLogFormat("site_bots")
func (t *Table) NginxLogFormat(name string) string {
	expr := func(n string) string { f, _ := t.Get(n); return f.Nginx.Expr }
	combined := fmt.Sprintf(`%s - %s [%s] "%s" %s %s "%s" "%s"`,
		expr("client"), expr("user"), expr("time"), expr("request"),
		expr("status"), expr("bytes"), expr("referer"), expr("ua"))
	var pieces []string
	for _, f := range t.Fields {
		if f.Combined {
			continue
		}
		v := f.Nginx.Expr
		if f.Quoted {
			v = `"` + v + `"`
		}
		pieces = append(pieces, f.Name+"="+v)
	}
	var lines []string
	lines = append(lines, combined)
	line := ""
	for _, p := range pieces {
		if line != "" && len(line)+len(p) > 64 {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += p
	}
	if line != "" {
		lines = append(lines, line)
	}
	var b strings.Builder
	b.WriteString("log_format " + name)
	for i, l := range lines {
		if i < len(lines)-1 {
			l += " "
		}
		b.WriteString("\n    '" + l + "'")
	}
	b.WriteString(";")
	return b.String()
}
