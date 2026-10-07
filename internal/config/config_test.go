package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write puts content in a new file in a temporary directory and returns its path.
func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "logagent.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const minimal = `
version: 1
server: nginx
logs:
  - path: /var/log/nginx/access.log
`

func TestLoadMinimalAppliesDefaults(t *testing.T) {
	c, err := Load(write(t, minimal))
	if err != nil {
		t.Fatal(err)
	}
	if c.Source.Command != "nginx -T" {
		t.Errorf("default config command = %q, want %q", c.Source.Command, "nginx -T")
	}
	if c.Proxy.Behind != "none" {
		t.Errorf("default proxy.behind = %q, want none", c.Proxy.Behind)
	}
	r := c.Retention
	if r.Layer1MinDays != 14 || r.Layer1MaxDays != 90 || r.Layer2Days != 28 || r.Layer3Days != 730 {
		t.Errorf("default retention = %+v", r)
	}
	if len(c.InternalRanges()) != 0 {
		t.Errorf("internal ranges must have no built-in default, got %v", c.InternalRanges())
	}
}

func TestLoadFullConfiguration(t *testing.T) {
	c, err := Load(write(t, `
version: 1
host: example-host
server: nginx
config:
  dump: /var/lib/logagent/nginx-T.txt
logs:
  - path: /var/log/nginx/access.log
  - path: /var/log/nginx/other.log
proxy:
  behind: cloudflare
  trusted_ranges: ["173.245.48.0/20", "2400:cb00::/32"]
internal_ranges:
  - "131.215.0.0/16"
  - "2001:db8::/32"
retention:
  logrotate: /etc/logrotate.d/nginx
  layer2_days: 14
fields:
  bot_score: optional
  ja3: not_applicable
  cf_ray: required
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "example-host" || c.Source.Dump == "" || len(c.Logs) != 2 {
		t.Errorf("unexpected values: %+v", c)
	}
	if c.Proxy.RealIPHeader != "CF-Connecting-IP" {
		t.Errorf("cloudflare must default real_ip_header, got %q", c.Proxy.RealIPHeader)
	}
	if c.Source.Command != "" {
		t.Errorf("a dump was given, so no default command is wanted, got %q", c.Source.Command)
	}
	if len(c.InternalRanges()) != 2 || c.InternalRanges()[0].String() != "131.215.0.0/16" {
		t.Errorf("internal ranges = %v", c.InternalRanges())
	}
	if len(c.TrustedRanges()) != 2 {
		t.Errorf("trusted ranges = %v", c.TrustedRanges())
	}
	if c.Retention.Layer2Days != 14 || c.Retention.Layer3Days != 730 {
		t.Errorf("a given value must win and the rest default: %+v", c.Retention)
	}
	if c.Fields["bot_score"] != "optional" {
		t.Errorf("fields = %v", c.Fields)
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	cases := []struct {
		name, yaml, mention string
	}{
		{"empty file", "", "version"},
		{"no version", "server: nginx\nlogs:\n  - path: /x\n", "version"},
		{"future version", "version: 2\nserver: nginx\nlogs:\n  - path: /x\n", "version"},
		{"unknown top-level key", minimal + "internal_range: [\"10.0.0.0/8\"]\n", "internal_range"},
		{"unknown nested key", minimal + "retention:\n  layer2_day: 5\n", "layer2_day"},
		{"bad server", "version: 1\nserver: caddy\nlogs:\n  - path: /x\n", "server"},
		{"no server", "version: 1\nlogs:\n  - path: /x\n", "server"},
		{"no logs", "version: 1\nserver: nginx\n", "logs"},
		{"log without path", "version: 1\nserver: nginx\nlogs:\n  - {}\n", "logs[0]"},
		{"bad proxy", minimal + "proxy:\n  behind: fastly\n", "proxy.behind"},
		{"bad cidr", minimal + "internal_ranges:\n  - \"131.215.0.0/16\"\n  - \"131.215.0/16\"\n", "internal_ranges[1]"},
		{"bare address", minimal + "internal_ranges:\n  - \"10.1.2.3\"\n", "internal_ranges[0]"},
		{"host bits set", minimal + "internal_ranges:\n  - \"131.215.0.1/16\"\n", "131.215.0.0/16"},
		{"bad trusted range", minimal + "proxy:\n  behind: cloudflare\n  trusted_ranges: [\"nope\"]\n", "proxy.trusted_ranges[0]"},
		{"zero retention", minimal + "retention:\n  layer2_days: 0\n", "retention.layer2_days"},
		{"negative retention", minimal + "retention:\n  layer3_days: -1\n", "retention.layer3_days"},
		{"min above max", minimal + "retention:\n  layer1_min_days: 100\n  layer1_max_days: 90\n", "layer1_min_days"},
		{"unknown field name", minimal + "fields:\n  bot_scor: optional\n", "fields.bot_scor"},
		{"bad field setting", minimal + "fields:\n  bot_score: maybe\n", "fields.bot_score"},
		{"not yaml", "version: [1\n", ""},
	}
	for _, c := range cases {
		p := write(t, c.yaml)
		_, err := Load(p)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", c.name, err)
			continue
		}
		if c.mention != "" && !strings.Contains(err.Error(), c.mention) {
			t.Errorf("%s: error %q does not mention %q", c.name, err, c.mention)
		}
		if !strings.Contains(err.Error(), p) {
			t.Errorf("%s: error %q does not name the file", c.name, err)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nothing.yaml"))
	if !errors.Is(err, ErrNoConfig) {
		t.Errorf("error = %v, want ErrNoConfig", err)
	}
}

func TestExampleConfigurationLoads(t *testing.T) {
	c, err := Load("../../examples/logagent.yaml")
	if err != nil {
		t.Fatalf("examples/logagent.yaml must always load: %v", err)
	}
	if len(c.InternalRanges()) == 0 {
		t.Error("the example should show internal_ranges")
	}
}

func TestLocatePrecedence(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(minimal), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	flagFile, envFile, defFile := mk("flag.yaml"), mk("env.yaml"), mk("default.yaml")
	saved := DefaultPath
	defer func() { DefaultPath = saved }()
	DefaultPath = defFile
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == EnvVar {
				return v
			}
			return ""
		}
	}

	if got, err := Locate(flagFile, env(envFile)); err != nil || got != flagFile {
		t.Errorf("flag must win: %q, %v", got, err)
	}
	if got, err := Locate("", env(envFile)); err != nil || got != envFile {
		t.Errorf("env must beat the default: %q, %v", got, err)
	}
	if got, err := Locate("", env("")); err != nil || got != defFile {
		t.Errorf("default expected: %q, %v", got, err)
	}
}

