// Package bump has the goal, the brief and the tool of the bump role (spec 07.9 and 12.1).
//
// m2 bump moves the tree to a new release of Mathlib, and it applies each rename that the deprecation data or the type hashes give. The bump role does the rest of the rebinding. The target of a bump job is a node with a Lean binding that does not resolve or is deprecated with the pinned Mathlib. The run finds the declaration that states the same result and changes formal[].decl of the binding with the tool rebind. The role writes no Lean, thus a break of the build of the library of math2 is not a job of bump. A binding has no commit field in schema node.v1, because the commit is the pin in lean/lake-manifest.json. Thus the role changes only the decl.
//
// [Goal] holds when the changes are inside the field permission set of bump, the run changed at least one binding, each changed binding did not resolve or was deprecated before the run, and each Lean binding of the target resolves and is not deprecated after it. When the old declaration resolves, the new declaration must have its kind. [AutoMerge] reports a run whose rebinds all take the new name from the deprecation data of Mathlib. Such a run can merge with no review. A person reviews each other rebind. No role writes a register, thus the run does not record the rebind in registers/bindings.reg as m2 bump does, and the person who merges the pull request records it. [Brief] gives the first message of a run.
package bump

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/api"
	bumper "github.com/tamnd/math2-agent/bump"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/workspace"
)

// A Rebinding is a change of the decl of one Lean binding of the target.
type Rebinding struct {
	Index int    // the index of the binding in the formal field
	Old   string // the declaration before the run
	New   string // the declaration after the run
}

// Rebinds returns the Lean bindings of the target whose decl the changes change, in order. A change that adds or removes a binding, or that changes the decl of a binding that is not a Lean 4 binding, is an error.
func Rebinds(target string, changes []workspace.Change) ([]Rebinding, error) {
	p := tools.NodePath(target)
	for _, c := range changes {
		if c.Path != p || c.Old == nil || c.New == nil {
			continue
		}
		o, err := node.Parse(c.Path, c.Old)
		if err != nil {
			return nil, err
		}
		n, err := node.Parse(c.Path, c.New)
		if err != nil {
			return nil, err
		}
		old, cur := entries(o), entries(n)
		if len(old) != len(cur) {
			return nil, fmt.Errorf("the change of %s adds or removes bindings", target)
		}
		var out []Rebinding
		for i := range old {
			a, _ := old[i]["decl"].(string)
			b, _ := cur[i]["decl"].(string)
			if a == b {
				continue
			}
			if cur[i]["system"] != "lean4" {
				return nil, fmt.Errorf("formal[%d] of %s is not a Lean 4 binding, and bump rebinds only Lean 4 bindings", i, target)
			}
			out = append(out, Rebinding{Index: i, Old: a, New: b})
		}
		return out, nil
	}
	return nil, nil
}

// entries returns the bindings of the formal field of the node.
func entries(n *node.Node) []map[string]any {
	var out []map[string]any
	fs, _ := n.Meta["formal"].([]any)
	for _, f := range fs {
		m, _ := f.(map[string]any)
		out = append(out, m)
	}
	return out
}

// leanDecls returns the declarations of the Lean bindings of the node, in order.
func leanDecls(n *node.Node) []string {
	var out []string
	for _, f := range entries(n) {
		if d, _ := f["decl"].(string); f["system"] == "lean4" && d != "" {
			out = append(out, d)
		}
	}
	return out
}

// target reads the target node of the job from the work tree.
func target(ws *workspace.Workspace, id string) (*node.Node, error) {
	p := tools.NodePath(id)
	if p == "" {
		return nil, fmt.Errorf("bump: the target %q is not a tag or a provisional id", id)
	}
	text, err := ws.Read(p)
	if err != nil {
		return nil, fmt.Errorf("bump: %w", err)
	}
	return node.Parse(p, text)
}

// byDecl returns the records by declaration.
func byDecl(recs []lean.Record) map[string]lean.Record {
	out := map[string]lean.Record{}
	for _, r := range recs {
		out[r.Decl] = r
	}
	return out
}

