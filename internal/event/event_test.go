package event

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"net/netip"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// line is one request as the 19-field format records it, in the shape the
// sample package hands over: variable name without the dollar sign to value.
// It holds a query string, a referer and a full client address on purpose, so
// the privacy tests have something to prove absent.
func line() map[string]string {
	return map[string]string{
		"remote_addr":             "203.0.113.77",
		"time_local":              "08/Oct/2026:22:27:13 +0000",
		"request":                 "GET /search?q=private+topic HTTP/1.1",
		"status":                  "200",
		"body_bytes_sent":         "100",
		"http_referer":            "https://example.org/secret-referer",
		"http_user_agent":         "Mozilla/5.0 (Windows NT 10.0) Chrome/148",
		"request_time":            "0.095",
		"upstream_response_time":  "0.096",
		"http_cf_ray":             "9ab12c34d56e",
		"http_cf_ipcountry":       "US",
		"realip_remote_addr":      "198.41.128.1",
		"http_accept_language":    "en-US,en;q=0.9",
		"http_sec_ch_ua":          `"Chromium";v="148"`,
		"http_sec_ch_ua_platform": `"Windows"`,
		"http_cf_bot_score":       "-",
		"http_cf_ja3_hash":        "-",
		"http_cf_ja4":             "-",
		"upstream_cache_status":   "HIT",
	}
}

var testKey = []byte("daily-key-one")

// opts is a minimal set of options: lookups that return fixed answers.
func opts() Options {
	return Options{
		Key:    testKey,
		Class:  func(string) string { return "ui-lookup" },
		Family: func(string) (string, bool) { return "undeclared Windows", false },
	}
}

func mustBuild(t *testing.T, v map[string]string, o Options) Event {
	t.Helper()
	ev, out, err := Build(v, o)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !out.Kept {
		t.Fatalf("Build dropped the line: %+v", out)
	}
	return ev
}

func TestBuildFillsEveryField(t *testing.T) {
	ev := mustBuild(t, line(), opts())
	want := Event{
		V: 1, T: time.Date(2026, 10, 8, 22, 27, 13, 0, time.UTC), Method: "GET", Class: "ui-lookup",
		Status: 200, RT: 0.095, URT: 0.096, Timed: true, Cache: "HIT",
		UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Family: "undeclared Windows", Declared: "none",
		Country: "US", Plat: "Windows", CH: true, Lang: "en-us", Via: true,
		Net: "203.0.113.0/24",
	}
	got := ev
	got.Who = "" // checked on its own
	if got != want {
		t.Errorf("event =\n%+v\nwant\n%+v", got, want)
	}
	if len(ev.Who) != 16 {
		t.Errorf("Who = %q, want 16 hex characters", ev.Who)
	}
}

func TestTimeIsUTC(t *testing.T) {
	v := line()
	v["time_local"] = "09/Oct/2026:07:27:13 +0900"
	ev := mustBuild(t, v, opts())
	if want := time.Date(2026, 10, 8, 22, 27, 13, 0, time.UTC); !ev.T.Equal(want) || ev.T.Location() != time.UTC {
		t.Errorf("T = %v, want %v in UTC", ev.T, want)
	}
}

func TestUpstreamSecondsSumOverRetries(t *testing.T) {
	for in, want := range map[string]float64{"0.100": 0.1, "0.1, 0.2 : 0.3": 0.6, "-": 0, "": 0, "0.5, -": 0.5} {
		v := line()
		v["upstream_response_time"] = in
		if got := mustBuild(t, v, opts()).URT; math.Abs(got-want) > 1e-9 {
			t.Errorf("urt %q = %v, want %v", in, got, want)
		}
	}
}

// A request that took no measurable time and a line that has no timing at all
// are different things: the report divides upstream seconds by the requests
// that were timed, and a host that changed its log format mid-window has both.
func TestTimedSaysWhetherTheLogRecordedARequestTime(t *testing.T) {
	for in, want := range map[string]bool{"0.000": true, "0.095": true, "": false, "-": false, "junk": false} {
		v := line()
		v["request_time"] = in
		if got := mustBuild(t, v, opts()).Timed; got != want {
			t.Errorf("request_time %q: Timed = %v, want %v", in, got, want)
		}
	}
	v := line()
	delete(v, "request_time")
	if mustBuild(t, v, opts()).Timed {
		t.Error("Timed is true for a line with no request_time variable")
	}
}

func TestStockCombinedLineHasNoExtendedFields(t *testing.T) {
	v := map[string]string{
		"remote_addr": "203.0.113.77", "time_local": "08/Oct/2026:22:27:13 +0000",
		"request": "GET /x HTTP/1.1", "status": "404", "body_bytes_sent": "10",
		"http_referer": "-", "http_user_agent": "curl/8.0",
	}
	ev := mustBuild(t, v, opts())
	if ev.Status != 404 || ev.RT != 0 || ev.URT != 0 || ev.Cache != "" || ev.Country != "" || ev.Plat != "" ||
		ev.CH || ev.Via || ev.Lang != "" || ev.JA3 != "" || ev.JA4 != "" || ev.Bot != "" {
		t.Errorf("event = %+v", ev)
	}
}

