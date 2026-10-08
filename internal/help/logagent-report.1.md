%{app_name}-report(1) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name}-report - summarize a web server's traffic and what it costs the application

# SYNOPSIS

{app_name} report [OPTIONS]

# DESCRIPTION

{app_name} report reads a web server's access log once and prints what the
requests show: how many there were, what they asked for, what they cost the
application, and who they claim to be. It ranks by cost (upstream seconds), not
by request count, because a request that takes thirty seconds is not the same as
one that takes thirty milliseconds.

It is read-only and stores nothing. It reads the log, its rotated files and its
gzipped files in time order, one line at a time, and keeps only counts. The
report holds no client address, path, query string, referer or user agent, so it
can be pasted into an issue. Requests from an address in `internal_ranges`
({app_name}-config(5)) are counted and appear nowhere else; the report says how
many each range matched, and says so when none are configured. A family with
fewer than 10 distinct clients that does not name itself is folded into one row.

The log's format is found in the web server's configuration, the text printed by
`nginx -T`, from the first of these: the file named with `--dump`, the
`config.dump` file in the host configuration, or the output of `config.command`.
Which log is read comes from `--log` or, without it, the first log in the host
configuration.

## Sections

1. SOURCES: the time covered, the files read, the lines read, skipped and outside
   the window, the internal ranges, and notes about what is not configured.
2. REQUESTS PER DAY (UTC), and per hour for the last 48 hours, for all traffic
   and, with `--family`, for one family and its distinct clients.
3. STATUS: the response statuses, and 429 (our caps), 499 (the client gave up),
   502 and 504 (the upstream failing) per day.
4. BY PATH CLASS: requests, upstream seconds and seconds per request for each
   class in the host's `classes` rules.
5. BY FAMILY: requests, the share on API classes (a class whose name begins
   `api-`), upstream seconds, seconds per request and the share limited with
   429, for each family of client: a declared automated agent, or undeclared
   traffic by claimed platform.

Seconds per request count only requests the log gave a request time. A log that
was written with another format earlier in the window has requests with no time;
the report says how many.

# OPTIONS

-h, --help
: print this page

-j, --json
: print the report as JSON with stable keys; errors go to standard error as a JSON
  object with the exit class and code

-c, --config PATH
: the host configuration file, found otherwise from `$LOGAGENT_CONFIG` and then
  `/etc/logagent/logagent.yaml`

-d, --dump FILE
: read the web server's configuration from this saved `nginx -T` output

-l, --log PATH
: the access log to read, which must be one the web server's configuration names

--day DATE
: report on one complete UTC day, such as `2026-10-07`. This is the default, for
  yesterday.

--last SPAN
: report on the last SPAN up to now, such as `24h` or `7d`

--since TIME, --until TIME
: report from TIME up to, not including, TIME. Each is a date or an RFC 3339 time
  such as `2026-10-07T12:00:00Z`. `--since` alone reads to the end of the log. Only
  one of `--day`, `--last`, or `--since` with `--until` may be given.

-f, --family TEXT
: count, beside all traffic in section 2, the requests whose family name contains
  TEXT (ignoring case), with their distinct clients

-t, --top ROWS
: how many rows of the ranked tables to print; the rest are counted as "more". 0
  prints all. The default is 15. JSON always has every row.

Short options may be joined: `-jc PATH`.

# EXAMPLES

Yesterday's traffic, from the host's own configuration:

    logagent report

One bot over the last week, as JSON:

    logagent report --last 7d --family exa --json

# EXIT STATUS

0
: the report was printed

1
: the log's format is one the command cannot read yet (`escape=json`), or the
  server is not one it can report on yet

2
: the command line is wrong: an unknown option, a missing or bad value, a surplus
  argument, or two ways of choosing the window

65
: the log is mostly not in the format the web server's configuration names for
  it, a line is longer than 4 MB, or a gzipped log is damaged

66
: no host configuration file was found, or the access log, the dump or the command
  named does not exist

70
: an error nothing classified, which is a bug

74
: reading a file failed part way

77
: the operating system refused to read a file or run the command

78
: the host configuration file exists and is wrong, or the web server's
  configuration does not name the log, or its log format has no `$time_local`

# SEE ALSO

{app_name}(1), {app_name}-check(1), {app_name}-config(5), {app_name}-jsonl(5), {app_name}-privacy(7)
