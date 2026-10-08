package report

import (
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/caltechlibrary/logagent/internal/classify"
	"github.com/caltechlibrary/logagent/internal/logread"
	"github.com/caltechlibrary/logagent/internal/sample"
)

// The analysis scripts were the tier 1 we had. The report must say what they
// say. family_breakdown.expected is bot-family-breakdown.bash's own output for
// the frozen log beside it (see testdata/equivalence/README.md); this test
// compares the report's figures to it, figure for figure.

const eqDir = "testdata/equivalence/"

// authorsRules are the path classes the script had built in.
func authorsRules(t *testing.T) *classify.Classes {
	t.Helper()
	c, err := classify.NewClasses([]classify.Rule{
		{Name: "api-iiif", Prefix: "/api/iiif/"},
		{Name: "api-files", Pattern: `/api/records/[^/]+/(draft/)?files`},
		{Name: "api-versions", Pattern: `/api/records/[^/]+/versions`},
		{Name: "api-communities", Pattern: `/api/records/[^/]+/communities`},
		{Name: "api-record", Pattern: `/api/records/[^/]+$`},
		{Name: "api-search", Prefix: "/api/records"},
		{Name: "api-other", Prefix: "/api"},
		{Name: "ui-files", Pattern: `/records/[^/]+/files`},
		{Name: "ui-record", Pattern: `/records/[^/]+$`},
		{Name: "ui-search", Prefix: "/search"},
		{Name: "static", Pattern: `/(static|assets)/`},
	}, "other")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func reportOf(t *testing.T, w logread.Window) *Report {
	t.Helper()
	p, err := sample.Compile(realFormat)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Collect(eqDir+"access.log", p, Options{
		Window: w, Class: authorsRules(t).Class, Lookup: classify.DefaultFamilies().Lookup,
		MinClients: 1, // the script has no small-cohort fold; this test compares the counts
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// scriptRow is one table row of the script's output: the family, then its numbers.
type scriptRow struct {
	family string
	nums   []float64
}

// rows reads a section of the script's output: the lines after the header line
// that begins with header, up to the first blank line. Each line ends in
// want numbers (a trailing % is dropped); what precedes them is the family.
func rows(t *testing.T, text, section, header string, want int) []scriptRow {
	t.Helper()
	lines := strings.Split(text, "\n")
	var out []scriptRow
	in := false
	for i, l := range lines {
		if strings.HasPrefix(l, section) {
			in = true
			continue
		}
		if !in {
			continue
		}
		if strings.HasPrefix(l, header) {
			_ = i
			continue
		}
		if strings.TrimSpace(l) == "" {
			break
		}
		f := strings.Fields(l)
		// A2 rows end "... s/req  (N new-format reqs)": strip the words.
		var nums []float64
		var name []string
		for _, w := range f {
			w = strings.TrimSuffix(strings.TrimPrefix(w, "("), "%")
			if n, err := strconv.ParseFloat(w, 64); err == nil && len(f)-len(name) <= want+3 {
				nums = append(nums, n)
			} else if len(nums) == 0 {
				name = append(name, w)
			}
		}
		if len(nums) < want {
			t.Fatalf("%s: %q has %d numbers, want %d", section, l, len(nums), want)
		}
		out = append(out, scriptRow{family: strings.Join(name, " "), nums: nums[:want]})
	}
	if len(out) == 0 {
		t.Fatalf("no rows under %q", section)
	}
	return out
}

func familyByName(r *Report) map[string]FamilyRow {
	m := map[string]FamilyRow{}
	for _, f := range r.Families {
		m[f.Family] = f
	}
	return m
}

func within(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestFamilyFiguresMatchTheScriptForTheFrozenLog(t *testing.T) {
	data, err := os.ReadFile(eqDir + "family_breakdown.expected")
	if err != nil {
		t.Fatal(err)
	}
	expected := string(data)
	got := familyByName(reportOf(t, logread.Window{}))

	// Section A: requests, API share, requests not on API classes, upstream seconds
	// and API upstream seconds (the script rounds seconds to whole numbers).
	a := rows(t, expected, "A. per family", "family", 6)
	if len(a) != len(got) {
		t.Errorf("the script has %d families, the report %d: %v", len(a), len(got), keys(got))
	}
	for _, row := range a {
		f, ok := got[row.family]
		if !ok {
			t.Errorf("family %q is in the script's output and not in the report", row.family)
			continue
		}
		requests, share, uiOther, up, apiUp := row.nums[0], row.nums[1], row.nums[2], row.nums[3], row.nums[4]
		switch {
		case float64(f.Requests) != requests:
			t.Errorf("%s: requests %d, script %v", row.family, f.Requests, requests)
		case !within(f.APIShare*100, share, 0.05):
			t.Errorf("%s: API share %.2f%%, script %v%%", row.family, f.APIShare*100, share)
		case float64(f.Requests-f.APIRequests) != uiOther:
			t.Errorf("%s: ui+other %d, script %v", row.family, f.Requests-f.APIRequests, uiOther)
		case !within(f.UpstreamSeconds, up, 0.5):
			t.Errorf("%s: upstream seconds %.2f, script %v", row.family, f.UpstreamSeconds, up)
		case !within(f.APIUpstream, apiUp, 0.5):
			t.Errorf("%s: API upstream seconds %.2f, script %v", row.family, f.APIUpstream, apiUp)
		}
	}

	// Section A2: seconds per request and the number of timed requests.
	a2 := rows(t, expected, "A2. upstream seconds per request", "NO-HEADER", 2)
	if len(a2) != len(got) {
		t.Errorf("A2 has %d rows, the report %d families", len(a2), len(got))
	}
	for _, row := range a2 {
		f, ok := got[row.family]
		if !ok {
			t.Errorf("A2: family %q missing from the report", row.family)
			continue
		}
		if !within(f.PerRequest, row.nums[0], 0.0005) || float64(f.Timed) != row.nums[1] {
			t.Errorf("A2 %s: %.4f s/req over %d timed, script %v s/req over %v", row.family, f.PerRequest, f.Timed, row.nums[0], row.nums[1])
		}
	}
}

func TestTheDayOf07OctoberMatchesTheScriptsRequestsAnd429s(t *testing.T) {
	data, err := os.ReadFile(eqDir + "family_breakdown.expected")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	got := familyByName(reportOf(t, logread.Window{Since: day, Until: day.AddDate(0, 0, 1)}))
	b := rows(t, string(data), "B. 2026-10-07", "family", 3)
	if len(b) != len(got) {
		t.Errorf("B has %d rows, the report %d families", len(b), len(got))
	}
	for _, row := range b {
		f, ok := got[row.family]
		if !ok {
			t.Errorf("B: family %q missing from the report", row.family)
			continue
		}
		if float64(f.Requests) != row.nums[0] || float64(f.Limited) != row.nums[1] || !within(f.LimitedRate*100, row.nums[2], 0.005) {
			t.Errorf("B %s: %d requests, %d limited (%.2f%%); script %v, %v (%v%%)", row.family, f.Requests, f.Limited, f.LimitedRate*100, row.nums[0], row.nums[1], row.nums[2])
		}
	}
}

func keys(m map[string]FamilyRow) []string {
	var k []string
	for name := range m {
		k = append(k, name)
	}
	return k
}
