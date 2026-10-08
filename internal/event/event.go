// Package event is the common record the tiers read: one reduced request. It
// holds what detection and analysis need (time, status, cost, who the client
// claims to be) and never what a patron did (the path, the query string, the
// referer) or who they are (the full address). Build is the one place that rule
// is applied (DR-0002, DR-0006).
//
// The package imports only the standard library. The lookups that need
// configuration, the path class and the declared family, are passed in as
// functions.
package event

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Version is the version written into every event and required when one is read.
const Version = 1

// maxUserAgent is how many bytes of the user agent an event keeps.
const maxUserAgent = 200

// ErrInvalid means a log line or a stored event is malformed: a status that is
// not a number, an address that is not one, a time that cannot be read, an
// unknown field, or a missing key.
var ErrInvalid = errors.New("invalid event")

// Event is one reduced request. Its JSON form is one line.
type Event struct {
	// V is the version of this record.
	V int `json:"v"`
	// T is the request time in UTC, to the second.
	T time.Time `json:"t"`
	// Method is the HTTP method from a closed set; anything else is "other".
	Method string `json:"method,omitempty"`
	// Class is the path class chosen by the host's rules, never the path.
	Class string `json:"class,omitempty"`
	// Status is the response status.
	Status int `json:"status"`
	// RT is the request time in seconds; URT the upstream time, summed over retries.
	RT  float64 `json:"rt"`
	URT float64 `json:"urt"`
	// Timed is true when the log recorded a request time. A line without one
	// (an older log format) has RT and URT of zero, which is not a measurement.
	Timed bool `json:"timed,omitempty"`
	// Cache is nginx's cache status (HIT, MISS, BYPASS, ...), empty when no cache applied.
	Cache string `json:"cache,omitempty"`
	// UA is the user agent cut at 200 bytes. It is text the client chose: data, never an instruction.
	UA string `json:"ua,omitempty"`
	// Family is the declared agent or the platform bucket the user agent falls in.
	Family string `json:"family,omitempty"`
	// Declared is "unverified" for a declared automated agent and "none" otherwise.
	Declared string `json:"declared,omitempty"`
	// Country is the proxy's country code for the client.
	Country string `json:"country,omitempty"`
	// Plat is the client-hint platform, such as Windows.
	Plat string `json:"plat,omitempty"`
	// CH reports whether a Sec-CH-UA header was sent.
	CH bool `json:"ch,omitempty"`
	// Lang is the first Accept-Language tag, lowercased.
	Lang string `json:"lang,omitempty"`
	// Via reports whether the request came through the proxy (a request id was present).
	Via bool `json:"via,omitempty"`
	// JA3, JA4 and Bot are the proxy's TLS fingerprints and bot score, when sent.
	JA3 string `json:"ja3,omitempty"`
	JA4 string `json:"ja4,omitempty"`
	Bot string `json:"bot,omitempty"`
	// Net is the client address cut to /24 (IPv4) or /48 (IPv6).
	Net string `json:"net,omitempty"`
	// Who is a keyed hash of the full address: equal for one address under one key,
	// unlinkable across keys, and not a plain hash.
	Who string `json:"who,omitempty"`
}

// Options is what Build needs beyond the log line.
type Options struct {
	// Key is the secret for Who. It must not be empty; a run generates one and discards it.
	Key []byte
	// Internal lists the address ranges that produce no event, only a count.
	Internal []netip.Prefix
	// Class returns the path class for a path whose query string is already removed.
	// Nil leaves Class empty.
	Class func(path string) string
	// Family returns the family for a user agent and whether it is a declared automated agent.
	// Nil leaves Family empty and Declared "none".
	Family func(userAgent string) (family string, declared bool)
}

// Outcome says what Build did with a line.
type Outcome struct {
	// Kept is true when an event was built. It is false for an address in an internal range.
	Kept bool
	// Excluded is the internal range that matched, when Kept is false.
	Excluded netip.Prefix
}

