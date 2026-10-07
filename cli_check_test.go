package logagent

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/caltechlibrary/logagent/internal/config"
	"github.com/caltechlibrary/logagent/internal/fields"
	"github.com/caltechlibrary/logagent/internal/help"
)

const checkLog = "/var/log/nginx/access.log"

// goodDump is a host whose log format is the field table's: nothing missing.
var goodDump = "http {\n" + fields.Default().NginxLogFormat("full") + "\naccess_log " + checkLog + " full;\nserver { server_name a.example; }\n}\n"

// gapDump logs with the stock combined format, so required fields are missing.
const gapDump = "http {\naccess_log " + checkLog + " combined;\nserver { server_name a.example; }\n}\n"

// setup writes a configuration and a dump into a temporary directory and
// returns their paths.
func setup(t *testing.T, dumpText, extraYAML string) (cfgPath, dumpPath string) {
	t.Helper()
	dir := t.TempDir()
	dumpPath = filepath.Join(dir, "nginx-T.txt")
	if err := os.WriteFile(dumpPath, []byte(dumpText), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath = filepath.Join(dir, "logagent.yaml")
	yaml := "version: 1\nserver: nginx\nconfig:\n  dump: " + dumpPath + "\nlogs:\n  - path: " + checkLog + "\n" + extraYAML
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath, dumpPath
}

func TestCheckCleanHostExitsZero(t *testing.T) {
	cfg, _ := setup(t, goodDump, "")
	code, out, errOut := run("check", "--config", cfg)
	if code != ExitOK || errOut != "" {
		t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
	}
	if !strings.Contains(out, "No gaps found") || !strings.Contains(out, checkLog) {
		t.Errorf("stdout:\n%s", out)
	}
}

func TestCheckGapsExitOneAndAreListedWithSuggestions(t *testing.T) {
	cfg, _ := setup(t, gapDump, "")
	code, out, errOut := run("check", "--config", cfg)
	if code != 1 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	for _, want := range []string{"[gap]", "field-missing", "rt", "urt", "[note]", "log_format", "gaps", "notes"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("output contains terminal escape codes")
	}
}

func TestCheckJSON(t *testing.T) {
	cfg, _ := setup(t, gapDump, "")
	for _, flag := range []string{"--json", "-j"} {
		code, out, errOut := run("check", flag, "--config", cfg)
		if code != 1 || errOut != "" {
			t.Fatalf("%s: exit %d, stderr %q", flag, code, errOut)
		}
		var rep struct {
			Logs       []map[string]any `json:"logs"`
			Findings   []map[string]any `json:"findings"`
			ExitStatus int              `json:"exit_status"`
		}
		if err := json.Unmarshal([]byte(out), &rep); err != nil {
			t.Fatalf("%s: stdout is not JSON: %v\n%s", flag, err, out)
		}
		if len(rep.Logs) != 1 || len(rep.Findings) == 0 || rep.ExitStatus != 1 {
			t.Errorf("%s: %s", flag, out)
		}
	}
}

func TestCheckDumpFlagOverridesTheConfiguredDump(t *testing.T) {
	cfg, _ := setup(t, gapDump, "")
	_, good := setup(t, goodDump, "")
	code, _, errOut := run("check", "--config", cfg, "--dump", good)
	if code != ExitOK {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestCheckFindsTheConfigurationByEnvironment(t *testing.T) {
	cfg, _ := setup(t, goodDump, "")
	t.Setenv(config.EnvVar, cfg)
	if code, _, errOut := run("check"); code != ExitOK {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

// One test per documented exit code, as the workspace convention asks.
func TestCheckExitCodes(t *testing.T) {
	cfg, dumpPath := setup(t, goodDump, "")
	dir := t.TempDir()
	badCfg := filepath.Join(dir, "bad.yaml")
	os.WriteFile(badCfg, []byte("version: 1\nserver: nginx\nbogus: 1\nlogs:\n  - path: /x\n"), 0o600)
	apache := filepath.Join(dir, "apache.yaml")
	os.WriteFile(apache, []byte("version: 1\nserver: apache\nconfig:\n  dump: "+dumpPath+"\nlogs:\n  - path: /x\n"), 0o600)
	dirAsDump := dir
	badSyntax := filepath.Join(dir, "syntax.txt")
	os.WriteFile(badSyntax, []byte("http {\nlisten 80\n}\n"), 0o600)
	badInclude := filepath.Join(dir, "include.txt")
	os.WriteFile(badInclude, []byte("http { include missing.conf; server { } }\n"), 0o600)
	noServers := filepath.Join(dir, "noservers.txt")
	os.WriteFile(noServers, []byte("events {}\nhttp { }\n"), 0o600)
	savedDefault := config.DefaultPath
	defer func() { config.DefaultPath = savedDefault }()
	config.DefaultPath = filepath.Join(dir, "absent.yaml")
	t.Setenv(config.EnvVar, "")

	cases := []struct {
		name string
		args []string
		code int
	}{
		{"clean", []string{"check", "--config", cfg}, 0},
		{"unknown flag", []string{"check", "--no-such-flag"}, 2},
		{"surplus argument", []string{"check", "--config", cfg, "extra"}, 2},
		{"flag missing its value", []string{"check", "--config"}, 2},
		{"dump flag missing its value", []string{"check", "--dump"}, 2},
		{"no configuration anywhere", []string{"check"}, 66},
		{"named configuration absent", []string{"check", "--config", filepath.Join(dir, "nope.yaml")}, 66},
		{"dump absent", []string{"check", "--config", cfg, "--dump", filepath.Join(dir, "nope.txt")}, 66},
		{"configuration is wrong", []string{"check", "--config", badCfg}, 78},
		{"dump has a syntax error", []string{"check", "--config", cfg, "--dump", badSyntax}, 65},
		{"dump has an unresolvable include", []string{"check", "--config", cfg, "--dump", badInclude}, 65},
		{"dump has no servers", []string{"check", "--config", cfg, "--dump", noServers}, 65},
		{"dump is a directory", []string{"check", "--config", cfg, "--dump", dirAsDump}, 74},
		{"apache is unsupported", []string{"check", "--config", apache}, 1},
	}
	for _, c := range cases {
		code, out, errOut := run(c.args...)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d (stderr %q)", c.name, code, c.code, errOut)
		}
		if c.code >= 2 && out != "" {
			t.Errorf("%s: a failure wrote to stdout: %q", c.name, out)
		}
		if c.code >= 2 && errOut == "" {
			t.Errorf("%s: a failure wrote nothing to stderr", c.name)
		}
	}
	if code, _, errOut := run("check", "--config", cfg, "--dump", noServers); !strings.Contains(errOut, "server") || code != 65 {
		t.Errorf("no-servers message %q", errOut)
	}
}

func TestCheckPermissionRefusedExitsSeventySeven(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions are not enforced here")
	}
	cfg, dumpPath := setup(t, goodDump, "")
	if err := os.Chmod(dumpPath, 0); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("check", "--config", cfg); code != 77 {
		t.Errorf("exit %d, want 77 (stderr %q)", code, errOut)
	}
}

func TestCheckErrorsAreJSONOnStderrInJSONMode(t *testing.T) {
	code, out, errOut := run("check", "--json", "--config", filepath.Join(t.TempDir(), "nope.yaml"))
	if code != 66 || out != "" {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
	var e struct {
		Error struct {
			Class   string `json:"class"`
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errOut), &e); err != nil {
		t.Fatalf("stderr is not JSON: %v\n%s", err, errOut)
	}
	if e.Error.Class != "no_input" || e.Error.Code != 66 || e.Error.Message == "" {
		t.Errorf("error object = %+v", e.Error)
	}
}

// With no dump configured the dump comes from running the command, which
// defaults to nginx -T.
func TestCheckRunsTheConfiguredCommandWhenThereIsNoDump(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "logagent.yaml")
	os.WriteFile(cfg, []byte("version: 1\nserver: nginx\nlogs:\n  - path: "+checkLog+"\n"), 0o600)
	saved := commandRunner
	defer func() { commandRunner = saved }()
	var gotArgs []string
	commandRunner = func(name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte(goodDump), nil
	}
	if code, _, errOut := run("check", "--config", cfg); code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if strings.Join(gotArgs, " ") != "nginx -T" {
		t.Errorf("ran %v, want nginx -T", gotArgs)
	}
	// A command named in the configuration is split on spaces and not run by a shell.
	os.WriteFile(cfg, []byte("version: 1\nserver: nginx\nconfig:\n  command: sudo nginx -T\nlogs:\n  - path: "+checkLog+"\n"), 0o600)
	run("check", "--config", cfg)
	if strings.Join(gotArgs, " ") != "sudo nginx -T" {
		t.Errorf("ran %v", gotArgs)
	}
}

func TestCheckCommandFailures(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "logagent.yaml")
	os.WriteFile(cfg, []byte("version: 1\nserver: nginx\nlogs:\n  - path: "+checkLog+"\n"), 0o600)
	saved := commandRunner
	defer func() { commandRunner = saved }()
	cases := []struct {
		name string
		out  string
		err  error
		code int
	}{
		{"binary not found", "", exec.ErrNotFound, 66},
		{"permission refused", "", os.ErrPermission, 77},
		{"unclassified", "", errors.New("odd"), 70},
	}
	if runtime.GOOS != "windows" {
		// nginx -T exits non-zero when it finds its own configuration invalid.
		ee := exec.Command("sh", "-c", "exit 1").Run()
		cases = append(cases, struct {
			name string
			out  string
			err  error
			code int
		}{"the command exits non-zero", "nginx: [emerg] unknown directive \"x\" in /etc/nginx/nginx.conf:3", ee, 65})
	}
	for _, c := range cases {
		commandRunner = func(string, ...string) ([]byte, error) { return []byte(c.out), c.err }
		code, _, errOut := run("check", "--config", cfg)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d (stderr %q)", c.name, code, c.code, errOut)
		}
		if c.out != "" && !strings.Contains(errOut, "unknown directive") {
			t.Errorf("%s: the command's own message was lost: %q", c.name, errOut)
		}
	}
}

func TestCheckHelpAndItsExitStatusSection(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		code, out, _ := run("check", flag)
		if code != ExitOK || !strings.Contains(out, "# EXIT STATUS") {
			t.Errorf("check %s: exit %d\n%s", flag, code, out)
		}
	}
	p, err := help.Lookup("check")
	if err != nil {
		t.Fatal(err)
	}
	section := p.Text[strings.Index(p.Text, "# EXIT STATUS"):]
	// Every code the command can return is documented, and no other.
	for _, code := range []string{"0", "1", "2", "65", "66", "70", "74", "77", "78"} {
		if !strings.Contains(section, "\n"+code+"\n:") {
			t.Errorf("EXIT STATUS lacks code %s", code)
		}
	}
	for _, opt := range []string{"--config", "--dump", "--json", "-j", "--help"} {
		if !strings.Contains(p.Text, opt) {
			t.Errorf("page does not describe %s", opt)
		}
	}
	if strings.Contains(p.Text, "planned and is not implemented") {
		t.Error("the page still says the command is not implemented")
	}
}

func TestCheckShortOptionsCluster(t *testing.T) {
	cfg, dumpPath := setup(t, goodDump, "")
	for _, args := range [][]string{
		{"check", "-jc", cfg},
		{"check", "-j", "-c", cfg},
		{"check", "-c" + cfg},
		{"check", "--config=" + cfg, "-jd" + dumpPath},
		{"check", "-config", cfg, "-json"},
	} {
		if code, out, errOut := run(args...); code != ExitOK {
			t.Errorf("%v: exit %d, stderr %q\n%s", args, code, errOut, out)
		}
	}
	for _, args := range [][]string{{"check", "-jx"}, {"check", "-j=1"}, {"check", "--json=1"}, {"check", "-"}} {
		if code, _, _ := run(args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

func TestCheckApacheIsUnsupportedEvenWithNoDumpSource(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "logagent.yaml")
	os.WriteFile(cfg, []byte("version: 1\nserver: apache\nlogs:\n  - path: /x\n"), 0o600)
	code, _, errOut := run("check", "--config", cfg)
	if code != 1 || !strings.Contains(errOut, "unsupported") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}
