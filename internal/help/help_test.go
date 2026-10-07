package help

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestPagesAreNamedNameDotChapter(t *testing.T) {
	if len(Pages()) == 0 {
		t.Fatal("no help pages embedded")
	}
	seen := map[string]bool{}
	for _, p := range Pages() {
		if !regexp.MustCompile(`^logagent(-[a-z0-9]+)*$`).MatchString(p.Name) {
			t.Errorf("page name %q is not logagent or logagent-NAME", p.Name)
		}
		if p.Chapter != 1 && p.Chapter != 5 && p.Chapter != 7 {
			t.Errorf("%s: chapter %d is not 1, 5 or 7", p.Name, p.Chapter)
		}
		if seen[p.File()] {
			t.Errorf("duplicate page %s", p.File())
		}
		seen[p.File()] = true
	}
}

func TestEveryPageIsAWellFormedManPage(t *testing.T) {
	for _, p := range Pages() {
		title := "%{app_name}(1) user manual"
		if p.Name != "logagent" {
			title = "%{app_name}" + strings.TrimPrefix(p.Name, "logagent") + "(" + string(rune('0'+p.Chapter)) + ") user manual"
		}
		if !strings.HasPrefix(p.Text, title) {
			t.Errorf("%s: must start with %q", p.File(), title)
		}
		if !strings.Contains(p.Text, "\n# NAME\n") {
			t.Errorf("%s: no # NAME section", p.File())
		}
		out := Expand(p.Text, "logagent", "1.2.3", "2026-01-02", "abc1234")
		for _, m := range []string{"{app_name}", "{version}", "{release_date}", "{release_hash}"} {
			if strings.Contains(out, m) {
				t.Errorf("%s: %s left unexpanded", p.File(), m)
			}
		}
		if !strings.Contains(out, "1.2.3") {
			t.Errorf("%s: version not shown", p.File())
		}
	}
}

func TestPagesSortedChapterThenName(t *testing.T) {
	ps := Pages()
	for i := 1; i < len(ps); i++ {
		a, b := ps[i-1], ps[i]
		if a.Chapter > b.Chapter || (a.Chapter == b.Chapter && a.Name >= b.Name) {
			t.Errorf("%s before %s is out of order", a.File(), b.File())
		}
	}
}

func TestLookup(t *testing.T) {
	cases := []struct{ topic, want string }{
		{"logagent.1", "logagent.1"},  // exact page and chapter
		{"logagent", "logagent.1"},    // page name
		{"check", "logagent-check.1"}, // short topic
		{"logagent-check", "logagent-check.1"},
		{"jsonl", "logagent-jsonl.5"},
		{"tiers", "logagent-tiers.7"},
		{"privacy", "logagent-privacy.7"},
	}
	for _, c := range cases {
		p, err := Lookup(c.topic)
		if err != nil {
			t.Errorf("Lookup(%q): %v", c.topic, err)
			continue
		}
		if p.File() != c.want {
			t.Errorf("Lookup(%q) = %s, want %s", c.topic, p.File(), c.want)
		}
	}
}

func TestLookupUnknownTopic(t *testing.T) {
	for _, topic := range []string{"nosuch", "", "logagent.9", "check.5"} {
		if _, err := Lookup(topic); !errors.Is(err, ErrNotFound) {
			t.Errorf("Lookup(%q) error = %v, want ErrNotFound", topic, err)
		}
	}
}

func TestExpand(t *testing.T) {
	if got := Expand("{app_name} {version} {release_date} {release_hash}", "a", "1", "d", "h"); got != "a 1 d h" {
		t.Errorf("Expand = %q", got)
	}
}

func TestMustTextPanicsOnMissingPage(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustText of a missing page did not panic")
		}
	}()
	MustText("nosuch.1")
}

func TestLookupAmbiguousAcrossChapters(t *testing.T) {
	saved := pages
	defer func() { pages = saved }()
	pages = append(append([]Page(nil), pages...),
		Page{Name: "logagent-dup", Chapter: 1, Text: "x"},
		Page{Name: "logagent-dup", Chapter: 5, Text: "y"})
	if _, err := Lookup("dup"); !errors.Is(err, ErrAmbiguous) || !strings.Contains(err.Error(), "logagent-dup.1") || !strings.Contains(err.Error(), "logagent-dup.5") {
		t.Errorf("Lookup(dup) error = %v, want ErrAmbiguous naming both pages", err)
	}
	if p, err := Lookup("logagent-dup.5"); err != nil || p.Chapter != 5 {
		t.Errorf("exact lookup of a duplicated name failed: %v", err)
	}
}
