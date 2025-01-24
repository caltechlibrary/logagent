
# Log Agent

This is a simplified log processor that looks for explicit text on a line, parses the line for an IP address and then applies the associated action. It is inspired by fail2ban but is written in response fail2ban's complexity. Caltech Library needed a simple tool to do a narrow task that was oddly challenging using fail2ban. There is always a balancing act between a tool features and those that are simpler targeting a more specific issue.

## Approach

Log agent reads input line by line. If checks if a tag (explicit sub-string) is contained in that line. If a match is found then the agent extracts any IP addresses identified before applying a rule associated with the tag.

The log agent requires a configuration file written in YAML. The configuration holds an array of objects. Each object has the following attributes.

tag
: The explicit search string (i.e. not regular expressions)

action
: The command to execute if tag is found

Here's an example configuration YAML file.

~~~YAML
- tag: BadBot
  action: |
    sudo iptables -A INPUT -p tcp -m multiport
    --dports 80,443 -s {ipaddress} -j DROP
~~~

If the text "BadBot" is found in the log line. and the IP address "156.59.198.136" was found in the log line then the following command would be executed.

~~~shell
    sudo iptables -A INPUT -p tcp -m multiport
         --dports 80,443 -s 156.59.198.136 -j DROP
~~~

# USAGE:

~~~
logagent CONFIG_FILE LOG_FILE [OPTIONS]
~~~

The CONFIG_FILE is the YAML file used to configure the logagent and the log
file is the fail to process. The LOG_FILE must be provided.

~~~
sudo logagent badbots.yaml /var/log/nginx/access.log
~~~

If you wanted to test the agent configuration then the two examples below are helpful.

~~~
sudo logagent badbots.yaml /var/log/nginx/access.log --dry_run 
~~~

# OPTIONS

-h, --help
: display help

-v, --version
: display version

-l, --license
: display license

-d, --dry_run
: don't take any actions, instead write each action to standard out. This lets you capture them in a bash or Powershell script.

For more information see the following documentation pages.

- [User Manual](user_manual.md)
- [Installation](INSTALL.md) and compiling Log Agent from source
- [LICENSE](LICENSE)
- [About Log Agent](about.md)
- [Cite with CITATION.cff](CITATION.cff)
