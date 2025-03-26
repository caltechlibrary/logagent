
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
-a, --analysis       Specify analysis types (comma-separated): summary, ip, agent, path, time, endpoint
-o, --output         Specify output format: json, yaml, csv (default: json)
--ips                Shortcut for --analysis ip
--yaml               Shortcut for --output yaml
--endpoint          Shortcut for --analysis endpoint

## Example

To analyze log entries with default summary analysis:

```
deno run --allow-read loganalyst.ts < logfile.log
```

To analyze log entries with custom regex and specific analyses:

```
deno run --allow-read loganalyst.ts --regexp ".*\.zip" --analysis ip,agent --output yaml < logfile.log
```

To analyze log entries for endpoint analysis only:

```
deno run --allow-read loganalyst.ts --analysis endpoint < logfile.log
```

**Note:** When specifying a regex pattern, make sure to escape special characters properly. For example, use `\.` to match a literal dot.

  
