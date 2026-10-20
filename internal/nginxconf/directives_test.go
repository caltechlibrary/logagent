package nginxconf

import "testing"

func TestIsServerDirective(t *testing.T) {
	for _, w := range []string{"add_header", "listen", "location", "return"} {
		if !IsServerDirective(w) {
			t.Errorf("%q is a server directive", w)
		}
	}
	for _, w := range []string{"data.example.org", "_", "*.example.org", "max-age=15768000", ""} {
		if IsServerDirective(w) {
			t.Errorf("%q is not a server directive", w)
		}
	}
}
