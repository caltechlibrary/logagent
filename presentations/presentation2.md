---
title: "logagent 0.1.0, cost and not identity"
author: "R. S. Doiel, <rsdoiel@caltech.edu>"
institute: |
  Caltech Library,
  Digital Library Development
description: Caltech Library Digital Development Group presentation
urlcolor: blue
linkstyle: bold
aspectratio: 169
createDate: 2026-10-26
updateDate: 2026-10-26
pubDate: TBD
place: TBD
date: 2026-10-26
section-titles: false
toc: true
keywords: [ "logs", "bots", "privacy" ]
url: https://software.library.caltech.edu/logagent/presentations/presentation2.html
---

# What changed since the first talk

Presentation 1 described a tag-and-action scanner in TypeScript: find a string
in the log, ban the address.

logagent 0.1.0 is a rewrite in Go with a different approach.

# Why change?

- Identity is a weak signal: swarms rotate addresses, agent strings are claims
- Banning subnets can block readers
- Often the log did not hold the data needed to see the problem

# The approach: cost, not identity

- Libraries want harvesters to reach open collections
- What we cannot afford is load that takes the service down for readers
- Rank clients by what they cost the application, not by request count
- Judge behavior, not people

# Tiers, fast to slow

0. Capacity: fixed web server rules (concurrency limits, caching)
1. Detection: statistics over the log
2. Response: generated configuration, reviewed by a person, dry run by default
3. Analysis: long-term aggregates
4. Organizational: requests to others, and policy

Each tier works without the others.

# Built: `logagent check`

- Reads `nginx -T`
- Says what the access log format is missing, with a suggested change
- Checks the real client address behind a proxy
- Checks retention against logrotate
- Read-only

~~~shell
logagent check --config logagent.yaml --dump nginx-T.txt
~~~

# Built: `logagent report`

- Reads the log once, with rotated and gzipped files
- Ranks families of clients by upstream seconds
- Keeps only counts: no address, path, query string, referer or user agent
- Safe to paste into an issue

~~~shell
logagent report --config logagent.yaml
~~~

# Privacy

- Layer 1: the web server's own log, under host retention
- Layer 2: reduced events, no query string, referer or full path; weeks
- Layer 3: daily aggregates, no addresses; months or years
- The institution's own address ranges are never stored in detail or targeted

`logagent help privacy`

# Still planned

- `watch`: bursts and alerts
- `respond`: generated web server configuration, applied on confirmation
- `analyze`: review of aggregates
- Verifying declared crawlers

# fail2ban

Keep using it. It handles single-machine attacks well. logagent asks whether
the log can answer the question, and where the cost is.

# Reference

- GitHub Repository, <https://github.com/caltechlibrary/logagent>
- Website, <https://software.library.caltech.edu/logagent>
- Presentation 1 (the earlier TypeScript tool), `presentation1.md`
