package logagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/caltechlibrary/logagent/internal/fields"
)

const (
	capLine  = `2026/10/07 12:00:01 [warn] 1234#1234: *5 limiting connections by zone "api_conc", client: 203.0.113.9, server: a.example, request: "GET /api/x HTTP/1.1", host: "a.example"`
	timeLine = `2026/10/07 12:00:02 [error] 1234#1234: *6 upstream timed out (110: Connection timed out) while reading response header from upstream, client: 203.0.113.9, server: a.example, request: "GET /secret/x HTTP/1.1", upstream: "http://127.0.0.1:8000/secret/x", host: "a.example"`
	critLine = `2026/10/07 12:00:03 [crit] 1234#1234: *7 SSL_do_handshake() failed (SSL: error:0A000126) while SSL handshaking, client: 203.0.113.9, server: 0.0.0.0:443`
)

// errorLogHost writes an error log with the given lines and a configuration and
// dump for a host with no proxy that names it in its main-context error_log.
func errorLogHost(t *testing.T, lines ...string) (cfgPath, errFile string) {
	t.Helper()
	dir := t.TempDir()
	errFile = filepath.Join(dir, "error.log")
	logFile := filepath.Join(dir, "access.log")
	os.WriteFile(errFile, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	dumpText := "error_log " + errFile + " warn;\nhttp {\n" + fields.Default().NginxLogFormat("full") + "\naccess_log " + logFile + " full;\nserver { server_name a.example; }\n}\n"
	dumpPath := filepath.Join(dir, "nginx-T.txt")
	os.WriteFile(dumpPath, []byte(dumpText), 0o600)
	cfgPath = filepath.Join(dir, "logagent.yaml")
	os.WriteFile(cfgPath, []byte("version: 1\nserver: nginx\nconfig:\n  dump: "+dumpPath+"\nlogs:\n  - path: "+logFile+"\n"), 0o600)
	return cfgPath, errFile
}

func TestCheckCountsTheErrorLogByDefault(t *testing.T) {
	cfg, errFile := errorLogHost(t, capLine, capLine, timeLine)
	code, out, errOut := run("check", "--config", cfg)
	if code != ExitOK || errOut != "" {
		t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
	}
	for _, want := range []string{"ERROR LOG " + errFile, "limit-conn: 2 (api_conc 2)", "upstream-timeout: 1", "levels: "} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	for _, leak := range []string{"203.0.113.9", "a.example\"", "/secret", "GET /api", "127.0.0.1:8000"} {
		if strings.Contains(out, leak) {
			t.Errorf("the report holds log content %q:\n%s", leak, out)
		}
	}
}

func TestCheckErrorLogSampleSize(t *testing.T) {
	cfg, _ := errorLogHost(t, capLine, capLine, capLine, timeLine)
	_, out, _ := run("check", "--config", cfg, "--sample", "1")
	if !strings.Contains(out, "1 lines") || !strings.Contains(out, "upstream-timeout: 1") || strings.Contains(out, "limit-conn") {
		t.Errorf("the last line only:\n%s", out)
	}
	_, out, _ = run("check", "--config", cfg, "--sample", "0")
	if strings.Contains(out, "ERROR LOG") {
		t.Errorf("--sample 0 still reads the error log:\n%s", out)
	}
}

func TestCheckSkipsAnErrorLogThatIsNotOnThisMachine(t *testing.T) {
	cfg, errFile := errorLogHost(t, capLine)
	os.Remove(errFile)
	code, out, errOut := run("check", "--config", cfg)
	if code != ExitOK || errOut != "" || strings.Contains(out, "error-log-unavailable") || strings.Contains(out, "ERROR LOG") {
		t.Errorf("exit %d, stderr %q\n%s", code, errOut, out)
	}
}

func TestCheckSaysWhenTheErrorLogCannotBeRead(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions are not enforced here")
	}
	cfg, errFile := errorLogHost(t, capLine)
	os.Chmod(errFile, 0)
	code, out, _ := run("check", "--config", cfg)
	if code != ExitOK || !strings.Contains(out, "[note] error-log-unavailable") || !strings.Contains(out, "permission denied") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestCheckWarnsOfCriticalErrorLogLines(t *testing.T) {
	cfg, _ := errorLogHost(t, capLine, critLine, critLine)
	code, out, _ := run("check", "--config", cfg)
	if code != ExitOK || !strings.Contains(out, "[warn] error-log-critical") || !strings.Contains(out, "2 crit") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestCheckJSONHasTheErrorLogSection(t *testing.T) {
	cfg, errFile := errorLogHost(t, capLine, timeLine)
	_, out, _ := run("check", "--json", "--config", cfg)
	var rep struct {
		ErrorLogs []struct {
			Path  string `json:"path"`
			Lines int    `json:"lines"`
		} `json:"error_logs"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil || len(rep.ErrorLogs) != 1 || rep.ErrorLogs[0].Path != errFile || rep.ErrorLogs[0].Lines != 2 {
		t.Errorf("%v\n%s", err, out)
	}
}
