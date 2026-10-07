// Package nginxconf reads nginx configuration with a small tokenizer written
// against the standard library. Its input is the text `nginx -T` prints: a
// "# configuration file PATH:" line before each file, which lets every
// directive be traced to the file and line it was written on. Resolve then
// expands include directives in nginx's own order, so a caller can say which
// directive comes before which.
//
// The package reads the grammar and nothing more. It does not know which
// directives take a block, or what any directive means; nginx -t judges that.
package nginxconf

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// ErrSyntax is matched by every *SyntaxError.
var ErrSyntax = errors.New("nginx configuration syntax error")

// ErrInclude is matched by an include that cannot be resolved: a file absent
// from the dump, a bad pattern or a loop.
var ErrInclude = errors.New("nginx include error")

// SyntaxError is a problem in the text of one configuration file.
type SyntaxError struct {
	// File is the path from the dump, empty for a bare configuration.
	File string
	// Line is the line within that file, counted from 1.
	Line int
	// Msg says what is wrong.
	Msg string
}

// Error returns the message as FILE:LINE: text.
//
// @returns {string} the message, naming the file and line
// @example
//
//	fmt.Println(err) // /etc/nginx/nginx.conf:3: unexpected "}"
func (e *SyntaxError) Error() string {
	if e.File == "" {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
}

// Is reports whether target is ErrSyntax.
//
// @param target {error} the error to compare with
// @returns {bool} true for ErrSyntax
// @example
//
//	ok := errors.Is(err, nginxconf.ErrSyntax)
func (e *SyntaxError) Is(target error) bool { return target == ErrSyntax }

// Directive is one directive of the configuration, with its block if it has
// one.
type Directive struct {
	// Name is the first word. It is empty for a quoted empty string, as in a
	// map entry.
	Name string
	// Args are the remaining words, unquoted.
	Args []string
	// HasBlock is true when the directive ends in { } and not in a semicolon.
	HasBlock bool
	// Block holds the directives inside the braces, in order.
	Block []*Directive
	// File and Line say where the directive is written.
	File string
	Line int
	// Parent is the directive whose block holds this one; nil at top level.
	Parent *Directive
	// Include is the include directive that brought this directive's file in;
	// nil for a directive written in the main file. Set by Resolve.
	Include *Directive
}

// File is one configuration file of a dump.
type File struct {
	Path       string
	Directives []*Directive
}

// Dump is a parsed `nginx -T` dump: the files in the order nginx printed them,
// the main file first.
type Dump struct {
	Files []*File
}

var headerLine = regexp.MustCompile(`^# configuration file (.+):$`)

// Parse reads the text of an `nginx -T` dump, or a bare configuration with no
// file headers, which is taken as one file with an empty path. Lines before
// the first header (the "nginx: ... syntax is ok" lines) are ignored.
//
// @param text {string} the dump
// @returns {*Dump, error} the parsed files, or a *SyntaxError naming the file and line
// @example
//
//	d, err := nginxconf.Parse(text)
func Parse(text string) (*Dump, error) {
	lines := strings.Split(text, "\n")
	type chunk struct {
		path string
		body []string
	}
	var chunks []chunk
	for _, l := range lines {
		if m := headerLine.FindStringSubmatch(strings.TrimRight(l, "\r")); m != nil {
			chunks = append(chunks, chunk{path: m[1]})
			continue
		}
		if len(chunks) > 0 {
			chunks[len(chunks)-1].body = append(chunks[len(chunks)-1].body, l)
		}
	}
	if len(chunks) == 0 {
		chunks = []chunk{{body: lines}}
	}
	d := &Dump{}
	for _, c := range chunks {
		ds, err := parseFile(c.path, strings.Join(c.body, "\n"))
		if err != nil {
			return nil, err
		}
		d.Files = append(d.Files, &File{Path: c.path, Directives: ds})
	}
	return d, nil
}

// parseFile tokenizes and assembles one file.
func parseFile(file, src string) ([]*Directive, error) {
	var (
		top   []*Directive
		stack []*Directive // open blocks, outermost first
		words []string
		wline int
		line  = 1
		i     = 0
	)
	fail := func(l int, format string, a ...any) error {
		return &SyntaxError{File: file, Line: l, Msg: fmt.Sprintf(format, a...)}
	}
	add := func(d *Directive) {
		d.File = file
		if n := len(stack); n > 0 {
			d.Parent = stack[n-1]
			stack[n-1].Block = append(stack[n-1].Block, d)
		} else {
			top = append(top, d)
		}
	}
	mk := func(block bool) *Directive {
		d := &Directive{Name: words[0], Args: append([]string(nil), words[1:]...), HasBlock: block, Line: wline}
		if block {
			d.Block = []*Directive{}
		}
		words = nil
		return d
	}
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == ';':
			if len(words) == 0 {
				return nil, fail(line, `unexpected ";"`)
			}
			add(mk(false))
			i++
		case c == '{':
			if len(words) == 0 {
				return nil, fail(line, `unexpected "{"`)
			}
			d := mk(true)
			add(d)
			stack = append(stack, d)
			i++
		case c == '}':
			if len(words) > 0 {
				return nil, fail(wline, `directive %q is missing its semicolon before "}"`, words[0])
			}
			if len(stack) == 0 {
				return nil, fail(line, `unexpected "}"`)
			}
			stack = stack[:len(stack)-1]
			i++
		case c == '"' || c == '\'':
			start := line
			var b strings.Builder
			i++
			closed := false
			for i < len(src) {
				ch := src[i]
				if ch == '\\' && i+1 < len(src) {
					n := src[i+1]
					switch n {
					case '"', '\'', '\\':
						b.WriteByte(n)
					case 't':
						b.WriteByte('\t')
					case 'r':
						b.WriteByte('\r')
					case 'n':
						b.WriteByte('\n')
					default:
						b.WriteByte('\\')
						b.WriteByte(n)
					}
					if n == '\n' {
						line++
					}
					i += 2
					continue
				}
				if ch == c {
					closed = true
					i++
					break
				}
				if ch == '\n' {
					line++
				}
				b.WriteByte(ch)
				i++
			}
			if !closed {
				return nil, fail(start, "unterminated quote (%c) never closed", c)
			}
			if len(words) == 0 {
				wline = start
			}
			words = append(words, b.String())
		default:
			if len(words) == 0 {
				wline = line
			}
			j := i
			for j < len(src) {
				ch := src[j]
				if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == ';' {
					break
				}
				if ch == '{' {
					if j > i && src[j-1] == '$' { // ${name}
						k := strings.IndexByte(src[j:], '}')
						if k >= 0 && !strings.ContainsAny(src[j:j+k], " \t\r\n;") {
							j += k + 1
							continue
						}
					}
					break
				}
				j++
			}
			words = append(words, src[i:j])
			i = j
		}
	}
	if len(words) > 0 {
		return nil, fail(wline, "directive %q is missing its semicolon at the end of the file", words[0])
	}
	if len(stack) > 0 {
		return nil, fail(stack[0].Line, `block %q opened here is never closed: expecting "}"`, stack[0].Name)
	}
	return top, nil
}

