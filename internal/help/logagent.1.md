%{app_name}(1) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name}

# SYNOPSIS

{app_name} [OPTIONS]

{app_name} help [TOPIC]

{app_name} COMMAND [OPTIONS]

# DESCRIPTION

{app_name} is being rewritten as the Library's tool for detecting and
mitigating automated traffic on nginx and Apache 2 web servers. This build
provides the standard options and the help system. The commands are planned
and none of them is implemented yet.

# OPTIONS

-h, --help
: display help

-v, --version
: display version

-l, --license
: display license

# COMMANDS

help [TOPIC]
: show a help page. With no TOPIC show this one. `help --list` lists every page.

check
: (planned) validate that a web server's configuration logs the data detection needs

report
: (planned) summarize automated traffic and its cost

watch
: (planned) follow a log and raise alerts on bursts

respond
: (planned) generate, and with confirmation apply, web server configuration

analyze
: (planned) review aggregated history over weeks and months

Each command has its own manual page, named {app_name}-COMMAND(1), for example
{app_name}-check(1). Show one with `{app_name} help check`.

# TOPICS

{app_name}-config(5)
: the per-host configuration file

{app_name}-fields(5)
: the log fields the tiers need and how each server logs them

{app_name}-jsonl(5)
: the JSON Lines files {app_name} reads and writes (planned)

{app_name}-privacy(7)
: what is collected, kept and never kept

{app_name}-tiers(7)
: how defense is arranged in tiers

# INTERACTIVE USE

Run with no arguments at a terminal, {app_name} opens a menu of its commands.
`check` runs the check and shows the report; the others show a note that they
are not built yet and where to read about them. Arrow keys or `j` and `k` move,
Enter chooses, `q` or Esc goes back or quits. Colour is only a highlight on the
cursor row, which is also marked with `>`, and is off when `NO_COLOR` is set.
With no terminal, no arguments is a usage error.

# EXIT STATUS

0
: success

2
: usage error: an unknown option, command or help topic, a missing or surplus
  argument, or a command that is not implemented yet

66
: a configuration file was not found

78
: the configuration file exists and is wrong

# EXAMPLES

~~~
{app_name} --version
{app_name} help check
{app_name} help --list
~~~

