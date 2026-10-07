package nginxconf

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// one parses a bare configuration (no "# configuration file" header) and
// returns the directives of its single file.
func one(t *testing.T, text string) []*Directive {
	t.Helper()
	d, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(d.Files))
	}
	return d.Files[0].Directives
}

func TestSimpleDirectives(t *testing.T) {
	ds := one(t, "worker_processes 4;\nerror_log /var/log/nginx/error.log warn;\ndaemon on;\n")
	want := []struct {
		name string
		args []string
		line int
	}{
		{"worker_processes", []string{"4"}, 1},
		{"error_log", []string{"/var/log/nginx/error.log", "warn"}, 2},
		{"daemon", []string{"on"}, 3},
	}
	if len(ds) != len(want) {
		t.Fatalf("got %d directives, want %d", len(ds), len(want))
	}
	for i, w := range want {
		d := ds[i]
		if d.Name != w.name || !reflect.DeepEqual(d.Args, w.args) || d.Line != w.line || d.HasBlock {
			t.Errorf("directive %d = %+v, want %+v", i, d, w)
		}
	}
}

func TestDirectiveWithNoArguments(t *testing.T) {
	ds := one(t, "internal;\n")
	if len(ds) != 1 || ds[0].Name != "internal" || len(ds[0].Args) != 0 {
		t.Errorf("got %+v", ds)
	}
}

func TestCommentsAreSkipped(t *testing.T) {
	ds := one(t, "# a whole-line comment; with a semicolon { and a brace\nlisten 443 ssl; # trailing comment }\n#last")
	if len(ds) != 1 || ds[0].Name != "listen" || !reflect.DeepEqual(ds[0].Args, []string{"443", "ssl"}) {
		t.Errorf("got %+v", ds)
	}
	if ds[0].Line != 2 {
		t.Errorf("line = %d, want 2", ds[0].Line)
	}
}

func TestQuotedArguments(t *testing.T) {
	cases := []struct {
		src  string
		args []string
	}{
		{`add_header X-Frame-Options "SAMEORIGIN";`, []string{"X-Frame-Options", "SAMEORIGIN"}},
		{`add_header X 'two words';`, []string{"X", "two words"}},
		{`set $x "has ; and { and # inside";`, []string{"$x", "has ; and { and # inside"}},
		{`set $x "say \"hi\"";`, []string{"$x", `say "hi"`}},
		{`set $x 'it\'s';`, []string{"$x", "it's"}},
		{`set $x "back\\slash";`, []string{"$x", `back\slash`}},
		{`set $x "";`, []string{"$x", ""}},
		{`return 301 https://$host$request_uri;`, []string{"301", "https://$host$request_uri"}},
		{`set $x ${host}_y;`, []string{"$x", "${host}_y"}},
	}
	for _, c := range cases {
		ds := one(t, c.src)
		if len(ds) != 1 || !reflect.DeepEqual(ds[0].Args, c.args) {
			t.Errorf("%s: args = %q, want %q", c.src, ds[0].Args, c.args)
		}
	}
}

func TestLogFormatKeepsItsStringsAsSeparateArguments(t *testing.T) {
	ds := one(t, "log_format fmt\n    '$remote_addr \"$request\" '\n    '$status';\n")
	want := []string{"fmt", `$remote_addr "$request" `, "$status"}
	if len(ds) != 1 || !reflect.DeepEqual(ds[0].Args, want) {
		t.Errorf("args = %q, want %q", ds[0].Args, want)
	}
}

func TestBlocksNestAndKeepOrderAndParents(t *testing.T) {
	src := `http {
    server {
        listen 80;
        location /api/ {
            proxy_pass http://app;
        }
        location = /health { return 200; }
    }
    server { listen 81; }
}
`
	ds := one(t, src)
	if len(ds) != 1 || ds[0].Name != "http" || !ds[0].HasBlock || len(ds[0].Block) != 2 {
		t.Fatalf("http = %+v", ds[0])
	}
	srv := ds[0].Block[0]
	if srv.Name != "server" || len(srv.Block) != 3 || srv.Parent != ds[0] {
		t.Fatalf("server = %+v", srv)
	}
	loc := srv.Block[1]
	if loc.Name != "location" || !reflect.DeepEqual(loc.Args, []string{"/api/"}) || loc.Line != 4 {
		t.Errorf("location = %+v", loc)
	}
	if got := loc.Block[0].Context(); !reflect.DeepEqual(got, []string{"http", "server", "location"}) {
		t.Errorf("Context = %v", got)
	}
	if len(ds[0].Context()) != 0 {
		t.Errorf("top level Context = %v, want none", ds[0].Context())
	}
	if srv.Block[2].Args[0] != "=" || srv.Block[2].Args[1] != "/health" {
		t.Errorf("location args = %v", srv.Block[2].Args)
	}
}

