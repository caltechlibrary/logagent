package logread

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/caltechlibrary/logagent/internal/nginxconf"
	"github.com/caltechlibrary/logagent/internal/sample"
)

// testFormat is a small log_format; the real ones have 19 fields.
const testFormat = `$remote_addr - - [$time_local] "$request" $status "$http_user_agent"`

func parser(t testing.TB) *sample.Parser {
	t.Helper()
	p, err := sample.Compile(testFormat)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var day = time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)

// line is a request at hour h, minute m of the test day, numbered n.
func line(h, m, n int) string {
	return fmt.Sprintf(`203.0.113.%d - - [%s] "GET /r/%d HTTP/1.1" 200 "agent"`, n%250+1,
		day.Add(time.Duration(h)*time.Hour+time.Duration(m)*time.Minute).Format("02/Jan/2006:15:04:05 -0700"), n)
}

// put writes lines to dir/name, gzipped if the name ends in .gz, and sets its
// modification time.
func put(t testing.TB, dir, name string, mtime time.Time, lines ...string) {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	var w interface {
		Write([]byte) (int, error)
	} = f
	var gz *gzip.Writer
	if strings.HasSuffix(name, ".gz") {
		gz = gzip.NewWriter(f)
		w = gz
	}
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
	if gz != nil {
		gz.Close()
	}
	f.Close()
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// collect reads and returns the request numbers in the order they were delivered.
func collect(t *testing.T, path string, w Window) ([]string, Stats, error) {
	t.Helper()
	var got []string
	st, err := Read(path, parser(t), w, func(v sample.Values, _ time.Time) {
		got = append(got, strings.TrimPrefix(strings.Fields(v["request"])[1], "/r/"))
	})
	return got, st, err
}

func TestFilesAreFoundOldestFirstAndOnlyTheRotatedOnes(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"access.log", "access.log.1", "access.log.2.gz", "access.log.3.gz", "access.log.10.gz",
		"access.log.bak", "access.log.1.gz.tmp", "access.log.1x", "other.log.1", "access.log.old.gz", "access.log.7x", "access.log.8.gz.tmp"} {
		put(t, dir, n, day)
	}
	got, err := Files(filepath.Join(dir, "access.log"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range got {
		names = append(names, filepath.Base(f))
	}
	want := []string{"access.log.10.gz", "access.log.3.gz", "access.log.2.gz", "access.log.1", "access.log"}
	if strings.Join(names, " ") != strings.Join(want, " ") {
		t.Errorf("files = %v, want %v", names, want)
	}
}

func TestPlainAndGzippedFilesAreReadInTimeOrder(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "access.log.2.gz", day.Add(3*time.Hour), line(1, 0, 1), line(2, 0, 2))
	put(t, dir, "access.log.1", day.Add(5*time.Hour), line(3, 0, 3), line(4, 0, 4))
	put(t, dir, "access.log", day.Add(8*time.Hour), line(5, 0, 5), line(6, 0, 6))
	got, st, err := collect(t, filepath.Join(dir, "access.log"), Window{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "1,2,3,4,5,6" {
		t.Errorf("order = %v", got)
	}
	if st.Lines != 6 || st.Skipped != 0 || len(st.Files) != 3 {
		t.Errorf("stats = %+v", st)
	}
	if !st.First.Equal(day.Add(1*time.Hour)) || !st.Last.Equal(day.Add(6*time.Hour)) {
		t.Errorf("First %v Last %v", st.First, st.Last)
	}
}

func TestTheWindowIsHalfOpenAndByEventTimeNotByFile(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "access.log", day.Add(10*time.Hour),
		line(1, 0, 1), line(2, 0, 2), line(3, 0, 3), line(4, 0, 4), line(5, 0, 5))
	w := Window{Since: day.Add(2 * time.Hour), Until: day.Add(4 * time.Hour)}
	got, st, err := collect(t, filepath.Join(dir, "access.log"), w)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "2,3" { // 02:00 is in; 04:00 is out
		t.Errorf("got %v, want [2 3]", got)
	}
	if st.Lines != 2 || st.Outside != 3 {
		t.Errorf("Lines %d Outside %d, want 2 and 3", st.Lines, st.Outside)
	}
}

// logrotate gives a rotated file the modification time of its last line, so a
// file last touched before the window opens cannot hold a line in it.
func TestFilesLastWrittenBeforeTheWindowAreNotRead(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "access.log.2.gz", day.Add(1*time.Hour), line(1, 0, 1))
	put(t, dir, "access.log.1", day.Add(5*time.Hour), line(4, 0, 4))
	put(t, dir, "access.log", day.Add(8*time.Hour), line(6, 0, 6))
	_, st, err := collect(t, filepath.Join(dir, "access.log"), Window{Since: day.Add(3 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Files) != 2 || filepath.Base(st.Files[0]) != "access.log.1" {
		t.Errorf("files read = %v, want access.log.1 and access.log only", st.Files)
	}
	// A file whose mtime is newer than its lines (a restored copy) is still read and filtered by line time.
	put(t, dir, "access.log.2.gz", day.Add(9*time.Hour), line(1, 0, 1))
	got, st, err := collect(t, filepath.Join(dir, "access.log"), Window{Since: day.Add(3 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Files) != 3 || strings.Join(got, ",") != "4,6" {
		t.Errorf("files %v got %v, want 3 files and [4 6]", st.Files, got)
	}
}

func TestUnparsableLinesAreCountedAndBlankLinesAreNot(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "access.log", day, line(1, 0, 1), "", "this is not a log line", line(2, 0, 2),
		`203.0.113.9 - - [not a time] "GET /r/9 HTTP/1.1" 200 "a"`)
	got, st, err := collect(t, filepath.Join(dir, "access.log"), Window{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "1,2" || st.Lines != 2 || st.Skipped != 2 {
		t.Errorf("got %v, Lines %d Skipped %d; want [1 2], 2, 2", got, st.Lines, st.Skipped)
	}
}

func TestALogMostlyInAnotherFormatStopsWithErrMismatch(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf(`203.0.113.7 - - [08/Oct/2026:10:00:00 +0000] "GET /r/%d HTTP/1.1" 200 12 "-" "agent"`, i)) // combined: has bytes
	}
	put(t, dir, "access.log", day, lines...)
	_, st, err := collect(t, filepath.Join(dir, "access.log"), Window{})
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("err = %v, want ErrMismatch", err)
	}
	if st.Skipped != 30 {
		t.Errorf("Skipped = %d, want 30", st.Skipped)
	}
	// A third bad is noise, not a wrong format.
	lines = lines[:0]
	for i := 0; i < 30; i++ {
		if i%3 == 0 {
			lines = append(lines, "garbage")
		} else {
			lines = append(lines, line(1, 0, i))
		}
	}
	put(t, dir, "access.log", day, lines...)
	if _, _, err := collect(t, filepath.Join(dir, "access.log"), Window{}); err != nil {
		t.Errorf("a third unparsable: err = %v", err)
	}
	// Too few lines to judge: a short all-bad file is not a verdict.
	put(t, dir, "access.log", day, "garbage", "garbage", "garbage")
	if _, _, err := collect(t, filepath.Join(dir, "access.log"), Window{}); err != nil {
		t.Errorf("three bad lines: err = %v", err)
	}
}

func TestAMissingLogIsNotExistAndRotatedFilesAloneAreEnough(t *testing.T) {
	dir := t.TempDir()
	_, _, err := collect(t, filepath.Join(dir, "access.log"), Window{})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("nothing present: err = %v, want fs.ErrNotExist", err)
	}
	put(t, dir, "access.log.1", day, line(1, 0, 1))
	got, _, err := collect(t, filepath.Join(dir, "access.log"), Window{})
	if err != nil || len(got) != 1 {
		t.Errorf("rotated only: got %v, err %v", got, err)
	}
}

func TestALineThatIsTooLongIsAnErrorNotASilentCut(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "access.log", day, line(1, 0, 1), strings.Repeat("x", 5<<20), line(2, 0, 2))
	_, _, err := collect(t, filepath.Join(dir, "access.log"), Window{})
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Errorf("err = %v, want bufio.ErrTooLong", err)
	}
}

