package check

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/caltechlibrary/logagent/internal/errorlog"
)

const errPath = "/var/log/nginx/error.log"

// counts is what the error log sampler hands back for one log.
func counts() errorlog.Counts {
	return errorlog.Counts{
		Lines: 3412, First: "2026-10-05 21:40:12", Last: "2026-10-07 14:02:59",
		Levels:     map[string]int{"warn": 1117, "error": 128, "notice": 2},
		Categories: map[string]int{"limit-conn": 7, "upstream-timeout": 87, "other": 41},
		Zones:      map[string]map[string]int{"limit-conn": {"api_conc": 5, "iiif_conc": 2}},
	}
}

func withErrorLog(t *testing.T, main string, edit func(*Input)) *Report {
	t.Helper()
	text := nginxConf(main, "", "")
	return runWith(t, cfg("none", nil), text, edit)
}

func TestTheErrorLogCountsAreInTheReport(t *testing.T) {
	var asked []string
	r := withErrorLog(t, "error_log "+errPath+" warn;", func(in *Input) {
		in.ErrorSampler = func(path string) errorlog.Counts { asked = append(asked, path); return counts() }
	})
	if len(asked) != 1 || asked[0] != errPath {
		t.Errorf("sampler asked for %v", asked)
	}
	if len(r.ErrorLogs) != 1 {
		t.Fatalf("ErrorLogs = %+v", r.ErrorLogs)
	}
	e := r.ErrorLogs[0]
	if e.Path != errPath || e.Lines != 3412 || e.First != "2026-10-05 21:40:12" || e.Last != "2026-10-07 14:02:59" || e.Levels["warn"] != 1117 {
		t.Errorf("entry = %+v", e)
	}
	// Categories in the table's order, then other; only those with a count.
	var names []string
	for _, c := range e.Categories {
		names = append(names, c.Name)
	}
	if strings.Join(names, " ") != "limit-conn upstream-timeout other" {
		t.Errorf("categories = %v", names)
	}
	lc := e.Categories[0]
	if lc.Count != 7 || len(lc.Zones) != 2 || lc.Zones[0].Zone != "api_conc" || lc.Zones[0].Count != 5 || lc.Zones[1].Zone != "iiif_conc" {
		t.Errorf("limit-conn = %+v, want its zones by count", lc)
	}
	if len(e.Categories[1].Zones) != 0 {
		t.Errorf("a category with no zones has %v", e.Categories[1].Zones)
	}
	if len(r.Findings) != 0 || r.ExitCode() != 0 {
		t.Errorf("counts alone are not findings: %+v", r.Findings)
	}
}

func TestOnlyFileErrorLogsAreSampledAndEachOnce(t *testing.T) {
	var asked []string
	main := "error_log /var/log/nginx/error.log warn;\nerror_log /dev/null;\nerror_log stderr;\nerror_log syslog:server=unix:/dev/log;"
	withErrorLog(t, main, func(in *Input) {
		in.ErrorSampler = func(path string) errorlog.Counts { asked = append(asked, path); return counts() }
	})
	if len(asked) != 1 || asked[0] != errPath {
		t.Errorf("sampler asked for %v", asked)
	}
	asked = nil
	text := nginxConf("error_log /var/log/nginx/error.log warn;", "error_log /var/log/nginx/error.log warn;", "error_log /var/log/nginx/site.log;")
	runWith(t, cfg("none", nil), text, func(in *Input) {
		in.ErrorSampler = func(path string) errorlog.Counts { asked = append(asked, path); return errorlog.Counts{} }
	})
	if strings.Join(asked, " ") != "/var/log/nginx/error.log /var/log/nginx/site.log" {
		t.Errorf("sampler asked for %v", asked)
	}
}

func TestNoSamplerMeansNoErrorLogSection(t *testing.T) {
	r := withErrorLog(t, "error_log "+errPath+" warn;", nil)
	if len(r.ErrorLogs) != 0 {
		t.Errorf("ErrorLogs = %+v", r.ErrorLogs)
	}
}

func TestAnEmptyOrAbsentLogMakesNoEntryAndNoFinding(t *testing.T) {
	r := withErrorLog(t, "error_log "+errPath+" warn;", func(in *Input) {
		in.ErrorSampler = func(string) errorlog.Counts { return errorlog.Counts{} }
	})
	if len(r.ErrorLogs) != 0 || len(r.Findings) != 0 {
		t.Errorf("ErrorLogs = %+v findings = %+v", r.ErrorLogs, r.Findings)
	}
}

func TestCriticalLinesAreAWarningThatCountsTheLevels(t *testing.T) {
	c := counts()
	c.Levels["crit"], c.Levels["alert"], c.Levels["emerg"] = 3, 1, 0
	r := withErrorLog(t, "error_log "+errPath+" warn;", func(in *Input) {
		in.ErrorSampler = func(string) errorlog.Counts { return c }
	})
	got := find(r, "error-log-critical")
	if len(got) != 1 || got[0].Severity != Warn || got[0].Log != errPath {
		t.Fatalf("findings = %+v", r.Findings)
	}
	for _, want := range []string{"3 crit", "1 alert", "2026-10-05 21:40:12", errPath} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("message lacks %q: %s", want, got[0].Message)
		}
	}
	if strings.Contains(got[0].Message, "emerg") {
		t.Errorf("a level with no lines is named: %s", got[0].Message)
	}
	if got[0].At.Line != 1 {
		t.Errorf("At = %+v, want the error_log line", got[0].At)
	}
	if r.ExitCode() != 0 {
		t.Errorf("ExitCode = %d; critical lines are a warning", r.ExitCode())
	}
}

