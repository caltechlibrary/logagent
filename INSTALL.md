Installation for **logagent**
=============================

**logagent** is a work in progress: a Go tool for detecting and mitigating
automated traffic on nginx and Apache 2 web servers. At this stage the
`logagent` command provides only `--help`, `--version` and `--license`. See
[README.md](README.md) for where it is going.

It provides one command line program, `logagent`.

Installing a release
--------------------

Each release on <https://github.com/caltechlibrary/logagent/releases> has a zip
file per platform: Linux, macOS and Windows, each for amd64 (x86_64) and arm64
(aarch64). Download the one for your system and unzip it. It holds:

- `bin/logagent` (`bin\logagent.exe` on Windows)
- `man/`, the manual pages (`man1`, `man5` and `man7`), one for the command and one for each planned subcommand and topic
- `docs/`, the manual in Markdown
- `LICENSE`, `CITATION.cff` and `codemeta.json`

### POSIX (Linux and macOS)

~~~shell
mkdir -p "$HOME/bin" "$HOME/man"
cp bin/logagent "$HOME/bin/"
cp -R man/. "$HOME/man/"
export PATH="$HOME/bin:$PATH"
export MANPATH="$HOME/man:$MANPATH"
~~~

Add the two `export` lines to your shell's startup file to keep them.

### Windows

Copy `bin\logagent.exe` to a folder on your `PATH`.

Installing with Go
------------------

With a current stable Go installed:

~~~shell
go install github.com/caltechlibrary/logagent/cmd/logagent@latest
~~~

This puts `logagent` in `$(go env GOPATH)/bin` (or `$GOBIN`). It does not
install the manual page.

Installing from source
----------------------

### Required software

1. Git, to clone the repository
2. Go, the current stable release
3. Pandoc, to produce the manual page
4. make, `zip`, `grep`, `cut` and `sed`, which a POSIX system supplies (needed
   by the Makefile only; plain `go build` needs none of them)

Making a release also needs `gh` (the GitHub command line tool), and
`release.bash` needs `jq`.

### Compiling

~~~shell
git clone https://github.com/caltechlibrary/logagent
cd logagent
make
~~~

This builds `bin/logagent`, regenerates the manual pages in `docs/` from the
program's own help (`logagent help --list` names them), and renders them into
`man/`. You can also build with Go alone, which needs neither make nor
Pandoc:

~~~shell
go build -o bin/ ./cmd/...
~~~

On Windows, without make:

~~~powershell
go build -o bin\logagent.exe .\cmd\logagent
~~~

Check that it works:

~~~shell
./bin/logagent --version
~~~

### Testing

~~~shell
make test
~~~

which runs `go vet ./...` and `go test ./...`.

### Installing and uninstalling

~~~shell
make install
make uninstall
~~~

`make install` installs the program in `$HOME/bin` and the man pages in
`$HOME/man/man1`, `man5` and `man7`. Use `make install prefix=/usr/local` to install somewhere else
(that needs permission to write there). Make sure the `bin` directory is on your
`PATH` and the `man` directory is on your `MANPATH`.

Making a release
----------------

~~~shell
make release
./release.bash
~~~

`make release` refreshes `version.go` from `codemeta.json`, runs the tests and
builds the six zip files in `dist/`. `release.bash` (or `release.ps1` on
Windows) writes checksums, commits and pushes, and creates a draft release with
`gh` that you finish on GitHub. Update `version` and `releaseNotes` in
`codemeta.json` first.
