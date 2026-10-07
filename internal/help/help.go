// Package help holds logagent's manual pages. Each page is a Markdown file in
// this directory named NAME.CHAPTER.md, for example logagent-check.1.md, which
// is also the man page's name. Pages are embedded in the binary and found
// without a hand-kept list.
package help

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.md
var pagesFS embed.FS

var (
	// ErrNotFound is returned by Lookup when no page matches the topic.
	ErrNotFound = errors.New("no such help topic")

	// ErrAmbiguous is returned by Lookup when more than one page matches.
	ErrAmbiguous = errors.New("ambiguous help topic")
)

// Page is one manual page.
type Page struct {
	// Name is the man page name, such as "logagent-check".
	Name string
	// Chapter is the manual chapter: 1, 5 or 7.
	Chapter int
	// Text is the Markdown source with {app_name}, {version},
	// {release_date} and {release_hash} markers still in it.
	Text string
}

// File returns the page's name and chapter as they appear in a file name
// and on the man command line, such as "logagent-check.1".
//
// @returns {string} NAME.CHAPTER
// @example
//
//	p, _ := help.Lookup("check")
//	fmt.Println(p.File()) // "logagent-check.1"
func (p Page) File() string {
	return fmt.Sprintf("%s.%d", p.Name, p.Chapter)
}

var pages []Page

// init loads the embedded pages. A file that does not follow NAME.CHAPTER.md
// is a defect in the source tree, so it stops the program; the package tests
// catch it before a release.
func init() {
	entries, err := fs.ReadDir(pagesFS, ".")
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		base := strings.TrimSuffix(e.Name(), ".md")
		dot := strings.LastIndex(base, ".")
		if dot < 1 || len(base)-dot != 2 || base[dot+1] < '1' || base[dot+1] > '9' {
			panic(fmt.Sprintf("help: %s is not named NAME.CHAPTER.md", e.Name()))
		}
		text, err := fs.ReadFile(pagesFS, e.Name())
		if err != nil {
			panic(err)
		}
		pages = append(pages, Page{Name: base[:dot], Chapter: int(base[dot+1] - '0'), Text: string(text)})
	}
	sort.Slice(pages, func(i, j int) bool {
		if pages[i].Chapter != pages[j].Chapter {
			return pages[i].Chapter < pages[j].Chapter
		}
		return pages[i].Name < pages[j].Name
	})
}

// Pages returns every page, ordered by chapter and then name.
//
// @returns {[]Page} a copy of the page list
// @example
//
//	for _, p := range help.Pages() {
//		fmt.Println(p.File())
//	}
func Pages() []Page {
	return append([]Page(nil), pages...)
}

// Lookup finds a page from a topic. It tries, in order, an exact NAME.CHAPTER,
// then a page NAME, then a short topic that is NAME without the "logagent-"
// prefix. A step that matches more than one page (the same name in two
// chapters) is ambiguous and Lookup says which.
//
// @param topic {string} "logagent.1", "logagent-check" or "check"
// @returns {Page} the matching page
// @returns {error} ErrNotFound or ErrAmbiguous, wrapped with detail
// @example
//
//	p, err := help.Lookup("check")
//	if err != nil {
//		return err
//	}
//	fmt.Print(p.Text)
func Lookup(topic string) (Page, error) {
	steps := []func(Page) bool{
		func(p Page) bool { return p.File() == topic },
		func(p Page) bool { return p.Name == topic },
		func(p Page) bool { return p.Name == "logagent-"+topic },
	}
	for _, match := range steps {
		var found []Page
		for _, p := range pages {
			if topic != "" && match(p) {
				found = append(found, p)
			}
		}
		switch len(found) {
		case 0:
		case 1:
			return found[0], nil
		default:
			names := make([]string, len(found))
			for i, p := range found {
				names[i] = p.File()
			}
			return Page{}, fmt.Errorf("%w %q: use one of %s", ErrAmbiguous, topic, strings.Join(names, ", "))
		}
	}
	return Page{}, fmt.Errorf("%w %q", ErrNotFound, topic)
}

// MustText returns the raw text of the page with the given NAME.CHAPTER and
// panics if there is none. It exists to initialise package-level variables.
//
// @param file {string} NAME.CHAPTER, such as "logagent.1"
// @returns {string} the page's Markdown source
// @example
//
//	var HelpText = help.MustText("logagent.1")
func MustText(file string) string {
	for _, p := range pages {
		if p.File() == file {
			return p.Text
		}
	}
	panic("help: no page " + file)
}

// Expand replaces the markers {app_name}, {version}, {release_date} and
// {release_hash} in a block of help text.
//
// @param src {string} text containing markers
// @param appName {string} replaces {app_name}
// @param version {string} replaces {version}
// @param releaseDate {string} replaces {release_date}
// @param releaseHash {string} replaces {release_hash}
// @returns {string} the text with every marker replaced
// @example
//
//	s := help.Expand("{app_name} {version}", "logagent", "0.0.4", "", "")
//	fmt.Println(s) // "logagent 0.0.4"
func Expand(src, appName, version, releaseDate, releaseHash string) string {
	return strings.NewReplacer(
		"{app_name}", appName,
		"{version}", version,
		"{release_date}", releaseDate,
		"{release_hash}", releaseHash,
	).Replace(src)
}
