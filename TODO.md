# TODO

## Test `check` against a clone of the CaltechDATA production host

`new-data` (data.caltech.edu) has `server_name data.caltech.edu` lines with no
terminating semicolon (lines 5, 110 and 129 of `/etc/nginx/sites-available/default`
as of 2026-10-09). The `server-name-swallows-directive` finding was written from
that defect and is covered by synthetic tests only; it has not been run on the
real configuration.

Plan, for its own session:

1. Clone `new-data` with `clasm` to a throwaway test instance (a test instance
   never writes to a backup bucket, and is stopped or deleted afterward).
2. On the clone, take `nginx -T` and run `logagent check --config HOST.yaml --dump DUMP`
   (a build of `logagent` for the clone's architecture).
3. Confirm the three lines are reported, and that nothing else on the host is newly
   a gap. Note the `access-log-not-configured` and `field-missing` results too: this
   is also the first real run of the unlisted-log rule on that host.
4. Do not change the production host; the semicolon fix is for the author to raise
   with Tom.
