package classify

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// authors is CaltechAUTHORS's twelve classes, as the analysis scripts had them.
func authors() []Rule {
	return []Rule{
		{Name: "api-iiif", Prefix: "/api/iiif/"},
		{Name: "api-files", Pattern: `/api/records/[^/]+/(draft/)?files`},
		{Name: "api-versions", Pattern: `/api/records/[^/]+/versions`},
		{Name: "api-communities", Pattern: `/api/records/[^/]+/communities`},
		{Name: "api-record", Pattern: `/api/records/[^/]+$`},
		{Name: "api-search", Prefix: "/api/records"},
		{Name: "api-other", Prefix: "/api"},
		{Name: "ui-files", Pattern: `/records/[^/]+/files`},
		{Name: "ui-record", Pattern: `/records/[^/]+$`},
		{Name: "ui-search", Prefix: "/search"},
		{Name: "static", Pattern: `/(static|assets)/`},
	}
}

func mustClasses(t *testing.T, rules []Rule, fallback string) *Classes {
	t.Helper()
	c, err := NewClasses(rules, fallback)
	if err != nil {
		t.Fatalf("NewClasses: %v", err)
	}
	return c
}

func TestClassesReproduceTheScriptsTwelve(t *testing.T) {
	c := mustClasses(t, authors(), "other")
	for path, want := range map[string]string{
		"/api/iiif/record:abc:f.pdf/full/300,/0/default.png": "api-iiif",
		"/api/records/abc12-34/files":                        "api-files",
		"/api/records/abc12-34/draft/files/x/content":        "api-files",
		"/api/records/abc12-34/versions":                     "api-versions",
		"/api/records/abc12-34/communities":                  "api-communities",
		"/api/records/abc12-34":                              "api-record",
		"/api/records":                                       "api-search",
		"/api/user/records":                                  "api-other",
		"/records/abc12-34/files/The%20Tech.pdf":             "ui-files",
		"/records/abc12-34":                                  "ui-record",
		"/search":                                            "ui-search",
		"/static/js/pdfjs/build/pdf.min.mjs":                 "static",
		"/communities/caltechauthors/records":                "other",
		"/":                                                  "other",
	} {
		if got := c.Class(path); got != want {
			t.Errorf("Class(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestFirstMatchingRuleWins(t *testing.T) {
	c := mustClasses(t, []Rule{{Name: "first", Prefix: "/a"}, {Name: "second", Prefix: "/a/b"}}, "other")
	if got := c.Class("/a/b/c"); got != "first" {
		t.Errorf("got %q, want first", got)
	}
	c = mustClasses(t, []Rule{{Name: "second", Prefix: "/a/b"}, {Name: "first", Prefix: "/a"}}, "other")
	if got := c.Class("/a/b/c"); got != "second" {
		t.Errorf("got %q, want second", got)
	}
}

func TestThePatternIsAnchoredAtTheStartOfThePath(t *testing.T) {
	c := mustClasses(t, []Rule{{Name: "rec", Pattern: `/records/[^/]+$`}}, "other")
	if got := c.Class("/records/abc"); got != "rec" {
		t.Errorf("got %q, want rec", got)
	}
	if got := c.Class("/x/records/abc"); got != "other" {
		t.Errorf("a match in the middle of a path counted: %q", got)
	}
	if got := c.Class("/records/abc/files"); got != "other" {
		t.Errorf("the $ was ignored: %q", got)
	}
}

func TestTheFallbackIsUsedWhenNothingMatches(t *testing.T) {
	if got := mustClasses(t, nil, "").Class("/anything"); got != "other" {
		t.Errorf("empty fallback gave %q, want other", got)
	}
	if got := mustClasses(t, authors(), "misc").Class("/nope"); got != "misc" {
		t.Errorf("got %q, want misc", got)
	}
}

// The query string is what a patron searched for; it must never reach a rule.
func TestAQueryStringNeverReachesARule(t *testing.T) {
	c := mustClasses(t, []Rule{
		{Name: "leaked", Pattern: `.*private`},
		{Name: "search", Prefix: "/search"},
	}, "other")
	for _, p := range []string{"/search?q=private", "/search#private", "/search?q=a?b=private"} {
		if got := c.Class(p); got != "search" {
			t.Errorf("Class(%q) = %q, want search", p, got)
		}
	}
}

func TestBadRulesAreRejected(t *testing.T) {
	for name, rules := range map[string][]Rule{
		"no name":               {{Prefix: "/a"}},
		"neither prefix nor re": {{Name: "x"}},
		"both prefix and re":    {{Name: "x", Prefix: "/a", Pattern: "/b"}},
		"bad pattern":           {{Name: "x", Pattern: "("}},
		"empty prefix":          {{Name: "x", Prefix: ""}, {Name: "y", Pattern: ""}},
	} {
		if _, err := NewClasses(rules, ""); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	// The same class name on several rules is normal: api-files has two shapes.
	if _, err := NewClasses([]Rule{{Name: "x", Prefix: "/a"}, {Name: "x", Prefix: "/b"}}, ""); err != nil {
		t.Errorf("a repeated class name was rejected: %v", err)
	}
}

func TestEmbeddedFamiliesNameTheScriptsFamiliesAndAkashicAI(t *testing.T) {
	f := DefaultFamilies()
	for ua, want := range map[string]string{
		"Mozilla/5.0 (compatible; ExaSearchBot/1.0; +https://crawler.exa.ai/)":                           "Exa",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; ChatGPT-User/1.0":               "OpenAI ChatGPT-User",
		"Mozilla/5.0 (Macintosh) Chrome/131 Safari; compatible; OAI-SearchBot/1.4":                       "OpenAI OAI-SearchBot",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; Claude-User/1.0)":                "Anthropic Claude-User",
		"Mozilla/5.0 (Linux; Android 6.0.1) Chrome/153 Mobile Safari/537.36 (compatible; Googlebot/2.1)": "Google Googlebot-claim",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; bingbot/2.0)":                    "Microsoft bingbot",
		"Mozilla/5.0 (Macintosh) Version/17.4 Safari/605.1.15 (Applebot/0.1)":                            "Apple Applebot",
		"Mozilla/5.0 (Windows NT 10.0) Chrome/145 Safari/537.36 (compatible; meta-webindexer/1.1)":       "Meta webindexer",
		"meta-externalads/1.1 (+https://developers.facebook.com/docs/sharing/webmasters/crawler)":        "Meta externalads",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; PerplexityBot/1.0)":              "PerplexityBot",
		"Mozilla/5.0 (compatible; SemrushBot/7~bl; +http://www.semrush.com/bot.html)":                    "SemrushBot",
		"Mozilla/5.0 (compatible) SemanticScholarBot (+https://www.semanticscholar.org/crawler)":         "SemanticScholarBot",
		"Mozilla/5.0 (compatible; DotBot/1.2; +https://opensiteexplorer.org/dotbot)":                     "DotBot",
		"Mozilla/5.0 (compatible; Baiduspider/2.0; +http://www.baidu.com/search/spider.html)":            "Baiduspider",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; KeenableBot/1.0)":                "KeenableBot",
		"AkashicAI/2.0 (mailto:admin@akashicsymbiosis.ai)":                                               "AkashicAI",
		"python-requests/2.34.2": "python-requests",
	} {
		got, declared := f.Lookup(ua)
		if got != want || !declared {
			t.Errorf("Lookup(%q) = %q, %v; want %q, true", ua, got, declared, want)
		}
	}
}

