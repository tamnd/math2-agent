package graph

import (
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/node"
)

// sourceKeys are the identifier fields of a source, in the order of the schema.
var sourceKeys = []string{"zbmath", "arxiv", "doi", "book", "mr", "stacks", "kerodon", "mathoverflow", "mathse", "erdos", "formal_conjectures", "palomar", "hexagon", "url"}

// edges returns the edges in the front matter of n.
func edges(n *node.Node) []Edge {
	var out []Edge
	add := func(e Edge) {
		if e.From != "" && e.To != "" {
			out = append(out, e)
		}
	}
	from := n.ID
	list := func(t, key string) {
		for _, to := range n.List(key) {
			add(Edge{Type: t, From: from, To: to})
		}
	}

	add(Edge{Type: Proves, From: from, To: n.Get("proves")})
	for _, p := range n.List("proofs") {
		add(Edge{Type: Proves, From: p, To: from})
	}
	list(Uses, "uses")
	for _, t := range []string{Implies, Equivalent, Refutes} {
		for _, m := range maps(n.Meta[t]) {
			add(Edge{Type: t, From: from, To: str(m["target"]), Via: str(m["via"])})
		}
	}
	for _, m := range maps(n.Meta["resolves"]) {
		add(Edge{Type: Resolves, From: from, To: str(m["target"]), Label: str(m["outcome"])})
	}
	for _, m := range maps(n.Meta["generalizes"]) {
		add(Edge{Type: Generalizes, From: from, To: str(m["target"]), Label: str(m["map"])})
	}
	for _, m := range maps(n.Meta["bounds"]) {
		add(Edge{Type: Bounds, From: from, To: str(m["target"]), Via: str(m["via"]), Label: str(m["side"]) + " " + str(m["value"])})
	}

	add(Edge{Type: Defines, From: from, To: n.Get("defines")})
	list(InstanceOf, "instance_of")
	for _, m := range maps(n.Meta["not_instance_of"]) {
		add(Edge{Type: NotInstance, From: from, To: str(m["target"]), Label: fails(m["fails"])})
	}
	list(Illustrates, "illustrates")
	list(About, "about")
	list(Specializes, "specializes")
	list(Method, "methods")
	for _, m := range maps(n.Meta["analogous_to"]) {
		system, id := source(maps(m["source"]))
		add(Edge{Type: AnalogousTo, From: from, To: str(m["target"]), System: system, Label: id})
	}
	list(PartOf, "topics")

	for _, m := range maps(n.Meta["formal"]) {
		system := str(m["system"])
		if lib := str(m["library"]); lib != "" {
			system += "/" + lib
		}
		add(Edge{Type: Formalizes, From: str(m["decl"]), To: from, System: system, Label: str(m["match"])})
	}
	for _, m := range maps(n.Meta["sources"]) {
		system, id := source([]map[string]any{m})
		add(Edge{Type: Source, From: from, To: id, System: system, Label: str(m["locator"])})
	}
	for _, m := range maps(n.Meta["aligned"]) {
		add(Edge{Type: Aligned, From: str(m["record"]), To: from, Label: str(m["match"])})
	}
	if same, ok := n.Meta["same_as"].(map[string]any); ok {
		for system, id := range same {
			add(Edge{Type: SameAs, From: from, To: str(id), System: system})
		}
	}
	for _, m := range maps(n.Meta["close_to"]) {
		add(Edge{Type: CloseTo, From: from, To: str(m["id"]), System: str(m["system"])})
	}
	list(ResolvedBy, "resolved_by")
	if r, ok := n.Meta["retired"].(map[string]any); ok {
		for _, to := range strs(r["successor"]) {
			add(Edge{Type: Successor, From: from, To: to})
		}
	}
	return out
}

// source returns the system and the identifier of the first source in ms.
func source(ms []map[string]any) (system, id string) {
	for _, m := range ms {
		for _, k := range sourceKeys {
			if v := str(m[k]); v != "" {
				return k, v
			}
		}
	}
	return "", ""
}

// maps returns the mappings in v. A mapping gives a list with one item.
func maps(v any) []map[string]any {
	switch v := v.(type) {
	case map[string]any:
		return []map[string]any{v}
	case []any:
		var out []map[string]any
		for _, x := range v {
			if m, ok := x.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func strs(v any) []string {
	var out []string
	switch v := v.(type) {
	case string:
		out = append(out, v)
	case []any:
		for _, x := range v {
			if s := str(x); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// str returns a scalar as a string, or "" for a value that is not a scalar.
func str(v any) string {
	switch v := v.(type) {
	case nil, map[string]any, []any:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// fails returns the clause numbers of a not_instance_of edge as one label, for example "clauses 1, 3".
func fails(v any) string {
	cs := strs(v)
	if len(cs) == 0 {
		return ""
	}
	return "clauses " + strings.Join(cs, ", ")
}
