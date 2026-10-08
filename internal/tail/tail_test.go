package tail

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func numbered(n int) []string {
	var out []string
	for i := 1; i <= n; i++ {
		out = append(out, fmt.Sprintf("line %05d", i))
	}
	return out
}

func TestLastReturnsTheLastLinesWhole(t *testing.T) {
	p := write(t, strings.Join(numbered(25), "\n")+"\n")
	got, err := Last(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.Join(numbered(25)[15:], "\n") {
		t.Errorf("got %q", got)
	}
	for _, n := range []int{25, 26, 1000} {
		got, _ := Last(p, n)
		if got != strings.Join(numbered(25), "\n") {
			t.Errorf("n=%d: got %d lines, want all 25", n, strings.Count(got, "\n")+1)
		}
	}
}

func TestLastWithoutATrailingNewlineAndAcrossManyBlocks(t *testing.T) {
	lines := numbered(30000) // about 330 KB, several read blocks
	p := write(t, strings.Join(lines, "\n"))
	got, err := Last(p, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.Join(lines[29000:], "\n") {
		t.Errorf("first line %q, want %q", strings.SplitN(got, "\n", 2)[0], lines[29000])
	}
	// A line cut by a block boundary is never returned.
	for _, l := range strings.Split(got, "\n") {
		if len(l) != len("line 00000") {
			t.Fatalf("partial line %q", l)
		}
	}
}

func TestLastEdgeCases(t *testing.T) {
	if got, err := Last(write(t, ""), 10); err != nil || got != "" {
		t.Errorf("empty file: %q, %v", got, err)
	}
	if got, err := Last(write(t, "only\n"), 10); err != nil || got != "only" {
		t.Errorf("one line: %q, %v", got, err)
	}
	if _, err := Last(filepath.Join(t.TempDir(), "nope"), 10); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing file: error = %v", err)
	}
	if _, err := Last(t.TempDir(), 10); err == nil {
		t.Error("a directory was read")
	}
	if _, err := Last(write(t, "x\n"), 0); err == nil {
		t.Error("n = 0 accepted")
	}
}

func TestLastPermissionRefusedIsReportedAsSuch(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	p := write(t, "x\n")
	os.Chmod(p, 0)
	if _, err := Last(p, 10); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("error = %v", err)
	}
}
