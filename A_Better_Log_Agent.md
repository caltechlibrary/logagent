---
title: A Better Log Agent
---

# A Better Log Agent

In the ongoing war of with abusive bots identifying patterns of access and behaviors is important. You want the public website to be publicly available but also must avoid the denail of service attackes based on either limitations of machine resources and value or service rental costs.  As a result you need to see the problem coming at you such that you can defend yourself and your legitimate users.

## Log as a source of data

Logs can be monitored as a source of data for analysis. Web server software such as Apache2 and NginX produce logs that can be consumed as they are written. Getting these into a database for processing and analysis involves mapping the log formats into actional metadata elements.

From the point of composibility there needs to be a tool (function) that can trail the log and write it into a database records (e.g. rows in a table). The format of the log is described in the configuration of the web server. That can be used as the source structured that can be parsed and then used to tailing the metadata extraction for each new log entry. Beyond that is a mapping of the core log files to a schema suitable for your database (e.g. a JSON object or columns).

## Analysis

With the arrival of the abusive behavior of the AI companies battling the bots can be challenging. Fortunately we have tools and techniques that should be useful. Data Science has for the quarter of the century provided approachees for aggregating and analysis of real word data and extrapolating useful results. This log entries are essentially sensor data of your web service. You can apply the techniques you learned in the field of Data Science to identifying the abusive patterns based on IP address and networks attacking your systems. Since the typical AI Bots work in swarms banning at the subnet level yields the best results in mitigating the abuse.

## Next steps

1. Write a general parser that can read an Apache or NginX log file and identify the log formats needed for data harvesting
2. Using the resulting structure parse the log and crosswalk the results into a common record format
3. Using the common record format run analysis functions to identify potential problem requests
4. Compare resulting IP range mappings against a list of legitimate requestor IP addresses (or ranging)
5. Take the remaing problem IP address and generate the firewall commands that will be run to restrict access from those subnets

For improved results aggregate the log content across machines, the bots often attack more than one host at a time.

## Ideas, Someday, maybe

Write a service that runs the automate detect and ban process

## Questions

Q: Why not use fail to ban?

It is good for old fashioned bot attacks by single machines, it isn't suitable to defend against the swarms of AI bots that hit all at once. You should be using it but it's not the only thing needed to manage the bot attacks.

Q: What firewall is going to be used?

We use ufw as our machines run Ubuntu Server and that is the easiest to configure. The typical command looks something like, `sudo ufw insert 1 deny from IP_ADDRESS_OR_SUBNET` where IP_ADDRESS_OR_SUBNET uses the "/24" or "/16" IP address notation to specify a range.

NOTE: the "insert 1" insures that the rule is evaluate early. Firewall rule order count, making the ban rule happen early ensures it is applied to the requesting bot.


