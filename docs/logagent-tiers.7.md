%logagent-tiers(7) user manual | version 0.1.0 ef857f3
% R. S. Doiel
% 2026-10-26

# NAME

logagent-tiers - how defense against automated traffic is arranged

# DESCRIPTION

Defense is arranged in tiers by time scale. Each tier works without the ones
outside it, so the project can stop at any tier and still have delivered
something. No tier overrides a lower-numbered one: higher tiers propose, and a
change reaches tiers 0 to 2 only through tier 2's generate, approve and apply
path.

0 Capacity
: milliseconds. Fixed rules on the web server, always on, such as concurrency
  limits and caching. They need no daemon.

1 Detection
: seconds to minutes. Deterministic statistics over the access log: bursts,
  baselines, and groups of requests that share a fingerprint, ranked by the load
  they put on the application.

2 Response
: minutes to hours. Rules generate the configuration files for tier 0. A person
  approves them. It is a dry run by default, applied only on confirmation and
  only after the server's own configuration test passes.

3 Analysis
: days to months. Review of aggregated history for campaigns and seasonal
  change. A model may draft and a person judges.

4 Organizational
: weeks and up. Requests to other parties, and policy. These are decisions for
  people and not software.

This release implements part of tier 1: `check` and `report` read the
configuration and the access log and make no change. The rest of tier 1 and
tiers 2 and 3 are planned; see logagent(1) for the commands.

# SEE ALSO

logagent(1), logagent-jsonl(5), logagent-privacy(7)

