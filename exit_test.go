package logagent

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"testing"

	"github.com/caltechlibrary/logagent/internal/check"
	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/nginxconf"
)

// Every class maps to the number and name in the workspace table (DR-0014).
func TestClassifyMapsErrorsToTheWorkspaceExitCodes(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		class string
		code  int
	}{
		{"usage", usageError("bad flag"), "usage", 2},
		{"no configuration", fmt.Errorf("%w: nowhere", config.ErrNoConfig), "no_input", 66},
		{"invalid configuration", fmt.Errorf("%w: x", config.ErrInvalid), "config", 78},
		{"syntax error", &nginxconf.SyntaxError{File: "f", Line: 1, Msg: "m"}, "data", 65},
		{"unresolved include", fmt.Errorf("%w: x", nginxconf.ErrInclude), "data", 65},
		{"no servers", check.ErrNoServers, "data", 65},
		{"unsupported server", fmt.Errorf("%w: apache", check.ErrUnsupported), "negative", 1},
		{"missing file", &fs.PathError{Op: "open", Path: "p", Err: fs.ErrNotExist}, "no_input", 66},
		{"permission", &fs.PathError{Op: "open", Path: "p", Err: fs.ErrPermission}, "no_permission", 77},
		{"other path error", &fs.PathError{Op: "read", Path: "p", Err: errors.New("is a directory")}, "io", 74},
		{"exists", fmt.Errorf("x: %w", fs.ErrExist), "cant_create", 73},
		{"network", &net.OpError{Op: "dial", Err: errors.New("refused")}, "unavailable", 69},
		{"url", &url.Error{Op: "Get", URL: "u", Err: errors.New("x")}, "unavailable", 69},
		{"unclassified", errors.New("something nobody classified"), "internal", 70},
		{"wrapped", fmt.Errorf("while reading: %w", os.ErrNotExist), "no_input", 66},
	}
	for _, c := range cases {
		class, code := classify(c.err)
		if class != c.class || code != c.code {
			t.Errorf("%s: classify = %s/%d, want %s/%d", c.name, class, code, c.class, c.code)
		}
	}
}

func TestNoClassifiedCodeIsReservedByTheShell(t *testing.T) {
	for _, e := range []error{usageError("x"), config.ErrNoConfig, errors.New("x"), fs.ErrPermission} {
		if _, code := classify(e); code == 126 || code == 127 || code > 125 {
			t.Errorf("%v: code %d", e, code)
		}
	}
}
