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

Short options may be clustered, as in `-jh`. A short option that takes a value
ends the cluster. `--name=value` is accepted for options that take a value.

# FINDINGS

A finding has a severity: `gap` (data the tiers need is not logged, or the
configuration is wrong), `warn` (worth a look; no required data is lost) or
`note` (an optional improvement). Only gaps make the exit status 1.

field-missing
: a field is not in the log format. A required field is a gap, an optional field
  is a note.

field-empty
: the field is in the format, but a log sample found no value in any line. It
  needs counts from a sample of the log.

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
  address is trusted.

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
