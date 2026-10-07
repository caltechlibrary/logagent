%logagent-check(1) user manual | version 0.0.4 104e001
% R. S. Doiel
% 2026-10-07

# NAME

logagent-check - validate that a web server's configuration logs the data detection needs

# SYNOPSIS

logagent check [OPTIONS]

# DESCRIPTION

**This command is planned and is not implemented yet.** This page is a
placeholder and will be completed as the design and implementation are settled.

logagent check is intended to read an nginx or Apache 2 configuration and say
whether the data the detection tiers need is being logged, such as the real
client address behind a proxy, request and upstream times, and the headers that
fingerprint a client. For each gap it is intended to suggest the configuration
change that closes it. It would read configuration and, optionally, count how
often fields are present in a log; it would change nothing and print no
addresses.

# SEE ALSO

logagent(1), logagent-tiers(7)

