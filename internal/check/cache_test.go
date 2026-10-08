package check

import (
	"strings"
	"testing"
)

// cacheHost is a host whose one server has a location that uses proxy_cache,
// with the given directives inside that location and the given log_format text
// (so the cache status can be present or absent in the log).
func cacheHost(format, loc string) string {
	use := "combined"
	if format != "" {
		use = "full"
	}
	return "error_log /var/log/nginx/error.log warn;\nhttp {\n" + format + "\naccess_log " + logPath + " " + use + ";\n" +
		"proxy_cache_path /var/cache/nginx keys_zone=z:10m;\n" +
		"server { server_name a.example;\n  location /api/iiif/ {\n" + loc + "\n  }\n}\n}\n"
}

// withCacheStatus is the generated format with $upstream_cache_status added.
var withCacheStatus = strings.Replace(full, "$status", "$status $upstream_cache_status", 1)

func TestCookieInTheCacheBypassIsWarned(t *testing.T) {
	for _, loc := range []string{
		"proxy_cache z;\nproxy_cache_bypass $http_authorization $http_cookie;",
		"proxy_cache z;\nproxy_no_cache $cookie_session;",
	} {
		r := run(t, cfg("none", nil), cacheHost(withCacheStatus, loc))
		got := find(r, "cache-bypass-on-cookie")
		if len(got) != 1 || got[0].Severity != Warn {
			t.Fatalf("%q: findings = %+v", loc, r.Findings)
		}
		if !strings.Contains(got[0].Suggestion, "upstream_http_x_user_id") || !strings.Contains(got[0].Message, "/api/iiif/") {
			t.Errorf("%q: message %q suggestion %q", loc, got[0].Message, got[0].Suggestion)
		}
	}
}

func TestCacheThatDoesNotKeyOnCookiesIsNotWarned(t *testing.T) {
	loc := "proxy_cache z;\nproxy_cache_bypass $http_authorization $arg_token;\nproxy_no_cache $http_authorization $upstream_http_x_user_id;"
	r := run(t, cfg("none", nil), cacheHost(withCacheStatus, loc))
	if got := find(r, "cache-bypass-on-cookie"); len(got) != 0 {
		t.Fatalf("findings = %+v", got)
	}
}

func TestNoCacheNoCacheFindings(t *testing.T) {
	for _, loc := range []string{
		"proxy_pass http://127.0.0.1:5001;",
		"proxy_cache off;\nproxy_cache_bypass $http_cookie;",
	} {
		r := run(t, cfg("none", nil), cacheHost("", loc))
		for _, code := range []string{"cache-bypass-on-cookie", "cache-status-not-logged"} {
			if got := find(r, code); len(got) != 0 {
				t.Errorf("%q: %s: %+v", loc, code, got)
			}
		}
	}
}

func TestCacheStatusNotLoggedIsWarned(t *testing.T) {
	loc := "proxy_cache z;"
	r := run(t, cfg("none", nil), cacheHost(full, loc))
	got := find(r, "cache-status-not-logged")
	if len(got) != 1 || got[0].Severity != Warn {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if !strings.Contains(got[0].Suggestion, "$upstream_cache_status") || !strings.Contains(got[0].Message, "/api/iiif/") {
		t.Errorf("message %q suggestion %q", got[0].Message, got[0].Suggestion)
	}
	r = run(t, cfg("none", nil), cacheHost(withCacheStatus, loc))
	if got := find(r, "cache-status-not-logged"); len(got) != 0 {
		t.Errorf("status is logged, findings = %+v", got)
	}
}

func TestCacheFindingsNeverChangeTheExitStatus(t *testing.T) {
	r := run(t, cfg("none", nil), cacheHost(full, "proxy_cache z;\nproxy_cache_bypass $http_cookie;"))
	if len(find(r, "cache-bypass-on-cookie")) == 0 || len(find(r, "cache-status-not-logged")) == 0 {
		t.Fatalf("findings = %+v", r.Findings)
	}
	for _, f := range r.Findings {
		if (f.Code == "cache-bypass-on-cookie" || f.Code == "cache-status-not-logged") && f.Severity == Gap {
			t.Errorf("%s is a gap", f.Code)
		}
	}
}
