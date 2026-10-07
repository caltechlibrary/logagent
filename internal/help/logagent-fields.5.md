%{app_name}-fields(5) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name}-fields - the log fields {app_name} needs, and how each web server logs them

# DESCRIPTION

The tiers read the web server's access log, and a log only helps if it holds the
fields the tiers need. The fields are one table, built into {app_name}. The same
table drives {app_name} check, which says which fields a host's log format
lacks, and the `log_format` and `LogFormat` text {app_name} suggests, so the
advice and the check cannot disagree.

Each field has a name, a purpose, the tiers that need it, a default level, the
hosts it applies to, the nginx variable, the Apache format directive and what to
do when it is missing. The names are the labels used in the extended log format
already running on the CaltechAUTHORS production server.

# LEVELS

A field is `required`, `optional` or `not_applicable` for a host. The table gives
each field a default of required or optional. A host changes one with the
`fields` key of its configuration file ({app_name}-config(5)):

    fields:
      bot_score: optional
      ja3: not_applicable

A missing `required` field is a finding. A missing `optional` field is a note.
A `not_applicable` field is dropped. A name that is not in the table is an error
in the configuration file, so a misspelling is caught.

# WHERE A FIELD APPLIES

always
: every host.

behind_proxy
: only a host with a proxy in front of it (`proxy.behind` is not `none`).

behind:cloudflare
: only a host behind Cloudflare, because only Cloudflare sends the header.

A field named in the `fields` key as `required` or `optional` is included even
where its condition is not met. Naming a field says the host wants it.

# COMBINED FIELDS

These are the stock `combined` log format. Every host already logs them.

| Field | Default | Needed by | nginx | Apache |
|---|---|---|---|---|
| `client` | required | tier 1, 2 | `$remote_addr` | `%a` |
| `user` | optional | compatibility | `$remote_user` | `%u` |
| `time` | required | tier 1, 2, 3 | `$time_local` | `%t` |
| `request` | required | tier 1, 2 | `$request` | `%r` |
| `status` | required | tier 1, 2 | `$status` | `%>s` |
| `bytes` | required | tier 1 | `$body_bytes_sent` | `%B` |
| `referer` | optional | compatibility | `$http_referer` | `%{Referer}i` |
| `ua` | required | tier 1, 2, 3 | `$http_user_agent` | `%{User-Agent}i` |

`client` is the visitor's address only when the real-IP configuration is in
place. Behind a proxy without it, every request appears to come from the proxy.
The tools never keep `referer` ({app_name}-privacy(7)); it is in the list because
the server's own log has it and the generated format reproduces that log.

# EXTENDED FIELDS

These are added to the combined format, each as `label=value`.

| Field | Default | Applies | nginx | Apache |
|---|---|---|---|---|
| `rt` | required | always | `$request_time` | `%D` |
| `urt` | required | always | `$upstream_response_time` | unavailable |
| `cf_ray` | required | behind:cloudflare | `$http_cf_ray` | `%{CF-Ray}i` |
| `cf_country` | required | behind:cloudflare | `$http_cf_ipcountry` | `%{CF-IPCountry}i` |
| `peer` | required | behind_proxy | `$realip_remote_addr` | `%{c}a` |
| `lang` | optional | always | `$http_accept_language` | `%{Accept-Language}i` |
| `ch_ua` | optional | always | `$http_sec_ch_ua` | `%{Sec-CH-UA}i` |
| `ch_plat` | optional | always | `$http_sec_ch_ua_platform` | `%{Sec-CH-UA-Platform}i` |
| `bot_score` | optional | behind:cloudflare | `$http_cf_bot_score` | `%{CF-Bot-Score}i` |
| `ja3` | optional | behind:cloudflare | `$http_cf_ja3_hash` | `%{CF-JA3-Hash}i` |
| `ja4` | optional | behind:cloudflare | `$http_cf_ja4` | `%{CF-JA4}i` |

`rt`
: the total time to serve the request. nginx logs seconds and Apache `%D` logs
  microseconds, so a reader must convert.

`urt`
: the time the upstream took, which is the cost of a request. It holds several
  values when nginx retried. Stock Apache has no upstream response time (to be
  verified), so `check` reports it unavailable on Apache. Rank by `rt` there, or
  set the field to `not_applicable`.

`cf_ray`
: Cloudflare's request identifier. It is the handle central IT needs to find one
  request, so evidence for a Cloudflare request cites it.

`cf_country`
: the country Cloudflare placed the client in.

`peer`
: the address of the connection itself, which tells a proxy edge from a direct
  client. It needs the real-IP module.

`lang`, `ch_ua`, `ch_plat`
: the Accept-Language header and the Sec-CH-UA and Sec-CH-UA-Platform client
  hints. They are cohort signals: what a client claims about its language, its
  browser and its operating system.

`bot_score`, `ja3`, `ja4`
: Cloudflare's bot score and TLS fingerprints. They are logged as a dash unless
  central IT enables the bot protection headers managed transform for the zone.
  The format can name them and the log will still show `-` for every request,
  which is why `check` reads a sample of the log and not only the configuration.

# THE GENERATED FORMAT

For nginx the table produces this directive, which is the format running on the
CaltechAUTHORS production server. A missing header is logged as `-`, never as an
empty string.

    log_format caltechauthors_bots
        '$remote_addr - $remote_user [$time_local] "$request" '
        '$status $body_bytes_sent "$http_referer" "$http_user_agent" '
        'rt=$request_time urt="$upstream_response_time" '
        'cf_ray="$http_cf_ray" cf_country="$http_cf_ipcountry" '
        'peer=$realip_remote_addr '
        'lang="$http_accept_language" ch_ua="$http_sec_ch_ua" '
        'ch_plat="$http_sec_ch_ua_platform" '
        'bot_score="$http_cf_bot_score" ja3="$http_cf_ja3_hash" '
        'ja4="$http_cf_ja4"';

Where the directive goes matters. Put it in the file that nginx reads before the
`access_log` line that uses it, and see {app_name}-check(1).

# SEE ALSO

{app_name}(1), {app_name}-check(1), {app_name}-config(5), {app_name}-privacy(7)
