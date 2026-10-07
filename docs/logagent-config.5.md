%logagent-config(5) user manual | version 0.0.4 104e001
% R. S. Doiel
% 2026-10-07

# NAME

logagent-config - the logagent per-host configuration file

# DESCRIPTION

logagent reads one YAML configuration file for the host it runs on. The file
holds that host's addresses and policy, so a real configuration is kept on the
host and is not committed to a repository. An example with every key is in the
`examples/` directory of the source and of each release.

# FINDING THE FILE

The file is the first of these that exists:

1. the path given with `--config PATH`
2. the path in the environment variable `LOGAGENT_CONFIG`
3. `/etc/logagent/logagent.yaml`

A path named with `--config` or `LOGAGENT_CONFIG` must exist; logagent does not
fall back to the next place if it does not, so a typo is not hidden. If no file
is found the error lists every place that was looked at and the exit status is 66.
A file that exists and is wrong exits with status 78.

# FORMAT

The file is YAML. It is decoded strictly: an unknown key is an error, so a
misspelled key is caught and never silently ignored. Put address ranges in
quotes, because YAML would otherwise read some values as something other than
text. Every address range must be a CIDR range such as `"10.0.0.0/8"` with no
host bits set (`"131.215.0.1/16"` is rejected with a suggestion).

# KEYS

version
: required. The schema version. It must be `1`.

host
: a label for the host, shown in reports.

server
: required. `nginx` or `apache`. (Apache is accepted here, but `logagent check`
  reports it as unsupported until Apache support exists.)

config
: where the web server's configuration comes from. `dump` is the path of a saved
  `nginx -T`; `command` is a command to run. If neither is given for an nginx
  host the command is `nginx -T`.

logs
: required. A list of access logs, each with a `path`.

proxy
: what sits in front of the server. `behind` is `none` (the default) or
  `cloudflare`. `real_ip_header` names the header that carries the client
  address and defaults to `CF-Connecting-IP` when `behind` is `cloudflare`.
  `trusted_ranges` is an optional list of CIDR ranges that overrides the
  program's snapshot of the proxy's published ranges.

internal_ranges
: a list of CIDR ranges that belong to the institution: a campus network, remote
  sites, the Library's own addresses in a cloud service. Traffic from them is
  counted, never stored in detail and never targeted by a response. There is no
  built-in default. List a cloud provider's own addresses, never the provider's
  whole published range, which anyone renting a server there shares.

retention
: the retention policy, in whole days. `logrotate` is the path of the logrotate
  file that governs layer 1, the web server's own log. `layer1_min_days`
  (default 14) is the least the tools need and `layer1_max_days` (default 90) is
  the most policy allows. `layer2_days` (default 28) covers reduced events and
  `layer3_days` (default 730) covers daily aggregates. The minimum may not be
  above the maximum.

fields
: per-field overrides of the field table, a map from a field name to `required`,
  `optional` or `not_applicable`. The names are listed in logagent-fields(5).

# EXAMPLE

~~~yaml
version: 1
host: example-host
server: nginx
config:
  dump: /var/lib/logagent/nginx-T.txt
logs:
  - path: /var/log/nginx/access.log
proxy:
  behind: cloudflare
internal_ranges:
  - "131.215.0.0/16"
retention:
  layer2_days: 28
fields:
  bot_score: optional
~~~

# EXIT STATUS

66
: no configuration file was found

78
: the configuration file exists and is wrong

# SEE ALSO

logagent(1), logagent-check(1), logagent-fields(5), logagent-jsonl(5)

