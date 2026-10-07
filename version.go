package logagent

import _ "embed"

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