func TestMatchingIsCaseFoldedAndTheFirstEntryWins(t *testing.T) {
	f := DefaultFamilies()
	if got, _ := f.Lookup("EXASEARCHBOT/1.0"); got != "Exa" {
		t.Errorf("upper case: %q", got)
	}
	// A Chrome-looking string with a bot name appended is the bot, not the platform.
	if got, _ := f.Lookup("Mozilla/5.0 (Windows NT 10.0) Chrome/145 (compatible; meta-webindexer/1.1)"); got != "Meta webindexer" {
		t.Errorf("a declared family must be chosen before a platform: %q", got)
	}
}

func TestAnyOtherBotLikeAgentIsOtherDeclared(t *testing.T) {
	f := DefaultFamilies()
	for _, ua := range []string{"Sogou web spider/4.0", "SentryUptimeBot/1.0", "Mozilla/5.0 (compatible; YandexBot/3.0)", "Mozilla/5.0 (X11; Linux x86_64) Chrome/126 (compatible; HaloBot/1.0)", "Slurp"} {
		if got, declared := f.Lookup(ua); got != "other-declared" || !declared {
			t.Errorf("Lookup(%q) = %q, %v; want other-declared, true", ua, got, declared)
		}
	}
}

func TestUndeclaredTrafficFallsIntoPlatformBuckets(t *testing.T) {
	f := DefaultFamilies()
	for ua, want := range map[string]string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/148 Safari/537.36":    "undeclared Windows",
		"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 Chrome/148 Mobile Safari/537.36":    "undeclared mobile",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_3_1 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E": "undeclared mobile",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/153 Safari":     "undeclared Mac",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/126 Safari/537.36":              "undeclared Linux",
		"curl/8.0": "undeclared other",
		"":         "undeclared other",
	} {
		if got, declared := f.Lookup(ua); got != want || declared {
			t.Errorf("Lookup(%q) = %q, %v; want %q, false", ua, got, declared, want)
		}
	}
	// Mobile is looked for before Linux: an Android agent also says Linux.
	if got, _ := f.Lookup("Mozilla/5.0 (Linux; Android 6.0.1)"); got != "undeclared mobile" {
		t.Errorf("Android was filed as %q", got)
	}
}

