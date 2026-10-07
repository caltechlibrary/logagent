package proxyranges

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func pfx(t *testing.T, ss ...string) []netip.Prefix {
	t.Helper()
	var out []netip.Prefix
	for _, s := range ss {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func strs(ps []netip.Prefix) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.String())
	}
	return out
}

const small = `{"version":1,"provider":"cloudflare","retrieved":"2026-10-07","source":"https://www.cloudflare.com/ips/",
 "ipv4":["173.245.48.0/20","103.21.244.0/22"],"ipv6":["2400:cb00::/32"]}`

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestTheEmbeddedSnapshotIsWellFormed(t *testing.T) {
	s := Cloudflare()
	if s.Provider != "cloudflare" || s.Retrieved.IsZero() || !s.Retrieved.Before(time.Now().AddDate(0, 0, 1)) {
		t.Fatalf("provider %q retrieved %v", s.Provider, s.Retrieved)
	}
	if !strings.HasPrefix(s.Source, "https://") {
		t.Errorf("source = %q, want the page the list came from", s.Source)
	}
	var v4, v6 int
	seen := map[netip.Prefix]bool{}
	for _, p := range s.Prefixes {
		if p != p.Masked() {
			t.Errorf("%s has host bits set", p)
		}
		if seen[p] {
			t.Errorf("%s is listed twice", p)
		}
		seen[p] = true
		switch {
		case p.Addr().Is4() && p.Bits() >= 10:
			v4++
		case p.Addr().Is6() && p.Bits() >= 20:
			v6++
		default:
			t.Errorf("%s is too broad to be one proxy's range", p)
		}
	}
	if v4 < 10 || v6 < 5 {
		t.Errorf("%d IPv4 and %d IPv6 ranges; the published lists have more", v4, v6)
	}
}

func TestParse(t *testing.T) {
	s, err := Parse([]byte(small))
	if err != nil {
		t.Fatal(err)
	}
	if s.Provider != "cloudflare" || !s.Retrieved.Equal(day("2026-10-07")) {
		t.Errorf("snapshot = %+v", s)
	}
	if got := strings.Join(strs(s.Prefixes), " "); got != "173.245.48.0/20 103.21.244.0/22 2400:cb00::/32" {
		t.Errorf("prefixes = %s, want IPv4 then IPv6 in file order", got)
	}
}

func TestParseRejectsBadSnapshots(t *testing.T) {
	cases := []struct{ name, from, to, mention string }{
		{"version", `"version":1`, `"version":2`, "version"},
		{"unknown key", `"provider"`, `"bogus":1,"provider"`, "bogus"},
		{"no provider", `"provider":"cloudflare"`, `"provider":""`, "provider"},
		{"no date", `"retrieved":"2026-10-07"`, `"retrieved":""`, "retrieved"},
		{"bad date", `"retrieved":"2026-10-07"`, `"retrieved":"7 Oct 2026"`, "retrieved"},
		{"bad range", `"173.245.48.0/20"`, `"173.245.48.0/33"`, "173.245.48.0/33"},
		{"bare address", `"173.245.48.0/20"`, `"173.245.48.1"`, "173.245.48.1"},
		{"host bits", `"173.245.48.0/20"`, `"173.245.48.1/20"`, "173.245.48.1/20"},
		{"v6 in the v4 list", `"173.245.48.0/20"`, `"2400:cb00::/32"`, "ipv4"},
		{"v4 in the v6 list", `"2400:cb00::/32"`, `"173.245.48.0/20"`, "ipv6"},
		{"everything", `"173.245.48.0/20"`, `"0.0.0.0/0"`, "0.0.0.0/0"},
		{"duplicate", `"103.21.244.0/22"`, `"173.245.48.0/20"`, "173.245.48.0/20"},
		{"no ipv4", `"ipv4":["173.245.48.0/20","103.21.244.0/22"]`, `"ipv4":[]`, "ipv4"},
		{"no ipv6", `"ipv6":["2400:cb00::/32"]`, `"ipv6":[]`, "ipv6"},
	}
	for _, c := range cases {
		if !strings.Contains(small, c.from) {
			t.Fatalf("%s: fixture lacks %q", c.name, c.from)
		}
		_, err := Parse([]byte(strings.Replace(small, c.from, c.to, 1)))
		if !errors.Is(err, ErrMalformed) || !strings.Contains(err.Error(), c.mention) {
			t.Errorf("%s: error = %v, want ErrMalformed mentioning %q", c.name, err, c.mention)
		}
	}
	if _, err := Parse([]byte(`not json`)); !errors.Is(err, ErrMalformed) {
		t.Errorf("not JSON: error = %v", err)
	}
}

