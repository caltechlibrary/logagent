%logagent(1) user manual | version 0.0.4 104e001
% R. S. Doiel
% 2026-10-07

# NAME

logagent

# SYNOPSIS

logagent [OPTIONS]

logagent help [TOPIC]

logagent COMMAND [OPTIONS]

# DESCRIPTION

logagent is being rewritten as the Library's tool for detecting and
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

Each command has its own manual page, named logagent-COMMAND(1), for example
logagent-check(1). Show one with `logagent help check`.

# TOPICS

logagent-config(5)
: the per-host configuration file

logagent-fields(5)
: the log fields the tiers need and how each server logs them

logagent-jsonl(5)
: the JSON Lines files logagent reads and writes (planned)

logagent-privacy(7)
: what is collected, kept and never kept

logagent-tiers(7)
: how defense is arranged in tiers

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
logagent --version
logagent help check
logagent help --list
~~~

