package nginxconf

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRealConfigurations parses the files named by LOGAGENT_NGINX_FILES, a
// path list separated by the OS list separator, each a saved `nginx -T` dump
// or a single configuration file. It is skipped when the variable is unset, so
// no host's configuration is ever part of the repository. Run it against real
// data to find what the grammar still lacks:
//
//	LOGAGENT_NGINX_FILES=/path/a.conf:/path/b.txt go test -run RealConfigurations -v ./internal/nginxconf
func TestRealConfigurations(t *testing.T) {
	list := os.Getenv("LOGAGENT_NGINX_FILES")
	if list == "" {
		t.Skip("LOGAGENT_NGINX_FILES is not set")
	}
	for _, p := range filepath.SplitList(list) {
		if p == "" {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		d, err := Parse(string(src))
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		n := 0
		for _, f := range d.Files {
			Walk(f.Directives, func(*Directive) { n++ })
		}
		t.Logf("%s: %d file(s), %d directives", p, len(d.Files), n)
	}
}
