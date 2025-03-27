---
title: "Log Agent, rational and how it works"
author: "R. S. Doiel, <rsdoiel@caltech.edu>"
institute: |
  Caltech Library,
  Digital Library Development
description: Caltech Library Digital Development Group presentation
urlcolor: blue
linkstyle: bold
aspectratio: 169
createDate: 2025-01-24
updateDate: 2025-03-26
pubDate: TBD
place: TBD
date: 2025-03-26
section-titles: false
toc: true
keywords: [ "logs", "bots" ]
url: https://caltechlibrary.github.io/logagent/presentations/presentation1.html
---

# LogAgent and LogAnalyst, rational and usage

LogAgent
: A simple way to use a sub string in a log to trigger an action

LogAnalyst
: A simple way to aggregate counts by turning NginX log entries into structured data.

# Why LogAgent?

- fail2ban is very capable
- fail2ban became challenging to do a simple things

# Original Problem

- Caltech Library gets pounded by unruly spiders and bots
- They usually a easy to spot in the log using a unique string
- Creating/updating fail2ban filters became a time sink

Solution, use something simpler along side fail2ban.

# How does it work

- A very simple YAML file, defines a list of objects with a "tag" and "action" 
- Reads in a log and find tags then apply action

# What does the configuration look like?

- A list of objects
- Each object contains a tag and action attribute
- The tag is the string to search for
- The action is the command to execute
  - `{ipaddress}` can be used for the IP address found in the log line

# A single "tag" and "action"

Search for "BadBot" and ban it with iptables.

~~~yaml
- tag: BadBot
  action: |
    sudo iptables -A INPUT -p tcp -m multiport
    --dports 80,443 -s {ipaddress} -j DROP
~~~

# Running log agent

- YAML configuration `badbot.yaml`
- Log file at `/var/log/nginx/access.log`

~~~shell
sudo logagent badbot.yaml /var/log/nginx/access.log
~~~

# Explanation

For each IP address identified on a tagged log line the action will be executed. Given the example action that means the IP address associated with log lines containing "BadBot" are banned from connecting to ports 80 and 443.

# Why LogAnalyst

Sometimes the bots aren't the problem and the system runs well. Often I've found my self grepping the log for specific things to total up. LogAnalyst automates some of this activity. It reads in a stream of log entries, tokenizes them into a structured record then performs some analysis. The analyst is returned as a JSON object.

The tokenization part is the tricky bit, fortunately it's implemented as a TypeScript module.

# LogAnalyst in the future

I suspect I will add different types of analysis over time. As the `logtok.ts` module improves what I learn in LogAnalyst will be fed back into LogAgent.

# Reminder

LogAgent is an experimental "proof of concept" simple log processor written in TypeScript and compiled with Deno into a stand alone executable. It is a naive tool so take care with the action you assign for a tag.

# Reference

- GitHub Repository, <https://github.com/caltechlibrary/logagent>
- Website, <https://caltechlibrary.github.io/logagent>
- This presentation, <https://caltechlibrary.github.io/logagent/presentations/presentation1.html>
