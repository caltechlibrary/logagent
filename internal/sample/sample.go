// Package sample counts, in a web server's access log, how often each field
// holds a value. It reads a log written with a known nginx log_format and
// keeps only counts: no address, query string or user agent survives, so the
// result can go into a report that is pasted into an issue (DR-0003, DR-0002).
package sample

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/caltechlibrary/logagent/internal/check"
	"github.com/caltechlibrary/logagent/internal/fields"
	"github.com/caltechlibrary/logagent/internal/tail"
)

// ErrFormat means a log_format cannot be turned into a reader: it has no
// variables, or two variables with nothing between them to tell where one ends.
var ErrFormat = errors.New("cannot read this log format")

// Values maps a variable name, without the dollar sign, to what one log line
// held for it.
type Values map[string]string

// Parser reads lines written with one log_format.
type Parser struct {
	re   *regexp.Regexp
	vars []string // one per capture group, in order, repeats included
	// older are shorter prefixes of the format, longest first, for a tolerant
	// parser: lines written before fields were appended to the format.
	older []variant
}

// variant is the format cut after one of its variables.
type variant struct {
	re   *regexp.Regexp
	vars []string
}

type part struct {
	lit string // literal text, or
	v   string // a variable name
}

// Compile builds a Parser from a log_format's text as nginx concatenates it,
// the quoted pieces joined with nothing between them. Each variable ends at the
// literal character that follows it in the format.
//
// @param format {string} the log format text
// @returns {*Parser, error} the parser, or an error matching ErrFormat
// @example
//
//	p, err := sample.Compile(`$remote_addr "$request" $status`)
func Compile(format string) (*Parser, error) { return compile(format, false) }

// CompileTolerant is Compile, and the Parser it returns can also read, with
// ParseOlder, a line written in an older, shorter form of the format. A web
// server's log format grows by appending fields, so the log of a window that
// spans a change holds both. The older forms are the format cut after each
// variable down to the one holding the status (or the time, if there is no
// status), so a line cut before those is still not accepted. Parse is
// unchanged and strict.
//
// @param format {string} the current log format text
// @returns {*Parser, error} the parser, or an error matching ErrFormat
// @example
//
//	p, err := sample.CompileTolerant(text)
//	values, older, ok := p.ParseOlder(line)
func CompileTolerant(format string) (*Parser, error) { return compile(format, true) }

