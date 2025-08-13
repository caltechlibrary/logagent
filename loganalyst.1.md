%loganalyst(1) user manual | version 0.0.4 85a6082
% R. S. Doiel
% 2025-03-26

# NAME

loganalyst

# SYNOPSIS

loganalyst [OPTIONS]
loganalyst ACTION PARAMETERS

# DESCRIPTION

loganalyst can capture NginX/Apache logs entries into an SQLite3 database.
It use that database for summary and analys.

# ACTION

harvest  DB_NAME LOGFILE_PATH [OPTIONS]
: Import log entries into SQLite3 database

analyze DB_NAME [ANALYSIS_TYPE] [OPTIONS]
: Run analyses on harvested logs. Default analysis is a summary.

help
: Display this help page

# ANALYSIS_TYPE

ip
: IP address counts

agent
: User agent counts

path
: Request path counts

time
: Time period counts

endpoint
: Endpoint method counts

status
: HTTP status code counts

subnet
: Subnet swarming detection

summary
: All analyses (default)


# OPTIONS

-h, --help
: Display this help message

--license
: Display license text

--version
: Display version

-V, --verbose
: Display lines as they are being processed

-r, --regexp
: Specify a custom regex pattern to match log entries

-a, --analysis
: Specify analysis types (comma-separated): summary, ip, agent, path, time, endpoint

-a, --action=FILE
: Load actions from a YAML config file to block swarming subnets.

-o, --output=FMT
: Output format: json, yaml, or csv. Default: json.


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


### 1. Basic Analysis
~~~sh
cat access.log | deno run --allow-read loganalyst.ts
~~~
- Outputs a summary of log statistics in JSON format.

### 2. Store Logs in SQLite
~~~sh
cat access.log | deno run --allow-read --allow-write loganalyst.ts --db=logs.db
~~~
- Stores logs in logs.db and outputs a summary.

### 3. Analyze Subnets
~~~sh
cat access.log | deno run --allow-read loganalyst.ts --subnet --analysis=subnet
~~~
- Reports swarming subnets (e.g., /24 or /16).

### 4. Block Swarming Subnets
~~~sh
cat access.log | deno run --allow-read --allow-write --allow-run loganalyst.ts --subnet --action=swarm.yaml
~~~
- Detects swarming subnets and blocks them using commands from swarm.yaml.

### 5. Custom Regex and Output Format
~~~sh
cat access.log | deno run --allow-read loganalyst.ts --regex="GET /admin" --output=yaml
~~~

- Filters logs for /admin requests and outputs results in YAML.

## CONFIGURATION

### YAML Action File (e.g., swarm.yaml)
~~~yaml
- tag: "swarm"
  action: "iptables -A INPUT -s {ipaddress} -j DROP"
~~~
- Replace {ipaddress} with the detected subnet.

## OUTPUT FORMATS

- **JSON**: Default, structured output.
- **YAML**: Human-readable format.
- **CSV**: Flat, spreadsheet-friendly format.


