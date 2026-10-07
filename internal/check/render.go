package check

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

var severityRank = map[string]int{Gap: 0, Warn: 1, Note: 2}

// WriteText writes a report as plain text, with no terminal escape codes: each
// log with its fields counted, the findings from gaps to notes with their
// places and suggestions, and a summary line.
//
// @param w {io.Writer} where to write
// @param r {*Report} the report
// @returns {error} the first write error
// @example
//
//	err := check.WriteText(os.Stdout, report)
func WriteText(w io.Writer, r *Report) error {
	var b strings.Builder
	for _, l := range r.Logs {
		fmt.Fprintf(&b, "LOG %s (format %s)\n", l.Path, l.Format)
		fmt.Fprintf(&b, "  written by: %s\n  access_log at %s\n", strings.Join(l.Servers, ", "), l.At)
		if len(l.Fields) > 0 {
			n := map[string]int{}
			for _, f := range l.Fields {
				n[f.Status]++
			}
			fmt.Fprintf(&b, "  fields: %d ok, %d missing, %d empty\n", n["ok"], n["missing"], n["empty"])
		}
		if l.Suggested != "" {
			b.WriteString("  to add the missing fields:\n")
			writeIndented(&b, l.Suggested, "    ")
		}
		b.WriteString("\n")
	}
	findings := append([]Finding(nil), r.Findings...)
	sort.SliceStable(findings, func(i, j int) bool { return severityRank[findings[i].Severity] < severityRank[findings[j].Severity] })
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Severity]++
		head := "[" + f.Severity + "] " + f.Code
		if f.Field != "" {
			head += " " + f.Field
		}
		fmt.Fprintf(&b, "%s: %s\n", head, f.Message)
		if f.At != (Place{}) {
			fmt.Fprintf(&b, "    at %s\n", f.At)
		}
		if f.Suggestion != "" {
			writeIndented(&b, f.Suggestion, "    ")
		}
	}
	if len(findings) > 0 {
		b.WriteString("\n")
	}
	g, wn, nt := counts[Gap], counts[Warn], counts[Note]
	if g == 0 {
		b.WriteString("No gaps found.")
		if wn+nt > 0 {
			fmt.Fprintf(&b, " %s, %s.", plural(wn, "warning"), plural(nt, "note"))
		}
	} else {
		fmt.Fprintf(&b, "Found %s, %s, %s.", plural(g, "gap"), plural(wn, "warning"), plural(nt, "note"))
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func writeIndented(b *strings.Builder, text, indent string) {
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if l == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(indent + l + "\n")
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
