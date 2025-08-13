export function fmtHelp(
  txt: string,
  appName: string,
  version: string,
  releaseDate: string,
  releaseHash: string,
): string {
  return txt.replaceAll("{app_name}", appName).replaceAll("{version}", version)
    .replaceAll("{release_date}", releaseDate).replaceAll(
      "{release_hash}",
      releaseHash,
    );
}

export const helpTextLogAgent =
  `%{app_name}(1) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name}

# SYNOPSIS

{app_name} [OPTIONS]
{app_name} YAML_FILE LOG_FILE [OPTIONS]

# DESCRIPTION

Log agent reads input line by line. If checks if a tag (explicit sub string)
is contained in that line. If a match is found then the agent extracts any
IP addresses identified before applying a rule associated with the tag.

The log agent requires a configuration file written in yaml. The configuration
holds an array of object where each object is an agent configuration. The
object has the following attributes.

tag
: The explicit string to search for

action
: The command to execute if tag is found

Here's an example configuration YAML file.

~~~yaml
- tag: BadBot
  action: |
    sudo iptables
    -p tcp -m multiport
    --dports http,https
    -j DROP
    -s {ipaddress}
~~~

If the text "BadBot" is found in the log line. and the IP address "156.59.198.136" was found in the log line then the following command would be executed.

~~~shell
    sudo iptables \\
       -p tcp -m multiport \\
       --dports http,https \\
       -j DROP \\
       -s 156.59.198.136
~~~

# OPTIONS

Options come as the last parameter(s) on the command line.

-h, --help
: display help

-v, --version
: display version

-l, --license
: display license

-d, --dry_run
: display the commands for matching tags in the configuration. Nice
for generating bash or Powershell scripts.

# EXAMPLES

In example we're looking for log lines that have the text "BadBot"
or "BadSpider". We'll use iptables to ban them.

Here's the YAML config called "badbots.yaml"

~~~
- tag: BadBot
  action: |
    sudo iptables
        -p tcp -m multiport
        --dports http,https
        -j DROP
        -s {ipaddress}
- tag: BadSpider
  action: |
    sudo iptables
        -I logagent_badbot
        -p tcp -m multiport
        --dports http,https
        -j DROP
        -s {ipaddress}
~~~

When you run '{app_name}' with the '--dry_run' option it
will show you the commends that will be executed for log lines
with tags. Here's an example using the YAML config on "access.log"

~~~
{app_name} badbots.yaml access.log --dry_run
~~~

If this looks OK then you can apply the tags and actions like this.

~~~
{app_name} badbots.yaml access.log
~~~

`;

export const helpTextLogAnalyst =
  `%{app_name}(1) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name}

# SYNOPSIS

{app_name} [OPTIONS]
{app_name} ACTION PARAMETERS

# DESCRIPTION

{app_name} can capture NginX/Apache logs entries into an SQLite3 database.
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
{app_name} < logfile.log
~~~

To analyze log entries with custom regex and specific analyses:

~~~
{app_name} --regexp ".*\.zip" --analysis ip,agent --output yaml < logfile.log
~~~

To analyze log entries for endpoint analysis only:

~~~
{app_name} --analysis endpoint < logfile.log
~~~

**Note:** When specifying a regex pattern, make sure to escape special characters properly. For example, use \`.\` to match a literal dot.


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
- Replace \{ipaddress\} with the detected subnet.

## OUTPUT FORMATS

- **JSON**: Default, structured output.
- **YAML**: Human-readable format.
- **CSV**: Flat, spreadsheet-friendly format.

`;
