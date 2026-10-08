// Package errorlog counts, in an nginx error log, how many lines fall in each
// category of message and at each level. It keeps only counts, plus the names
// of limit_conn and limit_req zones, which are configuration and not patron
// data: no client address, request, path or message text survives, so the
// result can go into a report that is pasted into an issue (DR-0002, DR-0003).
//
// The categories are data, errors.json: a name, a pattern and whether the
// pattern captures a zone name. A line that matches none is counted as "other"
// by its level and its text is never shown.
package errorlog

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/caltechlibrary/logagent/internal/tail"
)

//go:embed errors.json
var defaultJSON []byte

// ErrMalformed means a category table is not valid.
var ErrMalformed = errors.New("malformed error log category table")

// Other is the category of every line that no other category matches.
const Other = "other"

// Category is one kind of message.
type Category struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	// Zone says the pattern's one capture group is a zone name to count.
	Zone bool `json:"zone"`
	re   *regexp.Regexp
}

// Table is a validated list of categories, tried in order.
type Table struct {
	Version    int        `json:"version"`
	Categories []Category `json:"categories"`
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Parse strictly decodes and validates a category table.
//
// @param data {[]byte} the JSON text
// @returns {*Table, error} the table, or an error matching ErrMalformed
// @example
//
//	tab, err := errorlog.Parse(data)
func Parse(data []byte) (*Table, error) {
	var t Table
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if t.Version != 1 {
		return nil, fmt.Errorf("%w: version must be 1 (got %d)", ErrMalformed, t.Version)
	}
	seen := map[string]bool{}
	for i := range t.Categories {
		c := &t.Categories[i]
		switch {
		case !namePattern.MatchString(c.Name) || c.Name == Other:
			return nil, fmt.Errorf("%w: category name %q must be lower-case words joined by dashes, and not %q", ErrMalformed, c.Name, Other)
		case seen[c.Name]:
			return nil, fmt.Errorf("%w: duplicate category %q", ErrMalformed, c.Name)
		case c.Description == "":
			return nil, fmt.Errorf("%w: %s: description is required", ErrMalformed, c.Name)
		case c.Pattern == "":
			return nil, fmt.Errorf("%w: %s: pattern is required", ErrMalformed, c.Name)
		}
		seen[c.Name] = true
		re, err := regexp.Compile(c.Pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: pattern: %v", ErrMalformed, c.Name, err)
		}
		switch {
		case c.Zone && re.NumSubexp() != 1:
			return nil, fmt.Errorf("%w: %s: a zone category needs exactly one capture group, the zone name", ErrMalformed, c.Name)
		case !c.Zone && re.NumSubexp() != 0:
			return nil, fmt.Errorf("%w: %s: the pattern has a capture group but zone is not set", ErrMalformed, c.Name)
		}
		c.re = re
	}
	return &t, nil
}

// Default returns the table built into the program. It panics if the embedded
// file is invalid, which a test prevents.
//
// @returns {*Table} the default table
// @example
//
//	names := errorlog.Default().Names()
func Default() *Table {
	t, err := Parse(defaultJSON)
	if err != nil {
		panic("logagent: embedded errors.json: " + err.Error())
	}
	return t
}

// Names returns the category names in table order. Other is not among them.
//
// @returns {[]string} the names
// @example
//
//	for _, n := range tab.Names() { fmt.Println(n) }
func (t *Table) Names() []string {
	out := make([]string, len(t.Categories))
	for i, c := range t.Categories {
		out[i] = c.Name
	}
	return out
}

// Counts is what a stretch of an error log held, as numbers.
type Counts struct {
	// Lines is how many lines looked like nginx error log lines; Skipped how
	// many did not (blank lines are neither).
	Lines   int `json:"lines"`
	Skipped int `json:"skipped"`
	// First and Last are the times of the first and last lines, as the log
	// wrote them (the server's local time), as YYYY-MM-DD HH:MM:SS.
	First string `json:"first"`
	Last  string `json:"last"`
	// Levels counts lines by level, Categories by category (Other included).
	Levels     map[string]int `json:"levels"`
	Categories map[string]int `json:"categories"`
	// Zones counts, for a category with zones, lines by zone name.
	Zones map[string]map[string]int `json:"zones"`
	// Err says why the log could not be read; the counts are then empty.
	Err string `json:"error"`
}

var linePattern = regexp.MustCompile(`^(\d{4})/(\d{2})/(\d{2}) (\d{2}:\d{2}:\d{2}) \[([a-z]+)\] \d+#\d+: (?:\*\d+ )?(.*)$`)

// Count reads every line of r.
//
// @param r {io.Reader} the error log lines
// @param tab {*Table} the categories
// @returns {Counts} the counts
// @example
//
//	c := errorlog.Count(strings.NewReader(text), errorlog.Default())
func Count(r io.Reader, tab *Table) Counts {
	c := Counts{Levels: map[string]int{}, Categories: map[string]int{}, Zones: map[string]map[string]int{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		text := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(text) == "" {
			continue
		}
		m := linePattern.FindStringSubmatch(text)
		if m == nil {
			c.Skipped++
			continue
		}
		c.Lines++
		when := m[1] + "-" + m[2] + "-" + m[3] + " " + m[4]
		if c.First == "" {
			c.First = when
		}
		c.Last = when
		c.Levels[m[5]]++
		category := Other
		for _, cat := range tab.Categories {
			sub := cat.re.FindStringSubmatch(m[6])
			if sub == nil {
				continue
			}
			category = cat.Name
			if cat.Zone {
				if c.Zones[cat.Name] == nil {
					c.Zones[cat.Name] = map[string]int{}
				}
				c.Zones[cat.Name][sub[1]]++
			}
			break
		}
		c.Categories[category]++
	}
	return c
}

// File counts over the last n lines of the log at path.
//
// @param path {string} the error log
// @param n {int} how many lines from the end, at least 1
// @param tab {*Table} the categories
// @returns {Counts, error} the counts, or the error from reading the file
// @example
//
//	c, err := errorlog.File("/var/log/nginx/error.log", 10000, errorlog.Default())
func File(path string, n int, tab *Table) (Counts, error) {
	text, err := tail.Last(path, n)
	if err != nil {
		return Counts{}, err
	}
	return Count(strings.NewReader(text), tab), nil
}
