// Package formalize has the goal and the brief of the formalize role (spec 10.2 and 12.4).
//
// A formalize run appends bindings to the formal field of the target node, and adds the Lean files of the new declarations. A statement with no proof is a definition of a proposition, def S : Prop, because rule F06 refuses sorry (spec 07.2). [Goal] holds when the changes are inside the field permission set of formalize, the target has a new Lean binding, the library builds, and [Verify] accepts each new declaration: it has the right kind for the kind of the node, it uses no axiom other than the standard axioms and no native_decide, and the degenerate-statement checks do not find that its hypotheses are contradictory or that its negation has a short proof. The full F1 battery runs in the build of the pull request. [Brief] gives the first message of a run.
package formalize

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/workspace"
)

// matches are the values of the match field of a binding (spec 07.3).
var matches = []string{"exact", "stronger", "weaker", "variant", "partial"}

// Binding is a binding that a formalize run appended to the target.
type Binding struct {
	Decl    string
	Library string
	Match   string
}

var errNoBinding = errors.New("the run has not appended a binding to the formal field of the target yet")

// Added returns the bindings that the changes append to the formal field of the target, and the kind of the target. A new binding that is not a Lean 4 binding with a decl and a match is an error.
func Added(target string, changes []workspace.Change) ([]Binding, string, error) {
	p := tools.NodePath(target)
	i := slices.IndexFunc(changes, func(c workspace.Change) bool { return c.Path == p })
	if i < 0 || changes[i].Old == nil || changes[i].New == nil {
		return nil, "", errNoBinding
	}
	o, err := node.Parse(p, changes[i].Old)
	if err != nil {
		return nil, "", err
	}
	n, err := node.Parse(p, changes[i].New)
	if err != nil {
		return nil, "", err
	}
	before, _ := o.Meta["formal"].([]any)
	after, _ := n.Meta["formal"].([]any)
	if len(after) <= len(before) {
		return nil, "", errNoBinding
	}
	var out []Binding
	for k, v := range after[len(before):] {
		m, _ := v.(map[string]any)
		var b Binding
		b.Decl, _ = m["decl"].(string)
		b.Library, _ = m["library"].(string)
		b.Match, _ = m["match"].(string)
		at := fmt.Sprintf("formal[%d]", len(before)+k)
		switch {
		case m["system"] != "lean4":
			return nil, "", fmt.Errorf("%s has the system %v, and formalize binds Lean 4 declarations", at, m["system"])
		case b.Decl == "":
			return nil, "", fmt.Errorf("%s has no decl", at)
		case !slices.Contains(matches, b.Match):
			return nil, "", fmt.Errorf("%s has the match %q, and it must be one of %s", at, b.Match, strings.Join(matches, ", "))
		}
		out = append(out, b)
	}
	return out, n.Kind(), nil
}

// definitional reports whether a node of the kind binds to a definition and not to a statement.
func definitional(kind string) bool { return kind == "definition" || kind == "concept" }

// Verify reports each problem of the records of the new declarations for a node of the kind. Each problem is one sentence that the model can act on.
func Verify(kind string, recs []lean.Record) error {
	var errs []error
	for _, r := range recs {
		d := r.Decl
		switch {
		case r.Error != "":
			errs = append(errs, fmt.Errorf("the Lean library has no declaration %s: %s; check the name, and import its module in lean/Math2.lean", d, r.Error))
			continue
		case definitional(kind) && r.Kind != "def" && r.Kind != "inductive":
			errs = append(errs, fmt.Errorf("%s is a %s, and a %s node binds to a def, a structure or an inductive type", d, r.Kind, kind))
		case !definitional(kind) && !r.Statement():
			errs = append(errs, fmt.Errorf("%s is a %s, and a %s node binds to a statement: a theorem, or a definition of a proposition when it has no proof", d, r.Kind, kind))
		}
		var bad []string
		for _, a := range r.Axioms {
			if !slices.Contains(lean.StandardAxioms, a) {
				bad = append(bad, a)
			}
		}
		if len(bad) > 0 {
			errs = append(errs, fmt.Errorf("%s uses the axioms %s, and only %s are allowed; state a result with no proof as a definition of a proposition", d, strings.Join(bad, ", "), strings.Join(lean.StandardAxioms, ", ")))
		}
		if len(r.NativeDecideAxioms) > 0 {
			errs = append(errs, fmt.Errorf("%s uses native_decide", d))
		}
		if c := r.Checks; c != nil {
			if c.Vacuous != "" {
				errs = append(errs, fmt.Errorf("the hypotheses of %s are contradictory: %s proves False from them", d, c.Vacuous))
			}
			if c.Refuted != "" {
				errs = append(errs, fmt.Errorf("%s is false: %s proves its negation", d, c.Refuted))
			}
		}
	}
	return errors.Join(errs...)
}

// Goal returns the goal predicate of a formalize run on the work tree: the changes are inside the field permission set of formalize, the target has a new Lean binding, the library builds, and [Verify] accepts the new declarations. A build that fails is a goal that does not hold, with the reason.
func Goal(ws *workspace.Workspace, env tools.Lean, job *agent.Job) agent.Goal {
	return agent.OnChanges(ws, func(ctx context.Context, changes []workspace.Change) (bool, string, error) {
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		added, kind, err := Added(job.Target, changes)
		if err != nil {
			return false, err.Error(), nil
		}
		var decls []string
		for _, b := range added {
			decls = append(decls, b.Decl)
		}
		recs, err := env.Extract(ctx, decls, true)
		if err != nil {
			if ctx.Err() != nil {
				return false, "", ctx.Err()
			}
			return false, err.Error(), nil
		}
		if err := Verify(kind, recs); err != nil {
			return false, strings.ReplaceAll(err.Error(), "\n", "; "), nil
		}
		return true, "", nil
	})
}

// Brief returns the first message of a formalize run: the job, what to write for the kind of the target, and the target node from the work tree in a data block.
func Brief(ws *workspace.Workspace, job *agent.Job) (string, error) {
	p := tools.NodePath(job.Target)
	if p == "" {
		return "", fmt.Errorf("formalize: the target %q is not a tag or a provisional id", job.Target)
	}
	text, err := ws.Read(p)
	if err != nil {
		return "", fmt.Errorf("formalize: %w", err)
	}
	n, err := node.Parse(p, text)
	if err != nil {
		return "", fmt.Errorf("formalize: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: formalize the target %s, a %s node. The goal of the job: %s.\n\n", job.ID, job.Target, n.Kind(), strings.TrimSuffix(job.Goal, "."))
	if definitional(n.Kind()) {
		b.WriteString("Bind the node to a def, a structure or an inductive type that defines the same notion.\n\n")
	} else {
		fmt.Fprintf(&b, "Bind the node to a statement. When Mathlib has no theorem that states it, write the statement as a definition of a proposition, def name_statement : Prop := …, with the attribute @[m2 %q].\n\n", job.Target)
	}
	fmt.Fprintf(&b, "The target node:\n\n%s\n", tools.Data(p, tools.Licence(p), string(text)))
	return b.String(), nil
}