// A day of CaltechAUTHORS is about 450,000 lines. Reading it must not hold it.
func TestAGoodSizedLogIsReadInBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("large fixture")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriter(f)
	const n = 450000
	for i := 0; i < n; i++ {
		fmt.Fprintln(w, line(i/20000, (i/300)%60, i))
	}
	w.Flush()
	f.Close()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	count := 0
	start := time.Now()
	st, err := Read(path, parser(t), Window{}, func(sample.Values, time.Time) { count++ })
	elapsed := time.Since(start)
	runtime.GC()
	runtime.ReadMemStats(&after)
	if err != nil || count != n || st.Lines != n {
		t.Fatalf("count %d Lines %d err %v", count, st.Lines, err)
	}
	t.Logf("read %d lines in %v (%.0f lines/s)", n, elapsed, float64(n)/elapsed.Seconds())
	if grew := int64(after.HeapAlloc) - int64(before.HeapAlloc); grew > 20<<20 {
		t.Errorf("heap grew by %d MB reading %d lines", grew>>20, n)
	}
	if elapsed > 60*time.Second {
		t.Errorf("took %v", elapsed)
	}
}

// formatTree resolves nginx configuration text.
func formatTree(t *testing.T, text string) []*nginxconf.Directive {
	t.Helper()
	d, err := nginxconf.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := d.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestFormatForFindsTheNamedFormatJoinedAsNginxDoes(t *testing.T) {
	tree := formatTree(t, `http {
  log_format ext '$remote_addr [$time_local] '
                 '"$request" $status';
  access_log /var/log/nginx/access.log ext buffer=32k gzip flush=5s;
  server { server_name a.example; }
}`)
	got, err := FormatFor(tree, "/var/log/nginx/access.log")
	if err != nil {
		t.Fatal(err)
	}
	if want := `$remote_addr [$time_local] "$request" $status`; got != want {
		t.Errorf("format = %q, want %q", got, want)
	}
}

func TestFormatForUsesCombinedWhenNoFormatIsNamed(t *testing.T) {
	for name, line := range map[string]string{
		"no format":    "access_log /var/log/nginx/access.log;",
		"named":        "access_log /var/log/nginx/access.log combined;",
		"only options": "access_log /var/log/nginx/access.log buffer=32k;",
	} {
		got, err := FormatFor(formatTree(t, "http {\n"+line+"\nserver { server_name a.example; }\n}"), "/var/log/nginx/access.log")
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !strings.Contains(got, `$time_local`) || !strings.Contains(got, `"$http_user_agent"`) || strings.Contains(got, "$request_time") {
			t.Errorf("%s: format = %q, want nginx's combined", name, got)
		}
	}
}

func TestFormatForRefusesWhatItCannotRead(t *testing.T) {
	for name, c := range map[string]struct {
		conf string
		want error
	}{
		"not configured":     {"http {\naccess_log /var/log/other.log;\nserver { server_name a.example; }\n}", ErrNotConfigured},
		"escape=json":        {"http {\nlog_format j escape=json '{\"t\":\"$time_local\"}';\naccess_log /var/log/nginx/access.log j;\nserver { server_name a.example; }\n}", ErrUnsupported},
		"no time in format":  {"http {\nlog_format nt '$remote_addr $status';\naccess_log /var/log/nginx/access.log nt;\nserver { server_name a.example; }\n}", ErrNoTime},
		"format not defined": {"http {\naccess_log /var/log/nginx/access.log nope;\nserver { server_name a.example; }\n}", ErrNotConfigured},
	} {
		if _, err := FormatFor(formatTree(t, c.conf), "/var/log/nginx/access.log"); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	// escape=default and escape=none are the plain text format and are fine.
	tree := formatTree(t, "http {\nlog_format p escape=none '$remote_addr [$time_local]';\naccess_log /var/log/nginx/access.log p;\nserver { server_name a.example; }\n}")
	if got, err := FormatFor(tree, "/var/log/nginx/access.log"); err != nil || got != "$remote_addr [$time_local]" {
		t.Errorf("escape=none: %q, %v", got, err)
	}
}

// realFormat is the 19-field extended format both Caltech hosts write.
const realFormat = `$remote_addr - $remote_user [$time_local] "$request" ` +
	`$status $body_bytes_sent "$http_referer" "$http_user_agent" ` +
	`rt=$request_time urt="$upstream_response_time" ` +
	`cf_ray="$http_cf_ray" cf_country="$http_cf_ipcountry" ` +
	`peer=$realip_remote_addr ` +
	`lang="$http_accept_language" ch_ua="$http_sec_ch_ua" ` +
	`ch_plat="$http_sec_ch_ua_platform" ` +
	`bot_score="$http_cf_bot_score" ja3="$http_cf_ja3_hash" ja4="$http_cf_ja4" cache="$upstream_cache_status"`

// This is the number DR-0006 and the plan's decision point asked for: whether
// the regular-expression parser is fast enough on the real format.
func TestTheRealFormatReadsAtAUsefulSpeed(t *testing.T) {
	if testing.Short() {
		t.Skip("large fixture")
	}
	p, err := sample.Compile(realFormat)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriter(f)
	const n = 450000
	for i := 0; i < n; i++ {
		fmt.Fprintf(w, `203.0.113.%d - - [08/Oct/2026:%02d:%02d:%02d +0000] "GET /api/records/abc%d-xyz/versions HTTP/1.1" 200 1234 "https://example.org/r?q=1" `+
			`"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36" rt=0.095 urt="0.096, 0.2" `+
			`cf_ray="9ab12c34d56e-LAX" cf_country="US" peer=198.41.128.1 lang="en-US,en;q=0.9" ch_ua="\x22Chromium\x22;v=\x22148\x22" ch_plat="\x22Windows\x22" `+
			`bot_score="-" ja3="-" ja4="-" cache="HIT"`+"\n", i%250+1, (i/20000)%24, (i/300)%60, i%60, i)
	}
	w.Flush()
	f.Close()
	count := 0
	start := time.Now()
	st, err := Read(path, p, Window{}, func(sample.Values, time.Time) { count++ })
	elapsed := time.Since(start)
	if err != nil || count != n || st.Skipped != 0 {
		t.Fatalf("count %d skipped %d err %v", count, st.Skipped, err)
	}
	t.Logf("19-field format: %d lines in %v (%.0f lines/s)", n, elapsed, float64(n)/elapsed.Seconds())
	if elapsed > 60*time.Second {
		t.Errorf("took %v", elapsed)
	}
}

// A format that grew: today's has rt= appended to the test format.
func tolerant(t testing.TB) *sample.Parser {
	t.Helper()
	p, err := sample.CompileTolerant(testFormat + ` rt=$request_time`)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLinesInAnOlderShorterFormAreReadAndCounted(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, line(1, i, i)) // the format before rt= was added
	}
	for i := 30; i < 60; i++ {
		lines = append(lines, line(2, i-30, i)+" rt=0.250")
	}
	put(t, dir, "access.log", day.Add(10*time.Hour), lines...)
	var older, withRT int
	st, err := Read(filepath.Join(dir, "access.log"), tolerant(t), Window{}, func(v sample.Values, _ time.Time) {
		if _, has := v["request_time"]; has {
			withRT++
		} else {
			older++
		}
	})
	if err != nil {
		t.Fatalf("a window holding both forms was refused: %v", err)
	}
	if st.Lines != 60 || st.Older != 30 || st.Skipped != 0 || older != 30 || withRT != 30 {
		t.Errorf("Lines %d Older %d Skipped %d; callback saw %d older and %d with rt", st.Lines, st.Older, st.Skipped, older, withRT)
	}
}

// logrotate gives a rotated file the time of its last line, so the file for the
// day before can be the one that has to be opened to see whether it holds the
// first second of the window. It may well be in the previous format throughout.
func TestAnAllOlderFormatRotatedFileOpenedForTheWindowEdgeIsNotAMismatch(t *testing.T) {
	dir := t.TempDir()
	var old []string
	for i := 0; i < 40; i++ {
		old = append(old, line(23, i%60, i))
	}
	put(t, dir, "access.log.2.gz", day.Add(1*time.Second), old...) // mtime just after the window opens
	put(t, dir, "access.log", day.Add(8*time.Hour), line(6, 0, 99)+" rt=0.100")
	st, err := Read(filepath.Join(dir, "access.log"), tolerant(t), Window{Since: day}, func(sample.Values, time.Time) {})
	if err != nil {
		t.Fatalf("err = %v, want none: the older form is a form of the format, not a different format", err)
	}
	if len(st.Files) != 2 || st.Older+st.Outside < 40 {
		t.Errorf("files %v, Older %d, Outside %d", st.Files, st.Older, st.Outside)
	}
}

func TestAStrictParserStillCallsAnOlderFormatAMismatch(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, line(1, i, i))
	}
	put(t, dir, "access.log", day, lines...)
	strict, err := sample.Compile(testFormat + ` rt=$request_time`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Read(filepath.Join(dir, "access.log"), strict, Window{}, func(sample.Values, time.Time) {}); !errors.Is(err, ErrMismatch) {
		t.Errorf("err = %v, want ErrMismatch", err)
	}
}
