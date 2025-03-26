%loganalyst(1) user manual | version 0.0.3 00568b8
% R. S. Doiel
% 2025-03-25

# NAME

loganalyst

# SYNOPSIS

loganalyst [OPTIONS] < LOG_FILEPATH

# DESCRIPTION

Log Analyst analyzes NginX logs entries through aggregating counts. 
It works on a stream of entries, one per lines. It supports various
options such as filter using an optional regex (`--regexp` or `-r`).
The analyst corresponds the columns defined in the NginX access log
aggregated with counts. The intent is to be able to quickly scan
an access log file for spikes or patterns.

# OPTIONS

-h, --help
: Display this help message

--license
: Display license text

--version
: Display version

--verbose
: Display lines as they are being processed

-r, --regexp
: Specify a custom regex pattern to match log entries

-a, --analysis
: Specify analysis types (comma-separated): summary, ip, agent, path, time, endpoint

-o, --output
: Specify output format: json, yaml, csv (default: json)

--ips
: Shortcut for --analysis ip

--yaml
: Shortcut for --output yaml

--endpoint
: Shortcut for --analysis endpoint

## EXAMPLES

To analyze log entries with default summary analysis:

~~~
loganalyst < logfile.log
~~~

To analyze log entries with custom regex and specific analyses:

~~~
loganalyst --regexp ".*.zip" --analysis ip,agent --output yaml < logfile.log
~~~

To analyze log entries for endpoint analysis only:

~~~
loganalyst --analysis endpoint < logfile.log
~~~

**Note:** When specifying a regex pattern, make sure to escape special characters properly. For example, use `.` to match a literal dot.
  

