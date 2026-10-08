// Package logread reads a web server's access log, with its rotated and
// gzipped files, once, in time order, and hands each line to a callback as the
// values of the log format's variables. It keeps nothing: memory is bounded
// by one line (DR-0006, decision 3). Turning those values into events, with the
// privacy rule, is the caller's job (internal/event), so that rule lives in one
// place.
package logread

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/caltechlibrary/logagent/internal/nginxconf"
	"github.com/caltechlibrary/logagent/internal/sample"
)

var (
	// ErrMismatch means most of a log's lines do not match the format that was
	// named for it: the log is probably written with another format, or is the
	// wrong file. It corresponds to exit status 65.
	ErrMismatch = errors.New("log is not in the named format")
	// ErrNotConfigured means the nginx configuration has no access_log for the
	// path, or names a log_format it does not define.
	ErrNotConfigured = errors.New("log not found in the nginx configuration")
	// ErrUnsupported means the log_format is one the reader cannot read (escape=json).
	ErrUnsupported = errors.New("log format not supported")
	// ErrNoTime means the log_format has no $time_local, so lines cannot be placed in a window.
	ErrNoTime = errors.New("log format has no $time_local")
)

// Combined is nginx's built-in combined log format.
const Combined = `$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"`

// timeLayout is $time_local.
const timeLayout = "02/Jan/2006:15:04:05 -0700"

const (
	// judgeAfter is how many lines a file must have before it can be called the
	// wrong format; a short file is noise, not a verdict.
	judgeAfter = 20
	// earlyAfter is how many lines are read before a clearly wrong format is
	// reported without reading the rest of a large file.
	earlyAfter = 200
)

// Window is the span of event time to read: Since is included, Until is not.
// A zero value is no limit on that side.
type Window struct {
	Since, Until time.Time
}

// Stats says what a read covered.
type Stats struct {
	// Files lists the files read, oldest first.
	Files []string
	// Lines counts lines delivered to the callback.
	Lines int
	// Skipped counts lines that did not match the format or had no readable time.
	Skipped int
	// Outside counts lines that matched but fell outside the window.
	Outside int
	// First and Last are the earliest and latest times among the delivered lines.
	First, Last time.Time
}

var rotated = func(base string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(base) + `\.([0-9]+)(\.gz)?$`)
}

