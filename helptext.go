package logagent

// LogagentHelpText is the manual page for the logagent command, written in
// Markdown with {app_name}, {version}, {release_date} and {release_hash}
// markers that FmtHelp expands. `make man` renders it to docs/logagent.1.md
// and a man page.
//
// @example
//
//	fmt.Print(logagent.FmtHelp(logagent.LogagentHelpText, "logagent",
//		logagent.Version, logagent.ReleaseDate, logagent.ReleaseHash))
const LogagentHelpText = `%{app_name}(1) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name}

# SYNOPSIS

{app_name} [OPTIONS]

# DESCRIPTION

{app_name} is being rewritten as the Library's tool for detecting and
mitigating automated traffic on nginx and Apache web servers. This build
provides only the standard options. The commands planned for it are
` + "`report`" + `, ` + "`watch`" + `, ` + "`respond`" + ` and ` + "`analyze`" + `; none of them exists yet.

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
{app_name} --version
~~~

`
