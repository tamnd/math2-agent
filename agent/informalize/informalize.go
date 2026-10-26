// Package informalize has the goal and the brief of the informalize role (corpus K6.9, K7.3 and K7.5).
//
// The target of an informalize job is a declaration of Mathlib that no person described, in the form mathlib:DECL, as in the record plane. The run writes the node of the declaration: a statement node for a theorem or a proposition, and a definition or a concept node for another definition. The node has a name, its main section, its uses and one binding at F0 to the declaration: an exact Lean binding with no fidelity reviews. When a node binds the declaration already, the run writes the sections of that node, and only the sections that informalize wrote and no person confirmed.
//
// The run gets the declaration, its type, its docstring, its module and the nodes of math2 that bind the constants of its type. The docstrings of Mathlib are Apache-2.0 text, thus the run can start from one. Compiling is not faithfulness: the F1 battery of spec 10.4 runs on the pair in the build, as on each other binding, and the sample audit counts informalize as a stratum of its own (corpus K9.7).
package informalize

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/curate"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/agent/write"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/ingest/mathlib"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/notation"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/workspace"
)

// Agent is the prefix of the agent of each provenance line that an informalize run makes, before the version of the role.
const Agent = "m2-agent/informalize@"

// Name returns the name of the declaration of the target, which is in the form mathlib:DECL.
func Name(target string) (string, bool) {
	name, ok := strings.CutPrefix(target, "mathlib:")
	return name, ok && name != ""
}

// Decl is what the run knows of the declaration of its target.
type Decl struct {
	Name      string // for example Subgroup.card_subgroup_dvd_card
	Kind      string // the kind in Lean: theorem, def, inductive and so on
	Statement bool   // a theorem, or a definition whose type is a proposition
	Module    string
	URL       string // the place of the declaration in the documentation of Mathlib
	Type      string // the type as Lean prints it
	Doc       string // the docstring, or ""

	Uses        map[string]string      // each constant of the type that a node of math2 binds, with the tag of the node
	Node        string                 // the tag of the node that binds the declaration, or ""
	Conventions []*notation.Convention // the conventions of the register whose concept is a node of Uses
}

