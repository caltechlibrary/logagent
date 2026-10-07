package logagent

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestVersionMatchesCodemeta fails when version.go has drifted from
// codemeta.json. Run `make version.go` to bring them back in step.
func TestVersionMatchesCodemeta(t *testing.T) {
	src, err := os.ReadFile("codemeta.json")
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(src, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Version == "" {
		t.Fatal("codemeta.json has no version")
	}
	if Version != meta.Version {
		t.Errorf("version.go Version = %q, codemeta.json version = %q; run make version.go", Version, meta.Version)
	}
}

func TestReleaseFieldsLookRight(t *testing.T) {
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(ReleaseDate) {
		t.Errorf("ReleaseDate %q is not YYYY-MM-DD", ReleaseDate)
	}
	if !regexp.MustCompile(`^[0-9a-f]{7,40}$`).MatchString(ReleaseHash) {
		t.Errorf("ReleaseHash %q is not a git hash", ReleaseHash)
	}
}

func TestLicenseTextIsTheLicenseFile(t *testing.T) {
	src, err := os.ReadFile("LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(LicenseText) != strings.TrimSpace(string(src)) {
		t.Error("LicenseText differs from the LICENSE file")
	}
}

func TestFmtHelpReplacesEveryMarker(t *testing.T) {
	got := FmtHelp("{app_name} {version} {release_date} {release_hash}", "a", "1", "d", "h")
	if got != "a 1 d h" {
		t.Errorf("FmtHelp = %q", got)
	}
}
