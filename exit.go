package logagent

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os/exec"

	"github.com/caltechlibrary/logagent/internal/check"
	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/logread"
	"github.com/caltechlibrary/logagent/internal/nginxconf"
	"github.com/caltechlibrary/logagent/internal/sample"
)

// The exit codes of the workspace convention (DR-0014) that logagent can
// return. The numbers are the BSD sysexits values except 0, 1 and 2.
const (
	exitNegative     = 1
	exitData         = 65
	exitNoInput      = 66
	exitUnavailable  = 69
	exitInternal     = 70
	exitCantCreate   = 73
	exitIO           = 74
	exitNoPermission = 77
	exitConfig       = 78
)

// classed is an error that carries its own class and exit code.
type classed struct {
	class string
	code  int
	err   error
}

func (e *classed) Error() string { return e.err.Error() }
func (e *classed) Unwrap() error { return e.err }

// usageError is a command line that is wrong: nothing was attempted.
func usageError(msg string) error {
	return &classed{class: "usage", code: ExitUsage, err: errors.New(msg)}
}

// dataError is content the command read that is wrong.
func dataError(err error) error {
	return &classed{class: "data", code: exitData, err: err}
}

// classify maps an error to its class name and exit code. It is the one place
// that decides; an error nothing classified is internal (70), so a missing
// classification shows up.
//
// @param err {error} the error to classify
// @returns {string, int} the class name from the workspace table and its exit code
// @example
//
//	class, code := classify(err)
func classify(err error) (string, int) {
	var c *classed
	switch {
	case errors.As(err, &c):
		return c.class, c.code
	case errors.Is(err, config.ErrNoConfig):
		return "no_input", exitNoInput
	case errors.Is(err, config.ErrInvalid):
		return "config", exitConfig
	case errors.Is(err, nginxconf.ErrSyntax), errors.Is(err, nginxconf.ErrInclude), errors.Is(err, check.ErrNoServers):
		return "data", exitData
	case errors.Is(err, logread.ErrMismatch), errors.Is(err, bufio.ErrTooLong), errors.Is(err, gzip.ErrHeader),
		errors.Is(err, gzip.ErrChecksum), errors.Is(err, io.ErrUnexpectedEOF):
		return "data", exitData
	case errors.Is(err, logread.ErrNotConfigured), errors.Is(err, logread.ErrNoTime):
		return "config", exitConfig
	case errors.Is(err, check.ErrUnsupported), errors.Is(err, logread.ErrUnsupported), errors.Is(err, sample.ErrFormat):
		return "negative", exitNegative
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, exec.ErrNotFound):
		return "no_input", exitNoInput
	case errors.Is(err, fs.ErrPermission):
		return "no_permission", exitNoPermission
	case errors.Is(err, fs.ErrExist):
		return "cant_create", exitCantCreate
	}
	var pe *fs.PathError
	var ne net.Error
	var ue *url.Error
	switch {
	case errors.As(err, &pe):
		return "io", exitIO
	case errors.As(err, &ne), errors.As(err, &ue):
		return "unavailable", exitUnavailable
	}
	return "internal", exitInternal
}