func TestAnErrorLogThatDoesNotLookLikeOneIsAWarningAndNotCounted(t *testing.T) {
	c := errorlog.Counts{Lines: 100, Skipped: 900, Levels: map[string]int{"error": 100}, Categories: map[string]int{"other": 100}}
	r := withErrorLog(t, "error_log "+errPath+" warn;", func(in *Input) {
		in.ErrorSampler = func(string) errorlog.Counts { return c }
	})
	got := find(r, "error-log-mismatch")
	if len(got) != 1 || got[0].Severity != Warn || !strings.Contains(got[0].Message, "900") {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if len(r.ErrorLogs) != 0 {
		t.Errorf("a mismatched log was counted: %+v", r.ErrorLogs)
	}
	c = errorlog.Counts{Lines: 990, Skipped: 10, Levels: map[string]int{"error": 990}, Categories: map[string]int{"other": 990}}
	r = withErrorLog(t, "error_log "+errPath+" warn;", func(in *Input) {
		in.ErrorSampler = func(string) errorlog.Counts { return c }
	})
	if len(find(r, "error-log-mismatch")) != 0 || len(r.ErrorLogs) != 1 {
		t.Errorf("a few skipped lines are normal: %+v %+v", r.Findings, r.ErrorLogs)
	}
}

func TestAnErrorLogThatCouldNotBeReadIsANote(t *testing.T) {
	r := withErrorLog(t, "error_log "+errPath+" warn;", func(in *Input) {
		in.ErrorSampler = func(string) errorlog.Counts { return errorlog.Counts{Err: "open " + errPath + ": permission denied"} }
	})
	got := find(r, "error-log-unavailable")
	if len(got) != 1 || got[0].Severity != Note || !strings.Contains(got[0].Message, "permission denied") || len(r.ErrorLogs) != 0 {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestTheErrorLogSectionMarshalsToJSON(t *testing.T) {
	r := withErrorLog(t, "error_log "+errPath+" warn;", func(in *Input) {
		in.ErrorSampler = func(string) errorlog.Counts { return counts() }
	})
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		ErrorLogs []struct {
			Path       string         `json:"path"`
			Lines      int            `json:"lines"`
			Levels     map[string]int `json:"levels"`
			Categories []struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
				Zones []struct {
					Zone  string `json:"zone"`
					Count int    `json:"count"`
				} `json:"zones"`
			} `json:"categories"`
		} `json:"error_logs"`
	}
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.ErrorLogs) != 1 || back.ErrorLogs[0].Lines != 3412 || back.ErrorLogs[0].Categories[0].Zones[0].Zone != "api_conc" {
		t.Errorf("json = %s", data)
	}
	// With no sampler the key is present and empty.
	data, _ = json.Marshal(withErrorLog(t, "error_log "+errPath+" warn;", nil))
	if !strings.Contains(string(data), `"error_logs":[]`) {
		t.Errorf("json = %s", data)
	}
}

func TestWriteTextShowsTheErrorLogCounts(t *testing.T) {
	r := withErrorLog(t, "error_log "+errPath+" warn;", func(in *Input) {
		in.ErrorSampler = func(string) errorlog.Counts { return counts() }
	})
	out := render(t, r)
	for _, want := range []string{
		"ERROR LOG " + errPath, "3412 lines", "2026-10-05 21:40:12 to 2026-10-07 14:02:59",
		"limit-conn: 7 (api_conc 5, iiif_conc 2)", "upstream-timeout: 87", "other: 41", "levels: ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "LOG /var/log/nginx/access.log") > strings.Index(out, "ERROR LOG") {
		t.Errorf("the error log is listed before the access log:\n%s", out)
	}
	// The levels read from the most severe down.
	levels := out[strings.Index(out, "levels: "):]
	levels = levels[:strings.Index(levels, "\n")]
	if strings.Index(levels, "error") > strings.Index(levels, "warn") || strings.Index(levels, "warn") > strings.Index(levels, "notice") {
		t.Errorf("levels line = %q", levels)
	}
}

func TestCategoriesFromAnotherTableAreKeptAfterTheKnownOnesAndBeforeOther(t *testing.T) {
	c := counts()
	c.Categories["zzz-custom"], c.Categories["aaa-custom"] = 3, 4
	r := withErrorLog(t, "error_log "+errPath+" warn;", func(in *Input) {
		in.ErrorSampler = func(string) errorlog.Counts { return c }
	})
	var names []string
	for _, cc := range r.ErrorLogs[0].Categories {
		names = append(names, cc.Name)
	}
	if strings.Join(names, " ") != "limit-conn upstream-timeout aaa-custom zzz-custom other" {
		t.Errorf("categories = %v", names)
	}
}

func TestZonesWithTheSameCountAreInNameOrder(t *testing.T) {
	c := counts()
	c.Zones = map[string]map[string]int{"limit-conn": {"b_zone": 3, "a_zone": 3, "c_zone": 9}}
	c.Categories["limit-conn"] = 15
	r := withErrorLog(t, "error_log "+errPath+" warn;", func(in *Input) {
		in.ErrorSampler = func(string) errorlog.Counts { return c }
	})
	var zones []string
	for _, z := range r.ErrorLogs[0].Categories[0].Zones {
		zones = append(zones, z.Zone)
	}
	if strings.Join(zones, " ") != "c_zone a_zone b_zone" {
		t.Errorf("zones = %v", zones)
	}
}
