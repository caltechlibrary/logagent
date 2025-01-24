Installation for development of **Log Agent**
===========================================

**Log Agent** is an experimental, proof of concept, log processor.  Log Agent scans each line of a log file for an explicit string, finds the IP address and in the log line and applies an designated action.

It provides a `logagent` command line program.

Quick install with curl or irm
------------------------------

There is an experimental installer.sh script that can be run with the following command to install latest table release. This may work for macOS, Linux and if you’re using Windows with the Unix subsystem. This would be run from your shell (e.g. Terminal on macOS).

~~~shell
curl https://caltechlibrary.github.io/logagent/installer.sh | sh
~~~

This will install `logagent` in your `$HOME/bin` directory.

If you are running Windows 10 or 11 use the Powershell command below.

~~~ps1
irm https://caltechlibrary.github.io/logagent/installer.ps1 | iex
~~~

Installing from source
----------------------

### Required software

1. Git (to clone the cold repository on GitHub)
2. Deno >= 2.1.7

### Compiling **LogAgent**

Deno is used to compile the TypeScript and dependent JavaScript files into an executable.

1. Use Deno's task to build project

~~~shell
deno task build
~~~

This will provide the `logagent` command in the "bin" folder in your repository directory.

You can check to make sure `logagent` works for your system. The compiled version is self contain and can be copied someplace in your path. 

### Manual install on POSIX

~~~shell
mkdir -p $HOME/bin
export PATH="$HOME/bin:$PATH"
cp bin/logagent $HOME/bin/
export MANPATH="$MANPATH:$HOME/man"
cp -vR man $HOME/
~~~

### Manual install for Windows via Powershell

~~~ps1
mkdir $HOME/bin
$PATH = [Environment]::GetEnvironmentVariable("PATH")
[Environment]::SetEnvironmentVariable("PATH", "$PATH;$HOME/bin")
copy ./bin/logagent.exe $HOME/bin/
~~~
