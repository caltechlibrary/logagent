package logagent

import "github.com/caltechlibrary/logagent/internal/help"

// The help texts below are the manual pages in internal/help, as Markdown with
// {app_name}, {version}, {release_date} and {release_hash} markers that FmtHelp
// expands. They are variables, not constants, because they come from files
// embedded in the binary. This file keeps the exported names that the other
// Caltech Library Go modules use; new code can use internal/help directly.
var (
	// LogagentHelpText is the logagent(1) manual page.
	//
	// @example
	//
	//	fmt.Print(logagent.FmtHelp(logagent.LogagentHelpText, "logagent",
	//		logagent.Version, logagent.ReleaseDate, logagent.ReleaseHash))
	LogagentHelpText = help.MustText("logagent.1")

	// LogagentCheckHelpText is the logagent-check(1) manual page (planned command).
	LogagentCheckHelpText = help.MustText("logagent-check.1")

	// LogagentReportHelpText is the logagent-report(1) manual page (planned command).
	LogagentReportHelpText = help.MustText("logagent-report.1")

	// LogagentWatchHelpText is the logagent-watch(1) manual page (planned command).
	LogagentWatchHelpText = help.MustText("logagent-watch.1")

	// LogagentRespondHelpText is the logagent-respond(1) manual page (planned command).
	LogagentRespondHelpText = help.MustText("logagent-respond.1")

	// LogagentAnalyzeHelpText is the logagent-analyze(1) manual page (planned command).
	LogagentAnalyzeHelpText = help.MustText("logagent-analyze.1")

	// LogagentConfigHelpText is the logagent-config(5) manual page, the per-host
	// configuration file.
	LogagentConfigHelpText = help.MustText("logagent-config.5")

	// LogagentJSONLHelpText is the logagent-jsonl(5) manual page, the JSON Lines
	// schema (placeholder).
	LogagentJSONLHelpText = help.MustText("logagent-jsonl.5")

	// LogagentTiersHelpText is the logagent-tiers(7) manual page.
	LogagentTiersHelpText = help.MustText("logagent-tiers.7")
)

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
	return help.Expand(src, appName, version, releaseDate, releaseHash)
}
