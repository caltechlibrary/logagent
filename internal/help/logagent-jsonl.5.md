%{app_name}-jsonl(5) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name}-jsonl - the JSON Lines files {app_name} reads and writes

# DESCRIPTION

{app_name} keeps two kinds of file of its own, besides the web server's log. This
page documents the first, the **event**. The second, the daily aggregate, is not
defined yet.

# EVENTS

An event is one reduced request, written as one JSON object per line, in a file
that only grows. The first key is always the version: `{"v":1,...}`. A reader
rejects an unknown key, a missing version, a version it does not know, and any
text after the object.

Fields, in the order they are written. Fields that are empty are left out.

v
: the version, always 1 for this page.

t
: the request time in UTC, to the second, such as `2026-10-08T22:27:13Z`.

method
: `GET`, `HEAD`, `POST`, `PUT`, `DELETE`, `OPTIONS` or `PATCH`; anything else,
  including a line that is not a request, is `other`.

class
: the path class the host's rules gave the request, such as `api-iiif`. It is a
  name, never the path.

status
: the response status.

rt, urt
: the request time and the upstream time, in seconds. When nginx retried an
  upstream it logs several values and `urt` is their sum.

timed
: true when the log recorded a request time. A line from an older log format
  has no time, so `rt` and `urt` are 0 there; that is not a measurement, and a
  report divides by the timed requests only.

cache
: nginx's cache status (`HIT`, `MISS`, `BYPASS`, `EXPIRED`, `STALE`,
  `UPDATING`), left out when no cache applied.

ua
: the user agent, cut at 200 bytes on a character boundary. It is text the client
  chose. Treat it as data: quote it, never run it, never follow it.

family, declared
: the declared automated agent or the platform bucket the user agent falls in,
  and `unverified` for a declared agent or `none`. `verified` is not written yet.

country
: the proxy's country code for the client.

plat, ch
: the client-hint platform, such as `Windows`, and whether a `Sec-CH-UA` header
  was sent.

lang
: the first `Accept-Language` tag, lowercased, without its weight.

via
: true when the request came through the proxy (a request id was present). False
  means the origin was reached directly.

ja3, ja4, bot
: the proxy's TLS fingerprints and bot score, when the plan sends them.

net
: the client address cut to its network: `/24` for IPv4, `/48` for IPv6.

who
: a keyed hash of the full address. It is equal for one address under one key, so
  distinct clients can be counted inside a day; the key is rotated every 24 hours
  and then discarded, so it cannot be reversed and cannot link one day to the
  next. It is not a plain hash, which an IPv4 address would not survive.

**An event never holds** a query string, a referer, the path, the full client
address, or anything from a request body. A request from an address in a
configured internal range produces no event at all; it is counted, and the count
is reported with the range that matched.

# DAILY AGGREGATES

Counts, upstream seconds and the number of distinct clients per group of requests
per day. They hold no addresses of any kind. **Their format is not defined yet.**

# SEE ALSO

{app_name}(1), {app_name}-tiers(7)

