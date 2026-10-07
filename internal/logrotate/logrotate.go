// Package logrotate reads logrotate configuration far enough to say how long
// it keeps a log. The web server's own access log is layer 1 of the data
// layers (DR-0002), and its retention is whatever logrotate leaves, so check
// compares that with the floor the tiers need and the ceiling policy allows.
//
// The package reads the options that decide how long rotated logs live
// (interval, rotate, maxage, size, maxsize) and skips the rest, including the
// scripts. It does not follow include directives; a caller that wants a second
// file's global options parses that file too and passes them as defaults.
package logrotate

import (
	"errors"
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
)

// ErrSyntax is matched by every *SyntaxError.
var ErrSyntax = errors.New("logrotate configuration syntax error")

// SyntaxError is a problem in a logrotate file.
type SyntaxError struct {
	File string
	Line int
	Msg  string
}

// Error returns the message as FILE:LINE: text.
//
// @returns {string} the message
// @example
//
//	fmt.Println(err) // /etc/logrotate.d/nginx:3: rotate needs a whole number
func (e *SyntaxError) Error() string { return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg) }

// Is reports whether target is ErrSyntax.
//
// @param target {error} the error to compare with
// @returns {bool} true for ErrSyntax
// @example
//
//	ok := errors.Is(err, logrotate.ErrSyntax)
func (e *SyntaxError) Is(target error) bool { return target == ErrSyntax }

// Options are the settings that decide how long logs are kept.
type Options struct {
	// Interval is hourly, daily, weekly, monthly or yearly; empty if unset.
	Interval string
	// Rotate is how many rotated copies are kept; -1 keeps them without limit.
	Rotate    int
	RotateSet bool
	// MaxAge removes rotated copies older than this many days.
	MaxAge    int
	MaxAgeSet bool
	// Size rotates only when the log reaches this size, ignoring the interval.
	Size string
	// MaxSize rotates when the log reaches this size even before the interval.
	MaxSize string
}

// IntervalSet reports whether an interval was given.
//
// @returns {bool} true when Interval is set
// @example
//
//	if !opts.IntervalSet() { fmt.Println("no interval") }
func (o Options) IntervalSet() bool { return o.Interval != "" }

// Stanza is one block of logrotate options for a list of logs.
type Stanza struct {
	// Paths are the log paths or patterns the block governs.
	Paths   []string
	Options Options
	// File and Line say where the block starts, at its first path.
	File string
	Line int
}

// File is a parsed logrotate file.
type File struct {
	Path string
	// Global holds the options written outside any block, which are defaults
	// for the blocks that do not set them.
	Global Options
	// Includes are the include directives, recorded and not followed.
	Includes []string
	Stanzas  []*Stanza
}

var intervals = map[string]bool{"hourly": true, "daily": true, "weekly": true, "monthly": true, "yearly": true}

var scriptStarts = map[string]bool{"prerotate": true, "postrotate": true, "firstaction": true, "lastaction": true, "preremove": true}

// fields splits a line into words, keeping a quoted word whole.
func fields(line string) []string {
	var out []string
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '"' || c == '\'':
			j := strings.IndexByte(line[i+1:], c)
			if j < 0 {
				out = append(out, line[i+1:])
				i = len(line)
			} else {
				out = append(out, line[i+1:i+1+j])
				i += j + 2
			}
		default:
			j := i
			for j < len(line) && line[j] != ' ' && line[j] != '\t' {
				j++
			}
			out = append(out, line[i:j])
			i = j
		}
	}
	return out
}

func isPathWord(w string) bool {
	return strings.HasPrefix(w, "/") || strings.HasPrefix(w, "*") || strings.HasPrefix(w, "~")
}

