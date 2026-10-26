package agent

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/tag"
	"github.com/tamnd/math2-agent/workspace"
)

// The Lean directories that no role writes: the attribute and the adapters, the certificate checkers, and the named hypotheses, which are the only place where an axiom can be (spec 20).
var leanFixed = []string{"lean/Math2/Meta/", "lean/Math2/Cert/", "lean/Math2/Hypotheses/"}

// Permit reports each change that the field permission set of the role does not allow (spec 12.1). The target is the target of the job. A change outside the set fails even if it is correct. Permit knows the sets of formalize, prove, explain and curate. The sets of the other roles come with their roles.
//
// Formalize can append bindings with no fidelity reviews to the formal field of the target, add Lean files under lean/Math2/, and add import lines to lean/Math2.lean. Prove can add proof and statement nodes with bindings with no fidelity reviews, change formal[].decl of the target, add and change Lean files under lean/Math2/ except in Conjectures/, and add import lines to lean/Math2.lean. Explain can add and change the explanations of the target that are marked agent, and add example nodes with no formal field and no explanation marked human. Explain writes no Lean. Curate can add statement and concept nodes with no formal field, no crosswalk edges and no explanations, change the fields uses, about, msc, name and aliases of any node, and add duplicate proposals of the role to the dedup queue. Curate writes no Lean. No role writes in lean/Math2/Meta/, lean/Math2/Cert/ or lean/Math2/Hypotheses/, and no role deletes a file.
func Permit(role, target string, changes []workspace.Change) error {
	if !slices.Contains([]string{"formalize", "prove", "explain", "curate"}, role) {
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
	case (role == "explain" || role == "curate") && strings.HasPrefix(p, "lean/"):
		return fmt.Errorf("%s writes no Lean", role)
	case role == "curate" && p == align.DedupQueue:
		return permitQueue(c.Old, c.New)
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
	switch role {
	case "explain":
		return permitExplain(target, c, n)
	case "curate":
		return permitCurate(c, n)
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

// explanationField matches the provenance field of one explanation, such as body.explanations.L2.
var explanationField = regexp.MustCompile(`^body\.explanations\.(L[1-4])$`)

// permitExplain allows a new example node, and a change of the target that only adds, changes or removes explanations that are marked agent.
func permitExplain(target string, c workspace.Change, n *node.Node) error {
	if c.Old == nil {
		if k := n.Kind(); k != "example" {
			return fmt.Errorf("explain adds example nodes, and this node is of the kind %s", k)
		}
		if n.Meta["formal"] != nil {
			return errors.New("explain writes no Lean, thus a new example has no formal field")
		}
		for f, v := range prov.Fields(n) {
			if m := explanationField.FindStringSubmatch(f); m != nil && !markedAgent(v, m[1]) {
				return fmt.Errorf("the explanation %s of the new node is not marked \"### %s agent\"", m[1], m[1])
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
	before, after := prov.Fields(o), prov.Fields(n)
	for _, f := range slices.Sorted(maps.Keys(union(before, after))) {
		if before[f] == after[f] {
			continue
		}
		m := explanationField.FindStringSubmatch(f)
		if m == nil {
			return fmt.Errorf("explain writes only explanations, and the change changes the field %s", f)
		}
		if old, ok := before[f]; ok && !markedAgent(old, m[1]) {
			return fmt.Errorf("explain does not change the explanation %s, because it is not marked agent", m[1])
		}
		if v, ok := after[f]; ok && !markedAgent(v, m[1]) {
			return fmt.Errorf("the explanation %s is not marked \"### %s agent\"", m[1], m[1])
		}
	}
	return nil
}

// curated are the fields of a node that curate can change.
var curated = []string{"uses", "about", "msc", "name", "aliases"}

// uncurated are the fields that a new node of curate does not have: the bindings, which formalize writes, the crosswalk edges, which align writes, and the fields of a retirement, which a curator writes.
var uncurated = []string{"formal", "same_as", "close_to", "aligned", "retired", "resolved_by"}

// permitCurate allows a new statement or concept node with no field of uncurated and no explanation, and a change of a node that changes only the fields of curated.
func permitCurate(c workspace.Change, n *node.Node) error {
	if c.Old == nil {
		if k := n.Kind(); k != "statement" && k != "concept" {
			return fmt.Errorf("curate adds statement and concept nodes, and this node is of the kind %s", k)
		}
		for _, k := range uncurated {
			if n.Meta[k] != nil {
				return fmt.Errorf("a new node of curate has no field %s", k)
			}
		}
		for f := range prov.Fields(n) {
			if strings.HasPrefix(f, "body.explanations.") {
				return errors.New("curate writes no explanations, and the explain role writes them")
			}
		}
		return nil
	}
	o, err := node.Parse(c.Path, c.Old)
	if err != nil {
		return err
	}
	if o.Body != n.Body {
		return errors.New("curate does not change the body of a node that exists")
	}
	for _, k := range slices.Sorted(maps.Keys(union(o.Meta, n.Meta))) {
		if !slices.Contains(curated, k) && !reflect.DeepEqual(o.Meta[k], n.Meta[k]) {
			return fmt.Errorf("curate changes only the fields %s of a node, and the change changes the field %s", strings.Join(curated, ", "), k)
		}
	}
	return nil
}

// permitQueue allows a change of the dedup queue that keeps each proposal and adds duplicate proposals of the curate role.
func permitQueue(before, after []byte) error {
	old, err := align.ParseQueue(before)
	if err != nil {
		return fmt.Errorf("the queue before the change: %w", err)
	}
	ps, err := align.ParseQueue(after)
	if err != nil {
		return err
	}
	have := map[string]align.Proposal{}
	for _, p := range ps {
		have[p.ID] = p
	}
	for _, p := range old {
		if q, ok := have[p.ID]; !ok || !reflect.DeepEqual(p, q) {
			return fmt.Errorf("curate adds proposals to the queue and does not change or remove the proposal %s", p.ID)
		}
		delete(have, p.ID)
	}
	for _, id := range slices.Sorted(maps.Keys(have)) {
		p := have[id]
		want := align.NewDuplicate(p.Node, p.Target)
		switch {
		case p.Node == p.Target || !isNode(p.Node) || !isNode(p.Target):
			return fmt.Errorf("the proposal %s does not name two nodes", id)
		case p.ID != want.ID || p.Node != want.Node || p.Edge != want.Edge || p.System != want.System:
			return fmt.Errorf("the proposal %s is not a duplicate proposal of %s and %s", id, p.Node, p.Target)
		case !strings.HasPrefix(p.By, "m2-agent/curate@"):
			return fmt.Errorf("the proposal %s is by %q, and curate adds only its own proposals", id, p.By)
		}
	}
	return nil
}

// isNode reports whether the id is a tag or a provisional id.
func isNode(id string) bool { return tag.IsTag(id) || tag.IsProvisional(id) }

// markedAgent reports whether each explanation heading in the text of an explanation field is "### L agent" for the level. The text of a field joins its explanations with no line break, thus a heading is not always at the start of a line.
func markedAgent(text, level string) bool {
	return strings.Count(text, "### ") == strings.Count(text, "### "+level+" agent")
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

func union[V any](a, b map[string]V) map[string]V {
	u := maps.Clone(a)
	if u == nil {
		u = map[string]V{}
	}
	maps.Copy(u, b)
	return u
}