func TestEmptyBlockIsABlock(t *testing.T) {
	ds := one(t, "events {}\ntypes { }\n")
	for _, d := range ds {
		if !d.HasBlock || len(d.Block) != 0 {
			t.Errorf("%s: HasBlock=%v len=%d", d.Name, d.HasBlock, len(d.Block))
		}
	}
}

// nginx splits arguments on whitespace only, so the parentheses of an if stay
// attached to its first and last words.
func TestIfAndMapAndRegexLocation(t *testing.T) {
	src := `server {
    if ($host = authors.library.caltech.edu) {
        return 301 https://$host$request_uri;
    }
    location ~* \.(js|css)$ { expires 1d; }
}
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
    ~^x     keep;
}
`
	ds := one(t, src)
	iff := ds[0].Block[0]
	if iff.Name != "if" || !reflect.DeepEqual(iff.Args, []string{"($host", "=", "authors.library.caltech.edu)"}) {
		t.Errorf("if = %+v", iff)
	}
	loc := ds[0].Block[1]
	if !reflect.DeepEqual(loc.Args, []string{"~*", `\.(js|css)$`}) {
		t.Errorf("regex location args = %q", loc.Args)
	}
	m := ds[1]
	if m.Name != "map" || len(m.Block) != 3 || m.Block[1].Name != "" || m.Block[2].Name != "~^x" {
		t.Errorf("map = %+v", m)
	}
	if m.Block[0].Name != "default" || m.Block[0].Args[0] != "upgrade" || m.Block[1].Args[0] != "close" {
		t.Errorf("map entries = %+v %+v", m.Block[0], m.Block[1])
	}
}

func TestSyntaxErrorsNameFileAndLine(t *testing.T) {
	cases := []struct {
		name, src, mention string
		line               int
	}{
		{"unterminated quote", "set $x \"abc;\nlisten 80;\n", "quote", 1},
		{"missing semicolon at end", "listen 80", "semicolon", 1},
		{"missing close brace", "http {\n  server {\n    listen 80;\n", "}", 1},
		{"stray close brace", "listen 80;\n}\n", "}", 2},
		{"open brace with no directive", "{ listen 80; }", "{", 1},
		{"semicolon with no directive", ";", ";", 1},
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		if !errors.Is(err, ErrSyntax) {
			t.Errorf("%s: error = %v, want ErrSyntax", c.name, err)
			continue
		}
		var se *SyntaxError
		if !errors.As(err, &se) {
			t.Errorf("%s: error is not a *SyntaxError", c.name)
			continue
		}
		if se.Line != c.line {
			t.Errorf("%s: line = %d, want %d (%v)", c.name, se.Line, c.line, err)
		}
		if !strings.Contains(err.Error(), c.mention) {
			t.Errorf("%s: %q does not mention %q", c.name, err, c.mention)
		}
	}
}

func TestEmptyAndCommentOnlyInput(t *testing.T) {
	for _, src := range []string{"", "\n\n", "# nothing\n"} {
		d, err := Parse(src)
		if err != nil {
			t.Errorf("Parse(%q): %v", src, err)
			continue
		}
		for _, f := range d.Files {
			if len(f.Directives) != 0 {
				t.Errorf("Parse(%q): got directives %+v", src, f.Directives)
			}
		}
	}
}

// A dump is what `nginx -T` prints: a "# configuration file PATH:" line before
// each file, in the order nginx read them, then nothing in between to say where
// an include was. Lines of `nginx:` chatter (the syntax check) may surround it.
const dump = `nginx: the configuration file /etc/nginx/nginx.conf syntax is ok
nginx: configuration file /etc/nginx/nginx.conf test is successful
# configuration file /etc/nginx/nginx.conf:
user www-data;
events {}
http {
    access_log /var/log/nginx/access.log;
    include /etc/nginx/mime.types;
    include conf.d/*.conf;
    include sites-enabled/*;
}

# configuration file /etc/nginx/mime.types:
types {
    text/html html;
}

# configuration file /etc/nginx/conf.d/20-format.conf:
log_format bots '$remote_addr $request_time';

# configuration file /etc/nginx/conf.d/10-real-ip.conf:
set_real_ip_from 173.245.48.0/20;
real_ip_header CF-Connecting-IP;

# configuration file /etc/nginx/sites-enabled/authors:
server {
    listen 443 ssl;
    include snippets/ssl.conf;
    location / { proxy_pass http://app; }
}

# configuration file /etc/nginx/snippets/ssl.conf:
ssl_protocols TLSv1.2 TLSv1.3;
`

