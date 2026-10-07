package fields

import (
	"strings"
	"testing"

	"github.com/caltechlibrary/logagent/internal/help"
)

// The logagent-fields(5) page is written by hand, so this test keeps it in
// step with the table: every field, with both of its expressions, must appear.
func TestFieldsPageDocumentsEveryField(t *testing.T) {
	p, err := help.Lookup("fields")
	if err != nil {
		t.Fatal(err)
	}
	if p.File() != "logagent-fields.5" {
		t.Fatalf("Lookup(fields) = %s, want logagent-fields.5", p.File())
	}
	for _, f := range Default().Fields {
		if !strings.Contains(p.Text, "`"+f.Name+"`") {
			t.Errorf("page does not document field %s", f.Name)
		}
		if !strings.Contains(p.Text, f.Nginx.Expr) {
			t.Errorf("page lacks the nginx expression of %s (%s)", f.Name, f.Nginx.Expr)
		}
		if f.Apache.Expr != "" && !strings.Contains(p.Text, f.Apache.Expr) {
			t.Errorf("page lacks the Apache expression of %s (%s)", f.Name, f.Apache.Expr)
		}
	}
	for _, level := range []string{Required, Optional, NotApplicable} {
		if !strings.Contains(p.Text, level) {
			t.Errorf("page never mentions %s", level)
		}
	}
	if !strings.Contains(norm(p.Text), norm(Default().NginxLogFormat("caltechauthors_bots"))) {
		t.Error("the log_format shown in the page is not what the table generates")
	}
}
