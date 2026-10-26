# Change log

All notable changes to logagent, newest first.

## v0.1.0 (2026-10-26)

A clean-slate rewrite in Go. The Deno/TypeScript tool of v0.0.4 (a tag-and-action
log scanner, a log aggregator and a set of `ufw` ban shell helpers) is retired
and remains in the repository's history. The approach changed from banning
addresses to judging cost and behavior, and from acting on its own to checking
and reporting first (workspace decision records DR-0001 to DR-0009).

### Added

- `logagent check`: reads the text of `nginx -T` and says whether the access log
  records what detection needs. It checks each log format's fields, include
  order, `access_log off`, the real client address behind a proxy (with a dated
  snapshot of the Cloudflare ranges and `--refresh-ranges`), a cache that
  bypasses on cookies or does not log its status, a `server_name` missing its
  semicolon, `error_log` configuration, and log retention against logrotate. It
  counts how often each field holds a value in the end of the log, keeping only
  counts. It recognises structured (`escape=json`) formats and checks them, but
  does not read them. Text or `--json` output, with a suggested change and the
  file and line for each gap.
- `logagent report`: reads an access log, with its rotated and gzipped files,
  once and in time order, and prints ten sections: sources, requests per day and
  hour, status, path classes, families of client, undeclared cohorts,
  concurrency, cache, the 429s, and self-described agents not in the declared
  list. Ranked by upstream seconds, not request count. It keeps only counts and
  holds no address, path, query string, referer or user agent, so a report can
  be pasted into an issue. Text or `--json`.
- A terminal menu, run with no arguments at a terminal, with a screen for
  `check`.
- A strict per-host YAML configuration file, found by option, environment
  variable or `/etc` (logagent-config(5)), with an example in `examples/`.
- Manual pages for the command, each subcommand and each topic (logagent-config,
  logagent-fields, logagent-jsonl, logagent-privacy, logagent-tiers), shown with
  `logagent help` and installed as man pages.
- The event record (logagent-jsonl(5)): one reduced request with no query string,
  referer or full path, and a daily-keyed hash in place of the address. Defined
  and used in memory by `report`; no command writes it to disk yet.
- The privacy stance as code and as a manual page: three layers of data,
  configurable retention, institutional address ranges that are counted and never
  stored in detail, and groups of fewer than 10 clients folded together.
- Exit codes follow the workspace convention (0, 1, 2, 65, 66, 70, 74, 78).
- Release scripts for six platforms (Linux, macOS, Windows; amd64 and arm64).

### Changed

- The language is Go, built with the standard library plus `gopkg.in/yaml.v3`
  and the Charm terminal libraries.
- Only nginx is supported. Apache 2 was dropped from the supported set: `check`
  reports an Apache configuration as unsupported and exits 1, and the Apache
  columns of the field table are kept for a later release.
- Firewall bans are no longer part of the approach. The `ufw`-based helper
  scripts and the Deno prompts and search page were removed.
- `A_Better_Log_Agent.md` is rewritten for the new approach. `presentation2.md`
  is added; `presentation1.md` is kept as the record of the earlier tool.

### Not built

`watch` (bursts and alerts), `respond` (generated server configuration, applied
on confirmation) and `analyze` (review of daily aggregates) have manual pages
that say they are planned. So are reading `escape=json` logs, verifying declared
crawlers, and Apache support.

## v0.0.4 (2025-03-26)

The last release of the Deno/TypeScript proof of concept: LogAgent, which finds
tags in a log and runs an action for the address, and LogAnalyst, which turns
nginx log lines into records and prints aggregate counts.