func TestLocateFailures(t *testing.T) {
	dir := t.TempDir()
	saved := DefaultPath
	defer func() { DefaultPath = saved }()
	DefaultPath = filepath.Join(dir, "absent-default.yaml")
	noEnv := func(string) string { return "" }

	// A path named on the command line must exist; falling back would hide a typo.
	_, err := Locate(filepath.Join(dir, "typo.yaml"), noEnv)
	if !errors.Is(err, ErrNoConfig) || !strings.Contains(err.Error(), "typo.yaml") {
		t.Errorf("missing flag path: %v", err)
	}
	// The same for the environment variable.
	_, err = Locate("", func(string) string { return filepath.Join(dir, "envtypo.yaml") })
	if !errors.Is(err, ErrNoConfig) || !strings.Contains(err.Error(), "envtypo.yaml") {
		t.Errorf("missing env path: %v", err)
	}
	// With nothing given, the message lists every place it looked.
	_, err = Locate("", noEnv)
	if !errors.Is(err, ErrNoConfig) || !strings.Contains(err.Error(), "absent-default.yaml") || !strings.Contains(err.Error(), EnvVar) {
		t.Errorf("nothing found: %v", err)
	}
}

func TestRangesMaxAgeDays(t *testing.T) {
	c, err := Load(write(t, minimal))
	if err != nil {
		t.Fatal(err)
	}
	if c.Proxy.RangesMaxAgeDays != 90 {
		t.Errorf("default proxy.ranges_max_age_days = %d, want 90", c.Proxy.RangesMaxAgeDays)
	}
	c, err = Load(write(t, minimal+"proxy:\n  behind: cloudflare\n  ranges_max_age_days: 30\n"))
	if err != nil || c.Proxy.RangesMaxAgeDays != 30 {
		t.Errorf("explicit value: %v, %d", err, c.Proxy.RangesMaxAgeDays)
	}
	for _, v := range []string{"0", "-5"} {
		_, err := Load(write(t, minimal+"proxy:\n  ranges_max_age_days: "+v+"\n"))
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "proxy.ranges_max_age_days") {
			t.Errorf("ranges_max_age_days %s: error = %v", v, err)
		}
	}
}
