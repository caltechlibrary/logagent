package logagent

import (
	_ "embed"
	"strings"
)

// The three constants below are rewritten by `make version.go` from
// codemeta.json, the current date and the current git hash. Keep each on its
// own line in this exact form; the Makefile edits them with sed.
const (
	// Version is the release number, taken from codemeta.json.
	//
	// @example
	//
	//	fmt.Println(logagent.Version) // "0.0.4"
	Version = "0.0.4"

	// ReleaseDate is the date version.go was last updated (YYYY-MM-DD).
	//
	// @example
	//
	//	fmt.Println(logagent.ReleaseDate) // "2026-10-07"
	ReleaseDate = "2026-10-07"

	// ReleaseHash is the git hash of HEAD when version.go was last updated.
	//
	// @example
	//
	//	fmt.Println(logagent.ReleaseHash) // "104e001"
	ReleaseHash = "104e001"
)

// LicenseText is the contents of the LICENSE file, embedded at build time so
// it cannot drift from the file.
//
// @example
//
//	fmt.Println(logagent.LicenseText)
//
//go:embed LICENSE
var LicenseText string

// FmtHelp expands the curly brace markers {app_name}, {version},
// {release_date} and {release_hash} in a block of help text.
//
// @param src {string} the help text containing markers
// @param appName {string} replaces {app_name}
// @param version {string} replaces {version}
// @param releaseDate {string} replaces {release_date}
// @param releaseHash {string} replaces {release_hash}
// @returns {string} the text with every marker replaced
// @example
//
//	s := logagent.FmtHelp("{app_name} {version}", "logagent", "0.0.4", "", "")
//	fmt.Println(s) // "logagent 0.0.4"
func FmtHelp(src string, appName string, version string, releaseDate string, releaseHash string) string {
	r := strings.NewReplacer(
		"{app_name}", appName,
		"{version}", version,
		"{release_date}", releaseDate,
		"{release_hash}", releaseHash,
	)
	return r.Replace(src)
}
