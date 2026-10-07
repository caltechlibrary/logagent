# logagent

logagent helps Caltech Library detect and mitigate automated traffic against
its public web services. It works from the Library's own servers, reading
nginx and Apache 2 logs and configuration, so it does not depend on any one
vendor's edge or on challenge pages that burden readers. It is being rewritten
in Go and is a work in progress.

## Status

**Work in progress.** The Go rewrite has a module skeleton and nothing more:
`logagent --help`, `--version` and `--license`. The earlier Deno/TypeScript
proof of concept (a tag-and-action log scanner in the spirit of fail2ban, and a
log aggregator) is retired and remains in the repository's history.

## Approach

Defense is arranged in tiers by time scale. Each tier works without the ones
outside it, and no tier overrides a lower one.

0. **Capacity.** Fixed rules on the web server itself, such as concurrency
   limits and caching. They need no daemon.
1. **Detection.** Deterministic statistics over the access log: bursts,
   baselines, and groups of requests ("cohorts") that share a fingerprint, ranked
   by the load they put on the application rather than by request count.
2. **Response.** Rules generate configuration for the web server. A person
   approves it, it is a dry run by default, and it is applied only on
   confirmation and only after the server's own configuration test passes.
3. **Analysis.** Long-horizon review of aggregates, such as campaigns and
   seasonal change, with a person judging the result.
4. **Organizational.** Requests to other parties, and policy, which are
   people's decisions and not software's.

The planned command is a single `logagent` with subcommands:

`check`
: read a web server's configuration and say whether the data detection needs is
  being logged, suggesting the change when it is not

`report`
: summarize automated traffic and its cost, by family of client

`watch`
: follow a log and raise alerts on bursts

`respond`
: generate, and with confirmation apply, web server configuration

`analyze`
: review aggregated history over weeks and months

None of these subcommands exists yet.

## Privacy

Libraries protect the privacy of their readers, and logagent is built to keep
that stance while keeping services available (`logagent help privacy`).

- It judges **behavior** (rate, path pattern, cost), not identity, and uses what
  it collects only to keep services available and secure.
- It keeps three layers of data: the web server's own log, under the host's
  retention policy; reduced events, with the address truncated and no query
  string, referer or full path, kept for weeks; and daily aggregates with no
  addresses, kept for months or years. Retention for each layer is
  configurable.
- It never stores search terms or referers. Address ranges that belong to the
  institution itself (a campus network, remote sites, the Library's own cloud
  addresses) are configurable and are never stored in detail or targeted.

## Building

Required: Go (current stable) and Pandoc (for the man pages). The `make` targets
also use the `git`, `grep`, `cut`, `sed` and `zip` that the operating system
supplies. `release.bash` additionally needs `gh` and `jq`, and `release.ps1`
needs `gh`.

~~~shell
make              # build bin/logagent, docs/ and man/
make test         # go vet and go test
make install      # install in $HOME/bin and $HOME/man (prefix=... to change)
make release      # build the zip files in dist/, then run release.bash
~~~

It also builds with plain Go commands:

~~~shell
go build ./cmd/...
go install github.com/caltechlibrary/logagent/cmd/logagent@latest
~~~

## Documentation

- [Manual page](docs/logagent.1.md)
- [LICENSE](LICENSE)
- [Cite with CITATION.cff](CITATION.cff)