func TestMethodIsAClosedSet(t *testing.T) {
	for _, m := range []string{"GET", "HEAD", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"} {
		v := line()
		v["request"] = m + " /x HTTP/1.1"
		if got := mustBuild(t, v, opts()).Method; got != m {
			t.Errorf("method %q became %q", m, got)
		}
	}
	for _, r := range []string{"FROBNICATE /x HTTP/1.1", "-", "\\x16\\x03\\x01"} {
		v := line()
		v["request"] = r
		if got := mustBuild(t, v, opts()).Method; got != "other" {
			t.Errorf("request %q: method %q, want other", r, got)
		}
	}
}

func TestClassIsAskedAboutThePathWithoutItsQuery(t *testing.T) {
	var asked []string
	o := opts()
	o.Class = func(p string) string { asked = append(asked, p); return "c" }
	mustBuild(t, line(), o)
	if len(asked) != 1 || asked[0] != "/search" {
		t.Errorf("Class was asked %q, want [/search]", asked)
	}
}

// What a patron searched for, who they are and where they came from must
// not survive into an event (DR-0002, decision 3).
func TestAnEventNeverHoldsTheQueryTheRefererThePathOrTheAddress(t *testing.T) {
	ev := mustBuild(t, line(), opts())
	b, err := Encode(ev)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, s := range []string{"private", "topic", "secret-referer", "example.org", "203.0.113.77", "/search", "?q="} {
		if strings.Contains(text, s) {
			t.Errorf("event %s contains %q", text, s)
		}
	}
	if ev.Net != "203.0.113.0/24" {
		t.Errorf("Net = %q, want 203.0.113.0/24", ev.Net)
	}
	v6 := line()
	v6["remote_addr"] = "2001:db8:1234:5678:9abc:def0:1234:5678"
	if got := mustBuild(t, v6, opts()).Net; got != "2001:db8:1234::/48" {
		t.Errorf("IPv6 Net = %q, want 2001:db8:1234::/48", got)
	}
}

func TestWhoIsAKeyedHashOfTheAddress(t *testing.T) {
	a := mustBuild(t, line(), opts()).Who
	if b := mustBuild(t, line(), opts()).Who; a != b {
		t.Errorf("same key, same address: %q and %q", a, b)
	}
	o := opts()
	o.Key = []byte("daily-key-two")
	if b := mustBuild(t, line(), o).Who; a == b {
		t.Errorf("a different key gave the same Who %q", a)
	}
	other := line()
	other["remote_addr"] = "203.0.113.78"
	if b := mustBuild(t, other, opts()).Who; a == b {
		t.Errorf("a different address gave the same Who %q", a)
	}
	// A plain hash of an IPv4 address can be reversed by trying every address.
	sum := sha256.Sum256([]byte("203.0.113.77"))
	if strings.HasPrefix(hex.EncodeToString(sum[:]), a) {
		t.Errorf("Who %q is a plain SHA-256 of the address", a)
	}
}

func TestAnInternalAddressProducesNoEvent(t *testing.T) {
	campus := netip.MustParsePrefix("203.0.113.0/24")
	o := opts()
	o.Internal = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), campus}
	ev, out, err := Build(line(), o)
	if err != nil {
		t.Fatal(err)
	}
	if out.Kept || out.Excluded != campus {
		t.Errorf("outcome = %+v, want dropped and attributed to %v", out, campus)
	}
	if ev != (Event{}) {
		t.Errorf("event for an internal address = %+v, want the zero event", ev)
	}
	v6 := line()
	v6["remote_addr"] = "2001:db8::1"
	o.Internal = []netip.Prefix{netip.MustParsePrefix("2001:db8::/32")}
	if _, out, _ := Build(v6, o); out.Kept {
		t.Errorf("an internal IPv6 address was kept: %+v", out)
	}
	o.Internal = []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")}
	if _, out, _ := Build(line(), o); !out.Kept {
		t.Errorf("an address outside every internal range was dropped: %+v", out)
	}
}

