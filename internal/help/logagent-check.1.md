%{app_name}-check(1) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name}-check - validate that a web server's configuration logs the data detection needs

# SYNOPSIS

{app_name} check [OPTIONS]

# DESCRIPTION

{app_name} check reads a web server's configuration and says whether the data the
detection tiers need is being logged: the fields of each access log's format
({app_name}-fields(5)), whether nginx will accept that format where it is defined,
whether any `access_log off` hides traffic, and, behind a proxy, whether the real
client address is recovered. For each gap it prints a suggested change as text,
with the file and line it belongs near.

It is read-only. It changes no file, applies nothing and prints no client
address, query string or user agent, so its output can be pasted into an issue.

The configuration it reads is the text printed by `nginx -T`, which holds every
included file with a `# configuration file PATH:` line before each. It comes from
the first of these:

1. the file named with `--dump`
2. the `config.dump` file named in the host configuration ({app_name}-config(5))
3. the output of `config.command`, which is `nginx -T` unless the host
   configuration says otherwise. The command is split on spaces and is not run
   by a shell, so `sudo nginx -T` works and a pipeline does not.

Only nginx is supported. An Apache configuration is reported as unsupported, and
the exit status is 1, until Apache has been surveyed and checked.

# READING THE LOG

A format can name a header that the proxy never sends: the field is in the log
and every line has a dash. The configuration cannot show that, so {app_name} check
also reads the end of each access log, using the log format it found, and counts
how many lines held a value for each field. It keeps only the counts. No address,
query string or user agent is stored or printed, so the report can be pasted into
an issue. Lines that do not match the format are counted as skipped.

# OPTIONS

-h, --help
: show this page

-j, --json
: print the report as JSON, with an `exit_status`, and print a failure as a JSON
  error object on standard error, with its class name beside the number

-c, --config PATH
: the host configuration file. Without it the file is `$LOGAGENT_CONFIG`, then
  `/etc/logagent/logagent.yaml`

-d, --dump FILE
: read the web server's configuration from this saved `nginx -T` text, ignoring
  `config.dump` and `config.command`

-s, --sample LINES
: count fields in the last LINES lines of each access log (default 10000). `0`
  turns the counts off. A log that is not on this machine, as when a dump was
  copied from a host, is skipped without comment

-r, --refresh-ranges
: behind a proxy, fetch the proxy's published list of ranges and compare the host
  with that, instead of the list built into {app_name}. Nothing is written; the
  live list is used for this run only. If it cannot be fetched a note says so and
  the built-in list is used. Without this option {app_name} never touches the
  network

Short options may be clustered, as in `-jh`. A short option that takes a value
ends the cluster. `--name=value` is accepted for options that take a value.

# FINDINGS

A finding has a severity: `gap` (data the tiers need is not logged, or the
configuration is wrong), `warn` (worth a look; no required data is lost) or
`note` (an optional improvement). Only gaps make the exit status 1.

When the same problem is found in several server blocks, such as no real-IP
setting in three of them, it is reported once. The message names every server and
the finding lists the other places in `also`.

field-missing
: a field is not in the log format. A required field is a gap, an optional field
  is a note.

field-empty
: the field is in the format, but no sampled line held a value for it. Look at
  what sends the header (the proxy, for the Cloudflare fields), not at this
  server. Fields that nothing but the stock format uses, such as the user name and
  the referer, are never reported this way.

sample-unavailable
: the log could not be read or its format could not be read (a note).

sample-mismatch
: most sampled lines do not match the log format, so the counts are not used. The
  log may have been written with another format, or be the wrong file (a
  warning).

format-defined-after-use
: the `log_format` comes after the `access_log` that names it, which `nginx -t`
  rejects. This is what happens when a format is put in a file that is included
  later than the line using it.

format-undefined
: an `access_log` names a format that is not defined.

access-log-off
: a server (gap) or a location (warn) does not write an access log.

access-log-default
: a server sets no `access_log`, so nginx uses its compiled-in default.

log-not-found
: a log listed in the host configuration is written by no `access_log`.

real-ip-missing, real-ip-header-mismatch, real-ip-no-trusted-proxies, real-ip-trust-too-wide
: behind a proxy, the visitor's address is not recovered, is read from the wrong
  header, is ignored because no proxy is trusted, or can be forged because every
  address is trusted. The suggestion lists the `set_real_ip_from` lines to use.

real-ip-ranges-missing
: the trust list does not cover every range the proxy publishes (a warning).
  Requests that arrive through a missing range are logged with the proxy's
  address. The ranges compared with are `proxy.trusted_ranges` if the host
  configuration lists them, otherwise the list built into {app_name}.

real-ip-trusts-other-ranges
: the trust list holds a range that is not inside the proxy's, such as an
  internal load balancer or a range wider than the proxy's own (a note). Any
  address in it can set the address that is logged.

ranges-snapshot-old
: the built-in list of the proxy's ranges is older than
  `proxy.ranges_max_age_days` days, 90 by default (a warning). Use
  `--refresh-ranges`, or list the ranges in `proxy.trusted_ranges`.

ranges-refresh-failed
: `--refresh-ranges` could not fetch the live list (a note).

# EXAMPLES

Check a dump copied from a host:

    {app_name} check --config logagent.yaml --dump nginx-T.txt

Check this host and keep the report as data:

    sudo {app_name} check --json > check.json

# EXIT STATUS

0
: no gaps were found

1
: the check ran and found gaps, or the server is not one it can check yet

2
: the command line is wrong: an unknown option, a missing value, a surplus
  argument

65
: the web server's configuration could not be read as nginx configuration: a
  syntax error, an include that is not in the dump, no server block, or
  `nginx -T` exiting with an error

66
: no host configuration file was found, or the dump file or the command named
  does not exist

70
: an error nothing classified, which is a bug

74
: reading the dump failed part way, for example the path is a directory

77
: the operating system refused to read a file or run the command

78
: the host configuration file exists and is wrong

# SEE ALSO

{app_name}(1), {app_name}-config(5), {app_name}-fields(5), {app_name}-tiers(7)