// broken returns the problem of the record with the pinned Mathlib, or "" when it resolves and is not deprecated.
func broken(r lean.Record) string {
	switch {
	case r.Error != "":
		return "it does not resolve: " + strings.TrimSpace(r.Error)
	case r.Deprecated != nil && r.Deprecated.NewName != "":
		return "it is deprecated, and the deprecation names " + r.Deprecated.NewName
	case r.Deprecated != nil:
		return "it is deprecated, and the deprecation names no replacement"
	}
	return ""
}

// Goal returns the goal predicate of a bump run on the work tree: the changes are inside the field permission set of bump, the run rebinds at least one binding of the target, each old declaration did not resolve or was deprecated, the new declaration has the kind and the type hash of an old declaration that resolves, and each Lean binding of the target resolves and is not deprecated. A rebind to another type hash needs the constraint type_change of the job. A build that fails is a goal that does not hold, with the reason.
func Goal(ws *workspace.Workspace, env tools.Lean, job *agent.Job) agent.Goal {
	return agent.OnChanges(ws, func(ctx context.Context, changes []workspace.Change) (bool, string, error) {
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		rbs, err := Rebinds(job.Target, changes)
		if err != nil {
			return false, err.Error(), nil
		}
		if len(rbs) == 0 {
			return false, "the run changed no binding of the target. Rebind each Lean binding that does not resolve or is deprecated", nil
		}
		n, err := target(ws, job.Target)
		if err != nil {
			return false, "", err
		}
		decls := leanDecls(n)
		for _, rb := range rbs {
			decls = append(decls, rb.Old)
		}
		recs, err := env.Extract(ctx, decls, false)
		if err != nil {
			if ctx.Err() != nil {
				return false, "", ctx.Err()
			}
			return false, err.Error(), nil
		}
		rs := byDecl(recs)
		var problems []string
		for _, rb := range rbs {
			o, cur := rs[rb.Old], rs[rb.New]
			if broken(o) == "" {
				problems = append(problems, fmt.Sprintf("%s resolves and is not deprecated, thus bump does not change formal[%d]. Bind %s again", rb.Old, rb.Index, rb.Old))
			} else if o.Error == "" && cur.Error == "" && cur.Kind != o.Kind {
				problems = append(problems, fmt.Sprintf("%s is a %s and %s is a %s. Bind a declaration of the same kind", rb.Old, o.Kind, rb.New, cur.Kind))
			} else if o.Error == "" && cur.Error == "" && o.TypeHash != "" && cur.TypeHash != o.TypeHash && !job.Constraints.TypeChange {
				problems = append(problems, fmt.Sprintf("%s has the type hash %s and %s has the type hash %s. A rebind keeps the statement, thus bind a declaration of the same type, or the job must permit type_change", rb.Old, o.TypeHash, rb.New, cur.TypeHash))
			}
		}
		for _, d := range leanDecls(n) {
			if why := broken(rs[d]); why != "" {
				problems = append(problems, fmt.Sprintf("the binding of %s still fails: %s", d, why))
			}
		}
		if len(problems) > 0 {
			return false, strings.Join(problems, "; "), nil
		}
		return true, "", nil
	})
}

// AutoMerge reports whether the changes of a bump run on the target can merge with no review: the run rebinds at least one binding, and each new declaration is the replacement that the deprecation of the old declaration names (spec 07.9, step 3). recs are the records of the old declarations with the pinned Mathlib. Each other rebind needs a person.
func AutoMerge(target string, changes []workspace.Change, recs []lean.Record) (bool, error) {
	if len(changes) == 0 {
		return false, nil
	}
	for _, c := range changes {
		if c.Path != tools.NodePath(target) {
			return false, nil
		}
	}
	rbs, err := Rebinds(target, changes)
	if err != nil || len(rbs) == 0 {
		return false, err
	}
	rs := byDecl(recs)
	for _, rb := range rbs {
		d := rs[rb.Old].Deprecated
		if d == nil || d.NewName == "" || d.NewName != rb.New {
			return false, nil
		}
	}
	return true, nil
}

