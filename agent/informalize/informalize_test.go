package informalize_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/informalize"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/workspace"
)

// fakeLean gives the records of the declarations of a fixture, and an error record for each other declaration. The types are short forms that we wrote for the tests.
type fakeLean map[string]lean.Record

func (fakeLean) Check(context.Context, string) (*lean.Elaboration, error) {
	return nil, errors.New("informalize writes no Lean")
}

func (f fakeLean) Extract(_ context.Context, decls []string, _ bool) ([]lean.Record, error) {
	out := make([]lean.Record, len(decls))
	for i, d := range decls {
		r, ok := f[d]
		if !ok {
			r = lean.Record{Decl: d, Error: "unknown declaration"}
		}
		out[i] = r
	}
	return out, nil
}

var env = fakeLean{
	"Subgroup.card_subgroup_dvd_card": {
		Decl: "Subgroup.card_subgroup_dvd_card", Kind: "theorem", Module: "Mathlib.GroupTheory.Coset.Card",
		StatementPP:              "∀ {G : Type u_1} [Group G] [Fintype G] (H : Subgroup G) [Fintype H], Fintype.card H ∣ Fintype.card G",
		UsesConstantsInStatement: []string{"Dvd.dvd", "Fintype.card", "Group", "Subgroup"},
	},
	"IsCyclic": {
		Decl: "IsCyclic", Kind: "inductive", Module: "Mathlib.GroupTheory.SpecificGroups.Cyclic",
		StatementPP: "(G : Type u_1) → [Group G] → Prop", UsesConstantsInStatement: []string{"Group"},
	},
}

const (
	groupPath = "nodes/G5S/G5S1M9.md"
	nodePath  = "nodes/~/~1a2b3c4d.md"
	group     = "---\ntag: G5S1M9\nkind: definition\nname: Group\nformal:\n  - system: lean4\n    library: mathlib\n    decl: Group\n    match: exact\n---\n\n## Definition\n\nA **group** is a set with an associative operation, an identity and inverses.\n"
	register  = "- id: group-law\n  concept: G5S1M9\n  default: multiplicative\n  options:\n    - option: multiplicative\n    - option: additive\n"
	lagrange  = "---\ntag: ~1a2b3c4d\nkind: statement\nrole: theorem\nname: Lagrange's theorem\nuses: [G5S1M9]\nformal:\n  - system: lean4\n    library: mathlib\n    decl: Subgroup.card_subgroup_dvd_card\n    match: exact\n---\n\n## Statement\n\nLet $G$ be a finite [[group|G5S1M9]] and let $H$ be a subgroup of $G$. Then the order of $H$ divides the order of $G$.\n"
	concept   = "---\ntag: ~2b3c4d5e\nkind: concept\nname: Cyclic group\n---\n\n## Summary\n\nA cyclic group is a group that one element generates.\n"
	cyclic    = "---\ntag: ~1a2b3c4d\nkind: definition\nname: Cyclic group (definition)\naliases: [cyclic-group]\ndefines: ~2b3c4d5e\nuses: [G5S1M9]\nformal:\n  - system: lean4\n    library: mathlib\n    decl: IsCyclic\n    match: exact\n---\n\n## Definition\n\nA [[group|G5S1M9]] $G$ is a **cyclic group** when some $g \\in G$ generates $G$.\n"
)