// Build reduces one parsed log line to an event. The line is a map from an
// nginx variable name, without the dollar sign, to its value, which is what the
// sample package returns. An address in one of o.Internal's ranges returns the
// zero event and an Outcome naming the range, so the caller can count it; the
// rest of the line is not read.
//
// @param v {map[string]string} the parsed line
// @param o {Options} the key, internal ranges and lookups
// @returns {Event} the reduced request
// @returns {Outcome} whether an event was built, or the internal range that excluded the line
// @returns {error} ErrInvalid for a malformed line or an empty key
// @example
//
//	ev, out, err := event.Build(values, event.Options{Key: key})
//	if err == nil && out.Kept {
//		fmt.Println(ev.Status, ev.Class)
//	}
func Build(v map[string]string, o Options) (Event, Outcome, error) {
	if len(o.Key) == 0 {
		return Event{}, Outcome{}, fmt.Errorf("%w: no key", ErrInvalid)
	}
	addr, err := netip.ParseAddr(v["remote_addr"])
	if err != nil {
		return Event{}, Outcome{}, fmt.Errorf("%w: client address %q", ErrInvalid, v["remote_addr"])
	}
	addr = addr.Unmap()
	for _, p := range o.Internal {
		if p.Contains(addr) {
			return Event{}, Outcome{Excluded: p}, nil
		}
	}
	t, err := time.Parse("02/Jan/2006:15:04:05 -0700", v["time_local"])
	if err != nil {
		return Event{}, Outcome{}, fmt.Errorf("%w: time %q", ErrInvalid, v["time_local"])
	}
	status, err := strconv.Atoi(v["status"])
	if err != nil {
		return Event{}, Outcome{}, fmt.Errorf("%w: status %q", ErrInvalid, v["status"])
	}
	method, path := splitRequest(v["request"])
	ua := cutUserAgent(dash(v["http_user_agent"]))
	ev := Event{
		V: Version, T: t.UTC().Truncate(time.Second), Method: method, Status: status,
		RT: seconds(v["request_time"]), URT: sumSeconds(v["upstream_response_time"]), Timed: isNumber(v["request_time"]),
		Cache: dash(v["upstream_cache_status"]), UA: ua, Declared: "none",
		Country: dash(v["http_cf_ipcountry"]), Plat: strings.Trim(dash(v["http_sec_ch_ua_platform"]), `"`),
		CH: dash(v["http_sec_ch_ua"]) != "", Lang: firstLanguage(v["http_accept_language"]),
		Via: dash(v["http_cf_ray"]) != "", JA3: dash(v["http_cf_ja3_hash"]), JA4: dash(v["http_cf_ja4"]),
		Bot: dash(v["http_cf_bot_score"]), Net: truncate(addr), Who: keyed(o.Key, addr),
	}
	if o.Class != nil {
		ev.Class = o.Class(path)
	}
	if o.Family != nil {
		var declared bool
		ev.Family, declared = o.Family(ua)
		if declared {
			ev.Declared = "unverified"
		}
	}
	return ev, Outcome{Kept: true}, nil
}

// Encode returns the event as one JSON line, without the newline.
//
// @param ev {Event} the event
// @returns {[]byte} the JSON object, beginning {"v":1,
// @returns {error} an error from encoding, which does not happen for an Event
// @example
//
//	b, _ := event.Encode(ev)
//	fmt.Println(string(b))
func Encode(ev Event) ([]byte, error) {
	return json.Marshal(ev)
}

// Decode reads one event from a JSON line. It is strict: an unknown field, a
// missing or different version, or text after the object is an error.
//
// @param b {[]byte} one JSON line
// @returns {Event} the event
// @returns {error} ErrInvalid, wrapping what was wrong
// @example
//
//	ev, err := event.Decode(line)
func Decode(b []byte) (Event, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var ev Event
	if err := dec.Decode(&ev); err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if dec.More() {
		return Event{}, fmt.Errorf("%w: text after the object", ErrInvalid)
	}
	if ev.V != Version {
		return Event{}, fmt.Errorf("%w: version %d, want %d", ErrInvalid, ev.V, Version)
	}
	return ev, nil
}

var methods = map[string]bool{"GET": true, "HEAD": true, "POST": true, "PUT": true, "DELETE": true, "OPTIONS": true, "PATCH": true}

// splitRequest returns the method and the path, without its query string, of a
// request line. A line that is not a request ("-", binary junk) gives "other"
// and an empty path.
func splitRequest(req string) (method, path string) {
	parts := strings.Fields(req)
	if len(parts) < 2 || !methods[parts[0]] {
		return "other", ""
	}
	path = parts[1]
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	return parts[0], path
}

// dash turns nginx's "-" for an empty value into "".
func dash(s string) string {
	if s == "-" {
		return ""
	}
	return s
}

// seconds reads one duration in seconds; a dash or junk is 0.
func seconds(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

// isNumber reports whether s is a number, which a request time is when the log recorded one.
func isNumber(s string) bool {
	_, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return err == nil
}

// sumSeconds adds the values nginx writes for an upstream that was retried:
// "0.1, 0.2 : 0.3" is 0.6. A dash counts as 0.
func sumSeconds(s string) float64 {
	var total float64
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ':' || r == ' ' }) {
		total += seconds(f)
	}
	return total
}

// cutUserAgent keeps at most maxUserAgent bytes, backing up to a character boundary.
func cutUserAgent(ua string) string {
	if len(ua) <= maxUserAgent {
		return ua
	}
	n := maxUserAgent
	for n > 0 && !utf8.RuneStart(ua[n]) {
		n--
	}
	return ua[:n]
}

// firstLanguage returns the first Accept-Language tag, lowercased, without its weight.
func firstLanguage(s string) string {
	s = dash(strings.TrimSpace(s))
	if i := strings.IndexAny(s, ",;"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// truncate cuts an address to its /24 (IPv4) or /48 (IPv6) network.
func truncate(a netip.Addr) string {
	bits := 48
	if a.Is4() {
		bits = 24
	}
	return netip.PrefixFrom(a, bits).Masked().String()
}

// keyed is a keyed hash of the address, cut to 16 hex characters: enough to
// count distinct clients inside one run, not enough to reverse.
func keyed(key []byte, a netip.Addr) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(a.String()))
	return hex.EncodeToString(m.Sum(nil)[:8])
}
