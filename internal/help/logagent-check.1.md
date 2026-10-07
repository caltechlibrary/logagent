%{app_name}-check(1) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name}-check - validate that a web server's configuration logs the data detection needs

# SYNOPSIS

{app_name} check [OPTIONS]

# DESCRIPTION

**This command is planned and is not implemented yet.** This page is a
placeholder and will be completed as the design and implementation are settled.

{app_name} check is intended to read an nginx or Apache 2 configuration and say
whether the data the detection tiers need is being logged, such as the real
client address behind a proxy, request and upstream times, and the headers that
fingerprint a client. For each gap it is intended to suggest the configuration
change that closes it. It would read configuration and, optionally, count how
often fields are present in a log; it would change nothing and print no
addresses.

# SEE ALSO

{app_name}(1), {app_name}-tiers(7)

