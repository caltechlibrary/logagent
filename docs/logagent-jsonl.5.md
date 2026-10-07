%logagent-jsonl(5) user manual | version 0.0.4 104e001
% R. S. Doiel
% 2026-10-07

# NAME

logagent-jsonl - the JSON Lines files logagent reads and writes

# DESCRIPTION

**The schema is not defined yet.** This page is a placeholder. It will document
the JSON written by logagent, one object per line, when the design is settled.

What is decided is the shape of the data. logagent keeps two kinds of file of
its own, besides the web server's log:

reduced events
: append-only, one per request or per detected burst, holding the time, the
  status, the path class (not the path), upstream seconds, the user agent, the
  country, whether client hints were present, whether a declared bot was
  verified, the client address truncated to a network prefix, and a hash of the
  full address made with a key that is rotated daily and then discarded. They
  never hold a query string or a referer, and never an address from the
  configured internal ranges.

daily aggregates
: counts, upstream seconds and the number of distinct clients per group of
  requests per day. They hold no addresses of any kind.

Field names, types and versioning will be added here.

# SEE ALSO

logagent(1), logagent-tiers(7)

