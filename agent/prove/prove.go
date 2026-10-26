// Package prove has the goal and the brief of the prove role (spec 12.4).
//
// A result with no proof is stated in the library as a definition of a proposition, S : Prop, because rule F06 refuses sorry (spec 07.2). A prove run adds a theorem T : S and changes the decl of the binding of the target from S to T. The run works against a pinned statement (spec 12.2): [NewPin] reads the binding of the target and the library at the base of the run, and keeps the value hash of S and of each definition of math2 that S uses. [Goal] holds when the changes are inside the field permission set of prove, the library builds, and [Pin.Verify] accepts the records: T is a theorem whose type is S, its axioms are in the standard set, it has no native_decide unless the job permits it, and the closure of S is not changed. [Brief] gives the first message of a run.
package prove

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/workspace"
)

// Pin is a pinned statement (spec 12.2).
type Pin struct {
	Target       string            `json:"target"`    // the id of the node of the job
	Binding      int               `json:"binding"`   // the index of the binding in the formal field of the target
	Statement    string            `json:"statement"` // the definition S : Prop that the binding names at the base
	Module       string            `json:"module"`    // the module that declares the statement
	Levels       []string          `json:"levels,omitempty"`
	Closure      map[string]string `json:"closure"` // the hash of the statement and of each declaration of math2 that it uses: the value hash, or the type hash for a theorem or a declaration with no value
	NativeDecide bool              `json:"native_decide,omitempty"`
}

// inMath2 reports whether the declaration is in the namespace of the Lean library of math2, thus a run can change it. Lean and Mathlib are fixed by the toolchain pin.
func inMath2(decl string) bool { return strings.HasPrefix(decl, "Math2.") }

// closureHash returns the hash of the record that a run must not change.
func closureHash(r lean.Record) string {
	if r.Kind == "theorem" || r.ValueHash == "" {
		return r.TypeHash
	}
	return r.ValueHash
}

// binding returns the decl of each formal entry of the node, with "" for an entry that is not a Lean binding to the library of math2.
func binding(n *node.Node) []string {
	list, _ := n.Meta["formal"].([]any)
	out := make([]string, len(list))
	for i, v := range list {
		m, _ := v.(map[string]any)
		if m["system"] == "lean4" && m["library"] == "math2" {
			out[i], _ = m["decl"].(string)
		}
	}
	return out
}

// target reads and parses the node of the job from the work tree.
func target(ws *workspace.Workspace, id string) (*node.Node, error) {
	p := tools.NodePath(id)
	if p == "" {
		return nil, fmt.Errorf("the target %q is not a tag or a provisional id", id)
	}
	data, err := ws.Read(p)
	if err != nil {
		return nil, err
	}
	return node.Parse(p, data)
}

// NewPin reads the pin of the job from the work tree, before the run changes it. The pin is on the first binding of the target to a definition of a proposition in the library of math2. The closure has the statement, each declaration of math2 that its value uses, and so on. The pin permits native_decide when the constraints of the job do.
func NewPin(ctx context.Context, ws *workspace.Workspace, env tools.Lean, job *agent.Job) (*Pin, error) {
	n, err := target(ws, job.Target)
	if err != nil {
		return nil, fmt.Errorf("prove: %w", err)
	}
	p := &Pin{Target: job.Target, Binding: -1, Closure: map[string]string{}, NativeDecide: job.Constraints.NativeDecide}
	var proved []string
	for i, d := range binding(n) {
		if d == "" {
			continue
		}
		recs, err := env.Extract(ctx, []string{d}, false)
		if err != nil {
			return nil, fmt.Errorf("prove: %w", err)
		}
		if len(recs) != 1 || !recs[0].Statement() {
			continue
		}
		if r := recs[0]; r.Kind == "def" {
			p.Binding, p.Statement, p.Module, p.Levels = i, d, r.Module, r.LevelParams
			break
		}
		proved = append(proved, d)
	}
	switch {
	case p.Binding < 0 && len(proved) > 0:
		return nil, fmt.Errorf("prove: the target %s is bound to the theorem %s, thus it has a proof", job.Target, proved[0])
	case p.Binding < 0:
		return nil, fmt.Errorf("prove: the target %s has no binding to a definition of a proposition in the library of math2", job.Target)
	}
	next := []string{p.Statement}
	for len(next) > 0 {
		recs, err := env.Extract(ctx, next, false)
		if err != nil {
			return nil, fmt.Errorf("prove: %w", err)
		}
		next = nil
		for _, c := range recs {
			if c.Error != "" {
				return nil, fmt.Errorf("prove: Lean has no record of %s in the closure of %s: %s", c.Decl, p.Statement, c.Error)
			}
			p.Closure[c.Decl] = closureHash(c)
			if c.Kind == "theorem" {
				continue
			}
			for _, d := range append(slices.Clone(c.UsesConstantsInStatement), c.UsesDecls...) {
				if _, ok := p.Closure[d]; !ok && inMath2(d) && !slices.Contains(next, d) {
					next = append(next, d)
				}
			}
		}
	}
	return p, nil
}

// ProofType returns the canonical form of the type of a proof of the statement: the constant of the statement with its universe parameters (spec 07.4).
func (p *Pin) ProofType() string {
	parts := []string{"c " + p.Statement}
	for i := range p.Levels {
		parts = append(parts, fmt.Sprintf("u%d", i))
	}
	return strings.Join(parts, " ") + "\n"
}

// Decls returns the closure of the pin in order.
func (p *Pin) Decls() []string { return slices.Sorted(maps.Keys(p.Closure)) }

