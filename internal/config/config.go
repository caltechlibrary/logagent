// Package config reads logagent's per-host configuration file, a YAML file
// that is decoded strictly and validated before anything uses it. A
// configuration that is present but wrong is an error (workspace exit status
// 78), and one that cannot be found is a different error (66).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

var (
	// ErrNoConfig means no configuration file could be found or read because
	// it does not exist. It corresponds to exit status 66.
	ErrNoConfig = errors.New("no configuration file")

	// ErrInvalid means a configuration file exists and is wrong: not YAML, an
	// unknown key, a bad value. It corresponds to exit status 78.
	ErrInvalid = errors.New("invalid configuration")
)

const (
	// EnvVar is the environment variable that names the configuration file.
	EnvVar = "LOGAGENT_CONFIG"

	// SchemaVersion is the only value of the file's `version` key this build
	// understands.
	SchemaVersion = 1
)

// DefaultPath is where Locate looks when neither --config nor EnvVar names a
// file. It is a variable so tests can point it elsewhere.
var DefaultPath = "/etc/logagent/logagent.yaml"

// Config is the contents of a logagent configuration file. See the
// logagent-config(5) manual page for what each key means.
type Config struct {
	Version       int               `yaml:"version"`
	Host          string            `yaml:"host"`
	Server        string            `yaml:"server"`
	Source        Source            `yaml:"config"`
	Logs          []Log             `yaml:"logs"`
	Proxy         Proxy             `yaml:"proxy"`
	InternalCIDRs []string          `yaml:"internal_ranges"`
	Retention     Retention         `yaml:"retention"`
	Fields        map[string]string `yaml:"fields"`

	internal []netip.Prefix
	trusted  []netip.Prefix
}

// Source says where the web server's configuration comes from: a saved
// `nginx -T` dump, or a command to run. If neither is given for an nginx host
// the command defaults to `nginx -T`.
type Source struct {
	Dump    string `yaml:"dump"`
	Command string `yaml:"command"`
}

// Log is one access log to read.
type Log struct {
	Path string `yaml:"path"`
}

// Proxy describes what sits in front of the web server.
type Proxy struct {
	Behind        string   `yaml:"behind"`
	RealIPHeader  string   `yaml:"real_ip_header"`
	TrustedRanges []string `yaml:"trusted_ranges"`
}

// Retention holds the retention policy, in days, for the three layers of data
// and the path of the logrotate file that governs layer 1.
type Retention struct {
	Logrotate     string `yaml:"logrotate"`
	Layer1MinDays int    `yaml:"layer1_min_days"`
	Layer1MaxDays int    `yaml:"layer1_max_days"`
	Layer2Days    int    `yaml:"layer2_days"`
	Layer3Days    int    `yaml:"layer3_days"`
}

// InternalRanges returns the configured internal ranges: addresses that belong
// to the institution, counted but never stored in detail or targeted.
//
// @returns {[]netip.Prefix} the parsed ranges, empty if none are configured
// @example
//
//	for _, p := range cfg.InternalRanges() {
//		fmt.Println(p)
//	}
func (c *Config) InternalRanges() []netip.Prefix {
	return append([]netip.Prefix(nil), c.internal...)
}

// TrustedRanges returns proxy.trusted_ranges, which override the module's
// snapshot of the proxy's published ranges.
//
// @returns {[]netip.Prefix} the parsed ranges, empty if none are configured
// @example
//
//	fmt.Println(len(cfg.TrustedRanges()))
func (c *Config) TrustedRanges() []netip.Prefix {
	return append([]netip.Prefix(nil), c.trusted...)
}

