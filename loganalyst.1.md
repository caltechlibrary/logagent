
# Log Analyst

Analyze NginX log files to identify entries matching a specified regex.

## Usage

```
loganalyst.ts [options]
```

## Options

-h, --help           Display this help message
--verbose            Display lines as they are being processed
-r, --regexp         Specify a custom regex pattern to match log entries
--user-agents        Analyze user agent strings and their counts (default: true)
--paths              Analyze paths and their counts (default: true)
--ips                Analyze IP addresses and their counts (default: true)
--frequency          Analyze frequency of requests over time (default: true)
--format             Output format: json, yaml, csv (default: json)

## Example

To analyze log entries ending with '/file':

```
deno run --allow-read loganalyst.ts < logfile.log
```

To analyze log entries matching a custom regex pattern (e.g., any entry containing '.zip'):

```
deno run --allow-read loganalyst.ts --regexp ".*\.zip" < logfile.log
```

To output the analysis in YAML format:

```
deno run --allow-read loganalyst.ts --format yaml < logfile.log
```

**Note:** When specifying a regex pattern, make sure to escape special characters properly. For example, use `\.` to match a literal dot.

  
