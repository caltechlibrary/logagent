package nginxconf

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed directives.json
var directivesJSON []byte

// serverDirectives is the set of directive names that open a line in a server
// block.
var serverDirectives = func() map[string]bool {
	var t struct {
		Version int      `json:"version"`
		Note    string   `json:"note"`
		Server  []string `json:"server"`
	}
	dec := json.NewDecoder(bytes.NewReader(directivesJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil || t.Version != 1 {
		panic(fmt.Sprintf("nginxconf: embedded directives.json: version %d, %v", t.Version, err))
	}
	set := map[string]bool{}
	for _, n := range t.Server {
		set[n] = true
	}
	return set
}()

// IsServerDirective reports whether the word is the name of a directive that
// is written in a server block. A server_name argument that is one is the next
// directive swallowed by a missing semicolon. The set is data, in
// directives.json.
//
// @param word {string} a word from a directive's arguments
// @returns {bool} true for a server-block directive name
// @example
//
//	nginxconf.IsServerDirective("add_header") // true
//	nginxconf.IsServerDirective("data.example.org") // false
func IsServerDirective(word string) bool { return serverDirectives[word] }
