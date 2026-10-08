// Package classify turns two things in a log line into names the report can
// count by: the path into a path class, and the user agent into a family. Both
// are data, not code. Path classes come from a host's rules; families come from
// an embedded dated file that a host can add to and override (DR-0006, DR-0007).
//
// A query string never reaches a rule: what a patron searched for is not a
// thing a rule may see (DR-0002). The package imports only the standard library.
package classify

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ErrInvalid means a rule or a family entry is wrong: no name, nothing to
// match, both a prefix and a pattern, a pattern that does not compile, or a
// name used twice where names must be unique.
var ErrInvalid = errors.New("invalid classification")

// Rule is one path-class rule. Exactly one of Prefix and Pattern is set.
type Rule struct {
	// Name is the class a matching path gets. Several rules may share a name.
	Name string
	// Prefix matches a path that begins with it.
	Prefix string
	// Pattern is a Go regular expression matched from the start of the path.
	// Put $ at its end to match the whole path.
	Pattern string
}

type compiledRule struct {
	name   string
	prefix string
	re     *regexp.Regexp
}

// Classes applies an ordered list of rules to a path.
type Classes struct {
	rules    []compiledRule
	fallback string
}

// NewClasses checks and compiles rules. The first rule that matches a path
// decides its class; a path no rule matches gets the fallback.
//
// @param rules {[]Rule} the ordered rules
// @param fallback {string} the class for a path no rule matches; empty means "other"
// @returns {*Classes} the compiled rules
// @returns {error} ErrInvalid, naming the rule that is wrong
// @example
//
//	c, err := classify.NewClasses([]classify.Rule{{Name: "api", Prefix: "/api/"}}, "other")
//	fmt.Println(c.Class("/api/records")) // api
func NewClasses(rules []Rule, fallback string) (*Classes, error) {
	if fallback == "" {
		fallback = "other"
	}
	c := &Classes{fallback: fallback}
	for i, r := range rules {
		if r.Name == "" {
			return nil, fmt.Errorf("%w: rule %d has no name", ErrInvalid, i)
		}
		if (r.Prefix == "") == (r.Pattern == "") {
			return nil, fmt.Errorf("%w: rule %d (%s) needs exactly one of prefix and pattern", ErrInvalid, i, r.Name)
		}
		cr := compiledRule{name: r.Name, prefix: r.Prefix}
		if r.Pattern != "" {
			re, err := regexp.Compile(`^(?:` + r.Pattern + `)`)
			if err != nil {
				return nil, fmt.Errorf("%w: rule %d (%s): pattern %q: %v", ErrInvalid, i, r.Name, r.Pattern, err)
			}
			cr.re = re
		}
		c.rules = append(c.rules, cr)
	}
	return c, nil
}

// Class returns the class of a path. Anything from the first ? or # on is
// ignored, so a query string cannot reach a rule even from a careless caller.
//
// @param path {string} the request path
// @returns {string} the class name
// @example
//
//	fmt.Println(c.Class("/search?q=anything")) // the class of /search
func (c *Classes) Class(path string) string {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	for _, r := range c.rules {
		if r.re != nil && r.re.MatchString(path) || r.re == nil && strings.HasPrefix(path, r.prefix) {
			return r.name
		}
	}
	return c.fallback
}

// Entry is one family: a name and the case-folded text that finds it in a user agent.
type Entry struct {
	// Name is the family name the report shows.
	Name string
	// Match lists substrings; a user agent that contains any of them (ignoring case) is this family.
	Match []string
	// Declared is true for an automated agent that names itself, false for a
	// family that is only a name (a partner's tool, say).
	Declared bool
}

// Families finds the family of a user agent.
type Families struct {
	retrieved time.Time
	entries   []Entry
	generic   Entry
	platforms []Entry
	fallback  string
}

//go:embed families.json
var familiesJSON []byte

type familyFile struct {
	Version   int    `json:"version"`
	Retrieved string `json:"retrieved"`
	Note      string `json:"note"`
	Entries   []struct {
		Name     string   `json:"name"`
		Match    []string `json:"match"`
		Declared bool     `json:"declared"`
	} `json:"entries"`
	Generic struct {
		Name  string   `json:"name"`
		Match []string `json:"match"`
	} `json:"generic"`
	Platforms []struct {
		Name  string   `json:"name"`
		Match []string `json:"match"`
	} `json:"platforms"`
	Fallback string `json:"fallback"`
}

