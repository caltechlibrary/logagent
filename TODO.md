# TODO

## Still open after the `new-data` check

- Run `check` on a real host that has an `access_log` its `logs` list omits.
  The `access-log-not-configured` rule has only run on synthetic tests; the
  `new-data` run listed the one access log, so the rule never fired there.
- Re-run `check` on `new-data` with the full `nginx -T`, not the SSM copy below
  (`redirect-map.conf` and `modules-enabled` were stubbed). It cannot change the
  three `server_name` findings, but it would remove the caveat.

## Done: `check` against the CaltechDATA production host (2026-10-23)

`new-data` (data.caltech.edu) had `server_name data.caltech.edu` lines with no
terminating semicolon. The `server-name-swallows-directive` finding was written
from that defect and had run only on synthetic tests.

Result: it reports exactly the three known lines (5, 110 and 129 of
`/etc/nginx/sites-enabled/default`) and no other gap, warning or note. The
`caltechdata_bots` log format has 19 of 19 fields and the Cloudflare real-IP
file is in place.

How, and why no clone was needed: `check` reads only the text of `nginx -T`. The
config files were read on production over AWS SSM (`AWS-RunShellScript`, `cat`
only; `nginx -t` and `-T` were not run, because `nginx -t` creates cache
directories) and assembled into a dump in `nginx -T` format. A disk clone was
planned and dropped.

The same defect is in the caltechdata repository's `nginx-sites-enabled-default`
(lines 3 and 96, `newdata.caltechlibrary.dev`) on `main`, `granian` and
`rsdoiel`. `check` reports both there too, and none once the semicolons are
added. The production file differs from the repository's, so the repository fix
does not fix `new-data`. The semicolon fix on production is for the author to
raise with Tom.