// Verify reports each way in which the records do not keep the pin. The records come from the library of the work tree, and proof is the declaration that the binding names after the run. Each problem is one sentence that the model can act on.
func (p *Pin) Verify(proof string, recs []lean.Record) error {
	by := map[string]lean.Record{}
	for _, r := range recs {
		by[r.Decl] = r
	}
	var errs []error
	r, ok := by[proof]
	switch {
	case proof == p.Statement:
		errs = append(errs, fmt.Errorf("formal[%d].decl of %s still names the statement %s; write a theorem of type %s and change the decl to it", p.Binding, p.Target, p.Statement, p.Statement))
	case !ok:
		errs = append(errs, fmt.Errorf("the extraction gave no record of %s", proof))
	case r.Error != "":
		errs = append(errs, fmt.Errorf("the Lean library has no declaration %s: %s; check the name, and import its module in lean/Math2.lean", proof, r.Error))
	case r.Kind != "theorem":
		errs = append(errs, fmt.Errorf("%s is a %s, not a theorem", proof, r.Kind))
	case r.Type != p.ProofType():
		errs = append(errs, fmt.Errorf("the type of %s is %s, and it must be exactly %s", proof, r.StatementPP, p.Statement))
	}
	if ok && r.Error == "" && proof != p.Statement {
		var bad []string
		for _, a := range r.Axioms {
			switch {
			case a == "sorryAx":
				errs = append(errs, fmt.Errorf("the proof %s uses sorry", proof))
			case !slices.Contains(lean.StandardAxioms, a):
				bad = append(bad, a)
			}
		}
		if len(bad) > 0 {
			errs = append(errs, fmt.Errorf("the proof %s uses the axioms %s, and only %s are allowed", proof, strings.Join(bad, ", "), strings.Join(lean.StandardAxioms, ", ")))
		}
		if len(r.NativeDecideAxioms) > 0 && !p.NativeDecide {
			errs = append(errs, fmt.Errorf("the proof %s uses native_decide, and the job does not permit it; use decide, decide_cbv or cbv", proof))
		}
	}
	for _, d := range p.Decls() {
		c, ok := by[d]
		switch {
		case !ok || c.Error != "":
			errs = append(errs, fmt.Errorf("the statement uses %s, and the library no longer has it", d))
		case closureHash(c) != p.Closure[d]:
			errs = append(errs, fmt.Errorf("the run changed %s, which the statement uses; put it back as it was", d))
		}
	}
	return errors.Join(errs...)
}

// Goal returns the goal predicate of a prove run on the work tree: the changes are inside the field permission set of prove, the library builds, and the pin holds. A build that fails is a goal that does not hold, with the reason.
func Goal(ws *workspace.Workspace, env tools.Lean, job *agent.Job, pin *Pin) agent.Goal {
	return agent.OnChanges(ws, func(ctx context.Context, changes []workspace.Change) (bool, string, error) {
		if len(changes) == 0 {
			return false, "the run has not changed a file yet", nil
		}
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		n, err := target(ws, job.Target)
		if err != nil {
			return false, err.Error(), nil
		}
		decls := binding(n)
		if pin.Binding >= len(decls) || decls[pin.Binding] == "" {
			return false, fmt.Sprintf("the target has no formal[%d] that binds to the library of math2", pin.Binding), nil
		}
		proof := decls[pin.Binding]
		recs, err := env.Extract(ctx, append([]string{proof}, pin.Decls()...), false)
		if err != nil {
			if ctx.Err() != nil {
				return false, "", ctx.Err()
			}
			return false, err.Error(), nil
		}
		if err := pin.Verify(proof, recs); err != nil {
			return false, strings.ReplaceAll(err.Error(), "\n", "; "), nil
		}
		return true, "", nil
	})
}

// ModuleFile returns the path in the tree of the file of a module of the Lean library, for example lean/Math2/Algebra/Lagrange.lean for Math2.Algebra.Lagrange.
func ModuleFile(module string) string {
	return "lean/" + strings.ReplaceAll(module, ".", "/") + ".lean"
}

// Brief returns the first message of a prove run: the job, the pinned statement, and the target node and the file of the statement from the work tree, each in a data block.
func Brief(ws *workspace.Workspace, job *agent.Job, pin *Pin) (string, error) {
	p, file := tools.NodePath(job.Target), ModuleFile(pin.Module)
	nodeText, err := ws.Read(p)
	if err != nil {
		return "", fmt.Errorf("prove: %w", err)
	}
	leanText, err := ws.Read(file)
	if err != nil {
		return "", fmt.Errorf("prove: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: prove the target %s. The goal of the job: %s.\n\n", job.ID, job.Target, strings.TrimSuffix(job.Goal, "."))
	fmt.Fprintf(&b, "The pinned statement is the definition %s : Prop in %s. formal[%d] of the target binds to it. Write a theorem of type %s, move the attribute @[m2 %q] from %s to the theorem, and change formal[%d].decl of the target to the name of the theorem. Do not change the value of %s or of a definition that it uses.\n\n", pin.Statement, file, pin.Binding, pin.Statement, job.Target, pin.Statement, pin.Binding, pin.Statement)
	if pin.NativeDecide {
		b.WriteString("The job permits native_decide. Use it only when decide, decide_cbv and cbv cannot do the same work.\n\n")
	} else {
		b.WriteString("The job does not permit native_decide.\n\n")
	}
	fmt.Fprintf(&b, "The target node:\n\n%s\n\nThe file of the statement:\n\n%s\n", tools.Data(p, tools.Licence(p), string(nodeText)), tools.Data(file, tools.Licence(file), string(leanText)))
	return b.String(), nil
}
