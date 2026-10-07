%{app_name}-privacy(7) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name}-privacy - what {app_name} collects, keeps and never keeps

# DESCRIPTION

Libraries protect the privacy of their readers, and a web server's log records
who asked for what. Libraries must also keep their resources available, and
automated traffic can take a service down for everyone. {app_name} is built to
do the second without giving up the first.

This page describes the stance the design commits to. **Most of it is not
implemented yet**: this build reads a configuration file and shows help, and
does not yet read logs or write any data. Treat what follows as the rules the
code must meet as it is written.

# PURPOSE

{app_name} uses what it collects to keep services available and secure, and for
nothing else. It does not build a reading history for a person, does not link a
person's activity across days beyond a coarse network prefix, and is not used to
evaluate individuals.

It judges **behavior** (request rate, path pattern, load on the application),
not identity. Whether a client is one of many readers or part of an automated
wave is decided from what it does.

# THREE LAYERS OF DATA

Layer 1, the web server's own log
: Full detail, written by the web server and kept under the host's own log
  retention (at most `retention.layer1_max_days`, 90 by default). {app_name}
  reads it and copies nothing from it. It needs at least `retention.layer1_min_days`
  (14 by default) of it to work.

Layer 2, reduced events
: Written by {app_name}, kept for `retention.layer2_days` (28 by default). An
  event holds the time, the status, the path class (not the path), the upstream
  seconds, the user agent, the country, whether client hints were present,
  whether a declared bot was verified, the client address truncated to a network
  prefix (/24 for IPv4, /48 for IPv6), and a hash of the full address made with a
  key that is rotated every 24 hours and then discarded. The hash lets distinct
  clients be counted within one day and cannot be reversed or linked across days.

Layer 3, daily aggregates
: Counts, upstream seconds and the number of distinct clients per group of
  requests per day. No addresses of any kind. Kept for `retention.layer3_days`
  (730 by default). The analysis commands read only this layer.

# NEVER KEPT

Layers 2 and 3 never contain:

- a query string (a search address such as `/search?q=...` reveals what someone
  was researching),
- a referer,
- a full path (only its class),
- a full client address,
- an address inside the configured `internal_ranges`.

# INTERNAL RANGES

Addresses that belong to the institution are people (campus users) or the
Library's own systems. Traffic from `internal_ranges` is counted in layer 3 and
written nowhere else, and is never the target of a response. The list is
configuration, with no built-in default: a campus network, remote sites, and the
Library's own addresses in a cloud service, but never a cloud provider's whole
published range.

# SMALL GROUPS

A group of requests with fewer than 10 distinct clients in a day is folded into
an `other` group, so a rare combination of user agent and country cannot point at
one reader. The threshold is configuration. Aggregates should be checked against
it before they are shared outside the Library.

# WHAT THE CHECK READS

`{app_name} check` reads configuration and may sample a log to count how often
each field is present. It prints counts only: no address, query string, referer
or user agent. Its output can be pasted into an issue without a privacy review.

# SEE ALSO

{app_name}(1), {app_name}-config(5), {app_name}-jsonl(5), {app_name}-tiers(7)

