// Package proxyranges holds a dated snapshot of the address ranges a proxy
// publishes for the servers it connects from, and compares a web server's
// real-IP trust list with it. The snapshot is built into the program; a fresh
// copy can be fetched from the proxy's published lists.
//
// A list of ranges is a security setting, so the snapshot says when it was
// taken, and callers warn when it is old (DR-0003).
package proxyranges

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

//go:embed cloudflare.json
var cloudflareJSON []byte

// ErrMalformed means a list of ranges, built in or fetched, cannot be trusted:
// it is not valid, or is empty, or holds a range too broad to be one proxy's.
var ErrMalformed = errors.New("malformed list of proxy ranges")

// maxBody is the most bytes read from one published list. The real lists are
// well under a kilobyte.
const maxBody = 256 * 1024

// Sources are the two pages that publish a proxy's ranges, one range per line.
type Sources struct {
	IPv4, IPv6 string
}

// CloudflareSources are Cloudflare's published lists.
var CloudflareSources = Sources{
	IPv4: "https://www.cloudflare.com/ips-v4",
	IPv6: "https://www.cloudflare.com/ips-v6",
}

// Snapshot is a proxy's list of ranges as it was on one day.
type Snapshot struct {
	// Provider names the proxy.
	Provider string
	// Retrieved is the day the list was taken, at midnight UTC.
	Retrieved time.Time
	// Source says where the list came from.
	Source string
	// Prefixes are the ranges, IPv4 first and then IPv6, in the order published.
	Prefixes []netip.Prefix
}

// build checks the lists and assembles a snapshot.
func build(provider string, retrieved time.Time, source string, v4, v6 []string) (*Snapshot, error) {
	if provider == "" {
		return nil, fmt.Errorf("%w: provider is required", ErrMalformed)
	}
	if retrieved.IsZero() {
		return nil, fmt.Errorf("%w: retrieved is required, as YYYY-MM-DD", ErrMalformed)
	}
	s := &Snapshot{Provider: provider, Retrieved: retrieved, Source: source}
	seen := map[netip.Prefix]bool{}
	for _, list := range []struct {
		name  string
		items []string
		is4   bool
	}{{"ipv4", v4, true}, {"ipv6", v6, false}} {
		if len(list.items) == 0 {
			return nil, fmt.Errorf("%w: the %s list is empty", ErrMalformed, list.name)
		}
		for _, text := range list.items {
			p, err := netip.ParsePrefix(text)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %q is not a CIDR range", ErrMalformed, list.name, text)
			}
			if p != p.Masked() {
				return nil, fmt.Errorf("%w: %s: %q has host bits set", ErrMalformed, list.name, text)
			}
			if p.Addr().Is4() != list.is4 || p.Addr().Is4In6() {
				return nil, fmt.Errorf("%w: %q is not an %s range", ErrMalformed, text, list.name)
			}
			if (list.is4 && p.Bits() < 8) || (!list.is4 && p.Bits() < 16) {
				return nil, fmt.Errorf("%w: %s: %q is too broad to be one proxy's range", ErrMalformed, list.name, text)
			}
			if seen[p] {
				return nil, fmt.Errorf("%w: %s: %q is listed twice", ErrMalformed, list.name, text)
			}
			seen[p] = true
			s.Prefixes = append(s.Prefixes, p)
		}
	}
	return s, nil
}