func compile(format string, tolerant bool) (*Parser, error) {
	var parts []part
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			parts = append(parts, part{lit: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(format); {
		c := format[i]
		if c != '$' {
			lit.WriteByte(c)
			i++
			continue
		}
		j := i + 1
		var name string
		if j < len(format) && format[j] == '{' {
			end := strings.IndexByte(format[j:], '}')
			if end < 0 {
				return nil, fmt.Errorf("%w: ${ is never closed", ErrFormat)
			}
			name = format[j+1 : j+end]
			j += end + 1
		} else {
			k := j
			for k < len(format) && isNameChar(format[k]) {
				k++
			}
			name = format[j:k]
			j = k
		}
		if name == "" {
			return nil, fmt.Errorf("%w: a $ that names no variable", ErrFormat)
		}
		flush()
		parts = append(parts, part{v: name})
		i = j
	}
	flush()
	var b strings.Builder
	b.WriteString("^")
	p := &Parser{}
	for i, pt := range parts {
		if pt.v == "" {
			b.WriteString(regexp.QuoteMeta(pt.lit))
			continue
		}
		switch {
		case i == len(parts)-1:
			b.WriteString("(.*)")
		case parts[i+1].v != "":
			return nil, fmt.Errorf("%w: $%s and $%s follow each other with nothing between", ErrFormat, pt.v, parts[i+1].v)
		default:
			b.WriteString("([^" + regexp.QuoteMeta(parts[i+1].lit[:1]) + "]*)")
		}
		p.vars = append(p.vars, pt.v)
	}
	b.WriteString("$")
	if len(p.vars) == 0 {
		return nil, fmt.Errorf("%w: it has no variables", ErrFormat)
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	p.re = re
	if tolerant {
		p.older = olderForms(parts)
	}
	return p, nil
}

// olderForms builds the format's shorter forms, longest first. Each keeps the
// first k variables, down to the one that holds the status, and ends after that
// variable's closing quote or bracket, if the format has one, or at the end of
// the line.
func olderForms(parts []part) []variant {
	var at []int // index in parts of each variable
	for i, pt := range parts {
		if pt.v != "" {
			at = append(at, i)
		}
	}
	min := len(at) // no shorter form unless a required variable is found
	found := false
	for _, need := range []string{"status", "time_local"} {
		for k, i := range at {
			if parts[i].v == need {
				min, found = k+1, true
				break
			}
		}
		if found {
			break
		}
	}
	var out []variant
	for keep := len(at) - 1; keep >= min && keep >= 1; keep-- {
		last := at[keep-1]
		var b strings.Builder
		b.WriteString("^")
		var vars []string
		for i := 0; i < last; i++ {
			if parts[i].v == "" {
				b.WriteString(regexp.QuoteMeta(parts[i].lit))
				continue
			}
			b.WriteString("([^" + regexp.QuoteMeta(parts[i+1].lit[:1]) + "]*)")
			vars = append(vars, parts[i].v)
		}
		next := parts[last+1].lit // the literal after the last kept variable
		closing := ""
		for closing != next && strings.ContainsRune(`"')]`, rune(next[len(closing)])) {
			closing += next[len(closing) : len(closing)+1]
		}
		stop := next[:1]
		if closing != "" {
			stop = closing[:1]
		}
		b.WriteString("([^" + regexp.QuoteMeta(stop) + "]*)" + regexp.QuoteMeta(closing) + "$")
		vars = append(vars, parts[last].v)
		re, err := regexp.Compile(b.String())
		if err != nil {
			continue
		}
		out = append(out, variant{re: re, vars: vars})
	}
	return out
}

// ParseOlder reads one log line, accepting an older, shorter form of the format
// if the parser was made with CompileTolerant. The values of a line in an older
// form hold only the variables it had.
//
// @param line {string} the line, with or without a trailing newline
// @returns {Values} what the line held for each variable it had
// @returns {bool} true when the line matched an older, shorter form of the format
// @returns {bool} true when the line matched the format or one of its older forms
// @example
//
//	values, older, ok := p.ParseOlder(line)
func (p *Parser) ParseOlder(line string) (Values, bool, bool) {
	if v, ok := p.Parse(line); ok {
		return v, false, true
	}
	line = strings.TrimRight(line, "\r\n")
	for _, o := range p.older {
		m := o.re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		v := Values{}
		for i, name := range o.vars {
			if _, dup := v[name]; !dup {
				v[name] = m[i+1]
			}
		}
		return v, true, true
	}
	return nil, false, false
}

func isNameChar(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// Variables lists the variables of the format in order of first use, once each.
//
// @returns {[]string} the variable names without the dollar sign
// @example
//
//	names := p.Variables()
func (p *Parser) Variables() []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range p.vars {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// Parse reads one log line.
//
// @param line {string} the line, with or without a trailing newline
// @returns {Values, bool} what the line held for each variable, and whether it matched the format
// @example
//
//	values, ok := p.Parse(line)
func (p *Parser) Parse(line string) (Values, bool) {
	line = strings.TrimRight(line, "\r\n")
	m := p.re.FindStringSubmatch(line)
	if m == nil {
		return nil, false
	}
	v := Values{}
	for i, name := range p.vars {
		if _, dup := v[name]; !dup {
			v[name] = m[i+1]
		}
	}
	return v, true
}

// Count reads every line of r and counts, for each field of the table whose
// variable is in the format, the lines in which it held a value other than
// nothing or a dash. Lines that do not match the format are counted as skipped.
//
// @param r {io.Reader} the log lines
// @param p {*Parser} the reader for the log's format
// @param tab {*fields.Table} the field table, which names each field's variable
// @returns {check.Sample} the counts
// @example
//
//	s := sample.Count(strings.NewReader(text), p, fields.Default())
func Count(r io.Reader, p *Parser, tab *fields.Table) check.Sample {
	inFormat := map[string]bool{}
	for _, v := range p.Variables() {
		inFormat[v] = true
	}
	s := check.Sample{Present: map[string]int{}}
	for _, f := range tab.Fields {
		if inFormat[strings.TrimPrefix(f.Nginx.Expr, "$")] {
			s.Present[f.Name] = 0
		}
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		text := sc.Text()
		if strings.TrimSpace(text) == "" {
			continue
		}
		v, ok := p.Parse(text)
		if !ok {
			s.Skipped++
			continue
		}
		s.Lines++
		for _, f := range tab.Fields {
			if _, counted := s.Present[f.Name]; !counted {
				continue
			}
			if val := v[strings.TrimPrefix(f.Nginx.Expr, "$")]; val != "" && val != "-" {
				s.Present[f.Name]++
			}
		}
	}
	return s
}

// File counts over the last n lines of the log at path, reading from the end so
// a large log is not read through.
//
// @param path {string} the access log
// @param n {int} how many lines from the end to read, at least 1
// @param p {*Parser} the reader for the log's format
// @param tab {*fields.Table} the field table
// @returns {check.Sample, error} the counts, or the error from opening or reading the file
// @example
//
//	s, err := sample.File("/var/log/nginx/access.log", 10000, p, fields.Default())
func File(path string, n int, p *Parser, tab *fields.Table) (check.Sample, error) {
	text, err := tail.Last(path, n)
	if err != nil {
		return check.Sample{}, err
	}
	return Count(strings.NewReader(text), p, tab), nil
}
