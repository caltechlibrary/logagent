package logagent

import (
	"fmt"
	"io"
)

// Exit codes used by the logagent command. They are the subset of the
// workspace convention (DR-0014) that the command can return so far.
const (
	// ExitOK means success.
	//
	// @example
	//
	//	os.Exit(logagent.ExitOK)
	ExitOK = 0

	// ExitUsage means the command line was wrong and nothing was attempted.
	//
	// @example
	//
	//	os.Exit(logagent.ExitUsage)
	ExitUsage = 2
)

// Run is the logagent command line entry point. It writes results to stdout,
// diagnostics to stderr and returns the process exit code, so cmd/logagent is
// only a wrapper around it.
//
// @param appName {string} the name to show in help and usage messages
// @param args {[]string} the command line arguments, without the program name
// @param stdout {io.Writer} receives help, version and license text
// @param stderr {io.Writer} receives usage errors
// @returns {int} the exit code, ExitOK or ExitUsage
// @example
//
//	code := logagent.Run("logagent", []string{"--version"}, os.Stdout, os.Stderr)
//	os.Exit(code)
func Run(appName string, args []string, stdout, stderr io.Writer) int {
	usage := func(msg string) int {
		fmt.Fprintf(stderr, "%s: %s\n", appName, msg)
		fmt.Fprintf(stderr, "usage: %s [-h|--help] [-v|--version] [-l|--license]\n", appName)
		return ExitUsage
	}
	if len(args) == 0 {
		return usage("nothing to do")
	}
	if len(args) > 1 {
		return usage(fmt.Sprintf("surplus argument %q", args[1]))
	}
	switch args[0] {
	case "-h", "-help", "--help":
		fmt.Fprint(stdout, FmtHelp(LogagentHelpText, appName, Version, ReleaseDate, ReleaseHash))
	case "-v", "-version", "--version":
		fmt.Fprintf(stdout, "%s %s %s\n", appName, Version, ReleaseHash)
	case "-l", "-license", "--license":
		fmt.Fprint(stdout, LicenseText)
	default:
		if len(args[0]) > 0 && args[0][0] == '-' {
			return usage(fmt.Sprintf("unknown option %q", args[0]))
		}
		return usage(fmt.Sprintf("unknown command %q", args[0]))
	}
	return ExitOK
}