// Resolve returns the top-level directives of the main file with every include
// replaced, in place, by the directives of the files it names. A relative
// include is looked up under the main file's directory, as nginx does, and a
// pattern's matches are taken in sorted order. The dump itself is not changed;
// the result is a copy.
//
// @returns {[]*Directive, error} the resolved tree, or an error matching ErrInclude
// @example
//
//	tree, err := dump.Resolve()
func (d *Dump) Resolve() ([]*Directive, error) {
	if len(d.Files) == 0 {
		return nil, nil
	}
	r := resolver{files: d.Files, byPath: map[string]*File{}, prefix: path.Dir(d.Files[0].Path)}
	if d.Files[0].Path == "" {
		r.prefix = ""
	}
	for _, f := range d.Files {
		r.byPath[f.Path] = f
	}
	return r.expand(d.Files[0].Directives, nil, nil, []string{d.Files[0].Path})
}

type resolver struct {
	files  []*File
	byPath map[string]*File
	prefix string
}

func (r *resolver) expand(in []*Directive, parent, via *Directive, active []string) ([]*Directive, error) {
	out := []*Directive{}
	for _, d := range in {
		if d.Name == "include" && !d.HasBlock && len(d.Args) == 1 {
			files, err := r.match(d)
			if err != nil {
				return nil, err
			}
			for _, f := range files {
				for _, a := range active {
					if a == f.Path {
						return nil, fmt.Errorf("%w: %s:%d includes %s again (a loop)", ErrInclude, d.File, d.Line, f.Path)
					}
				}
				sub, err := r.expand(f.Directives, parent, d, append(active[:len(active):len(active)], f.Path))
				if err != nil {
					return nil, err
				}
				out = append(out, sub...)
			}
			continue
		}
		c := *d
		c.Parent, c.Include = parent, via
		c.Args = append([]string(nil), d.Args...)
		if d.HasBlock {
			block, err := r.expand(d.Block, &c, via, active)
			if err != nil {
				return nil, err
			}
			c.Block = block
		}
		out = append(out, &c)
	}
	return out, nil
}

