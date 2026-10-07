package logagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/caltechlibrary/logagent/internal/check"
	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/fields"
	"github.com/caltechlibrary/logagent/internal/help"
	"github.com/caltechlibrary/logagent/internal/nginxconf"
	"github.com/caltechlibrary/logagent/internal/sample"
)

// commandRunner runs the command that prints the web server's configuration.
// It is a variable so tests do not need a web server installed.
var commandRunner = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// checkOptions are the parsed options of `logagent check`.
type checkOptions struct {
	help, json   bool
	config, dump string
	// sample is how many lines from the end of each log to count; 0 means none.
	sample int
}

// defaultSample is how many lines of each log the check counts unless told otherwise.
const defaultSample = 10000

// newCheckOptions returns the options with their defaults.
func newCheckOptions() checkOptions { return checkOptions{sample: defaultSample} }

// parseCheckArgs reads the options of the check command: long options with one
// or two dashes, `--name=value`, short options, and clusters of short options
// such as -jh, where a short option that takes a value ends the cluster.
func parseCheckArgs(args []string) (checkOptions, error) {
	o := newCheckOptions()
	long := map[string]string{"help": "h", "json": "j", "config": "c", "dump": "d", "sample": "s"}
	var badValue error
	set := func(short string, value string) {
		switch short {
		case "h":
			o.help = true
		case "j":
			o.json = true
		case "c":
			o.config = value
		case "d":
			o.dump = value
		case "s":
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				badValue = usageError(fmt.Sprintf("--sample needs a whole number of lines, 0 or more (got %q)", value))
				return
			}
			o.sample = n
		}
	}
	takesValue := func(short string) bool { return short == "c" || short == "d" || short == "s" }
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			return o, usageError(fmt.Sprintf("surplus argument %q", a))
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-"), "=")
		if short, ok := long[name]; ok && (strings.HasPrefix(a, "--") || len(name) > 1) {
			if takesValue(short) {
				if !hasValue {
					if i+1 >= len(args) {
						return o, usageError(fmt.Sprintf("option %s needs a value", a))
					}
					i++
					value = args[i]
				}
			} else if hasValue {
				return o, usageError(fmt.Sprintf("option %s takes no value", a))
			}
			set(short, value)
			continue
		}
		if strings.HasPrefix(a, "--") || len(name) == 0 {
			return o, usageError(fmt.Sprintf("unknown option %q", a))
		}
		cluster := strings.TrimPrefix(a, "-")
		for j := 0; j < len(cluster); j++ {
			s := string(cluster[j])
			switch {
			case s == "h" || s == "j":
				set(s, "")
			case takesValue(s):
				v := cluster[j+1:]
				if v == "" {
					if i+1 >= len(args) {
						return o, usageError(fmt.Sprintf("option -%s needs a value", s))
					}
					i++
					v = args[i]
				}
				set(s, v)
				j = len(cluster)
			default:
				return o, usageError(fmt.Sprintf("unknown option -%s", s))
			}
		}
	}
	return o, badValue
}

// runCheck is `logagent check`. It reads the web server configuration the host
// configuration points at, prints what the check found and returns the exit
// code: 0 when no gaps, 1 when gaps, otherwise the class of the failure.
func runCheck(appName string, args []string, stdout, stderr io.Writer) int {
	opt, err := parseCheckArgs(args)
	jsonMode := opt.json
	fail := func(err error) int {
		class, code := classify(err)
		if jsonMode {
			var e struct {
				Error struct {
					Class   string `json:"class"`
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			e.Error.Class, e.Error.Code, e.Error.Message = class, code, err.Error()
			data, _ := json.Marshal(e)
			fmt.Fprintln(stderr, string(data))
		} else {
			fmt.Fprintf(stderr, "%s check: %s\n", appName, err)
			if class == "usage" {
				fmt.Fprintf(stderr, "usage: %s check [-h|--help] [-j|--json] [-c|--config PATH] [-d|--dump FILE] [-s|--sample LINES]\n", appName)
			}
		}
		return code
	}
	if err != nil {
		return fail(err)
	}
	if opt.help {
		p, err := help.Lookup("check")
		if err != nil {
			return fail(err)
		}
		fmt.Fprint(stdout, FmtHelp(p.Text, appName, Version, ReleaseDate, ReleaseHash))
		return ExitOK
	}
	report, err := buildReport(opt)
	if err != nil {
		return fail(err)
	}
	if jsonMode {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fail(err)
		}
		fmt.Fprintln(stdout, string(data))
	} else if err := check.WriteText(stdout, report); err != nil {
		return fail(err)
	}
	return report.ExitCode()
}

// buildReport locates the host configuration, reads the web server's
// configuration and checks it. The command and the interface both use it.
func buildReport(opt checkOptions) (*check.Report, error) {
	path, err := config.Locate(opt.config, os.Getenv)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if cfg.Server != "nginx" {
		return nil, fmt.Errorf("%w: %s configurations cannot be checked yet", check.ErrUnsupported, cfg.Server)
	}
	text, err := readDump(opt, cfg)
	if err != nil {
		return nil, err
	}
	dump, err := nginxconf.Parse(string(text))
	if err != nil {
		return nil, err
	}
	in := check.Input{Config: cfg, Dump: dump}
	if opt.sample > 0 {
		in.Sampler = logSampler(opt.sample)
	}
	return check.Run(in)
}

// logSampler returns the function the check calls to count fields in a log. A
// log that is not on this machine, as when a dump was copied from a host, is
// skipped without comment; any other failure is reported as a note.
func logSampler(lines int) func(path, format string) check.Sample {
	return func(path, format string) check.Sample {
		p, err := sample.Compile(format)
		if err != nil {
			return check.Sample{Err: err.Error()}
		}
		s, err := sample.File(path, lines, p, fields.Default())
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return check.Sample{}
		case err != nil:
			return check.Sample{Err: err.Error()}
		}
		return s
	}
}

// readDump returns the web server's configuration text: from --dump, from the
// configured dump file, or from the configured command.
func readDump(opt checkOptions, cfg *config.Config) ([]byte, error) {
	file := opt.dump
	if file == "" {
		file = cfg.Source.Dump
	}
	if file != "" {
		return os.ReadFile(file)
	}
	argv := strings.Fields(cfg.Source.Command)
	if len(argv) == 0 {
		return nil, usageError("no configuration source: set config.dump or config.command, or pass --dump")
	}
	out, err := commandRunner(argv[0], argv[1:]...)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, dataError(fmt.Errorf("%s exited with status %d: %s", strings.Join(argv, " "), ee.ExitCode(), firstLines(string(out), 5)))
		}
		return nil, err
	}
	return out, nil
}

// firstLines returns up to n lines of text, for an error message.
func firstLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "...")
	}
	return strings.Join(lines, "; ")
}