func TestParseDumpSplitsFilesAndNumbersLinesPerFile(t *testing.T) {
	d, err := Parse(dump)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range d.Files {
		paths = append(paths, f.Path)
	}
	want := []string{
		"/etc/nginx/nginx.conf", "/etc/nginx/mime.types", "/etc/nginx/conf.d/20-format.conf",
		"/etc/nginx/conf.d/10-real-ip.conf", "/etc/nginx/sites-enabled/authors", "/etc/nginx/snippets/ssl.conf",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("files = %v, want %v", paths, want)
	}
	// The first line after a header is line 1 of that file.
	rip := d.Files[3].Directives[1]
	if rip.Name != "real_ip_header" || rip.Line != 2 || rip.File != "/etc/nginx/conf.d/10-real-ip.conf" {
		t.Errorf("real_ip_header = %+v", rip)
	}
	if h := d.Files[0].Directives[2]; h.Name != "http" || h.Line != 3 || h.File != "/etc/nginx/nginx.conf" {
		t.Errorf("http = %+v", h)
	}
}

func TestDumpWithOnlyChatterBeforeTheFirstHeader(t *testing.T) {
	d, err := Parse("nginx: [warn] something\n# configuration file /a.conf:\nuser x;\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Files) != 1 || d.Files[0].Path != "/a.conf" || d.Files[0].Directives[0].Line != 1 {
		t.Errorf("files = %+v", d.Files)
	}
}

func TestSyntaxErrorInADumpNamesTheFileItIsIn(t *testing.T) {
	_, err := Parse("# configuration file /a.conf:\nuser x;\n# configuration file /b.conf:\nlisten 80\n")
	var se *SyntaxError
	if !errors.As(err, &se) || se.File != "/b.conf" || se.Line != 1 {
		t.Errorf("error = %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "/b.conf:1") {
		t.Errorf("message %q lacks file:line", err)
	}
}

func names(ds []*Directive) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Name)
	}
	return out
}

func TestResolveExpandsIncludesInPlaceInIncludeOrder(t *testing.T) {
	d, err := Parse(dump)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := d.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if got := names(tree); !reflect.DeepEqual(got, []string{"user", "events", "http"}) {
		t.Fatalf("top level = %v", got)
	}
	http := tree[2]
	// access_log, then mime.types (a types block), then conf.d sorted
	// (10-real-ip before 20-format although the dump lists them the other way),
	// then the server from sites-enabled. The include directives themselves are
	// replaced by what they brought in.
	got := names(http.Block)
	want := []string{"access_log", "types", "set_real_ip_from", "real_ip_header", "log_format", "server"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("http children = %v, want %v", got, want)
	}
	// This ordering is the whole point: here log_format comes after access_log,
	// which is why a log_format cannot be used by an access_log above it.
	if http.Block[4].File != "/etc/nginx/conf.d/20-format.conf" || http.Block[4].Line != 1 {
		t.Errorf("log_format = %+v", http.Block[4])
	}
	for _, c := range http.Block {
		if c.Parent != http {
			t.Errorf("%s: Parent not rewritten to the including block", c.Name)
		}
	}
}

func TestResolveRecordsWhichIncludeBroughtADirectiveIn(t *testing.T) {
	d, _ := Parse(dump)
	tree, err := d.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	srv := tree[2].Block[5]
	if srv.Include == nil || srv.Include.Name != "include" || srv.Include.Line != 7 || srv.Include.File != "/etc/nginx/nginx.conf" {
		t.Errorf("server.Include = %+v", srv.Include)
	}
	// An include inside a server block, resolved relative to the prefix.
	ssl := srv.Block[1]
	if ssl.Name != "ssl_protocols" || ssl.File != "/etc/nginx/snippets/ssl.conf" || ssl.Parent != srv {
		t.Errorf("ssl_protocols = %+v", ssl)
	}
	if ssl.Include == nil || ssl.Include.File != "/etc/nginx/sites-enabled/authors" {
		t.Errorf("ssl_protocols.Include = %+v", ssl.Include)
	}
	// A directive written in the main file was not included by anything.
	if tree[0].Include != nil {
		t.Errorf("user.Include = %+v, want nil", tree[0].Include)
	}
}

