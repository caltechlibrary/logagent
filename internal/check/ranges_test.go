package check

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/proxyranges"
)

// withRealIP is a host behind Cloudflare whose http block holds the given
// set_real_ip_from lines and the right header.
func withRealIP(lines ...string) string {
	extra := "real_ip_header CF-Connecting-IP;\n"
	for _, l := range lines {
		extra += "set_real_ip_from " + l + ";\n"
	}
	return host(full, extra)
}

func runWith(t *testing.T, c *config.Config, text string, edit func(*Input)) *Report {
	t.Helper()
	in := Input{Ranges: testRanges(t), Now: testNow, Config: c, Dump: dump(t, text)}
	if edit != nil {
		edit(&in)
	}
	r, err := Run(in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestTheRealIPSuggestionListsTheProxysRanges(t *testing.T) {
	r := run(t, cfg("cloudflare", nil), host(full, ""))
	got := find(r, "real-ip-missing")
	if len(got) != 1 {
		t.Fatalf("findings = %+v", r.Findings)
	}
	for _, want := range []string{"set_real_ip_from 173.245.48.0/20;", "set_real_ip_from 2400:cb00::/32;", "real_ip_header CF-Connecting-IP;", "2026-10-01", "https://www.cloudflare.com/ips/"} {
		if !strings.Contains(got[0].Suggestion, want) {
			t.Errorf("suggestion lacks %q:\n%s", want, got[0].Suggestion)
		}
	}
	if strings.Contains(got[0].Suggestion, "<each range") {
		t.Errorf("the placeholder is still there:\n%s", got[0].Suggestion)
	}
}

func TestConfiguredRangesReplaceTheSnapshot(t *testing.T) {
	c := cfg("cloudflare", nil)
	c.Proxy.TrustedRanges = []string{"198.51.100.0/24"}
	r := runWith(t, c, host(full, ""), nil)
	got := find(r, "real-ip-missing")
	if len(got) != 1 || !strings.Contains(got[0].Suggestion, "set_real_ip_from 198.51.100.0/24;") || strings.Contains(got[0].Suggestion, "173.245.48.0/20") {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestATrustListMissingProxyRangesIsAWarningListingThem(t *testing.T) {
	r := run(t, cfg("cloudflare", nil), withRealIP("173.245.48.0/20"))
	got := find(r, "real-ip-ranges-missing")
	if len(got) != 1 || got[0].Severity != Warn {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if !strings.Contains(got[0].Message, "2400:cb00::/32") || strings.Contains(got[0].Message, "173.245.48.0/20") {
		t.Errorf("message = %s", got[0].Message)
	}
	if !strings.Contains(got[0].Suggestion, "set_real_ip_from 2400:cb00::/32;") || strings.Contains(got[0].Suggestion, "set_real_ip_from 173.245.48.0/20;") {
		t.Errorf("suggestion should hold the missing line only:\n%s", got[0].Suggestion)
	}
	if got[0].At.Line == 0 {
		t.Errorf("At = %+v, want the set_real_ip_from line", got[0].At)
	}
	if r.ExitCode() != 0 {
		t.Errorf("ExitCode = %d; a partial list is a warning", r.ExitCode())
	}
}

func TestACompleteTrustListIsClean(t *testing.T) {
	r := run(t, cfg("cloudflare", nil), withRealIP("173.245.48.0/20", "2400:cb00::/32"))
	if len(r.Findings) != 0 {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestWiderAndOtherTrustedRangesAreNotes(t *testing.T) {
	// A range that contains Cloudflare's covers it, but trusts addresses that are not Cloudflare's.
	r := run(t, cfg("cloudflare", nil), withRealIP("173.245.0.0/16", "2400::/16"))
	if len(find(r, "real-ip-ranges-missing")) != 0 {
		t.Errorf("a covering range was reported missing: %+v", r.Findings)
	}
	got := find(r, "real-ip-trusts-other-ranges")
	if len(got) != 1 || got[0].Severity != Note || !strings.Contains(got[0].Message, "173.245.0.0/16") || !strings.Contains(got[0].Message, "2400::/16") {
		t.Errorf("findings = %+v", r.Findings)
	}
	// An internal range, such as a load balancer's, is reported the same way.
	r = run(t, cfg("cloudflare", nil), withRealIP("173.245.48.0/20", "2400:cb00::/32", "10.0.0.0/8"))
	got = find(r, "real-ip-trusts-other-ranges")
	if len(got) != 1 || !strings.Contains(got[0].Message, "10.0.0.0/8") || strings.Contains(got[0].Message, "173.245.48.0/20") {
		t.Errorf("findings = %+v", r.Findings)
	}
	if r.ExitCode() != 0 {
		t.Errorf("ExitCode = %d", r.ExitCode())
	}
}

func TestTrustingEverythingIsReportedOnceAndNotAlsoAsRanges(t *testing.T) {
	r := run(t, cfg("cloudflare", nil), withRealIP("0.0.0.0/0"))
	if len(find(r, "real-ip-trust-too-wide")) != 1 || len(find(r, "real-ip-ranges-missing")) != 0 || len(find(r, "real-ip-trusts-other-ranges")) != 0 {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestTheSameMissingRangesInManyServersAreOneFinding(t *testing.T) {
	text := "http {\n" + full + "\naccess_log " + logPath + " full;\nreal_ip_header CF-Connecting-IP;\nset_real_ip_from 173.245.48.0/20;\n" +
		"server { server_name a.example; }\nserver { server_name b.example; }\n}\n"
	r := run(t, cfg("cloudflare", nil), text)
	got := find(r, "real-ip-ranges-missing")
	if len(got) != 1 || !strings.Contains(got[0].Message, "a.example, b.example") {
		t.Errorf("findings = %+v", got)
	}
}

func TestConfiguredRangesAreTheListTheTrustListIsComparedWith(t *testing.T) {
	c := cfg("cloudflare", nil)
	c.Proxy.TrustedRanges = []string{"198.51.100.0/24", "203.0.113.0/24"}
	r := runWith(t, c, withRealIP("198.51.100.0/24"), nil)
	got := find(r, "real-ip-ranges-missing")
	if len(got) != 1 || !strings.Contains(got[0].Message, "203.0.113.0/24") || strings.Contains(got[0].Message, "2400:cb00::/32") {
		t.Errorf("findings = %+v", r.Findings)
	}
	r = runWith(t, c, withRealIP("198.51.100.0/24", "203.0.113.0/24"), nil)
	if len(r.Findings) != 0 {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestAnOldSnapshotIsAWarning(t *testing.T) {
	old := func(days int) func(*Input) {
		return func(in *Input) { in.Now = testSnap().Retrieved.AddDate(0, 0, days) }
	}
	c := cfg("cloudflare", nil)
	r := runWith(t, c, withRealIP("173.245.48.0/20", "2400:cb00::/32"), old(91))
	got := find(r, "ranges-snapshot-old")
	if len(got) != 1 || got[0].Severity != Warn || !strings.Contains(got[0].Message, "2026-10-01") || !strings.Contains(got[0].Message, "91 days") {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if !strings.Contains(got[0].Suggestion, "--refresh-ranges") || !strings.Contains(got[0].Suggestion, "trusted_ranges") {
		t.Errorf("suggestion = %s", got[0].Suggestion)
	}
	if r.ExitCode() != 0 {
		t.Errorf("ExitCode = %d; an old list is a warning", r.ExitCode())
	}
	// Exactly at the limit is still fresh.
	if r := runWith(t, c, withRealIP("173.245.48.0/20", "2400:cb00::/32"), old(90)); len(find(r, "ranges-snapshot-old")) != 0 {
		t.Errorf("90 days is not old: %+v", r.Findings)
	}
	// The limit is configurable.
	c.Proxy.RangesMaxAgeDays = 30
	if r := runWith(t, c, withRealIP("173.245.48.0/20", "2400:cb00::/32"), old(31)); len(find(r, "ranges-snapshot-old")) != 1 {
		t.Errorf("31 days with a 30 day limit: %+v", r.Findings)
	}
	if r := runWith(t, c, withRealIP("173.245.48.0/20", "2400:cb00::/32"), old(30)); len(find(r, "ranges-snapshot-old")) != 0 {
		t.Errorf("30 days with a 30 day limit: %+v", r.Findings)
	}
}

func TestAgeIsNotJudgedWhenItDoesNotMatter(t *testing.T) {
	far := func(in *Input) { in.Now = testNow.AddDate(5, 0, 0) }
	// A host that lists its own ranges does not use the snapshot.
	c := cfg("cloudflare", nil)
	c.Proxy.TrustedRanges = []string{"173.245.48.0/20"}
	if r := runWith(t, c, withRealIP("173.245.48.0/20"), far); len(find(r, "ranges-snapshot-old")) != 0 {
		t.Errorf("configured ranges: %+v", r.Findings)
	}
	// A host with no proxy does not use it either.
	if r := runWith(t, cfg("none", nil), host(full, ""), far); len(find(r, "ranges-snapshot-old")) != 0 {
		t.Errorf("no proxy: %+v", r.Findings)
	}
	// A snapshot fetched today is fresh whatever the built-in one says.
	fresh := func(in *Input) {
		in.Ranges = &proxyranges.Snapshot{Provider: "cloudflare", Retrieved: testNow, Prefixes: testSnap().Prefixes}
	}
	if r := runWith(t, cfg("cloudflare", nil), withRealIP("173.245.48.0/20", "2400:cb00::/32"), fresh); len(r.Findings) != 0 {
		t.Errorf("fresh snapshot: %+v", r.Findings)
	}
}

func TestWithNoClockGivenTheRealOneIsUsed(t *testing.T) {
	r := runWith(t, cfg("cloudflare", nil), withRealIP("173.245.48.0/20", "2400:cb00::/32"), func(in *Input) {
		in.Now = time.Time{}
		in.Ranges = &proxyranges.Snapshot{Provider: "cloudflare", Retrieved: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Prefixes: testSnap().Prefixes}
	})
	if len(find(r, "ranges-snapshot-old")) != 1 {
		t.Errorf("a snapshot from the year 2000 checked against today: %+v", r.Findings)
	}
}

func TestAFailedRefreshIsANoteAndTheSnapshotIsStillUsed(t *testing.T) {
	r := runWith(t, cfg("cloudflare", nil), withRealIP("173.245.48.0/20"), func(in *Input) {
		in.RangesErr = errors.New("Get https://www.cloudflare.com/ips-v4: dial tcp: i/o timeout")
	})
	got := find(r, "ranges-refresh-failed")
	if len(got) != 1 || got[0].Severity != Note || !strings.Contains(got[0].Message, "i/o timeout") {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if len(find(r, "real-ip-ranges-missing")) != 1 {
		t.Errorf("the built-in snapshot was not used: %+v", r.Findings)
	}
}

// With no ranges handed in, the list built into the program is used.
func TestTheBuiltInSnapshotIsTheDefault(t *testing.T) {
	r, err := Run(Input{Now: testNow, Config: cfg("cloudflare", nil), Dump: dump(t, withRealIP("173.245.48.0/20", "2400:cb00::/32"))})
	if err != nil {
		t.Fatal(err)
	}
	got := find(r, "real-ip-ranges-missing")
	if len(got) != 1 {
		t.Fatalf("findings = %+v", r.Findings)
	}
	builtin := proxyranges.Cloudflare()
	for _, p := range builtin.Prefixes {
		if p == netip.MustParsePrefix("173.245.48.0/20") || p == netip.MustParsePrefix("2400:cb00::/32") {
			continue
		}
		if !strings.Contains(got[0].Message, p.String()) {
			t.Errorf("message does not list %s", p)
		}
	}
}
