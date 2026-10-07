package logagent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// logrotateHost is a host as sampleHost makes it, plus a retention.logrotate
// key naming <dir>/logrotate.d/nginx, which holds a stanza for its log with the
// given options. A non-empty defaults is written to <dir>/logrotate.conf.
func logrotateHost(t *testing.T, opts, defaults string) (cfgPath, lrFile string) {
	t.Helper()
	cfgPath, logFile := sampleHost(t, []string{"-"})
	dir := filepath.Join(filepath.Dir(cfgPath), "etc")
	os.MkdirAll(filepath.Join(dir, "logrotate.d"), 0o755)
	lrFile = filepath.Join(dir, "logrotate.d", "nginx")
	os.WriteFile(lrFile, []byte(logFile+" {\n"+opts+"\n}\n"), 0o600)
	if defaults != "" {
		os.WriteFile(filepath.Join(dir, "logrotate.conf"), []byte(defaults), 0o600)
	}
	f, _ := os.OpenFile(cfgPath, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("retention:\n  logrotate: " + lrFile + "\n")
	f.Close()
	return cfgPath, lrFile
}

func TestCheckLogrotateRetentionWithinPolicy(t *testing.T) {
	cfg, _ := logrotateHost(t, "daily\nrotate 14", "")
	code, out, errOut := run("check", "--config", cfg)
	if code != ExitOK || errOut != "" || strings.Contains(out, "logrotate") {
		t.Errorf("exit %d, stderr %q\n%s", code, errOut, out)
	}
}

func TestCheckLogrotateRetentionTooShortExitsOne(t *testing.T) {
	cfg, lr := logrotateHost(t, "daily\nrotate 5", "")
	code, out, _ := run("check", "--config", cfg)
	if code != 1 || !strings.Contains(out, "[gap] logrotate-retention-short") || !strings.Contains(out, lr+":1") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestCheckReadsGlobalDefaultsFromLogrotateConfNextToTheDirectory(t *testing.T) {
	cfg, _ := logrotateHost(t, "daily", "weekly\nrotate 14\n")
	if code, out, _ := run("check", "--config", cfg); code != ExitOK || strings.Contains(out, "logrotate") {
		t.Errorf("rotate 14 from logrotate.conf: exit %d\n%s", code, out)
	}
	cfg, _ = logrotateHost(t, "daily", "weekly\nrotate 3\n")
	if code, out, _ := run("check", "--config", cfg); code != 1 || !strings.Contains(out, "logrotate-retention-short") {
		t.Errorf("rotate 3 from logrotate.conf: exit %d\n%s", code, out)
	}
	// A named file that is not in a logrotate.d directory brings no second file in.
	cfg, lr := logrotateHost(t, "daily", "weekly\nrotate 14\n")
	moved := filepath.Join(filepath.Dir(filepath.Dir(lr)), "other", "nginx.logrotate")
	os.MkdirAll(filepath.Dir(moved), 0o755)
	os.Rename(lr, moved)
	data, _ := os.ReadFile(cfg)
	os.WriteFile(cfg, []byte(strings.Replace(string(data), lr, moved, 1)), 0o600)
	if _, out, _ := run("check", "--config", cfg); !strings.Contains(out, "logrotate-retention-short") {
		t.Errorf("only logrotate.d files read the global defaults:\n%s", out)
	}
}

func TestCheckSkipsALogrotateFileThatIsNotOnThisMachine(t *testing.T) {
	cfg, lr := logrotateHost(t, "daily\nrotate 1", "")
	os.Remove(lr)
	code, out, errOut := run("check", "--config", cfg)
	if code != ExitOK || errOut != "" || strings.Contains(out, "logrotate") {
		t.Errorf("exit %d, stderr %q\n%s", code, errOut, out)
	}
}

func TestCheckSaysWhenTheLogrotateFileCannotBeUsed(t *testing.T) {
	cfg, lr := logrotateHost(t, "daily\nrotate lots", "")
	code, out, _ := run("check", "--config", cfg)
	if code != ExitOK || !strings.Contains(out, "[warn] logrotate-unreadable") || !strings.Contains(out, lr+":3") {
		t.Errorf("syntax error: exit %d\n%s", code, out)
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return
	}
	cfg, lr = logrotateHost(t, "daily\nrotate 14", "")
	os.Chmod(lr, 0)
	if code, out, _ := run("check", "--config", cfg); code != ExitOK || !strings.Contains(out, "[warn] logrotate-unreadable") || !strings.Contains(out, "permission denied") {
		t.Errorf("permission: exit %d\n%s", code, out)
	}
}

func TestCheckWithNoLogrotateKeyDoesNotLookAtLogrotate(t *testing.T) {
	cfg, _ := sampleHost(t, []string{"-"})
	if _, out, _ := run("check", "--config", cfg); strings.Contains(out, "logrotate") {
		t.Errorf("output:\n%s", out)
	}
}