// Parse reads a logrotate file.
//
// @param file {string} the file's path, used in messages
// @param text {string} the file's contents
// @returns {*File, error} the parsed file, or a *SyntaxError naming the file and line
// @example
//
//	f, err := logrotate.Parse("/etc/logrotate.d/nginx", text)
func Parse(file, text string) (*File, error) {
	f := &File{Path: file}
	fail := func(line int, format string, a ...any) error {
		return &SyntaxError{File: file, Line: line, Msg: fmt.Sprintf(format, a...)}
	}
	var (
		cur        *Stanza
		pending    []string
		pendingAt  int
		scriptAt   int
		inScript   bool
		lineNumber int
	)
	apply := func(o *Options, w []string, line int) error {
		switch w[0] {
		case "hourly", "daily", "weekly", "monthly", "yearly":
			o.Interval = w[0]
		case "rotate":
			n, err := wholeNumber(w)
			if err != nil {
				return fail(line, "rotate needs a whole number (got %q)", strings.Join(w[1:], " "))
			}
			o.Rotate, o.RotateSet = n, true
		case "maxage":
			n, err := wholeNumber(w)
			if err != nil || n < 0 {
				return fail(line, "maxage needs a number of days (got %q)", strings.Join(w[1:], " "))
			}
			o.MaxAge, o.MaxAgeSet = n, true
		case "size", "maxsize":
			if len(w) < 2 {
				return fail(line, "%s needs a size", w[0])
			}
			if w[0] == "size" {
				o.Size = w[1]
			} else {
				o.MaxSize = w[1]
			}
		}
		return nil
	}
	for _, raw := range strings.Split(text, "\n") {
		lineNumber++
		line := strings.TrimSpace(raw)
		if inScript {
			if line == "endscript" {
				inScript = false
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		w := fields(line)
		if cur != nil { // inside a block
			switch {
			case line == "}":
				f.Stanzas = append(f.Stanzas, cur)
				cur = nil
			case scriptStarts[w[0]]:
				inScript, scriptAt = true, lineNumber
			default:
				if err := apply(&cur.Options, w, lineNumber); err != nil {
					return nil, err
				}
			}
			continue
		}
		// Outside a block: a global option, an include, a list of paths, or a brace.
		switch {
		case w[0] == "}":
			return nil, fail(lineNumber, `unexpected "}"`)
		case w[0] == "{":
			if len(pending) == 0 {
				return nil, fail(lineNumber, `unexpected "{" with no log path before it`)
			}
			cur = &Stanza{Paths: pending, File: file, Line: pendingAt}
			pending = nil
		case len(pending) == 0 && w[0] == "include" && len(w) > 1:
			f.Includes = append(f.Includes, w[1])
		case len(pending) == 0 && !isPathWord(w[0]):
			if err := apply(&f.Global, w, lineNumber); err != nil {
				return nil, err
			}
		default:
			if len(pending) == 0 {
				pendingAt = lineNumber
			}
			opens := false
			for _, word := range w {
				if word == "{" && !strings.HasPrefix(line, `"`) {
					opens = true
					break
				}
				pending = append(pending, word)
			}
			if opens {
				cur = &Stanza{Paths: pending, File: file, Line: pendingAt}
				pending = nil
			}
		}
	}
	switch {
	case inScript:
		return nil, fail(scriptAt, "script has no endscript")
	case cur != nil:
		return nil, fail(cur.Line, "block opened here is never closed")
	case len(pending) > 0:
		return nil, fail(pendingAt, "log paths with no block after them")
	}
	return f, nil
}

func wholeNumber(w []string) (int, error) {
	if len(w) < 2 {
		return 0, errors.New("missing")
	}
	return strconv.Atoi(w[1])
}

// For returns the first block whose paths match the log.
//
// @param logPath {string} the path of an access log
// @returns {*Stanza, bool} the governing block and whether there is one
// @example
//
//	s, ok := f.For("/var/log/nginx/access.log")
func (f *File) For(logPath string) (*Stanza, bool) {
	for _, s := range f.Stanzas {
		for _, p := range s.Paths {
			if p == logPath {
				return s, true
			}
			if ok, err := path.Match(p, logPath); err == nil && ok {
				return s, true
			}
		}
	}
	return nil, false
}

// Retention says how long a log is kept.
type Retention struct {
	// Interval, Rotate, MaxAgeDays and MaxSize are the effective settings.
	Interval   string
	Rotate     int
	MaxAgeDays int
	MaxSize    string
	// LowDays is how much history is certainly there, just after a rotation.
	// HighDays is the most there can be, just before one.
	LowDays, HighDays float64
	// Unlimited is true when rotated logs are never removed.
	Unlimited bool
	// Indeterminate says why the span cannot be counted in days, or is empty.
	Indeterminate string
}

// intervalDays is the shortest and longest an interval can be, in days.
var intervalDays = map[string][2]float64{
	"hourly": {1.0 / 24, 1.0 / 24}, "daily": {1, 1}, "weekly": {7, 7}, "monthly": {28, 31}, "yearly": {365, 365},
}

// Retention works out how long the block keeps its logs. Options the block does
// not set are taken from defaults, in order, and then from logrotate's own: no
// rotated copies are kept.
//
// @param defaults {...Options} global options to fall back on, most specific first
// @returns {Retention} the span in days, or why it cannot be counted
// @example
//
//	r := stanza.Retention(file.Global)
func (s *Stanza) Retention(defaults ...Options) Retention {
	o := s.Options
	for _, d := range defaults {
		if o.Interval == "" {
			o.Interval = d.Interval
		}
		if !o.RotateSet && d.RotateSet {
			o.Rotate, o.RotateSet = d.Rotate, true
		}
		if !o.MaxAgeSet && d.MaxAgeSet {
			o.MaxAge, o.MaxAgeSet = d.MaxAge, true
		}
		if o.Size == "" {
			o.Size = d.Size
		}
		if o.MaxSize == "" {
			o.MaxSize = d.MaxSize
		}
	}
	r := Retention{Interval: o.Interval, Rotate: o.Rotate, MaxAgeDays: o.MaxAge, MaxSize: o.MaxSize}
	if o.Size != "" {
		r.Indeterminate = fmt.Sprintf("it rotates by size (size %s), not by time, so the days kept depend on traffic", o.Size)
		return r
	}
	span, ok := intervalDays[o.Interval]
	if !ok {
		r.Indeterminate = "no rotation interval (daily, weekly, monthly...) is set in the block or in the defaults that were read"
		return r
	}
	low, high := span[0], span[1]
	if o.Rotate < 0 {
		if !o.MaxAgeSet {
			r.Unlimited = true
			return r
		}
		r.LowDays, r.HighDays = float64(o.MaxAge), float64(o.MaxAge)+high
		return r
	}
	r.LowDays, r.HighDays = float64(o.Rotate)*low, float64(o.Rotate+1)*high
	if o.MaxAgeSet {
		r.LowDays = math.Min(r.LowDays, float64(o.MaxAge))
		r.HighDays = math.Min(r.HighDays, float64(o.MaxAge)+high)
	}
	return r
}
