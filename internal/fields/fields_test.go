package fields

import (
	"regexp"
	"strings"
	"testing"
)

// minimalTable is a valid table to mutate in the rejection tests.
const minimalTable = `{"version":1,"fields":[
 {"name":"ua","description":"d","needed_by":["tier1"],"default":"required","applies":"always","combined":true,
  "nginx":{"expr":"$http_user_agent"},"apache":{"expr":"%{User-Agent}i"},"if_missing":"x"}]}`

func TestDefaultTableIsWellFormed(t *testing.T) {
	tab := Default()
	if len(tab.Fields) == 0 {
		t.Fatal("empty default table")
	}
	name := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	seen := map[string]bool{}
	for _, f := range tab.Fields {
		if !name.MatchString(f.Name) {
			t.Errorf("field name %q is not lower_snake_case", f.Name)
		}
		if seen[f.Name] {
			t.Errorf("duplicate field %q", f.Name)
		}
		seen[f.Name] = true
		if f.Description == "" || f.IfMissing == "" || len(f.NeededBy) == 0 {
			t.Errorf("%s: description, needed_by and if_missing are all required", f.Name)
		}
		if !strings.HasPrefix(f.Nginx.Expr, "$") {
			t.Errorf("%s: nginx expr %q is not a variable", f.Name, f.Nginx.Expr)
		}
		if f.Apache.Expr == "" && !(f.Apache.Unavailable && f.Apache.Note != "") {
			t.Errorf("%s: apache needs an expr, or unavailable with a note", f.Name)
		}
	}
	// The fields named in the example configuration and in DR-0002 must exist.
	for _, want := range []string{"client", "peer", "rt", "urt", "ua", "cf_ray", "cf_country", "bot_score", "ja3", "ja4", "lang", "ch_ua", "ch_plat"} {
		if !tab.Has(want) {
			t.Errorf("default table lacks field %q", want)
		}
	}
}

func TestParseRejectsBadTables(t *testing.T) {
	cases := []struct{ name, from, to, mention string }{
		{"wrong version", `"version":1`, `"version":2`, "version"},
		{"unknown key", `"combined":true,`, `"combined":true,"bogus":1,`, "bogus"},
		{"bad default", `"default":"required"`, `"default":"mandatory"`, "default"},
		{"bad applies", `"applies":"always"`, `"applies":"sometimes"`, "applies"},
		{"empty nginx expr", `"expr":"$http_user_agent"`, `"expr":""`, "nginx"},
		{"nginx expr not a variable", `"expr":"$http_user_agent"`, `"expr":"http_user_agent"`, "nginx"},
		{"apache with neither", `"apache":{"expr":"%{User-Agent}i"}`, `"apache":{}`, "apache"},
		{"no needed_by", `"needed_by":["tier1"]`, `"needed_by":[]`, "needed_by"},
		{"bad name", `"name":"ua"`, `"name":"User Agent"`, "name"},
	}
	for _, c := range cases {
		if !strings.Contains(minimalTable, c.from) {
			t.Fatalf("%s: test table lacks %q", c.name, c.from)
		}
		_, err := Parse([]byte(strings.Replace(minimalTable, c.from, c.to, 1)))
		if err == nil || !strings.Contains(err.Error(), c.mention) {
			t.Errorf("%s: error = %v, want one mentioning %q", c.name, err, c.mention)
		}
	}
	dup := strings.Replace(minimalTable, `}]}`, `},`+minimalTable[strings.Index(minimalTable, `{"name"`):], 1)
	if _, err := Parse([]byte(dup)); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("duplicate names: error = %v", err)
	}
	if _, err := Parse([]byte(minimalTable)); err != nil {
		t.Errorf("the minimal table must parse: %v", err)
	}
}

func names(rs []Requirement) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Field.Name)
	}
	return out
}

func find(rs []Requirement, name string) (Requirement, bool) {
	for _, r := range rs {
		if r.Field.Name == name {
			return r, true
		}
	}
	return Requirement{}, false
}

func TestEffectiveDependsOnTheProxy(t *testing.T) {
	tab := Default()
	none, err := tab.Effective("nginx", "none", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"peer", "cf_ray", "cf_country", "bot_score", "ja3", "ja4"} {
		if _, ok := find(none, n); ok {
			t.Errorf("%s must not apply to a host with no proxy", n)
		}
	}
	for _, n := range []string{"client", "rt", "urt", "ua", "lang", "ch_ua", "ch_plat"} {
		if _, ok := find(none, n); !ok {
			t.Errorf("%s must apply to every host", n)
		}
	}
	cf, _ := tab.Effective("nginx", "cloudflare", nil)
	for _, n := range []string{"peer", "cf_ray", "cf_country"} {
		if r, ok := find(cf, n); !ok || r.Level != "required" {
			t.Errorf("%s must be required behind cloudflare: %+v", n, r)
		}
	}
	if len(cf) <= len(none) {
		t.Errorf("cloudflare (%v) must add fields to none (%v)", names(cf), names(none))
	}
}

