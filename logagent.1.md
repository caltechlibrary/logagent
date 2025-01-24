%logagent(1) user manual | version 0.0.2 27cbda9
% R. S. Doiel
% 

# NAME

logagent

# SYNOPSIS

logagent [OPTIONS]
logagent YAML_FILE LOG_FILE [OPTIONS]

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
    sudo iptables \
       -p tcp -m multiport \
       --dports http,https \
       -j DROP \
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

When you run 'logagent' with the '--dry_run' option it
will show you the commends that will be executed for log lines
with tags. Here's an example using the YAML config on "access.log"

~~~
logagent badbots.yaml access.log --dry_run
~~~

If this looks OK then you can apply the tags and actions like this.

~~~
logagent badbots.yaml access.log
~~~


