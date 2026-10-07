package nginxconf

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed inherit.json
var inheritJSON []byte

// arrayDirectives is the set of directives nginx inherits as a whole list.
var arrayDirectives = func() map[string]bool {
	var t struct {
		Version int      `json:"version"`
		Note    string   `json:"note"`
		Array   []string `json:"array"`
	}
	dec := json.NewDecoder(bytes.NewReader(inheritJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil || t.Version != 1 {
		panic(fmt.Sprintf("nginxconf: embedded inherit.json: version %d, %v", t.Version, err))
	}
	set := map[string]bool{}
	for _, n := range t.Array {
		set[n] = true
	}
	return set
}()

// IsArray reports whether nginx inherits the named directive as a whole list:
// a context that defines any directive of that name replaces all the inherited
// ones, so an add_header in a location drops the server's add_header lines.
// The set is data, in inherit.json.
//
// @param name {string} the directive name
// @returns {bool} true for a directive inherited as a list
// @example
//
//	nginxconf.IsArray("add_header") // true
//	nginxconf.IsArray("proxy_buffering") // false
func IsArray(name string) bool { return arrayDirectives[name] }

// Effective returns the directives named name that are in force at a point of
// a resolved tree. From the context that holds at (at itself when it has a
// block) it looks outward, through enclosing locations, the server and the
// http block, and stops at the first context that defines the name. For an
// array directive (see IsArray) it returns all of that context's directives of
// the name; otherwise only the last. Directives inside an if block are not
// their context's own and are not counted. Top-level directives outside http
// are not consulted. The returned slice is the caller's; the directives in it
// are the tree's own, so each still says which context, file and line it came
// from.
//
// @param at {*Directive} the server, location or directive to ask about
// @param name {string} the directive name
// @returns {[]*Directive} the directives in force, or nil when there are none
// @example
//
//	for _, d := range nginxconf.Effective(loc, "proxy_buffering") {
//		fmt.Println(d.Args[0], "from", d.Parent.Name, d.File, d.Line)
//	}
func Effective(at *Directive, name string) []*Directive {
	ctx := at
	if ctx != nil && !ctx.HasBlock {
		ctx = ctx.Parent
	}
	for ; ctx != nil; ctx = ctx.Parent {
		var own []*Directive
		for _, d := range ctx.Block {
			if d.Name == name {
				own = append(own, d)
			}
		}
		if len(own) == 0 {
			continue
		}
		if IsArray(name) {
			return own
		}
		return []*Directive{own[len(own)-1]}
	}
	return nil
}

// Locations returns every location block under an http block's servers, nested
// locations included, in document order.
//
// @param tree {[]*Directive} a resolved tree
// @returns {[]*Directive} the location directives
// @example
//
//	for _, loc := range nginxconf.Locations(tree) {
//		fmt.Println(loc.Args)
//	}
func Locations(tree []*Directive) []*Directive {
	var out []*Directive
	var collect func(ds []*Directive)
	collect = func(ds []*Directive) {
		for _, d := range ds {
			if d.Name == "location" && d.HasBlock {
				out = append(out, d)
				collect(d.Block)
			}
		}
	}
	for _, srv := range Find(tree, "http", "server") {
		collect(srv.Block)
	}
	return out
}