// Rebind returns the tool rebind, which changes the decl of a Lean binding of the target of the job in the work tree. The write goes through write_file, thus the field permission set of the role applies to it.
func Rebind(ws *workspace.Workspace) agent.Tool {
	write := tools.Write(ws)
	return agent.Func{
		Def: llm.Tool{
			Name:        "rebind",
			Description: "Change the declaration of a Lean binding of the target node to the declaration that states the same result with the pinned Mathlib. The match and the other fields of the binding stay the same.",
			Schema: tools.Schema([]api.Param{
				{Name: "decl", Type: api.String, Required: true, Summary: "the declaration that the binding names now, for example Subgroup.card_subgroup_dvd_card"},
				{Name: "new", Type: api.String, Required: true, Summary: "the declaration that the binding names after the change"},
				{Name: "why", Type: api.String, Required: true, Summary: "why the new declaration states the same result, for example the deprecation entry or the map of the hypotheses"},
			}),
		},
		Fn: func(ctx context.Context, job *agent.Job, input json.RawMessage) (agent.Result, error) {
			var in struct{ Decl, New, Why string }
			if err := json.Unmarshal(input, &in); err != nil {
				return agent.Result{}, err
			}
			switch {
			case strings.TrimSpace(in.Why) == "":
				return agent.Result{}, errors.New("give the reason why the new declaration states the same result")
			case in.New == "" || strings.ContainsAny(in.New, " \t\n"):
				return agent.Result{}, fmt.Errorf("%q is not the name of a declaration", in.New)
			case in.New == in.Decl:
				return agent.Result{}, errors.New("the new declaration is the declaration of the binding")
			}
			n, err := target(ws, job.Target)
			if err != nil {
				return agent.Result{}, err
			}
			if !bumper.Rename(n, map[string]string{in.Decl: in.New}) {
				return agent.Result{}, fmt.Errorf("the target has no Lean binding of %s. Its Lean bindings are %s", in.Decl, strings.Join(leanDecls(n), ", "))
			}
			s, err := schema.Load(ws.Dir)
			if err != nil {
				return agent.Result{}, err
			}
			args, err := json.Marshal(map[string]string{"path": n.Path, "text": string(node.Format(n, s.Order))})
			if err != nil {
				return agent.Result{}, err
			}
			if _, err := write.Call(ctx, job, args); err != nil {
				return agent.Result{}, err
			}
			return agent.Result{Text: fmt.Sprintf("Changed the binding of %s from %s to %s.", job.Target, in.Decl, in.New)}, nil
		},
	}
}

// Brief returns the first message of a bump run: the job, each Lean binding of the target with its problem with the pinned Mathlib, and the target node from the work tree in a data block. A build of the library that fails is an error, because bump writes no Lean.
func Brief(ctx context.Context, ws *workspace.Workspace, env tools.Lean, job *agent.Job) (string, error) {
	n, err := target(ws, job.Target)
	if err != nil {
		return "", err
	}
	decls := leanDecls(n)
	if len(decls) == 0 {
		return "", fmt.Errorf("bump: the target %s has no Lean binding", job.Target)
	}
	recs, err := env.Extract(ctx, decls, false)
	if err != nil {
		return "", fmt.Errorf("bump: %w", err)
	}
	rs := byDecl(recs)
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: rebind the target %s, a %s node, after the bump of Mathlib. The goal of the job: %s.\n\nThe Lean bindings of the target with the pinned Mathlib:\n\n", job.ID, job.Target, n.Kind(), strings.TrimSuffix(job.Goal, "."))
	for i, f := range entries(n) {
		d, _ := f["decl"].(string)
		if f["system"] != "lean4" || d == "" {
			continue
		}
		why := broken(rs[d])
		if why == "" {
			why = "it resolves"
			if k := rs[d].Kind; k != "" {
				why = "it resolves, and it is a " + k
			}
		}
		fmt.Fprintf(&b, "- formal[%d] %s: %s.\n", i, d, why)
	}
	text, err := ws.Read(n.Path)
	if err != nil {
		return "", fmt.Errorf("bump: %w", err)
	}
	fmt.Fprintf(&b, "\nFor each binding that does not resolve or is deprecated, find the declaration that states the same result and change the binding with rebind.\n\nThe target node:\n\n%s\n", tools.Data(n.Path, tools.Licence(n.Path), string(text)))
	return b.String(), nil
}