// Load gets the declaration of the target from the pinned Mathlib with env, its docstring from the records rs, and the nodes of the work tree that bind it and the constants of its type. A nil rs gives no docstring.
func Load(ctx context.Context, ws *workspace.Workspace, env tools.Lean, rs *curate.Records, target string) (*Decl, error) {
	name, ok := Name(target)
	if !ok {
		return nil, fmt.Errorf("informalize: the target %q is not in the form mathlib:DECL", target)
	}
	recs, err := env.Extract(ctx, []string{name}, false)
	if err != nil {
		return nil, fmt.Errorf("informalize: %w", err)
	}
	if len(recs) != 1 || recs[0].Error != "" {
		why := "no record"
		if len(recs) == 1 {
			why = recs[0].Error
		}
		return nil, fmt.Errorf("informalize: the pinned Mathlib has no declaration %s: %s", name, why)
	}
	r := recs[0]
	d := &Decl{
		Name: name, Kind: r.Kind, Statement: r.Statement(), Module: r.Module, Type: r.StatementPP,
		URL:  mathlib.Record(lean.Decl{Decl: name, Module: r.Module}).URL,
		Uses: map[string]string{},
	}
	if rs != nil {
		if rec, err := rs.Get(target); err == nil {
			d.Doc = rec.Record.Text
		}
	}
	nodes, err := readTree(ws.Dir)
	if err != nil {
		return nil, err
	}
	bound := map[string]string{}
	for _, n := range nodes {
		for _, f := range bindings(n, false) {
			if _, ok := bound[f]; !ok {
				bound[f] = n.ID
			}
		}
		if d.Node == "" && slices.Contains(bindings(n, true), name) {
			d.Node = n.ID
		}
	}
	for _, c := range r.UsesConstantsInStatement {
		if t := bound[c]; t != "" && t != d.Node {
			d.Uses[c] = t
		}
	}
	cs, err := notation.LoadConventions(ws.Dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, c := range cs {
		if c.Concept != "" && slices.Contains(slices.Collect(maps.Values(d.Uses)), c.Concept) {
			d.Conventions = append(d.Conventions, c)
		}
	}
	return d, nil
}

// bindings returns the declarations of the Lean bindings of the node, or of its Mathlib bindings only.
func bindings(n *node.Node, mathlib bool) []string {
	var out []string
	for _, f := range entries(n) {
		if d, ok := f["decl"].(string); ok && f["system"] == "lean4" && (!mathlib || f["library"] == "mathlib") {
			out = append(out, d)
		}
	}
	return out
}

func entries(n *node.Node) []map[string]any {
	list, _ := n.Meta["formal"].([]any)
	var out []map[string]any
	for _, x := range list {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func readTree(dir string) ([]*node.Node, error) {
	files, err := check.ReadTree(dir)
	if err != nil {
		return nil, err
	}
	var out []*node.Node
	for _, f := range files {
		if n, err := node.Parse(f.Path, f.Data); err == nil {
			out = append(out, n)
		}
	}
	return out, nil
}

// NodeKind returns the kind of the node that binds the declaration: statement for a theorem or a proposition, and definition for another definition.
func (d *Decl) NodeKind() string {
	if d.Statement {
		return "statement"
	}
	return "definition"
}

var leanName = regexp.MustCompile(`_|[\p{L}]\.[\p{L}]`)

// LeanName reports whether the name of a node is a Lean name and not the name that a textbook uses: it is the declaration or its last component, or it has an underscore or a dot between two letters.
func (d *Decl) LeanName(name string) bool {
	last := d.Name[strings.LastIndex(d.Name, ".")+1:]
	return name == d.Name || name == last || leanName.MatchString(name)
}

// Goal returns the goal of an informalize run on the job, with the declaration d.
func Goal(ws *workspace.Workspace, job *agent.Job, d *Decl) agent.Goal {
	return agent.OnChanges(ws, func(_ context.Context, changes []workspace.Change) (bool, string, error) {
		if len(changes) == 0 {
			return false, "the run has not changed the tree yet", nil
		}
		if err := agent.Permit(job.Role, job.Target, changes); err != nil {
			return false, err.Error(), nil
		}
		findings, err := curate.Check(ws, changes)
		if err != nil {
			return false, "", err
		}
		if len(findings) > 0 {
			lines := make([]string, 0, 5)
			for _, f := range findings[:min(len(findings), 5)] {
				lines = append(lines, f.String())
			}
			return false, fmt.Sprintf("m2 check finds %d problems in the changed nodes: %s", len(findings), strings.Join(lines, "; ")), nil
		}
		nodes, err := readTree(ws.Dir)
		if err != nil {
			return false, "", err
		}
		var bound []*node.Node
		for _, n := range nodes {
			if slices.Contains(bindings(n, true), d.Name) {
				bound = append(bound, n)
			}
		}
		switch {
		case len(bound) == 0:
			return false, fmt.Sprintf("no node binds %s. Add the node in nodes/~/ with the binding {system: lean4, library: mathlib, decl: %s, match: exact}", d.Name, d.Name), nil
		case len(bound) > 1:
			var ids []string
			for _, n := range bound {
				ids = append(ids, n.ID)
			}
			return false, fmt.Sprintf("%d nodes bind %s: %s. A declaration has one node, thus keep one", len(bound), d.Name, strings.Join(ids, ", ")), nil
		}
		n := bound[0]
		if i := slices.IndexFunc(changes, func(c workspace.Change) bool { return c.Path == n.Path && c.Old != nil }); i >= 0 {
			old, err := node.Parse(n.Path, changes[i].Old)
			if err != nil {
				return false, "", err
			}
			if why, err := write.Own(ws.Dir, Agent, old, n); why != "" || err != nil {
				return false, why, err
			}
		}
		if why := d.check(n); why != "" {
			return false, why, nil
		}
		for _, c := range changes {
			if c.Old != nil || c.Path == n.Path {
				continue
			}
			m, err := node.Parse(c.Path, c.New)
			if err != nil {
				return false, "", err
			}
			if m.Kind() != "concept" || n.Get("defines") != m.ID {
				return false, fmt.Sprintf("the run adds %s, and informalize adds only the node of the declaration and the concept that it defines", m.ID), nil
			}
		}
		return true, "", nil
	})
}

// check checks the node of the declaration: its kind, its name, its uses, the bold term of a definition and its conventions. m2 check finds a missing main section.
func (d *Decl) check(n *node.Node) string {
	if k := n.Kind(); k != d.NodeKind() {
		return fmt.Sprintf("%s is a %s in Lean, thus its node is of the kind %s, and %s is of the kind %s", d.Name, d.Kind, d.NodeKind(), n.ID, k)
	}
	if d.LeanName(n.Name()) {
		return fmt.Sprintf("the name %q is a Lean name. Give the node the name that a textbook uses, in words", n.Name())
	}
	for _, c := range slices.Sorted(maps.Keys(d.Uses)) {
		if t := d.Uses[c]; !slices.Contains(n.List("uses"), t) {
			return fmt.Sprintf("the type of %s has %s, and the node %s binds it. Add %s to uses, and link it in the text", d.Name, c, t, t)
		}
	}
	if n.Kind() == "definition" && !write.Bold(n, prov.Fields(n)["body.definition"]) {
		return fmt.Sprintf("the Definition section of %s does not name the defined term in bold. Write the name of the node, or an alias, as **name** the first time", n.ID)
	}
	for _, c := range d.Conventions {
		for _, m := range conventions(n) {
			if m["convention"] == c.ID && m["option"] != c.Default {
				return fmt.Sprintf("the node names the option %v of the convention %s, and Mathlib uses %s, the default of math2. Remove the entry, and state the option %s in the text", m["option"], c.ID, c.Default, c.Default)
			}
		}
	}
	return ""
}

func conventions(n *node.Node) []map[string]any {
	list, _ := n.Meta["conventions"].([]any)
	var out []map[string]any
	for _, x := range list {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// Brief returns the first message of an informalize run: the job, the declaration with its type and its docstring in a data block, the nodes that bind the constants of its type, the conventions that apply, and the node to write.
func Brief(ws *workspace.Workspace, job *agent.Job, d *Decl) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Job %s: write the informal text of the Mathlib declaration %s, a %s in the module %s. The goal of the job: %s.\n\n", job.ID, d.Name, d.Kind, d.Module, strings.TrimSuffix(job.Goal, "."))
	text := d.Kind + " " + d.Name + " : " + d.Type
	if d.Doc != "" {
		text = "/-- " + d.Doc + " -/\n" + text
	}
	fmt.Fprintf(&b, "The declaration with the pinned Mathlib:\n\n%s\n\n", tools.Data(d.URL, "Apache-2.0", text))
	if len(d.Uses) > 0 {
		b.WriteString("The constants of its type that math2 has, with their nodes. Read each node with get, add it to uses and link it in the text:\n\n")
		for _, c := range slices.Sorted(maps.Keys(d.Uses)) {
			fmt.Fprintf(&b, "- %s: %s\n", c, d.Uses[c])
		}
		b.WriteString("\n")
	}
	if len(d.Conventions) > 0 {
		b.WriteString("These conventions change the meaning of the text. Mathlib uses the default of math2, thus state it in the text when it matters, and give the node no conventions field for it:\n\n")
		for _, c := range d.Conventions {
			fmt.Fprintf(&b, "- %s: %s\n", c.ID, c.Default)
		}
		b.WriteString("\n")
	}
	if d.Node != "" {
		p := tools.NodePath(d.Node)
		text, err := ws.Read(p)
		if err != nil {
			return "", fmt.Errorf("informalize: %w", err)
		}
		fmt.Fprintf(&b, "The node %s binds the declaration already. Write its sections in %s, and do not add a node. Change only the sections that informalize wrote and no person confirmed.\n\nThe node:\n\n%s\n", d.Node, p, tools.Data(p, tools.Licence(p), string(text)))
		return b.String(), nil
	}
	kind, main := "statement\nrole: theorem", "Statement"
	if !d.Statement {
		kind, main = "definition", "Definition"
		b.WriteString("A definition node defines a concept. Give defines the tag of the concept node of the notion. When math2 has no such concept, add a concept node in nodes/~/ with a Summary section, and give defines its provisional id.\n\n")
	}
	fmt.Fprintf(&b, "Add the node in nodes/~/ID.md, where ID is a new provisional id such as ~1a2b3c4d, in this form:\n\n")
	uses := ""
	if len(d.Uses) > 0 {
		uses = "uses: [" + strings.Join(slices.Compact(slices.Sorted(maps.Values(d.Uses))), ", ") + "]\n"
	}
	if !d.Statement {
		uses = "defines: TAG\n" + uses
	}
	fmt.Fprintf(&b, "---\ntag: ~ID\nkind: %s\nname: the name in words\n%sformal:\n  - system: lean4\n    library: mathlib\n    decl: %s\n    match: exact\n---\n\n## %s\n\nThe text.\n", kind, uses, d.Name, main)
	return b.String(), nil
}
