package audit

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/tamnd/math2-agent/build"
)

// g04 checks that the formal dependencies of each bound node are in its uses (rule G04). It reads the extraction of the last m2 build in build/. A bound declaration that reaches the declaration of another node depends on that node, so the uses list must name it.
func g04(a *auditor) outcome {
	var o outcome
	x, err := build.ReadExtraction(filepath.Join(a.root, "build"))
	if errors.Is(err, fs.ErrNotExist) {
		o.reason = "build/" + build.ExtractFile + " does not exist. Run m2 build first"
		return o
	}
	if err != nil {
		o.reason = err.Error()
		return o
	}
	records := map[string][]string{}
	for _, r := range x.Records {
		if r.Error == "" {
			records[r.Decl] = r.UsesBound
		}
	}
	// owner gives the nodes that bind each declaration, as a decl or as an adapter.
	owner := map[string][]string{}
	decls := map[string][]string{}
	for _, n := range a.nodes {
		for _, d := range leanDecls(n.Meta["formal"]) {
			owner[d] = append(owner[d], n.ID)
			decls[n.ID] = append(decls[n.ID], d)
		}
	}
	for _, b := range x.Bindings {
		owner[b.Decl] = append(owner[b.Decl], b.Tag)
	}
	for _, n := range a.scopeNodes() {
		if retired(n) || len(decls[n.ID]) == 0 {
			continue
		}
		o.checked++
		uses := n.List("uses")
		var missing []string
		for _, d := range decls[n.ID] {
			for _, dep := range records[d] {
				for _, tag := range owner[dep] {
					if tag != n.ID && !slices.Contains(uses, tag) && !slices.Contains(missing, tag) {
						missing = append(missing, tag)
						line, col := keyPos(n, "uses", "formal")
						o.add(n.Path, line, col, "the formal declaration %s uses %s, which %s binds, and %s is not in uses. Add %s to uses", d, dep, tag, tag, tag)
					}
				}
			}
		}
	}
	return o
}

// leanDecls returns the declarations and the adapters of the Lean bindings in a formal list.
func leanDecls(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, item := range list {
		m, _ := item.(map[string]any)
		if str(m["system"]) != "lean4" {
			continue
		}
		for _, k := range []string{"decl", "adapter"} {
			if d := str(m[k]); d != "" && !slices.Contains(out, d) {
				out = append(out, d)
			}
		}
	}
	return out
}