func TestAgeAndStaleness(t *testing.T) {
	s, _ := Parse([]byte(small)) // retrieved 2026-10-07
	for _, c := range []struct {
		now   string
		age   int
		stale bool
	}{
		{"2026-10-07", 0, false},
		{"2026-11-06", 30, false},
		{"2027-01-05", 90, false}, // exactly the limit is still fresh
		{"2027-01-06", 91, true},
		{"2026-10-01", -6, false}, // a clock behind the retrieval date is not stale
	} {
		if got := s.AgeDays(day(c.now)); got != c.age {
			t.Errorf("AgeDays(%s) = %d, want %d", c.now, got, c.age)
		}
		if got := s.Stale(day(c.now), 90); got != c.stale {
			t.Errorf("Stale(%s, 90) = %v, want %v", c.now, got, c.stale)
		}
	}
	if !s.Stale(day("2026-11-10"), 30) || s.Stale(day("2026-11-06"), 30) {
		t.Error("the limit is not taken from the argument")
	}
	// The time of day does not change the age.
	if s.AgeDays(day("2026-11-06").Add(23*time.Hour)) != 30 {
		t.Error("age counts partial days")
	}
}

func TestMissingFindsTheRangesATrustListDoesNotCover(t *testing.T) {
	s, _ := Parse([]byte(small))
	cases := []struct {
		name    string
		trusted []string
		want    string
	}{
		{"all of them", []string{"173.245.48.0/20", "103.21.244.0/22", "2400:cb00::/32"}, ""},
		{"none", nil, "173.245.48.0/20 103.21.244.0/22 2400:cb00::/32"},
		{"IPv6 left out", []string{"173.245.48.0/20", "103.21.244.0/22"}, "2400:cb00::/32"},
		{"one IPv4 left out", []string{"173.245.48.0/20", "2400:cb00::/32"}, "103.21.244.0/22"},
		{"a wider range covers", []string{"173.245.0.0/16", "103.0.0.0/8", "2400::/16"}, ""},
		{"a narrower range does not", []string{"173.245.48.0/24", "103.21.244.0/22", "2400:cb00::/32"}, "173.245.48.0/20"},
		{"IPv4 does not cover IPv6", []string{"0.0.0.0/1", "128.0.0.0/1"}, "2400:cb00::/32"},
		{"unrelated ranges", []string{"10.0.0.0/8", "192.168.0.0/16"}, "173.245.48.0/20 103.21.244.0/22 2400:cb00::/32"},
	}
	for _, c := range cases {
		got := strings.Join(strs(s.Missing(pfx(t, c.trusted...))), " ")
		if got != c.want {
			t.Errorf("%s: Missing = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestExtraFindsTrustedRangesThatAreNotTheProxys(t *testing.T) {
	s, _ := Parse([]byte(small))
	cases := []struct {
		name    string
		trusted []string
		want    string
	}{
		{"exactly the proxy's", []string{"173.245.48.0/20", "2400:cb00::/32"}, ""},
		{"inside a proxy range", []string{"173.245.48.0/24"}, ""},
		{"a wider range that contains one", []string{"173.245.0.0/16"}, "173.245.0.0/16"},
		{"an internal load balancer", []string{"173.245.48.0/20", "10.0.0.0/8"}, "10.0.0.0/8"},
		{"two others, in the order given", []string{"192.168.0.0/16", "10.0.0.0/8"}, "192.168.0.0/16 10.0.0.0/8"},
	}
	for _, c := range cases {
		got := strings.Join(strs(s.Extra(pfx(t, c.trusted...))), " ")
		if got != c.want {
			t.Errorf("%s: Extra = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDiff(t *testing.T) {
	a, _ := Parse([]byte(small))
	b, _ := Parse([]byte(strings.Replace(small, `"103.21.244.0/22"`, `"103.22.200.0/22"`, 1)))
	added, removed := Diff(a, b)
	if strings.Join(strs(added), " ") != "103.22.200.0/22" || strings.Join(strs(removed), " ") != "103.21.244.0/22" {
		t.Errorf("added %v removed %v", added, removed)
	}
	if a2, r2 := Diff(a, a); len(a2) != 0 || len(r2) != 0 {
		t.Errorf("a snapshot differs from itself: %v %v", a2, r2)
	}
}

// ranges serves the two published lists.
func ranges(t *testing.T, v4, v6 string, status int) (Sources, func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ips-v4", func(w http.ResponseWriter, r *http.Request) {
		if status != 0 {
			http.Error(w, "boom", status)
			return
		}
		w.Write([]byte(v4))
	})
	mux.HandleFunc("/ips-v6", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(v6)) })
	srv := httptest.NewServer(mux)
	return Sources{IPv4: srv.URL + "/ips-v4", IPv6: srv.URL + "/ips-v6"}, srv.Close
}

func TestFetchReadsThePublishedLists(t *testing.T) {
	src, stop := ranges(t, "173.245.48.0/20\r\n103.21.244.0/22\n\n", "2400:cb00::/32\n", 0)
	defer stop()
	s, err := Fetch(context.Background(), http.DefaultClient, src, day("2026-12-25"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(strs(s.Prefixes), " "); got != "173.245.48.0/20 103.21.244.0/22 2400:cb00::/32" {
		t.Errorf("prefixes = %s", got)
	}
	if !s.Retrieved.Equal(day("2026-12-25")) || s.Provider != "cloudflare" || s.Source != src.IPv4+" "+src.IPv6 {
		t.Errorf("snapshot = %+v", s)
	}
	if s.AgeDays(day("2026-12-25")) != 0 {
		t.Error("a fetched snapshot is not fresh")
	}
}

func TestFetchRefusesWhatItCannotTrust(t *testing.T) {
	cases := []struct {
		name, v4, v6 string
		status       int
		malformed    bool
	}{
		{"a line that is not a range", "173.245.48.0/20\nnot-a-range\n", "2400:cb00::/32\n", 0, true},
		{"host bits set", "173.245.48.1/20\n", "2400:cb00::/32\n", 0, true},
		{"an empty IPv4 list", "\n", "2400:cb00::/32\n", 0, true},
		{"an empty IPv6 list", "173.245.48.0/20\n", "", 0, true},
		{"a web page instead", "<html>blocked</html>", "<html>", 0, true},
		{"everything", "0.0.0.0/0\n", "2400:cb00::/32\n", 0, true},
		{"the server errors", "", "", 500, false},
	}
	for _, c := range cases {
		src, stop := ranges(t, c.v4, c.v6, c.status)
		_, err := Fetch(context.Background(), http.DefaultClient, src, day("2026-12-25"))
		stop()
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if c.malformed != errors.Is(err, ErrMalformed) {
			t.Errorf("%s: error = %v, ErrMalformed = %v", c.name, err, errors.Is(err, ErrMalformed))
		}
	}
}

func TestFetchWhenTheServerCannotBeReached(t *testing.T) {
	src, stop := ranges(t, "173.245.48.0/20\n", "2400:cb00::/32\n", 0)
	stop() // closed: connection refused
	if _, err := Fetch(context.Background(), http.DefaultClient, src, day("2026-12-25")); err == nil || errors.Is(err, ErrMalformed) {
		t.Errorf("error = %v, want a network error that is not ErrMalformed", err)
	}
}

func TestFetchHonoursTheContextAndLimitsTheBody(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := Fetch(ctx, http.DefaultClient, Sources{IPv4: slow.URL, IPv6: slow.URL}, day("2026-12-25")); err == nil {
		t.Error("a hung server did not time out")
	}
	// Every line is a valid, distinct range, so only the size can refuse it.
	var sb strings.Builder
	for i := 0; i < 30000; i++ {
		fmt.Fprintf(&sb, "10.%d.%d.0/24\n", i/256, i%256)
	}
	big := sb.String()
	src, stop := ranges(t, big, "2400:cb00::/32\n", 0)
	defer stop()
	if _, err := Fetch(context.Background(), http.DefaultClient, src, day("2026-12-25")); err == nil {
		t.Error("an oversized body was accepted")
	}
}

func TestTheDefaultSourcesAreCloudflaresPublishedLists(t *testing.T) {
	if CloudflareSources.IPv4 != "https://www.cloudflare.com/ips-v4" || CloudflareSources.IPv6 != "https://www.cloudflare.com/ips-v6" {
		t.Errorf("sources = %+v", CloudflareSources)
	}
}

// Cloudflare's IPv4 list has no newline after its last line.
func TestFetchAcceptsAListWithNoTrailingNewline(t *testing.T) {
	src, stop := ranges(t, "173.245.48.0/20\n131.0.72.0/22", "2400:cb00::/32", 0)
	defer stop()
	s, err := Fetch(context.Background(), http.DefaultClient, src, day("2026-12-25"))
	if err != nil || len(s.Prefixes) != 3 {
		t.Errorf("%v, %v", s, err)
	}
}
