package nginxconf

import (
	"reflect"
	"testing"
)

// resolved parses and resolves a bare configuration.
func resolved(t *testing.T, text string) []*Directive {
	t.Helper()
	d, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tree, err := d.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return tree
}

// args flattens the arguments of a list of directives for comparison.
func args(ds []*Directive) [][]string {
	var out [][]string
	for _, d := range ds {
		out = append(out, d.Args)
	}
	return out
}

// The 2026-10-06 incident in miniature: proxy_buffering off was written once at
// server level, where it looked harmless, and the IIIF location's proxy cache
// silently never worked because the location inherited it.
const incident = `http {
    server {
        proxy_buffering off;
        proxy_request_buffering off;
        add_header X-Request-ID $request_id;
        add_header Strict-Transport-Security "max-age=15768000";
        access_log /var/log/nginx/access.log fmt;
        location /api/iiif/ {
            proxy_cache iiif;
            proxy_pass http://app;
        }
        location /static/ {
            proxy_buffering on;
            add_header Cache-Control "public";
            access_log off;
        }
    }
}
`

func locationNamed(t *testing.T, tree []*Directive, arg string) *Directive {
	t.Helper()
	for _, l := range Locations(tree) {
		if len(l.Args) > 0 && l.Args[len(l.Args)-1] == arg {
			return l
		}
	}
	t.Fatalf("no location %s", arg)
	return nil
}

func TestASimpleDirectiveIsInheritedFromTheServer(t *testing.T) {
	tree := resolved(t, incident)
	iiif := locationNamed(t, tree, "/api/iiif/")
	got := Effective(iiif, "proxy_buffering")
	if len(got) != 1 || got[0].Args[0] != "off" {
		t.Fatalf("proxy_buffering = %v, want the server's off", args(got))
	}
	// The caller can tell where the value came from: not the location.
	if got[0].Parent.Name != "server" || got[0].Line != 3 {
		t.Errorf("came from %s line %d, want the server at line 3", got[0].Parent.Name, got[0].Line)
	}
	// And the directive written in the location itself is its own.
	own := Effective(iiif, "proxy_cache")
	if len(own) != 1 || own[0].Parent != iiif {
		t.Errorf("proxy_cache = %+v", own)
	}
}

func TestTheInnermostDefinitionWins(t *testing.T) {
	tree := resolved(t, incident)
	st := locationNamed(t, tree, "/static/")
	got := Effective(st, "proxy_buffering")
	if len(got) != 1 || got[0].Args[0] != "on" || got[0].Parent != st {
		t.Errorf("proxy_buffering = %v, want the location's on", args(got))
	}
	// Another directive the location did not touch still comes from the server.
	if got := Effective(st, "proxy_request_buffering"); len(got) != 1 || got[0].Args[0] != "off" {
		t.Errorf("proxy_request_buffering = %v", args(got))
	}
}

// add_header, proxy_set_header, access_log and their kind are inherited only if
// the inner level defines none of them; one in the location replaces them all.
func TestArrayDirectivesAreReplacedNotMerged(t *testing.T) {
	tree := resolved(t, incident)
	iiif := locationNamed(t, tree, "/api/iiif/")
	st := locationNamed(t, tree, "/static/")

	// No add_header in the location: both of the server's apply.
	got := Effective(iiif, "add_header")
	want := [][]string{{"X-Request-ID", "$request_id"}, {"Strict-Transport-Security", "max-age=15768000"}}
	if !reflect.DeepEqual(args(got), want) {
		t.Errorf("iiif add_header = %v, want %v", args(got), want)
	}
	// One add_header in the location: the security headers are gone.
	got = Effective(st, "add_header")
	if !reflect.DeepEqual(args(got), [][]string{{"Cache-Control", "public"}}) {
		t.Errorf("static add_header = %v, want only the location's", args(got))
	}
	// access_log off in the location replaces the server's access_log.
	if got := Effective(st, "access_log"); !reflect.DeepEqual(args(got), [][]string{{"off"}}) {
		t.Errorf("static access_log = %v", args(got))
	}
	if got := Effective(iiif, "access_log"); !reflect.DeepEqual(args(got), [][]string{{"/var/log/nginx/access.log", "fmt"}}) {
		t.Errorf("iiif access_log = %v", args(got))
	}
}

func TestWhichDirectivesAreArrays(t *testing.T) {
	for _, n := range []string{"add_header", "add_trailer", "proxy_set_header", "proxy_hide_header", "proxy_pass_header",
		"fastcgi_param", "uwsgi_param", "access_log", "error_log", "limit_req", "limit_conn", "allow", "deny",
		"set_real_ip_from", "error_page", "proxy_cache_valid", "proxy_cache_bypass", "proxy_no_cache", "proxy_redirect"} {
		if !IsArray(n) {
			t.Errorf("%s must be an array directive", n)
		}
	}
	for _, n := range []string{"proxy_buffering", "proxy_cache", "proxy_pass", "client_max_body_size", "real_ip_header",
		"proxy_read_timeout", "limit_conn_status", "gzip", "server_name"} {
		if IsArray(n) {
			t.Errorf("%s must not be an array directive", n)
		}
	}
}