// Parse reads a snapshot from its JSON file, strictly.
//
// @param data {[]byte} the JSON text
// @returns {*Snapshot, error} the snapshot, or an error matching ErrMalformed
// @example
//
//	s, err := proxyranges.Parse(data)
func Parse(data []byte) (*Snapshot, error) {
	var f struct {
		Version   int      `json:"version"`
		Provider  string   `json:"provider"`
		Retrieved string   `json:"retrieved"`
		Source    string   `json:"source"`
		IPv4      []string `json:"ipv4"`
		IPv6      []string `json:"ipv6"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("%w: version must be 1 (got %d)", ErrMalformed, f.Version)
	}
	var retrieved time.Time
	if f.Retrieved != "" {
		t, err := time.Parse("2006-01-02", f.Retrieved)
		if err != nil {
			return nil, fmt.Errorf("%w: retrieved %q is not a date written YYYY-MM-DD", ErrMalformed, f.Retrieved)
		}
		retrieved = t
	}
	return build(f.Provider, retrieved, f.Source, f.IPv4, f.IPv6)
}

// Cloudflare returns the snapshot of Cloudflare's ranges built into the
// program. It panics if the embedded file is invalid, which a test prevents.
//
// @returns {*Snapshot} the built-in snapshot
// @example
//
//	fmt.Println(proxyranges.Cloudflare().Retrieved.Format("2006-01-02"))
func Cloudflare() *Snapshot {
	s, err := Parse(cloudflareJSON)
	if err != nil {
		panic("logagent: embedded cloudflare.json: " + err.Error())
	}
	return s
}

func midnight(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// AgeDays returns how many whole days before now the list was taken. It is
// negative if the clock is behind the retrieval date.
//
// @param now {time.Time} the current time
// @returns {int} the age in days
// @example
//
//	age := snapshot.AgeDays(time.Now())
func (s *Snapshot) AgeDays(now time.Time) int {
	return int(midnight(now).Sub(midnight(s.Retrieved)).Hours() / 24)
}

// Stale reports whether the list is older than maxDays.
//
// @param now {time.Time} the current time
// @param maxDays {int} the oldest age that is still fresh
// @returns {bool} true when the list is older than maxDays
// @example
//
//	if snapshot.Stale(time.Now(), 90) { fmt.Println("refresh it") }
func (s *Snapshot) Stale(now time.Time, maxDays int) bool { return s.AgeDays(now) > maxDays }

func sameFamily(a, b netip.Prefix) bool { return a.Addr().Is4() == b.Addr().Is4() }

// covers reports whether outer contains all of inner.
func covers(outer, inner netip.Prefix) bool {
	return sameFamily(outer, inner) && outer.Bits() <= inner.Bits() && outer.Contains(inner.Addr())
}

// Missing returns the snapshot's ranges that no range in trusted covers, in
// the snapshot's order. A wider trusted range covers; a narrower one does not.
//
// @param trusted {[]netip.Prefix} the ranges a server trusts
// @returns {[]netip.Prefix} the snapshot ranges left uncovered
// @example
//
//	missing := snapshot.Missing(trusted)
func (s *Snapshot) Missing(trusted []netip.Prefix) []netip.Prefix {
	var out []netip.Prefix
	for _, p := range s.Prefixes {
		covered := false
		for _, t := range trusted {
			if covers(t, p) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, p)
		}
	}
	return out
}

// Extra returns the ranges in trusted that are not inside any of the
// snapshot's, in the order given: other networks, and ranges wider than the
// proxy's own.
//
// @param trusted {[]netip.Prefix} the ranges a server trusts
// @returns {[]netip.Prefix} the trusted ranges that are not the proxy's
// @example
//
//	extra := snapshot.Extra(trusted)
func (s *Snapshot) Extra(trusted []netip.Prefix) []netip.Prefix {
	var out []netip.Prefix
	for _, t := range trusted {
		inside := false
		for _, p := range s.Prefixes {
			if covers(p, t) {
				inside = true
				break
			}
		}
		if !inside {
			out = append(out, t)
		}
	}
	return out
}

// Diff returns the ranges in updated that are not in old, and those in old
// that are not in updated.
//
// @param old {*Snapshot} the earlier list
// @param updated {*Snapshot} the later list
// @returns {[]netip.Prefix, []netip.Prefix} the added ranges and the removed ranges
// @example
//
//	added, removed := proxyranges.Diff(builtin, fetched)
func Diff(old, updated *Snapshot) (added, removed []netip.Prefix) {
	in := func(list []netip.Prefix, p netip.Prefix) bool {
		for _, q := range list {
			if q == p {
				return true
			}
		}
		return false
	}
	for _, p := range updated.Prefixes {
		if !in(old.Prefixes, p) {
			added = append(added, p)
		}
	}
	for _, p := range old.Prefixes {
		if !in(updated.Prefixes, p) {
			removed = append(removed, p)
		}
	}
	return added, removed
}

// Fetch reads the proxy's two published lists and builds a snapshot dated
// today. It refuses a list that is empty, malformed, broader than one proxy
// could be, or larger than any real list.
//
// @param ctx {context.Context} cancels the requests
// @param client {*http.Client} makes the requests
// @param src {Sources} the two list addresses
// @param now {time.Time} the date to record
// @returns {*Snapshot, error} the snapshot, a network error, or an error matching ErrMalformed
// @example
//
//	s, err := proxyranges.Fetch(ctx, http.DefaultClient, proxyranges.CloudflareSources, time.Now())
func Fetch(ctx context.Context, client *http.Client, src Sources, now time.Time) (*Snapshot, error) {
	v4, err := fetchList(ctx, client, src.IPv4)
	if err != nil {
		return nil, err
	}
	v6, err := fetchList(ctx, client, src.IPv6)
	if err != nil {
		return nil, err
	}
	return build("cloudflare", midnight(now), src.IPv4+" "+src.IPv6, v4, v6)
}

func fetchList(ctx context.Context, client *http.Client, url string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("%w: %s is larger than %d bytes", ErrMalformed, url, maxBody)
	}
	var out []string
	for _, line := range strings.Split(string(body), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}
