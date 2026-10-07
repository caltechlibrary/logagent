package logagent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/caltechlibrary/logagent/internal/proxyranges"
)

// publishedRanges stands in for Cloudflare's two lists, counting the requests it gets.
func publishedRanges(t *testing.T) (hits *int32, closeServer func()) {
	t.Helper()
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		if strings.HasSuffix(r.URL.Path, "v6") {
			w.Write([]byte("2400:cb00::/32\n"))
			return
		}
		w.Write([]byte("173.245.48.0/20\n103.21.244.0/22\n"))
	}))
	saved := rangeSources
	rangeSources = proxyranges.Sources{IPv4: srv.URL + "/ips-v4", IPv6: srv.URL + "/ips-v6"}
	t.Cleanup(func() { rangeSources = saved; srv.Close() })
	return &n, srv.Close
}

func TestCheckUsesTheBuiltInRangesAndNeverTheNetworkByDefault(t *testing.T) {
	hits, _ := publishedRanges(t)
	cfg, _ := sampleHost(t, []string{"-"}) // trusts only 173.245.48.0/20
	code, out, errOut := run("check", "--config", cfg)
	if code != ExitOK || errOut != "" {
		t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
	}
	if atomic.LoadInt32(hits) != 0 {
		t.Errorf("the published lists were fetched %d times without --refresh-ranges", *hits)
	}
	if !strings.Contains(out, "[warn] real-ip-ranges-missing") || !strings.Contains(out, "104.16.0.0/13") {
		t.Errorf("the built-in list was not compared:\n%s", out)
	}
}

func TestCheckRefreshRangesComparesWithTheLiveLists(t *testing.T) {
	cfg, _ := sampleHost(t, []string{"-"})
	for _, args := range [][]string{{"--refresh-ranges"}, {"-r"}, {"-jr"}} {
		hits, _ := publishedRanges(t)
		code, out, errOut := run(append([]string{"check", "--config", cfg}, args...)...)
		if code != ExitOK || errOut != "" {
			t.Fatalf("%v: exit %d, stderr %q\n%s", args, code, errOut, out)
		}
		if atomic.LoadInt32(hits) != 2 {
			t.Errorf("%v: %d requests, want 2 (IPv4 and IPv6)", args, *hits)
		}
		if !strings.Contains(out, "103.21.244.0/22") || !strings.Contains(out, "2400:cb00::/32") || strings.Contains(out, "104.16.0.0/13") {
			t.Errorf("%v: the live lists were not used:\n%s", args, out)
		}
		if strings.Contains(out, "ranges-snapshot-old") || strings.Contains(out, "ranges-refresh-failed") {
			t.Errorf("%v: a fresh list was reported old or failed:\n%s", args, out)
		}
	}
}

func TestCheckRefreshFailureIsANoteAndTheBuiltInListIsUsed(t *testing.T) {
	cfg, _ := sampleHost(t, []string{"-"})
	_, stop := publishedRanges(t)
	stop() // connection refused
	code, out, errOut := run("check", "--config", cfg, "--refresh-ranges")
	if code != ExitOK || errOut != "" {
		t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
	}
	if !strings.Contains(out, "[note] ranges-refresh-failed") || !strings.Contains(out, "104.16.0.0/13") {
		t.Errorf("output:\n%s", out)
	}
}

func TestCheckRefreshIsSkippedWhenThereIsNoProxyToKnowAbout(t *testing.T) {
	hits, _ := publishedRanges(t)
	cfg, _ := setup(t, goodDump, "") // no proxy
	if code, _, _ := run("check", "--config", cfg, "-r"); code != ExitOK {
		t.Errorf("exit %d", code)
	}
	if atomic.LoadInt32(hits) != 0 {
		t.Errorf("fetched %d times for a host with no proxy", *hits)
	}
}