// Files lists a log and its rotated files, oldest first: access.log.10.gz
// before access.log.2.gz before access.log.1 before access.log. Only
// logrotate's numbered names are included; a file that is not there is left
// out, and when both access.log.N and access.log.N.gz exist the plain one is used.
//
// @param path {string} the live log, such as /var/log/nginx/access.log
// @returns {[]string} the files that exist, oldest first, empty when there are none
// @returns {error} the error from reading the directory, unless it does not exist
// @example
//
//	files, err := logread.Files("/var/log/nginx/access.log")
func Files(path string) ([]string, error) {
	dir, base := filepath.Dir(path), filepath.Base(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	re := rotated(base)
	type rot struct {
		n    int
		name string
		gz   bool
	}
	byN := map[int]rot{}
	live := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() == base {
			live = true
			continue
		}
		m := re.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		cur, seen := byN[n]
		if !seen || (cur.gz && m[2] == "") {
			byN[n] = rot{n: n, name: e.Name(), gz: m[2] != ""}
		}
	}
	list := make([]rot, 0, len(byN))
	for _, r := range byN {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
	var out []string
	for _, r := range list {
		out = append(out, filepath.Join(dir, r.name))
	}
	if live {
		out = append(out, path)
	}
	return out, nil
}

// Read delivers every line of the log and its rotated files that falls inside
// the window, oldest first, to fn. A rotated file whose modification time is
// before the window opens is not read: logrotate leaves a rotated file with the
// time of its last line, so it cannot hold a line inside the window. Lines are
// then filtered by their own time, so a window may open in the middle of a
// file. Lines that do not match the format are counted, not delivered; blank
// lines are ignored.
//
// @param path {string} the live log
// @param p {*sample.Parser} the reader for the log's format
// @param w {Window} the span of time to read
// @param fn {func(sample.Values, time.Time)} called once per line inside the window, with its time
// @returns {Stats} what was covered, also when an error is returned
// @returns {error} an error matching fs.ErrNotExist when no file is there, ErrMismatch when
// most lines are in another format, bufio.ErrTooLong for a line over 4 MB, or a read error
// @example
//
//	st, err := logread.Read(path, parser, logread.Window{}, func(v sample.Values, t time.Time) {
//		fmt.Println(t, v["status"])
//	})
func Read(path string, p *sample.Parser, w Window, fn func(sample.Values, time.Time)) (Stats, error) {
	var st Stats
	files, err := Files(path)
	if err != nil {
		return st, err
	}
	if len(files) == 0 {
		return st, fmt.Errorf("%w: %s", fs.ErrNotExist, path)
	}
	for _, f := range files {
		if f != path && !w.Since.IsZero() {
			if fi, err := os.Stat(f); err == nil && fi.ModTime().Before(w.Since) {
				continue
			}
		}
		st.Files = append(st.Files, f)
		if err := readFile(f, p, w, fn, &st); err != nil {
			return st, err
		}
	}
	return st, nil
}

func readFile(name string, p *sample.Parser, w Window, fn func(sample.Values, time.Time), st *Stats) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(name, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		defer gz.Close()
		r = gz
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var total, bad int
	wrong := func() bool { return total >= judgeAfter && bad*2 > total }
	for sc.Scan() {
		text := sc.Text()
		if strings.TrimSpace(text) == "" {
			continue
		}
		total++
		v, ok := p.Parse(text)
		var t time.Time
		if ok {
			t, err = time.Parse(timeLayout, v["time_local"])
			ok = err == nil
		}
		if !ok {
			bad++
			st.Skipped++
			if total == earlyAfter && wrong() {
				return fmt.Errorf("%s: %w", name, ErrMismatch)
			}
			continue
		}
		if (!w.Since.IsZero() && t.Before(w.Since)) || (!w.Until.IsZero() && !t.Before(w.Until)) {
			st.Outside++
			continue
		}
		st.Lines++
		if st.First.IsZero() || t.Before(st.First) {
			st.First = t
		}
		if t.After(st.Last) {
			st.Last = t
		}
		fn(v, t)
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if wrong() {
		return fmt.Errorf("%s: %w", name, ErrMismatch)
	}
	return nil
}

// FormatFor finds the log_format text that writes the log at path, from a
// resolved nginx configuration: the format named by the first access_log for
// that path, or nginx's combined format when none is named. The pieces of a
// quoted log_format are joined with nothing between them, as nginx does, which
// is the text sample.Compile wants.
//
// @param tree {[]*nginxconf.Directive} a resolved configuration, from Dump.Resolve
// @param path {string} the access log's path
// @returns {string} the format text
// @returns {error} ErrNotConfigured, ErrUnsupported for escape=json, or ErrNoTime
// @example
//
//	text, err := logread.FormatFor(tree, "/var/log/nginx/access.log")
//	parser, err := sample.Compile(text)
func FormatFor(tree []*nginxconf.Directive, path string) (string, error) {
	var al *nginxconf.Directive
	nginxconf.Walk(tree, func(d *nginxconf.Directive) {
		if al == nil && d.Name == "access_log" && len(d.Args) > 0 && d.Args[0] == path {
			al = d
		}
	})
	if al == nil {
		return "", fmt.Errorf("%w: no access_log for %s", ErrNotConfigured, path)
	}
	name := "combined"
	if len(al.Args) > 1 && !strings.Contains(al.Args[1], "=") && al.Args[1] != "gzip" {
		name = al.Args[1]
	}
	text := Combined
	found := false
	for _, d := range nginxconf.Find(tree, "http", "log_format") {
		if len(d.Args) > 0 && d.Args[0] == name {
			pieces := d.Args[1:]
			if len(pieces) > 0 && strings.HasPrefix(pieces[0], "escape=") {
				if pieces[0] == "escape=json" {
					return "", fmt.Errorf("%w: log_format %s uses escape=json", ErrUnsupported, name)
				}
				pieces = pieces[1:]
			}
			text, found = strings.Join(pieces, ""), true
			break
		}
	}
	if !found && name != "combined" {
		return "", fmt.Errorf("%w: access_log %s names log_format %s, which is not defined", ErrNotConfigured, path, name)
	}
	if !strings.Contains(text, "$time_local") && !strings.Contains(text, "${time_local}") {
		return "", fmt.Errorf("%w: %s", ErrNoTime, name)
	}
	return text, nil
}