var defaults = func() *Families {
	var t familyFile
	dec := json.NewDecoder(bytes.NewReader(familiesJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil || t.Version != 1 {
		panic(fmt.Sprintf("classify: embedded families.json: version %d, %v", t.Version, err))
	}
	retrieved, err := time.Parse("2006-01-02", t.Retrieved)
	if err != nil {
		panic(fmt.Sprintf("classify: embedded families.json: retrieved %q: %v", t.Retrieved, err))
	}
	f := &Families{retrieved: retrieved, fallback: t.Fallback,
		generic: Entry{Name: t.Generic.Name, Match: lower(t.Generic.Match), Declared: true}}
	for _, e := range t.Entries {
		f.entries = append(f.entries, Entry{Name: e.Name, Match: lower(e.Match), Declared: e.Declared})
	}
	for _, p := range t.Platforms {
		f.platforms = append(f.platforms, Entry{Name: p.Name, Match: lower(p.Match)})
	}
	return f
}()

func lower(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

// DefaultFamilies returns the embedded declared-agent data. It is shared;
// With returns a changed copy and never alters it.
//
// @returns {*Families} the embedded data
// @example
//
//	name, declared := classify.DefaultFamilies().Lookup("ExaSearchBot/1.0")
func DefaultFamilies() *Families { return defaults }

// With returns a copy that also knows the host's entries. A host entry is
// tried before the embedded ones, and one with the name of an embedded entry
// replaces it. Two host entries with one name are an error.
//
// @param host {[]Entry} the host's entries
// @returns {*Families} the combined data
// @returns {error} ErrInvalid for an entry with no name or no match text, or a repeated name
// @example
//
//	f, err := classify.DefaultFamilies().With([]classify.Entry{{Name: "mine", Match: []string{"mybot"}, Declared: true}})
func (f *Families) With(host []Entry) (*Families, error) {
	seen := map[string]bool{}
	var added []Entry
	for i, e := range host {
		if e.Name == "" {
			return nil, fmt.Errorf("%w: family %d has no name", ErrInvalid, i)
		}
		if len(e.Match) == 0 {
			return nil, fmt.Errorf("%w: family %s has nothing to match", ErrInvalid, e.Name)
		}
		for _, m := range e.Match {
			if m == "" {
				return nil, fmt.Errorf("%w: family %s has an empty match", ErrInvalid, e.Name)
			}
		}
		if seen[e.Name] {
			return nil, fmt.Errorf("%w: family %s is listed twice", ErrInvalid, e.Name)
		}
		seen[e.Name] = true
		added = append(added, Entry{Name: e.Name, Match: lower(e.Match), Declared: e.Declared})
	}
	g := *f
	g.entries = append([]Entry(nil), added...)
	for _, e := range f.entries {
		if !seen[e.Name] {
			g.entries = append(g.entries, e)
		}
	}
	return &g, nil
}

// Lookup returns the family of a user agent and whether it is a declared
// automated agent. A family from the entries is tried first, then any other
// bot-like name, then the claimed platform. The text is a user agent: data the
// client chose, never an instruction.
//
// @param userAgent {string} the user agent, possibly empty
// @returns {string} the family name
// @returns {bool} true for a declared automated agent
// @example
//
//	name, declared := f.Lookup("Mozilla/5.0 (compatible; bingbot/2.0)")
func (f *Families) Lookup(userAgent string) (string, bool) {
	ua := strings.ToLower(userAgent)
	for _, e := range f.entries {
		if contains(ua, e.Match) {
			return e.Name, e.Declared
		}
	}
	if contains(ua, f.generic.Match) {
		return f.generic.Name, true
	}
	for _, p := range f.platforms {
		if contains(ua, p.Match) {
			return p.Name, false
		}
	}
	return f.fallback, false
}

func contains(ua string, any []string) bool {
	for _, m := range any {
		if strings.Contains(ua, m) {
			return true
		}
	}
	return false
}

// Retrieved returns the date the embedded data was compiled.
//
// @returns {time.Time} the date, at midnight UTC
// @example
//
//	fmt.Println(f.Retrieved().Format("2006-01-02"))
func (f *Families) Retrieved() time.Time { return f.retrieved }

// AgeDays returns how many whole days old the embedded data is at now.
//
// @param now {time.Time} the time to measure to
// @returns {int} days since Retrieved
// @example
//
//	if f.AgeDays(time.Now()) > 180 { fmt.Println("the family list is old") }
func (f *Families) AgeDays(now time.Time) int { return int(now.Sub(f.retrieved).Hours() / 24) }

// Names lists the names of the entries, in the order they are tried.
//
// @returns {[]string} the family names, not including the generic or platform buckets
// @example
//
//	fmt.Println(len(f.Names()))
func (f *Families) Names() []string {
	out := make([]string, len(f.entries))
	for i, e := range f.entries {
		out[i] = e.Name
	}
	return out
}