func TestResolveIncludeEdgeCases(t *testing.T) {
	main := "# configuration file /etc/nginx/nginx.conf:\n"
	cases := []struct {
		name, text string
		wantErr    error
		wantNames  []string
	}{
		{"glob matching nothing is fine",
			main + "include conf.d/*.conf;\nuser x;\n", nil, []string{"user"}},
		{"plain include of a file missing from the dump",
			main + "include nothere.conf;\n", ErrInclude, nil},
		{"absolute path",
			main + "include /etc/other/a.conf;\n# configuration file /etc/other/a.conf:\nuser a;\n", nil, []string{"user"}},
		{"star does not cross a slash",
			main + "include *.d;\n# configuration file /etc/nginx/sub/a.d:\nuser a;\n", nil, nil},
		{"question mark",
			main + "include c?.conf;\n# configuration file /etc/nginx/c1.conf:\nuser a;\n# configuration file /etc/nginx/cc.conf:\nuser b;\n", nil, []string{"user", "user"}},
		{"include loop",
			main + "include a.conf;\n# configuration file /etc/nginx/a.conf:\ninclude a.conf;\n", ErrInclude, nil},
		{"mutual loop",
			main + "include a.conf;\n# configuration file /etc/nginx/a.conf:\ninclude b.conf;\n# configuration file /etc/nginx/b.conf:\ninclude a.conf;\n", ErrInclude, nil},
		{"same file included twice is allowed",
			main + "include a.conf;\ninclude a.conf;\n# configuration file /etc/nginx/a.conf:\nuser a;\n", nil, []string{"user", "user"}},
	}
	for _, c := range cases {
		d, err := Parse(c.text)
		if err != nil {
			t.Errorf("%s: Parse: %v", c.name, err)
			continue
		}
		tree, err := d.Resolve()
		if c.wantErr != nil {
			if !errors.Is(err, c.wantErr) {
				t.Errorf("%s: error = %v, want %v", c.name, err, c.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got := names(tree); !reflect.DeepEqual(got, c.wantNames) && !(len(got) == 0 && len(c.wantNames) == 0) {
			t.Errorf("%s: names = %v, want %v", c.name, got, c.wantNames)
		}
	}
}

func TestResolveOfABareConfigurationNeedsNoPrefix(t *testing.T) {
	d, err := Parse("user x;\nhttp { server { listen 80; } }\n")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := d.Resolve()
	if err != nil || len(tree) != 2 {
		t.Fatalf("tree = %v, err = %v", names(tree), err)
	}
}

func TestResolveDoesNotChangeTheParsedDump(t *testing.T) {
	d, _ := Parse(dump)
	before := len(d.Files[0].Directives[2].Block)
	if _, err := d.Resolve(); err != nil {
		t.Fatal(err)
	}
	if after := len(d.Files[0].Directives[2].Block); after != before {
		t.Errorf("http block has %d children after Resolve, had %d", after, before)
	}
	// And resolving twice gives the same answer.
	a, _ := d.Resolve()
	b, _ := d.Resolve()
	if !reflect.DeepEqual(names(a[2].Block), names(b[2].Block)) {
		t.Error("Resolve is not repeatable")
	}
}

func TestFindAndWalk(t *testing.T) {
	d, _ := Parse(dump)
	tree, _ := d.Resolve()
	if got := Find(tree, "http", "server", "listen"); len(got) != 1 || got[0].Args[0] != "443" {
		t.Errorf("Find listen = %+v", got)
	}
	if got := Find(tree, "http", "log_format"); len(got) != 1 {
		t.Errorf("Find log_format = %v", got)
	}
	if got := Find(tree, "http", "server", "nothing"); len(got) != 0 {
		t.Errorf("Find nothing = %v", got)
	}
	var order []string
	Walk(tree, func(d *Directive) { order = append(order, d.Name) })
	want := []string{"user", "events", "http", "access_log", "types", "text/html", "set_real_ip_from",
		"real_ip_header", "log_format", "server", "listen", "ssl_protocols", "location", "proxy_pass"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("Walk order = %v\nwant %v", order, want)
	}
}
