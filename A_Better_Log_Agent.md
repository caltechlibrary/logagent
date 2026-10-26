---
title: A Better Log Agent
---

# A Better Log Agent

A library wants its collections open to everyone, including harvesters. What it
cannot afford is traffic that takes the service down for the readers. Defense
therefore starts from cost and not from identity: a request that keeps the
application busy for thirty seconds matters more than a hundred that cost
nothing, whoever sent them. And because a library protects the privacy of its
readers, the defense must work without building a record of who read what.

This note is the rationale for `logagent` as rewritten in Go (version 0.1.0). It
replaces an earlier note that proposed a log scanner which banned addresses and
subnets with a firewall. That approach, and why it was set aside, is under
*What changed*.

## The log is sensor data

The web server's access log is the best sensor the service has, but only if it
records the right things: the time the request took, the time the application
took, whether a cache answered, the real client address behind a proxy, and the
client's declared agent. A log that omits them cannot be analyzed afterward.

So the first tool is `logagent check`. It reads the web server's configuration
(the text `nginx -T` prints), finds each access log's format and says what is
missing, with a suggested change and the file and line it belongs near. It also
checks that retention under logrotate matches the privacy policy. It is
read-only.

## Report by cost

`logagent report` reads an access log once, with its rotated and gzipped files,
and prints what the requests show, ranked by upstream seconds and not by request
count. It groups clients into families (a declared crawler, a browser platform,
a group that shares a fingerprint) and not by address. It keeps only counts: the
report holds no address, path, query string, referer or user agent, so it can be
pasted into an issue.

## Tiers of defense, from fast to slow

Each tier works without the ones outside it, and none overrides a lower one.

0. **Capacity.** Fixed rules in the web server, such as concurrency limits and
   caching. They react in milliseconds and need no daemon.
1. **Detection.** Statistics over the log: bursts, baselines, cohorts.
2. **Response.** Rules generate web server configuration. A person approves it;
   it is a dry run by default and is applied only after the server's own
   configuration test passes.
3. **Analysis.** Review of long-term aggregates, with a person judging.
4. **Organizational.** Requests to other parties, and policy.

Built so far: `check` and `report`. Planned: `watch` (bursts and alerts),
`respond` and `analyze`.

## Privacy

Three layers of data: the web server's own log, under the host's retention;
reduced events with no query string, referer or full path, kept for weeks; and
daily aggregates with no addresses, kept for months or years. Addresses that
belong to the institution are never stored in detail and never targeted. See
`logagent help privacy`.

## What changed

The first design (2025) read a log, matched sub strings and ran a firewall command for each match. That was simple and it was
useful against single machines. It was set aside because:

- identity is a poor signal. Swarms rotate addresses, and a declared agent
  string is a claim and not a fact (a large share of the requests that claimed
  to be a well-known search crawler could not be verified);
- a ban by subnet can block readers, and a tool that edits the firewall on its
  own is a hazard, so response became generated, reviewed configuration;
- the data detection needs was often not in the log at all, so checking the
  configuration came first.

## Questions

Q: Why not use fail2ban?

Use it. It is good against old-fashioned attacks by single machines.
`logagent` does not replace it. It looks at cost across cohorts and at whether
the log can answer the question at all.

Q: Does it depend on Cloudflare or on challenge pages?

No. It works at the first place the Library controls, on the Library's own
servers, and challenge pages burden readers.

## Ideas, someday, maybe

Aggregate across machines, since the same swarm often hits more than one host.
