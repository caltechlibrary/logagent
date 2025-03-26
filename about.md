---
title: logagent
abstract: "This software is for working with web logs (NginX) scanning log entries. It includes two tools. LogAgent will scan each line of a log file for an explicit string, finds the IP 
address and in the log line and applies an designated action. The second tool LogAnalyst is a tool explore some patterns in the log entries such as ip address, paths and methods, 
user agent strings."
authors:
  - family_name: Doiel
    given_name: R. S.
    id: https://orcid.org/0000-0003-0900-6903



repository_code: git+https://github.com/caltechlibrary/logagent
version: 0.0.3
license_url: https://data.caltech.edu/license
operating_system:
  - Linux
  - Windows
  - macOS

programming_language:
  - TypeScript

keywords:
  - logging
  - agents
  - analyzer


---

About this software
===================

## logagent 0.0.3

Proof of concept log agent. Tag/action executed once per IP address. Including a proof of concept log analyst tool.

### Authors

- R. S. Doiel, <https://orcid.org/0000-0003-0900-6903>





This software is for working with web logs (NginX) scanning log entries. It includes two tools. LogAgent will scan each line of a log file for an explicit string, finds the IP 
address and in the log line and applies an designated action. The second tool LogAnalyst is a tool explore some patterns in the log entries such as ip address, paths and methods, 
user agent strings.

- License: <https://data.caltech.edu/license>
- GitHub: <git+https://github.com/caltechlibrary/logagent>
- Issues: <https://github.com/caltechlibrary/logagent/issues>

### Programming languages

- TypeScript


### Operating Systems

- Linux
- Windows
- macOS


### Software Requirements

- deno &gt;&#x3D; 2.1.7

