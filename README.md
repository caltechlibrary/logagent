
# Log Agent

This is a simplified log processor that looks for explicit text on a line, parses the line for an IP address and then applies the action associted with the text. It is inspired by fail2ban but written in response to it's complexity. Caltech Library needed a simple tool to do a narrow task that was oddly challanging using fail2ban.

## approach

Log agent reads input line by line. If checks if a tag (explicit sub string) is contained in that line. If a match is found then the agent extracts any IP addresses identified before applying a rule associated with the tag.

The log agent requires a configuration file written in yaml. The configuration holds an array of object where each object is an agent configuration. The object has the following attributes.

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

# USAGE:

~~~
logagent OPTIONS CONFIG_FILE LOG_FILE
~~~

The CONFIG_FILE is the YAML file used to configure the logagent and the log
file is the fail to process. The LOG_FILE must be provided.

~~~
sudo logagent badbots.yaml /var/log/nginx/access.log
~~~

If you wanted to test the agent configuration then the two examples below are helpful.

~~~
sudo logagent --dry-run badbots.yaml /var/log/nginx/access.log
~~~

# OPTIONS

-h, --help
: display help

-v, --version
: display version

-l, --license
: display license

-d, --dry-run
: don't take any actions, instead write each action to standard out. This lets you cature them in a bash or Powershell script.
