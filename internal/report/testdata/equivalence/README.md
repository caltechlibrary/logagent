# Equivalence fixtures

`access.log` is a synthetic log of 900 requests in the 19-field format, written once
by a seeded generator (seed 20261008): every declared family, every platform bucket,
every path class of the CaltechAUTHORS example, 429s, 504s, retried upstream times
(`"0.1, 0.2"`) and a few requests with no request time. No production content.

`family_breakdown.expected` is what `bot-family-breakdown.bash` printed for that log
(caltechauthors repository, branch `rsdoiel`, commit `a020068`):

    LOG_DIR=$PWD/internal/report/testdata/equivalence bash bot-family-breakdown.bash

`equivalence_test.go` checks that `logagent report` agrees with it, figure for figure.
Both files are frozen. Never refresh them from a host; if the report should change, the
test says what the script disagrees with.

## Concurrency

`../concurrency/access.log` is a second synthetic log (700 requests in about four
minutes, durations of up to 18 seconds, API, IIIF and 429s) and
`../concurrency/concurrency.expected` is what `bot-concurrency.py` printed for it:

    LOG_DIR=$PWD/internal/report/testdata/concurrency python3 -I bot-concurrency.py

The test compares the span, the request and 429 counts, the percentiles and the
share of seconds at each level, for all `/api` and for `/api/iiif/`. The script's
campus group is not compared: internal-range requests are counted, not stored, so
the report has no concurrency for them.
