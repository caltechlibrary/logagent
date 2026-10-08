package logagent

import (
	"bytes"
	"strings"
	"testing"
)

// run is a test helper that calls Run and returns the exit code, stdout and
// stderr.
func run(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := Run("logagent", args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestHelpVersionLicenseExitZero(t *testing.T) {
	cases := []struct {
		args []string
		want string // text that must appear on stdout
	}{
		{[]string{"--help"}, "# NAME"},
		{[]string{"-h"}, "# NAME"},
		{[]string{"-help"}, "# NAME"},
		{[]string{"--version"}, Version},
		{[]string{"-v"}, Version},
		{[]string{"--license"}, "Redistribution and use"},
		{[]string{"-l"}, "Redistribution and use"},
	}
	for _, c := range cases {
		code, out, errOut := run(c.args...)
		if code != ExitOK {
			t.Errorf("%v: exit %d, want %d (stderr %q)", c.args, code, ExitOK, errOut)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%v: stdout lacks %q", c.args, c.want)
		}
		if errOut != "" {
			t.Errorf("%v: unexpected stderr %q", c.args, errOut)
		}
	}
}

func TestHelpTextHasNoUnexpandedMarkup(t *testing.T) {
	_, out, _ := run("--help")
	for _, marker := range []string{"{app_name}", "{version}", "{release_date}", "{release_hash}"} {
		if strings.Contains(out, marker) {
			t.Errorf("help still contains %s", marker)
		}
	}
	if !strings.Contains(out, Version) {
		t.Errorf("help does not show version %s", Version)
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	cases := [][]string{
		{},                         // nothing was asked for
		{"--no-such-flag"},         // unknown option
		{"-x"},                     // unknown short option
		{"nosuchverb"},             // unknown verb
		{"--help", "surplus"},      // surplus argument
		{"--version", "--license"}, // two terminal options
	}
	for _, args := range cases {
		code, out, errOut := run(args...)
		if code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
		if out != "" {
			t.Errorf("%v: usage error wrote to stdout: %q", args, out)
		}
		if errOut == "" {
			t.Errorf("%v: usage error wrote nothing to stderr", args)
		}
	}
}

func TestHelpCommand(t *testing.T) {
	code, out, errOut := run("help")
	if code != ExitOK || !strings.Contains(out, "# NAME") || errOut != "" {
		t.Errorf("help: exit %d, stderr %q", code, errOut)
	}
	code, out, _ = run("help", "check")
	if code != ExitOK || !strings.Contains(out, "logagent-check") || !strings.Contains(out, "# EXIT STATUS") {
		t.Errorf("help check: exit %d", code)
	}
	code, out, _ = run("help", "--list")
	if code != ExitOK {
		t.Fatalf("help --list: exit %d", code)
	}
	for _, want := range []string{"logagent.1", "logagent-check.1", "logagent-config.5", "logagent-jsonl.5", "logagent-privacy.7", "logagent-tiers.7"} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("help --list lacks %s", want)
		}
	}
}

func TestHelpUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"help", "nosuchtopic"},
		{"help", "check", "surplus"},
		{"help", "--list", "surplus"},
	} {
		code, out, errOut := run(args...)
		if code != ExitUsage || out != "" || errOut == "" {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, out, errOut)
		}
	}
}

// check and report are implemented; the other commands are still planned.
func TestPlannedVerbs(t *testing.T) {
	for _, verb := range []string{"watch", "respond", "analyze"} {
		code, out, errOut := run(verb, "--help")
		if code != ExitOK || !strings.Contains(out, "logagent-"+verb) || errOut != "" {
			t.Errorf("%s --help: exit %d", verb, code)
		}
		code, out, errOut = run(verb)
		if code != ExitUsage || out != "" || !strings.Contains(errOut, "not implemented") {
			t.Errorf("%s: exit %d, stderr %q", verb, code, errOut)
		}
	}
}
