
# Log Agent Project

A set of tools to automate some of the adhoc log analysis we perform on RDM repositories.

## LogAgent

This is a simplified log processor that looks for explicit text on a line, parses the line for an IP address and then applies the associated action. It is inspired by fail2ban but is written in response fail2ban's complexity. Caltech Library needed a simple tool to do a narrow task that was oddly challenging using fail2ban. There is always a balancing act between a tool features and those that are simpler targeting a more specific issue.

## LogAnalyst

This provides a quick and dirty way of viewing aggregated counts based on transform the NginX log entries into structed data.

## Approach

LogAgent and LogAnalyst read logs input line by line. LogAgent checks if a tag (explicit sub-string) is contained in that line. If a match is found then the agent extracts any IP addresses identified before applying a rule associated with the tag. LogAnalyst will look at the log entry transform it into structured data for aggregating various simple counts.

LogAgent requires a configuration file written in YAML. The configuration holds an array of objects. Each object has the following attributes.  LogAnalyst just reads the log file as a stream, usually from standard input. When the processing is complete a simple analysis is displayed as a JSON object.

## LogAgent Configuration

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

For more information see the following documentation pages.

- [User Manual](user_manual.md)
- [Installation](INSTALL.md) and compiling Log Agent from source
- [LICENSE](LICENSE)
- [About Log Agent](about.md)
- [Cite with CITATION.cff](CITATION.cff)
