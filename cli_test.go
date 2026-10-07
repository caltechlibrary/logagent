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