// Locate finds the configuration file: the path given with --config, then the
// path in the LOGAGENT_CONFIG environment variable, then DefaultPath. A path
// that was named explicitly must exist; Locate does not fall back from a typo.
//
// @param flagPath {string} the --config value, or "" if not given
// @param getenv {func(string) string} reads an environment variable, usually os.Getenv
// @returns {string} the path to read
// @returns {error} ErrNoConfig, naming every place looked, if there is no file
// @example
//
//	path, err := config.Locate(*configFlag, os.Getenv)
func Locate(flagPath string, getenv func(string) string) (string, error) {
	if flagPath != "" {
		if !exists(flagPath) {
			return "", fmt.Errorf("%w: %s (named with --config) does not exist", ErrNoConfig, flagPath)
		}
		return flagPath, nil
	}
	if p := getenv(EnvVar); p != "" {
		if !exists(p) {
			return "", fmt.Errorf("%w: %s (named by $%s) does not exist", ErrNoConfig, p, EnvVar)
		}
		return p, nil
	}
	if exists(DefaultPath) {
		return DefaultPath, nil
	}
	return "", fmt.Errorf("%w: looked at --config (not given), $%s (not set) and %s (absent)", ErrNoConfig, EnvVar, DefaultPath)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Load reads and validates a configuration file. It decodes strictly, so an
// unknown key is an error, fills in defaults for keys left out, and checks
// every value. Errors name the file and the key.
//
// @param path {string} the configuration file
// @returns {*Config} the validated configuration
// @returns {error} ErrNoConfig if the file does not exist, ErrInvalid if it is wrong
// @example
//
//	cfg, err := config.Load("/etc/logagent/logagent.yaml")
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %v", ErrNoConfig, err)
		}
		return nil, err
	}
	c := &Config{
		Proxy: Proxy{Behind: "none"},
		Retention: Retention{
			Layer1MinDays: 14,
			Layer1MaxDays: 90,
			Layer2Days:    28,
			Layer3Days:    730,
		},
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalid, path, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalid, path, err)
	}
	return c, nil
}

// validate checks every value, fills the defaults that depend on other values,
// and parses the address ranges.
func (c *Config) validate() error {
	if c.Version != SchemaVersion {
		return fmt.Errorf("version: required, and must be %d (got %d)", SchemaVersion, c.Version)
	}
	switch c.Server {
	case "nginx", "apache":
	default:
		return fmt.Errorf("server: must be nginx or apache (got %q)", c.Server)
	}
	if len(c.Logs) == 0 {
		return errors.New("logs: at least one log is required")
	}
	for i, l := range c.Logs {
		if l.Path == "" {
			return fmt.Errorf("logs[%d]: path is required", i)
		}
	}
	if c.Source.Dump == "" && c.Source.Command == "" && c.Server == "nginx" {
		c.Source.Command = "nginx -T"
	}
	switch c.Proxy.Behind {
	case "none":
	case "cloudflare":
		if c.Proxy.RealIPHeader == "" {
			c.Proxy.RealIPHeader = "CF-Connecting-IP"
		}
	default:
		return fmt.Errorf("proxy.behind: must be none or cloudflare (got %q)", c.Proxy.Behind)
	}
	var err error
	if c.internal, err = parseRanges("internal_ranges", c.InternalCIDRs); err != nil {
		return err
	}
	if c.trusted, err = parseRanges("proxy.trusted_ranges", c.Proxy.TrustedRanges); err != nil {
		return err
	}
	r := c.Retention
	for _, d := range []struct {
		key  string
		days int
	}{
		{"retention.layer1_min_days", r.Layer1MinDays},
		{"retention.layer1_max_days", r.Layer1MaxDays},
		{"retention.layer2_days", r.Layer2Days},
		{"retention.layer3_days", r.Layer3Days},
	} {
		if d.days < 1 {
			return fmt.Errorf("%s: must be at least 1 day (got %d)", d.key, d.days)
		}
	}
	if r.Layer1MinDays > r.Layer1MaxDays {
		return fmt.Errorf("retention.layer1_min_days (%d) is above retention.layer1_max_days (%d)", r.Layer1MinDays, r.Layer1MaxDays)
	}
	names := make([]string, 0, len(c.Fields))
	for name := range c.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		switch c.Fields[name] {
		case "required", "optional", "not_applicable":
		default:
			return fmt.Errorf("fields.%s: must be required, optional or not_applicable (got %q)", name, c.Fields[name])
		}
	}
	return nil
}

// parseRanges parses a list of CIDR ranges. A bare address, an unparseable
// value, or a range with host bits set (usually a typo) is an error that names
// the key and the position.
func parseRanges(key string, in []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for i, s := range in {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %q is not a CIDR range such as 10.0.0.0/8", key, i, s)
		}
		if p != p.Masked() {
			return nil, fmt.Errorf("%s[%d]: %q has host bits set; did you mean %q?", key, i, s, p.Masked().String())
		}
		out = append(out, p)
	}
	return out, nil
}