func TestUserAgentIsCutAt200BytesOnARuneBoundary(t *testing.T) {
	v := line()
	v["http_user_agent"] = strings.Repeat("é", 300) // two bytes each
	ua := mustBuild(t, v, opts()).UA
	if len(ua) > 200 || !utf8.ValidString(ua) || len(ua) < 198 {
		t.Errorf("UA is %d bytes, valid=%v", len(ua), utf8.ValidString(ua))
	}
	// Three-byte characters put byte 200 in the middle of one, so the cut must back up to 198.
	v["http_user_agent"] = strings.Repeat("€", 100)
	if ua := mustBuild(t, v, opts()).UA; len(ua) != 198 || !utf8.ValidString(ua) {
		t.Errorf("UA is %d bytes, valid=%v, want 198 and valid", len(ua), utf8.ValidString(ua))
	}
	v["http_user_agent"] = "short agent"
	if got := mustBuild(t, v, opts()).UA; got != "short agent" {
		t.Errorf("UA = %q", got)
	}
}

func TestLanguageHintsAndViaAreReducedToWhatTheDetectorsNeed(t *testing.T) {
	v := line()
	v["http_accept_language"] = "-"
	v["http_sec_ch_ua"] = "-"
	v["http_sec_ch_ua_platform"] = "-"
	v["http_cf_ray"] = "-"
	ev := mustBuild(t, v, opts())
	if ev.Lang != "" || ev.CH || ev.Plat != "" || ev.Via {
		t.Errorf("event = %+v, want empty language, no hints, not via the proxy", ev)
	}
	v["http_accept_language"] = "ZH-cn;q=0.9"
	if got := mustBuild(t, v, opts()).Lang; got != "zh-cn" {
		t.Errorf("Lang = %q, want zh-cn", got)
	}
}

func TestCloudflareValuesAreKeptWhenSentAndEmptyWhenNot(t *testing.T) {
	v := line()
	v["http_cf_bot_score"], v["http_cf_ja3_hash"], v["http_cf_ja4"] = "12", "e7d705a3286e19ea42f587b344ee6865", "t13d1516h2_8daaf6152771_b186095e22b6"
	ev := mustBuild(t, v, opts())
	if ev.Bot != "12" || ev.JA3 != "e7d705a3286e19ea42f587b344ee6865" || ev.JA4 != "t13d1516h2_8daaf6152771_b186095e22b6" {
		t.Errorf("event = %+v", ev)
	}
}

func TestFamilyAndDeclaredComeFromTheLookup(t *testing.T) {
	o := opts()
	o.Family = func(ua string) (string, bool) { return "Exa", true }
	ev := mustBuild(t, line(), o)
	if ev.Family != "Exa" || ev.Declared != "unverified" {
		t.Errorf("Family %q Declared %q, want Exa unverified", ev.Family, ev.Declared)
	}
	o.Class, o.Family = nil, nil
	ev = mustBuild(t, line(), o)
	if ev.Class != "" || ev.Family != "" || ev.Declared != "none" {
		t.Errorf("with no lookups: %+v", ev)
	}
}

func TestAMalformedLineIsInvalid(t *testing.T) {
	for name, edit := range map[string]func(map[string]string){
		"status":  func(v map[string]string) { v["status"] = "abc" },
		"time":    func(v map[string]string) { v["time_local"] = "yesterday" },
		"address": func(v map[string]string) { v["remote_addr"] = "not-an-address" },
		"no time": func(v map[string]string) { delete(v, "time_local") },
	} {
		v := line()
		edit(v)
		if _, _, err := Build(v, opts()); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

// An unkeyed hash of an address can be reversed by trying every address, so an
// event is never built without a key.
func TestBuildRefusesToRunWithoutAKey(t *testing.T) {
	o := opts()
	o.Key = nil
	if _, _, err := Build(line(), o); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestJSONLinesCarryAVersionAndRoundTrip(t *testing.T) {
	ev := mustBuild(t, line(), opts())
	b, err := Encode(ev)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), `{"v":1,`) || strings.ContainsAny(string(b), "\n") {
		t.Errorf("encoded = %q", b)
	}
	back, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if back != ev {
		t.Errorf("round trip =\n%+v\nwant\n%+v", back, ev)
	}
}

func TestDecodeIsStrict(t *testing.T) {
	for name, in := range map[string]string{
		"unknown field": `{"v":1,"t":"2026-10-08T22:27:13Z","status":200,"path":"/x"}`,
		"wrong version": `{"v":2,"t":"2026-10-08T22:27:13Z","status":200}`,
		"no version":    `{"t":"2026-10-08T22:27:13Z","status":200}`,
		"not json":      `status 200`,
		"trailing text": `{"v":1,"t":"2026-10-08T22:27:13Z","status":200} extra`,
	} {
		if _, err := Decode([]byte(in)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

// The package that holds the privacy rule stays free of the interface and
// configuration libraries, so it can be read and trusted on its own.
func TestEventImportsOnlyTheStandardLibrary(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	for dep := range strings.FieldsSeq(string(out)) {
		first, _, _ := strings.Cut(dep, "/")
		if strings.Contains(first, ".") && !strings.HasPrefix(dep, "github.com/caltechlibrary/logagent/internal/event") {
			t.Errorf("internal/event depends on %s", dep)
		}
	}
}
