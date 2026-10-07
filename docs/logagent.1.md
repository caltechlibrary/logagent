%logagent(1) user manual | version 0.0.4 104e001
% R. S. Doiel
% 2026-10-07

# NAME

logagent

# SYNOPSIS

logagent [OPTIONS]

# DESCRIPTION

logagent is being rewritten as the Library's tool for detecting and
mitigating automated traffic on nginx and Apache web servers. This build
provides only the standard options. The commands planned for it are
`report`, `watch`, `respond` and `analyze`; none of them exists yet.

# OPTIONS

-h, --help
: display help

-v, --version
: display version

-l, --license
: display license

# EXIT STATUS

0
: success

2
: usage error: an unknown option or command, a missing or surplus argument

# EXAMPLES

~~~
logagent --version
~~~

