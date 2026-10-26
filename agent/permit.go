package agent

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/workspace"
)

// The Lean directories that no role writes: the attribute and the adapters, the certificate checkers, and the named hypotheses, which are the only place where an axiom can be (spec 20).
var leanFixed = []string{"lean/Math2/Meta/", "lean/Math2/Cert/", "lean/Math2/Hypotheses/"}

// Permit reports each change that the field permission set of the role does not allow (spec 12.1). The target is the id of the node of the job. A change outside the set fails even if it is correct. Permit knows the sets of formalize and prove. The sets of the other roles come with their roles.
//
// Formalize can append bindings with no fidelity reviews to the formal field of the target, add Lean files under lean/Math2/, and add import lines to lean/Math2.lean. Prove can add proof and statement nodes with bindings with no fidelity reviews, change formal[].decl of the target, add and change Lean files under lean/Math2/ except in Conjectures/, and add import lines to lean/Math2.lean. No role writes in lean/Math2/Meta/, lean/Math2/Cert/ or lean/Math2/Hypotheses/, and no role deletes a file.
func Permit(role, target string, changes []workspace.Change) error {
	if role != "formalize" && role != "prove" {
		return fmt.Errorf("agent: the field permission set of %s is not written yet", role)
	}
	var errs []error
	for _, c := range changes {
		if err := permit(role, target, c); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.Path, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("agent: the changes are outside the permissions of %s: %w", role, errors.Join(errs...))
	}
	return nil
}

func permit(role, target string, c workspace.Change) error {
	p := c.Path
	switch {
	case c.New == nil:
		return errors.New("a role does not delete a file")
	case p == "lean/Math2.lean":
		return imports(c.Old, c.New)
	case strings.HasPrefix(p, "lean/Math2/") && strings.HasSuffix(p, ".lean"):
		for _, d := range leanFixed {
			if strings.HasPrefix(p, d) {
				return fmt.Errorf("no role writes in %s", d)
			}
		}
		if role == "formalize" && c.Old != nil {
			return errors.New("formalize adds Lean files and does not change them")
		}
		if role == "prove" && strings.HasPrefix(p, "lean/Math2/Conjectures/") {
			return errors.New("prove does not write the statements of conjectures")
		}
		return nil
	case strings.HasPrefix(p, "nodes/") && strings.HasSuffix(p, ".md"):
		return permitNode(role, target, c)
	}
	return fmt.Errorf("%s does not write this file", role)
}

// imports allows a change of the root module that only adds lines that import modules of Math2.
func imports(before, after []byte) error {
	o := slices.Collect(strings.Lines(string(before)))
	for line := range strings.Lines(string(after)) {
		if len(o) > 0 && line == o[0] {
			o = o[1:]
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 || f[0] != "public" || f[1] != "import" || !strings.HasPrefix(f[2], "Math2.") {
			return fmt.Errorf("the change of the root module does more than add imports of modules of Math2, at the line %q", strings.TrimSpace(line))
		}
	}
	if len(o) > 0 {
		return errors.New("the change of the root module does more than add imports of modules of Math2, and removes lines")
	}
	return nil
}

func permitNode(role, target string, c workspace.Change) error {
	n, err := node.Parse(c.Path, c.New)
	if err != nil {
		return err
	}
	formal := entries(n.Meta["formal"])
	if c.Old == nil {
		if role != "prove" {
			return fmt.Errorf("%s does not add nodes", role)
		}
		if k := n.Kind(); k != "proof" && k != "statement" {
			return fmt.Errorf("prove adds proof and statement nodes, and this node is a %s", k)
		}
		for _, f := range formal {
			if f["fidelity_reviews"] != nil {
				return errors.New("a new binding has no fidelity reviews")
			}
		}
		return nil
	}
	o, err := node.Parse(c.Path, c.Old)
	if err != nil {
		return err
	}
	if o.ID != target {
		return fmt.Errorf("the node %s is not the target %s", o.ID, target)
	}
	if o.Body != n.Body {
		return fmt.Errorf("%s does not change the body", role)
	}
	for _, k := range slices.Sorted(maps.Keys(union(o.Meta, n.Meta))) {
		if k != "formal" && !reflect.DeepEqual(o.Meta[k], n.Meta[k]) {
			return fmt.Errorf("%s does not change the field %s", role, k)
		}
	}
	old := entries(o.Meta["formal"])
	if role == "formalize" {
		if len(formal) < len(old) || !reflect.DeepEqual(old, formal[:len(old)]) {
			return errors.New("formalize appends bindings and does not change the bindings that are there")
		}
		for _, f := range formal[len(old):] {
			if f["fidelity_reviews"] != nil {
				return errors.New("a new binding has no fidelity reviews")
			}
		}
		return nil
	}
	if len(formal) != len(old) {
		return errors.New("prove changes formal[].decl and does not add or remove bindings")
	}
	for i := range old {
		a, b := maps.Clone(old[i]), maps.Clone(formal[i])
		delete(a, "decl")
		delete(b, "decl")
		if !reflect.DeepEqual(a, b) {
			return fmt.Errorf("prove changes only formal[%d].decl", i)
		}
	}
	return nil
}

// entries returns the formal entries of the front matter, or nil.
func entries(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, e := range list {
		m, _ := e.(map[string]any)
		out = append(out, m)
	}
	return out
}

func union(a, b map[string]any) map[string]any {
	u := maps.Clone(a)
	if u == nil {
		u = map[string]any{}
	}
	maps.Copy(u, b)
	return u
}