func TestEffectiveOverridesAndApache(t *testing.T) {
	tab := Default()
	rs, err := tab.Effective("nginx", "cloudflare", map[string]string{"cf_ray": "optional", "ja3": "not_applicable", "lang": "required"})
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := find(rs, "cf_ray"); r.Level != "optional" {
		t.Errorf("cf_ray override ignored: %+v", r)
	}
	if _, ok := find(rs, "ja3"); ok {
		t.Error("a not_applicable field must be dropped")
	}
	if _, err := tab.Effective("nginx", "none", map[string]string{"no_such_field": "optional"}); err == nil || !strings.Contains(err.Error(), "no_such_field") {
		t.Errorf("unknown override: error = %v", err)
	}
	ap, _ := tab.Effective("apache", "none", nil)
	if r, ok := find(ap, "urt"); !ok || !r.Unavailable {
		t.Errorf("urt must be reported unavailable on apache: %+v", r)
	}
	if r, _ := find(rs, "urt"); r.Unavailable {
		t.Error("urt is available on nginx")
	}
	if _, err := tab.Effective("caddy", "none", nil); err == nil {
		t.Error("unknown server accepted")
	}
	if _, err := tab.Effective("nginx", "fastly", nil); err == nil {
		t.Error("unknown proxy accepted")
	}
}

// norm makes two log_format texts comparable regardless of line breaks and
// the quoting of continued lines.
func norm(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "'", "")), " ")
}

// deployed is the log_format running on the CaltechAUTHORS production server
// since 2026-10-06 (caltechauthors/nginx-log-format-bot-fingerprint.conf). The
// table must regenerate it, so the recommended configuration and the check
// cannot disagree.
const deployed = `log_format caltechauthors_bots
    '$remote_addr - $remote_user [$time_local] "$request" '
    '$status $body_bytes_sent "$http_referer" "$http_user_agent" '
    'rt=$request_time urt="$upstream_response_time" '
    'cf_ray="$http_cf_ray" cf_country="$http_cf_ipcountry" '
    'peer=$realip_remote_addr '
    'lang="$http_accept_language" ch_ua="$http_sec_ch_ua" '
    'ch_plat="$http_sec_ch_ua_platform" '
    'bot_score="$http_cf_bot_score" ja3="$http_cf_ja3_hash" ja4="$http_cf_ja4"';`

func TestNginxLogFormatRegeneratesTheDeployedFormat(t *testing.T) {
	got := Default().NginxLogFormat("caltechauthors_bots")
	if norm(got) != norm(deployed) {
		t.Errorf("generated format differs from the deployed one:\n got: %s\nwant: %s", norm(got), norm(deployed))
	}
	if !strings.HasSuffix(strings.TrimSpace(got), ";") {
		t.Error("log_format must end with a semicolon")
	}
}

func TestTimeAndRequestHaveAlternativeSpellings(t *testing.T) {
	tab := Default()
	for name, want := range map[string]string{"time": "$time_iso8601", "request": "$request_method $uri"} {
		f, _ := tab.Get(name)
		found := false
		for _, a := range f.Nginx.Also {
			found = found || a == want
		}
		if !found {
			t.Errorf("%s: Also = %v, want it to include %q", name, f.Nginx.Also, want)
		}
		if f.Nginx.JSON == "" {
			t.Errorf("%s: no JSON pairs to suggest", name)
		}
	}
}

func TestAnAlternativeSpellingMustBeVariables(t *testing.T) {
	bad := strings.Replace(minimalTable, `"nginx":{"expr":"$http_user_agent"}`, `"nginx":{"expr":"$http_user_agent","also":["user agent"]}`, 1)
	if _, err := Parse([]byte(bad)); err == nil || !strings.Contains(err.Error(), "also") {
		t.Errorf("Parse err = %v, want a complaint about also", err)
	}
	ok := strings.Replace(minimalTable, `"nginx":{"expr":"$http_user_agent"}`, `"nginx":{"expr":"$http_user_agent","also":["$a $b"]}`, 1)
	if _, err := Parse([]byte(ok)); err != nil {
		t.Errorf("Parse: %v", err)
	}
}
