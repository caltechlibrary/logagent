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

export const helpText =
  `%{app_name}(1) user manual | version {version} {release_hash}
% R. S. Doiel
% {release_date}

# NAME

{app_name}

# SYNOPSIS

{app_name} [OPTIONS]
{app_name} [OPTIONS] YAML_FILE LOG_FILE

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
        -I logagent_badbot
        -p tcp -m multiport
        --dports http,https
        -J REJECT
        --reject-with imcp-port-unreachable
        -s {ipaddress}
~~~

If the text "BadBot" is found in the log line. and the IP address "156.59.198.136" was found in the log line then the following command would be executed.

~~~shell
    sudo iptables -I logagent_badbot \
       -p tcp -m multiport \
       --dports http,https \
       -j REJECT --reject-with icmp-port-unreachable \
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

-d, --dryrun
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
        -I logagent_badbot
        -p tcp -m multiport
        --dports http,https
        -J REJECT
        --reject-with imcp-port-unreachable
        -s {ipaddress}
- tag: BadSpider
  action: |
    sudo iptables
        -I logagent_badbot
        -p tcp -m multiport
        --dports http,https
        -J REJECT
        --reject-with imcp-port-unreachable
        -s {ipaddress}
~~~

When you run '{app_name}' with the '--dryrun' option it
will show you the commends that will be executed for log lines
with tags. Here's an example using the YAML config on "access.log"

~~~
{app_name} --dryrun badbots.yaml access.log
~~~

If this looks OK then you can apply the tags and actions like this.

~~~
{app_name} badbots.yaml access.log
~~~

`;
