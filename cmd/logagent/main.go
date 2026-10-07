// Command logagent detects and mitigates automated traffic on web servers.
// It is a thin wrapper around the logagent module.
package main

import (
	"os"
	"path"

	"github.com/caltechlibrary/logagent"
)

func main() {
	os.Exit(logagent.Run(path.Base(os.Args[0]), os.Args[1:], os.Stdout, os.Stderr))
}