// tree makes a git repository with the files in one commit and opens it as a workspace.
func tree(t *testing.T, files map[string]string) *workspace.Workspace {
	t.Helper()
	dir := t.TempDir()
	all := map[string]string{groupPath: group, "notation/conventions.yaml": register}
	for k, v := range files {
		all[k] = v
	}
	for name, text := range all {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	w, err := workspace.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func job(target string) *agent.Job {
	return &agent.Job{ID: "j-2026-10-27-1", Role: "informalize", Target: target, Goal: "the statement of the declaration in words", OnBehalfOf: "github:octocat"}
}

func TestName(t *testing.T) {
	for target, want := range map[string]bool{"mathlib:IsCyclic": true, "mathlib:": false, "IsCyclic": false, "3C53TD": false} {
		if _, ok := informalize.Name(target); ok != want {
			t.Errorf("Name(%q) is %v", target, ok)
		}
	}
	d := &informalize.Decl{Name: "Subgroup.card_subgroup_dvd_card"}
	for name, want := range map[string]bool{
		"Lagrange's theorem":              false,
		"The theorem of Lagrange (I.2.3)": false,
		"card_subgroup_dvd_card":          true,
		"Subgroup.card_subgroup_dvd_card": true,
		"Subgroup.card":                   true,
	} {
		if got := d.LeanName(name); got != want {
			t.Errorf("LeanName(%q) = %v", name, got)
		}
	}
}

func TestLoad(t *testing.T) {
	ws := tree(t, nil)
	d, err := informalize.Load(context.Background(), ws, env, nil, "mathlib:Subgroup.card_subgroup_dvd_card")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Statement || d.Kind != "theorem" || d.Node != "" || len(d.Uses) != 1 || d.Uses["Group"] != "G5S1M9" {
		t.Errorf("the declaration is %+v", d)
	}
	if d.URL != "https://leanprover-community.github.io/mathlib4_docs/Mathlib/GroupTheory/Coset/Card.html#Subgroup.card_subgroup_dvd_card" {
		t.Errorf("the URL is %s", d.URL)
	}
	if len(d.Conventions) != 1 || d.Conventions[0].ID != "group-law" {
		t.Errorf("the conventions are %+v", d.Conventions)
	}
	if _, err := informalize.Load(context.Background(), ws, env, nil, "mathlib:Subgroup.nope"); err == nil || !strings.Contains(err.Error(), "unknown declaration") {
		t.Errorf("a declaration that Mathlib does not have gives %v", err)
	}
	if _, err := informalize.Load(context.Background(), ws, env, nil, "G5S1M9"); err == nil {
		t.Error("a tag is not a declaration, and Load gave no error")
	}
	d, err = informalize.Load(context.Background(), tree(t, map[string]string{"nodes/3C5/3C53TD.md": strings.Replace(lagrange, "~1a2b3c4d", "3C53TD", 1)}), env, nil, "mathlib:Subgroup.card_subgroup_dvd_card")
	if err != nil || d.Node != "3C53TD" {
		t.Errorf("the node of a bound declaration is %q, %v", d.Node, err)
	}
}

func TestBrief(t *testing.T) {
	ws := tree(t, nil)
	d, err := informalize.Load(context.Background(), ws, env, nil, "mathlib:Subgroup.card_subgroup_dvd_card")
	if err != nil {
		t.Fatal(err)
	}
	d.Doc = "The order of a subgroup divides the order of the group."
	brief, err := informalize.Brief(ws, job("mathlib:Subgroup.card_subgroup_dvd_card"), d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Job j-2026-10-27-1: write the informal text of the Mathlib declaration Subgroup.card_subgroup_dvd_card, a theorem in the module Mathlib.GroupTheory.Coset.Card.",
		`licence="Apache-2.0">` + "\n/-- The order of a subgroup divides the order of the group. -/\ntheorem Subgroup.card_subgroup_dvd_card : ∀ {G",
		"- Group: G5S1M9\n",
		"- group-law: multiplicative\n",
		"in this form:\n\n---\ntag: ~ID\nkind: statement\n",
		"role: theorem\nname: the name in words\nuses: [G5S1M9]\nformal:\n  - system: lean4\n    library: mathlib\n    decl: Subgroup.card_subgroup_dvd_card\n    match: exact\n---\n\n## Statement\n",
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief has no %q:\n%s", want, brief)
		}
	}
	ws = tree(t, map[string]string{"nodes/3C5/3C53TD.md": strings.Replace(lagrange, "~1a2b3c4d", "3C53TD", 1)})
	if d, err = informalize.Load(context.Background(), ws, env, nil, "mathlib:Subgroup.card_subgroup_dvd_card"); err != nil {
		t.Fatal(err)
	}
	if brief, err = informalize.Brief(ws, job("mathlib:Subgroup.card_subgroup_dvd_card"), d); err != nil || !strings.Contains(brief, "The node 3C53TD binds the declaration already") || strings.Contains(brief, "Add the node") {
		t.Errorf("the brief of a bound declaration is %v\n%s", err, brief)
	}
}

var u = llm.Usage{Input: 1000, Output: 200}

func call(path, text string) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
}

func stuck() *llm.Response {
	return llmtest.Call(agent.StuckTool, map[string]string{"reason": "no"}, u)
}