func TestNestedLocationsInheritFromTheirParentLocationFirst(t *testing.T) {
	tree := resolved(t, `http {
    proxy_read_timeout 60s;
    server {
        proxy_read_timeout 30s;
        proxy_set_header Host $host;
        location /a/ {
            proxy_read_timeout 120s;
            location /a/b/ {
                proxy_set_header X-Inner 1;
            }
            location /a/c/ {
            }
        }
    }
}
`)
	b := locationNamed(t, tree, "/a/b/")
	c := locationNamed(t, tree, "/a/c/")
	if got := Effective(b, "proxy_read_timeout"); len(got) != 1 || got[0].Args[0] != "120s" {
		t.Errorf("b proxy_read_timeout = %v, want the outer location's 120s", args(got))
	}
	if got := Effective(b, "proxy_set_header"); !reflect.DeepEqual(args(got), [][]string{{"X-Inner", "1"}}) {
		t.Errorf("b proxy_set_header = %v, want only its own", args(got))
	}
	if got := Effective(c, "proxy_set_header"); !reflect.DeepEqual(args(got), [][]string{{"Host", "$host"}}) {
		t.Errorf("c proxy_set_header = %v, want the server's", args(got))
	}
}

func TestInheritanceReachesTheHttpBlock(t *testing.T) {
	tree := resolved(t, `http {
    gzip on;
    access_log /var/log/nginx/access.log;
    server {
        location / { }
    }
}
`)
	loc := Locations(tree)[0]
	if got := Effective(loc, "gzip"); len(got) != 1 || got[0].Parent.Name != "http" {
		t.Errorf("gzip = %+v", got)
	}
	if got := Effective(loc, "access_log"); len(got) != 1 || got[0].Parent.Name != "http" {
		t.Errorf("access_log = %+v", got)
	}
}

func TestAbsentDirectiveGivesNothing(t *testing.T) {
	tree := resolved(t, incident)
	iiif := locationNamed(t, tree, "/api/iiif/")
	for _, n := range []string{"proxy_cache_lock", "limit_conn", "no_such_directive"} {
		if got := Effective(iiif, n); len(got) != 0 {
			t.Errorf("%s = %v, want none", n, args(got))
		}
	}
}

// A directive inside an if block applies only when the condition holds, so it
// is not the location's own setting and does not replace or hide one.
func TestDirectivesInsideIfAreNotTheEnclosingContextsOwn(t *testing.T) {
	tree := resolved(t, `http { server {
    proxy_buffering off;
    location / {
        if ($arg_nocache) {
            proxy_buffering on;
        }
    }
} }
`)
	loc := Locations(tree)[0]
	got := Effective(loc, "proxy_buffering")
	if len(got) != 1 || got[0].Args[0] != "off" || got[0].Parent.Name != "server" {
		t.Errorf("proxy_buffering = %+v, want the server's off", got)
	}
}

func TestTheLastOfARepeatedSimpleDirectiveWins(t *testing.T) {
	tree := resolved(t, "http { server { gzip on; gzip off; location / { } } }\n")
	if got := Effective(Locations(tree)[0], "gzip"); len(got) != 1 || got[0].Args[0] != "off" {
		t.Errorf("gzip = %v", args(got))
	}
}

func TestEffectiveSeesDirectivesBroughtInByIncludes(t *testing.T) {
	d, err := Parse(`# configuration file /etc/nginx/nginx.conf:
http {
    server {
        include snippets/buffering.conf;
        location / { }
    }
}
# configuration file /etc/nginx/snippets/buffering.conf:
proxy_buffering off;
`)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := d.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	got := Effective(Locations(tree)[0], "proxy_buffering")
	if len(got) != 1 || got[0].File != "/etc/nginx/snippets/buffering.conf" || got[0].Include == nil {
		t.Fatalf("proxy_buffering = %+v", got)
	}
}

// Asking about a plain directive gives the view from where it sits.
func TestEffectiveOfANonBlockDirectiveIsItsEnclosingView(t *testing.T) {
	tree := resolved(t, incident)
	iiif := locationNamed(t, tree, "/api/iiif/")
	proxyPass := iiif.Block[1]
	if proxyPass.Name != "proxy_pass" {
		t.Fatalf("fixture changed: %s", proxyPass.Name)
	}
	if got := Effective(proxyPass, "proxy_buffering"); len(got) != 1 || got[0].Args[0] != "off" {
		t.Errorf("proxy_buffering = %v", args(got))
	}
}

func TestLocationsListsEveryLocationInDocumentOrder(t *testing.T) {
	tree := resolved(t, `http {
    server {
        location /one/ { location /one/two/ { } }
        location = /three { }
    }
    server { location @named { } }
}
location /not-in-http/ { }
`)
	var got []string
	for _, l := range Locations(tree) {
		got = append(got, l.Args[len(l.Args)-1])
	}
	want := []string{"/one/", "/one/two/", "/three", "@named"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Locations = %v, want %v", got, want)
	}
}

func TestEffectiveDoesNotModifyTheTree(t *testing.T) {
	tree := resolved(t, incident)
	iiif := locationNamed(t, tree, "/api/iiif/")
	before := len(iiif.Block)
	got := Effective(iiif, "add_header")
	got[0] = nil // the caller owns the slice it was given
	if len(iiif.Block) != before || Effective(iiif, "add_header")[0] == nil {
		t.Error("Effective returned a slice that aliases the tree")
	}
}