func TestAHostCanAddAndOverrideFamilies(t *testing.T) {
	f, err := DefaultFamilies().With([]Entry{
		{Name: "citation-weekend-agent", Match: []string{"citation-weekend-agent"}, Declared: true},
		{Name: "Exa", Match: []string{"crawler.exa.ai"}, Declared: true},
		{Name: "partner", Match: []string{"partnerbot"}, Declared: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, declared := f.Lookup("citation-weekend-agent/2.0"); got != "citation-weekend-agent" || !declared {
		t.Errorf("host-added: %q, %v", got, declared)
	}
	// The host's Exa entry replaces the embedded one, so the old match no longer finds it.
	if got, _ := f.Lookup("Mozilla/5.0 (compatible; ExaSearchBot/1.0; +https://crawler.exa.ai/)"); got != "Exa" {
		t.Errorf("override by new text: %q", got)
	}
	if got, _ := f.Lookup("ExaSearchBot/1.0"); got == "Exa" {
		t.Errorf("the embedded match survived an override: %q", got)
	}
	// An entry marked not declared is a named family but not an automated agent.
	if got, declared := f.Lookup("PartnerBot/1"); got != "partner" || declared {
		t.Errorf("not-declared entry: %q, %v", got, declared)
	}
	// The embedded set is untouched by With.
	if got, _ := DefaultFamilies().Lookup("ExaSearchBot/1.0"); got != "Exa" {
		t.Errorf("With changed the shared default: %q", got)
	}
}

func TestBadHostFamiliesAreRejected(t *testing.T) {
	for name, entries := range map[string][]Entry{
		"no name":     {{Match: []string{"x"}}},
		"no match":    {{Name: "x"}},
		"empty match": {{Name: "x", Match: []string{""}}},
		"duplicate":   {{Name: "x", Match: []string{"a"}}, {Name: "x", Match: []string{"b"}}},
	} {
		if _, err := DefaultFamilies().With(entries); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestTheFamilyDataIsDatedAndHasNoDuplicateNames(t *testing.T) {
	f := DefaultFamilies()
	r := f.Retrieved()
	if r.IsZero() || r.After(time.Now().AddDate(0, 0, 1)) {
		t.Errorf("Retrieved = %v", r)
	}
	if f.AgeDays(r.AddDate(0, 0, 30)) != 30 {
		t.Errorf("AgeDays = %d, want 30", f.AgeDays(r.AddDate(0, 0, 30)))
	}
	seen := map[string]bool{}
	for _, name := range f.Names() {
		if seen[name] {
			t.Errorf("family %q appears twice", name)
		}
		seen[name] = true
	}
}

// The package that applies the rules stays free of the interface and
// configuration libraries.
func TestClassifyImportsOnlyTheStandardLibrary(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	for dep := range strings.FieldsSeq(string(out)) {
		first, _, _ := strings.Cut(dep, "/")
		if strings.Contains(first, ".") && !strings.HasPrefix(dep, "github.com/caltechlibrary/logagent/internal/classify") {
			t.Errorf("internal/classify depends on %s", dep)
		}
	}
}