// run runs an informalize job on a new tree with the scripted model.
func run(t *testing.T, files map[string]string, target string, model ...*llm.Response) *agent.Record {
	t.Helper()
	ws := tree(t, files)
	role, err := agent.LoadRole(prompts.FS, "informalize")
	if err != nil {
		t.Fatal(err)
	}
	j := job(target)
	d, err := informalize.Load(context.Background(), ws, env, nil, target)
	if err != nil {
		t.Fatal(err)
	}
	brief, err := informalize.Brief(ws, j, d)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	rn := agent.Runner{Role: role, Model: llmtest.New(model...), Tools: ts, Goal: informalize.Goal(ws, j, d)}
	rec, err := rn.Run(context.Background(), j, 0, brief)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestRun(t *testing.T) {
	steps := []struct{ text, why string }{
		{strings.Replace(lagrange, "name: Lagrange's theorem", "name: card_subgroup_dvd_card", 1), "is a Lean name"},
		{strings.Replace(lagrange, "role: theorem\n", "", 1), "m2 check finds"},
		{strings.NewReplacer("kind: statement\nrole: theorem", "kind: definition", "uses:", "defines: G5S1M9\nuses:", "## Statement", "## Definition").Replace(lagrange), "its node is of the kind statement"},
		{strings.Replace(lagrange, "uses: [G5S1M9]\n", "", 1), "Add G5S1M9 to uses"},
		{strings.Replace(lagrange, "## Statement", "## Remarks", 1), "B03: the required section Statement is missing"},
		{strings.Replace(lagrange, "uses: [G5S1M9]\n", "conventions:\n  - {convention: group-law, option: additive}\nuses: [G5S1M9]\n", 1), "Mathlib uses multiplicative"},
		{lagrange, ""},
	}
	var model []*llm.Response
	for _, s := range steps {
		model = append(model, call(nodePath, s.text))
	}
	rec := run(t, nil, "mathlib:Subgroup.card_subgroup_dvd_card", model...)
	if rec.Outcome != agent.Success || len(rec.Turns) != len(steps) {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	for i, s := range steps {
		if got := rec.Turns[i].Why; s.why == "" && got != "" || !strings.Contains(got, s.why) {
			t.Errorf("after turn %d the goal says %q, and it must say %q", i+1, got, s.why)
		}
	}
}

func TestRunDefinition(t *testing.T) {
	const conceptPath = "nodes/~/~2b3c4d5e.md"
	plain := strings.Replace(cyclic, "**cyclic group**", "cyclic group", 1)
	rec := run(t, nil, "mathlib:IsCyclic",
		call(nodePath, strings.NewReplacer("decl: Subgroup.card_subgroup_dvd_card", "decl: IsCyclic", "Lagrange's theorem", "Cyclic groups").Replace(lagrange)),
		call(conceptPath, concept),
		call(nodePath, plain),
		stuck(),
	)
	for i, want := range []string{"its node is of the kind definition", "its node is of the kind definition", "does not name the defined term in bold"} {
		if got := rec.Turns[i].Why; !strings.Contains(got, want) {
			t.Errorf("after turn %d the goal says %q, and it must say %q", i+1, got, want)
		}
	}
	if rec := run(t, nil, "mathlib:IsCyclic", call(conceptPath, concept), call(nodePath, cyclic)); rec.Outcome != agent.Success {
		t.Errorf("a definition and its concept give %s, %q", rec.Outcome, rec.Reason)
	}
	other := strings.Replace(concept, "~2b3c4d5e", "~4d5e6f7a", 1)
	rec = run(t, nil, "mathlib:IsCyclic", call(conceptPath, concept), call("nodes/~/~4d5e6f7a.md", other), call(nodePath, cyclic), stuck())
	if got := rec.Turns[2].Why; !strings.Contains(got, "the run adds ~4d5e6f7a, and informalize adds only") {
		t.Errorf("a concept that the node does not define gives %q", got)
	}
}

func TestRunBound(t *testing.T) {
	const path = "nodes/3C5/3C53TD.md"
	bound := strings.Replace(lagrange, "~1a2b3c4d", "3C53TD", 1) + "\n## Remarks\n\nThe converse is false.\n"
	changed := strings.Replace(bound, "The converse is false.", "The converse is false for the alternating group $A_4$.", 1)
	files := map[string]string{path: bound, prov.Path("3C53TD"): `{"field":"*","op":"add","agent":null,"human":"github:octocat","at":"2026-10-21T10:00:00Z"}` + "\n"}
	rec := run(t, files, "mathlib:Subgroup.card_subgroup_dvd_card", call(nodePath, lagrange), stuck())
	if got := rec.Turns[0].Why; !strings.Contains(got, "2 nodes bind Subgroup.card_subgroup_dvd_card: 3C53TD, ~1a2b3c4d") {
		t.Errorf("a second node of the declaration gives %q", got)
	}
	rec = run(t, files, "mathlib:Subgroup.card_subgroup_dvd_card", call(path, changed), stuck())
	if got := rec.Turns[0].Why; !strings.Contains(got, "github:octocat wrote or confirmed body.remarks, and informalize does not change it") {
		t.Errorf("a change of the text of a person gives %q", got)
	}

	files[prov.Path("3C53TD")] += `{"field":"body.remarks","op":"add","agent":"m2-agent/informalize@0.1.0","human":null,"at":"2026-10-22T10:00:00Z"}` + "\n"
	if rec := run(t, files, "mathlib:Subgroup.card_subgroup_dvd_card", call(path, changed)); rec.Outcome != agent.Success {
		t.Errorf("a change of its own section gives %s, %q", rec.Outcome, rec.Reason)
	}
}