// match finds the files an include directive names.
func (r *resolver) match(d *Directive) ([]*File, error) {
	pat := d.Args[0]
	if !path.IsAbs(pat) {
		pat = path.Join(r.prefix, pat)
	}
	if !strings.ContainsAny(pat, "*?[") {
		f, ok := r.byPath[pat]
		if !ok {
			return nil, fmt.Errorf("%w: %s:%d includes %s, which is not in the dump", ErrInclude, d.File, d.Line, pat)
		}
		return []*File{f}, nil
	}
	var out []*File
	for _, f := range r.files {
		ok, err := path.Match(pat, f.Path)
		if err != nil {
			return nil, fmt.Errorf("%w: %s:%d: bad pattern %q: %v", ErrInclude, d.File, d.Line, pat, err)
		}
		if ok {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Context returns the names of the directives that enclose this one,
// outermost first, for example http, server, location.
//
// @returns {[]string} the enclosing names, nil at top level
// @example
//
//	ctx := d.Context() // [http server]
func (d *Directive) Context() []string {
	var out []string
	for p := d.Parent; p != nil; p = p.Parent {
		out = append([]string{p.Name}, out...)
	}
	return out
}

// Walk calls fn for every directive of ds and of the blocks inside them,
// parents before children, in document order.
//
// @param ds {[]*Directive} the directives to walk
// @param fn {func(*Directive)} called once per directive
// @example
//
//	nginxconf.Walk(tree, func(d *nginxconf.Directive) { fmt.Println(d.Name) })
func Walk(ds []*Directive, fn func(*Directive)) {
	for _, d := range ds {
		fn(d)
		Walk(d.Block, fn)
	}
}

// Find returns the directives reached by following names down the blocks: the
// first name among ds, the second among the children of those, and so on.
//
// @param ds {[]*Directive} where to start
// @param names {...string} the path of directive names
// @returns {[]*Directive} the directives at the end of the path, in order
// @example
//
//	listens := nginxconf.Find(tree, "http", "server", "listen")
func Find(ds []*Directive, names ...string) []*Directive {
	for i, n := range names {
		var next []*Directive
		for _, d := range ds {
			if d.Name == n {
				next = append(next, d)
			}
		}
		if i == len(names)-1 {
			return next
		}
		ds = nil
		for _, d := range next {
			ds = append(ds, d.Block...)
		}
	}
	return nil
}
