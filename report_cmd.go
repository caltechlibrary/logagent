package logagent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/caltechlibrary/logagent/internal/check"
	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/help"
	"github.com/caltechlibrary/logagent/internal/logread"
	"github.com/caltechlibrary/logagent/internal/nginxconf"
	"github.com/caltechlibrary/logagent/internal/report"
	"github.com/caltechlibrary/logagent/internal/sample"
)

// reportNow is the clock the report's default and relative windows use. It is
// a variable so tests can fix it.
var reportNow = time.Now

// defaultTop is how many rows of a ranked table the text report prints.
const defaultTop = 15

// reportOptions are the parsed options of `logagent report`.
type reportOptions struct {
	help, json bool
	// config and dump are as for check.
	config, dump string
	// day, last, since and until choose the window; at most one way of choosing.
	day, last, since, until string
	// family is a part of a family name for the match column of section 2.
	family string
	// log names the access log to read; empty means the first one in the configuration.
	log string
	top int
}

// optSpec describes one option: its long name, its short letter if it has
// one, and whether it takes a value.
type optSpec struct {
	long, short string
	value       bool
}

// parseOpts reads options in the forms the commands accept: --name, --name=value,
// --name value, -s, -s value, -svalue and clusters of short options such as -jc
// PATH, where an option that takes a value ends the cluster. A bare word is a
// surplus argument. set is called once per option and may reject its value.
func parseOpts(args []string, specs []optSpec, set func(long, value string) error) error {
	byLong, byShort := map[string]optSpec{}, map[string]optSpec{}
	for _, s := range specs {
		byLong[s.long] = s
		if s.short != "" {
			byShort[s.short] = s
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			return usageError(fmt.Sprintf("surplus argument %q", a))
		}
		if strings.HasPrefix(a, "--") {
			name, value, hasValue := strings.Cut(a[2:], "=")
			s, ok := byLong[name]
			if !ok {
				return usageError(fmt.Sprintf("unknown option %q", a))
			}
			switch {
			case s.value && !hasValue:
				if i+1 >= len(args) {
					return usageError(fmt.Sprintf("option %s needs a value", a))
				}
				i++
				value = args[i]
			case !s.value && hasValue:
				return usageError(fmt.Sprintf("option %s takes no value", a))
			}
			if err := set(s.long, value); err != nil {
				return err
			}
			continue
		}
		cluster := a[1:]
		for j := 0; j < len(cluster); j++ {
			s, ok := byShort[string(cluster[j])]
			if !ok {
				return usageError(fmt.Sprintf("unknown option -%s", string(cluster[j])))
			}
			value := ""
			if s.value {
				value = cluster[j+1:]
				if value == "" {
					if i+1 >= len(args) {
						return usageError(fmt.Sprintf("option -%s needs a value", s.short))
					}
					i++
					value = args[i]
				}
				j = len(cluster)
			}
			if err := set(s.long, value); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseReportArgs reads the options of the report command.
func parseReportArgs(args []string) (reportOptions, error) {
	o := reportOptions{top: defaultTop}
	err := parseOpts(args, []optSpec{
		{"help", "h", false}, {"json", "j", false}, {"config", "c", true}, {"dump", "d", true},
		{"day", "", true}, {"last", "", true}, {"since", "", true}, {"until", "", true},
		{"family", "f", true}, {"log", "l", true}, {"top", "t", true},
	}, func(name, value string) error {
		switch name {
		case "help":
			o.help = true
		case "json":
			o.json = true
		case "config":
			o.config = value
		case "dump":
			o.dump = value
		case "day":
			o.day = value
		case "last":
			o.last = value
		case "since":
			o.since = value
		case "until":
			o.until = value
		case "family":
			o.family = value
		case "log":
			o.log = value
		case "top":
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return usageError(fmt.Sprintf("--top needs a whole number of rows, 0 or more (got %q)", value))
			}
			o.top = n
		}
		return nil
	})
	return o, err
}

// parseWhen reads a date (2026-10-07, midnight UTC) or an RFC 3339 time.
func parseWhen(flag, s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, usageError(fmt.Sprintf("%s needs a date such as 2026-10-07 or a time such as 2026-10-07T12:00:00Z (got %q)", flag, s))
}

// parseSpan reads a length of time: Go's own (24h, 90m) or whole days (7d).
func parseSpan(s string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(n)
		if err == nil && days > 0 {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	} else if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d, nil
	}
	return 0, usageError(fmt.Sprintf("--last needs a length of time such as 24h or 7d (got %q)", s))
}

// reportWindow turns the options into the span of time to read. With none it
// is yesterday, UTC, a complete day.
func reportWindow(o reportOptions, now time.Time) (logread.Window, error) {
	now = now.UTC()
	way := 0
	for _, s := range []string{o.day, o.last} {
		if s != "" {
			way++
		}
	}
	if o.since != "" || o.until != "" {
		way++
	}
	if way > 1 {
		return logread.Window{}, usageError("choose one of --day, --last, or --since and --until")
	}
	switch {
	case o.day != "":
		d, err := parseWhen("--day", o.day)
		if err != nil {
			return logread.Window{}, err
		}
		return logread.Window{Since: d, Until: d.AddDate(0, 0, 1)}, nil
	case o.last != "":
		span, err := parseSpan(o.last)
		if err != nil {
			return logread.Window{}, err
		}
		return logread.Window{Since: now.Add(-span), Until: now}, nil
	case o.since != "" || o.until != "":
		var w logread.Window
		var err error
		if o.since != "" {
			if w.Since, err = parseWhen("--since", o.since); err != nil {
				return w, err
			}
		}
		if o.until != "" {
			if w.Until, err = parseWhen("--until", o.until); err != nil {
				return w, err
			}
		}
		if !w.Since.IsZero() && !w.Until.IsZero() && !w.Since.Before(w.Until) {
			return w, usageError("--since must be before --until")
		}
		return w, nil
	}
	y := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	return logread.Window{Since: y, Until: y.AddDate(0, 0, 1)}, nil
}

// runReport is `logagent report`. It reads the host configuration and the web
// server's, reads the access log through the window once, and prints what the
// requests show. It stores nothing. It returns 0 when the report is printed,
// otherwise the class of the failure.
func runReport(appName string, args []string, stdout, stderr io.Writer) int {
	opt, err := parseReportArgs(args)
	fail := func(err error) int {
		class, code := classify(err)
		if opt.json {
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
			fmt.Fprintf(stderr, "%s report: %s\n", appName, err)
			if class == "usage" {
				fmt.Fprintf(stderr, "usage: %s report [-h|--help] [-j|--json] [-c|--config PATH] [-d|--dump FILE] [--day DATE | --last SPAN | --since TIME [--until TIME]] [-f|--family TEXT] [-t|--top ROWS] [-l|--log PATH]\n", appName)
			}
		}
		return code
	}
	if err != nil {
		return fail(err)
	}
	if opt.help {
		p, err := help.Lookup("report")
		if err != nil {
			return fail(err)
		}
		fmt.Fprint(stdout, FmtHelp(p.Text, appName, Version, ReleaseDate, ReleaseHash))
		return ExitOK
	}
	r, err := buildTrafficReport(opt)
	if err != nil {
		return fail(err)
	}
	if opt.json {
		err = r.WriteJSON(stdout)
	} else {
		err = r.WriteText(stdout, opt.top)
	}
	if err != nil {
		return fail(err)
	}
	return ExitOK
}

// buildTrafficReport locates the host configuration, finds the access log's
// format in the web server's configuration and counts the log.
func buildTrafficReport(opt reportOptions) (*report.Report, error) {
	win, err := reportWindow(opt, reportNow())
	if err != nil {
		return nil, err
	}
	path, err := config.Locate(opt.config, os.Getenv)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if cfg.Server != "nginx" {
		return nil, fmt.Errorf("%w: %s logs cannot be reported on yet", check.ErrUnsupported, cfg.Server)
	}
	text, err := readDump(checkOptions{dump: opt.dump}, cfg)
	if err != nil {
		return nil, err
	}
	dump, err := nginxconf.Parse(string(text))
	if err != nil {
		return nil, err
	}
	tree, err := dump.Resolve()
	if err != nil {
		return nil, err
	}
	logPath := opt.log
	if logPath == "" {
		logPath = cfg.Logs[0].Path
	}
	format, err := logread.FormatFor(tree, logPath)
	if err != nil {
		return nil, err
	}
	parser, err := sample.Compile(format)
	if err != nil {
		return nil, err
	}
	return report.Collect(logPath, parser, report.Options{
		Window: win, Top: opt.top, Match: opt.family,
		Class: cfg.PathClasses().Class, Lookup: cfg.FamilyData().Lookup,
		Internal: cfg.InternalRanges(), NoClasses: !cfg.HasClasses(),
	})
}
