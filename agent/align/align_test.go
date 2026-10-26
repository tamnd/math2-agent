package align_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/align"
	"github.com/tamnd/math2-agent/agent/curate"
	"github.com/tamnd/math2-agent/agent/tools"
	aligner "github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/workspace"
)

// tree makes a git repository with the files in one commit and opens it as a workspace.
func tree(t *testing.T, files map[string]string) *workspace.Workspace {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
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

const (
	lagrangePath = "nodes/3C/3C53TD.md"
	lagrange     = "---\ntag: 3C53TD\nkind: statement\nrole: theorem\nname: Lagrange's theorem\nmsc: [20D60]\nuses: [G5S1M9]\nformal:\n  - system: lean4\n    library: math2\n    decl: Math2.Algebra.lagrange\n    match: exact\n---\n\n## Statement\n\nLet $G$ be a finite [[group|G5S1M9]] and let $H$ be a subgroup of $G$. Then the order of $H$ divides the order of $G$.\n"
	group        = "---\ntag: G5S1M9\nkind: concept\nname: Group\nmsc: [20A05]\n---\n\n## Summary\n\nA group is a set with an associative operation, an identity and inverses.\n"
	catalogue    = "sources:\n  - id: wikidata\n    name: Wikidata\n    licence: CC0-1.0\n    class: open\n  - id: arxiv\n    name: arXiv\n    licence: CC0-1.0\n    class: record\n  - id: mathlib\n    name: Mathlib\n    licence: Apache-2.0\n    class: open\n"
	paper        = "https://arxiv.org/abs/2401.00001"
	// The records are made up for the tests.
	theorem = "In group theory, the theorem that the order of each subgroup of a finite group divides the order of the group."
	cosets  = "The theorem that the cosets of a subgroup partition the group into sets of the same size."
)

// file returns the records as the text of a records file.
func file(t *testing.T, recs ...ingest.Record) string {
	t.Helper()
	var b strings.Builder
	for _, r := range recs {
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

func base(t *testing.T) map[string]string {
	return map[string]string{
		lagrangePath:         lagrange,
		"nodes/G5/G5S1M9.md": group,
		ingest.CatalogueFile: catalogue,
		"build/records/wikidata.jsonl": file(t,
			ingest.Record{ID: "Q1", Kind: "item", Title: "Lagrange's theorem", URL: "https://www.wikidata.org/wiki/Q1", Text: theorem},
			ingest.Record{ID: "Q2", Kind: "item", Title: "coset theorem", URL: "https://www.wikidata.org/wiki/Q2", Text: cosets},
			ingest.Record{ID: "Q3", Kind: "item", Title: "a rejected item", URL: "https://www.wikidata.org/wiki/Q3", Text: cosets},
		),
		"build/records/arxiv.jsonl": file(t, ingest.Record{ID: "2401.00001", Kind: "paper", Title: "Orders of subgroups", URL: paper}),
		"build/records/mathlib.jsonl": file(t,
			ingest.Record{ID: "Subgroup.card_subgroup_dvd_card", Kind: "theorem", Locator: "Mathlib.Algebra.Group.Subgroup.Finite", Meta: map[string]string{"type_hash": "aaaa"}},
			ingest.Record{ID: "Subgroup.index_mul_card", Kind: "theorem", Locator: "Mathlib.GroupTheory.Index", Meta: map[string]string{"type_hash": "bbbb"}},
		),
		aligner.Register: "3C53TD same_as wikidata Q3 rejected github:octocat 2026-10-01 -\n",
	}
}

// st is the build status of the tree: the Math2 binding of 3C53TD has the type hash aaaa.
var st = []status.Status{{Tag: "3C53TD", Bindings: []status.Binding{{Decl: "Math2.Algebra.lagrange", Match: "exact", Checked: "Math2.Algebra.lagrange", TypeHash: "aaaa"}}}}

// bound is the target with a Mathlib binding appended.
func bound(decl, match string) string {
	return strings.Replace(lagrange, "    match: exact\n---", "    match: exact\n  - system: lean4\n    library: mathlib\n    decl: "+decl+"\n    match: "+match+"\n---", 1)
}

var u = llm.Usage{Input: 1000, Output: 200}

func verdict(v, why string) *llm.Response {
	return llmtest.Call("judge", map[string]string{"verdict": v, "why": why}, u)
}

func job() *agent.Job {
	return &agent.Job{ID: "j-2026-10-26-1", Role: "align", Target: "3C53TD", Goal: "link the node to Wikidata", OnBehalfOf: "github:octocat"}
}

func TestPropose(t *testing.T) {
	ws := tree(t, base(t))
	rs := &curate.Records{Root: ws.Dir}
	p := align.Propose(ws, rs, "m2-agent/align@0.1.0")
	call := func(edge, ref, why string) (agent.Result, error) {
		in, _ := json.Marshal(map[string]string{"edge": edge, "ref": ref, "why": why})
		return p.Call(context.Background(), job(), in)
	}
	res, err := call("same_as", "wikidata:Q1", "the item is the theorem.")
	if err != nil || !strings.Contains(res.Text, "Proposed the edge same_as from 3C53TD to wikidata Q1, in queues/align/wikidata.jsonl") {
		t.Fatalf("propose_edge gives %+v, %v", res, err)
	}
	if _, err := call("same_as", "wikidata:Q1", "the item states the theorem."); err != nil {
		t.Fatal(err)
	}
	if _, err := call("aligned", "arxiv:2401.00001", "the paper states the theorem."); err != nil {
		t.Fatal(err)
	}
	data, err := ws.Read("queues/align/wikidata.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	ps, err := aligner.ParseQueue(data)
	if err != nil || len(ps) != 1 || ps[0].Why != "the item states the theorem." || ps[0].Source != "wikidata" || ps[0].By != "m2-agent/align@0.1.0" || ps[0].Records == "" {
		t.Errorf("the queue is %+v, %v", ps, err)
	}
	data, _ = ws.Read("queues/align/arxiv.jsonl")
	if ps, err := aligner.ParseQueue(data); err != nil || len(ps) != 1 || ps[0].Target != paper || ps[0].Edge != aligner.Aligned {
		t.Errorf("the queue of arxiv is %+v, %v", ps, err)
	}
	for _, c := range [][4]string{
		{"same_as", "wikidata:Q1", " ", "give the reason"},
		{"duplicate", "wikidata:Q2", "x", "is not one of"},
		{"aligned", "wikidata:Q2", "x", "only a paper"},
		{"same_as", "arxiv:2401.00001", "x", "is a paper"},
		{"same_as", "mathlib:Subgroup.card_subgroup_dvd_card", "x", "through its formal binding"},
		{"close_to", "wikidata:Q3", "x", "github:octocat rejected the edge from 3C53TD to wikidata Q3 on 2026-10-01"},
		{"same_as", "wikidata:Q9", "x", "no record Q9"},
	} {
		if _, err := call(c[0], c[1], c[2]); err == nil || !strings.Contains(err.Error(), c[3]) {
			t.Errorf("propose_edge %v gives %v, want %q", c[:3], err, c[3])
		}
	}
}

func TestSameType(t *testing.T) {
	ws := tree(t, base(t))
	rs := &curate.Records{Root: ws.Dir}
	for _, c := range []struct {
		decl, match string
		st          []status.Status
		want        string
	}{
		{"Subgroup.card_subgroup_dvd_card", "exact", st, ""},
		{"Subgroup.card_subgroup_dvd_card", "weaker", st, "thus its match is exact"},
		{"Subgroup.index_mul_card", "exact", st, "the type hash of Subgroup.index_mul_card is bbbb, and no Math2 binding of 3C53TD has it"},
		{"Subgroup.card_subgroup_dvd_card", "exact", nil, "Run m2 build"},
		{"Nat.nothing", "exact", st, "have no declaration Nat.nothing"},
	} {
		why, err := align.SameType(rs, c.st, "3C53TD", map[string]any{"decl": c.decl, "match": c.match})
		if err != nil || c.want == "" && why != "" || !strings.Contains(why, c.want) {
			t.Errorf("SameType(%s, %s) gives %q, %v, want %q", c.decl, c.match, why, err, c.want)
		}
	}
}

func TestAutoMerge(t *testing.T) {
	ws := tree(t, base(t))
	rs := &curate.Records{Root: ws.Dir}
	change := func(path, old, cur string) []workspace.Change {
		return []workspace.Change{{Path: path, Old: []byte(old), New: []byte(cur)}}
	}
	for _, c := range []struct {
		name    string
		changes []workspace.Change
		want    bool
	}{
		{"an identical type hash", change(lagrangePath, lagrange, bound("Subgroup.card_subgroup_dvd_card", "exact")), true},
		{"another type hash", change(lagrangePath, lagrange, bound("Subgroup.index_mul_card", "exact")), false},
		{"a proposal", change("queues/align/wikidata.jsonl", "", `{"id":"x"}`+"\n"), false},
		{"no change", nil, false},
	} {
		if got, err := align.AutoMerge(rs, st, job(), c.changes); err != nil || got != c.want {
			t.Errorf("%s: AutoMerge gives %v, %v", c.name, got, err)
		}
	}
}

func TestBrief(t *testing.T) {
	ws := tree(t, base(t))
	brief, err := align.Brief(ws, job())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Job j-2026-10-26-1: align the target 3C53TD, a statement node. The goal of the job: link the node to Wikidata.", "propose_edge", "type hash", `<data source="nodes/3C/3C53TD.md"`} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief has no %q:\n%s", want, brief)
		}
	}
	if _, err := align.Brief(ws, &agent.Job{Target: "7K3QX2"}); err == nil {
		t.Error("the brief of a node that is not there gives no error")
	}
}

func write(path, text string) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
}

func propose(edge, ref, why string) *llm.Response {
	return llmtest.Call("propose_edge", map[string]string{"edge": edge, "ref": ref, "why": why}, u)
}

// run runs an align job on a new tree with the scripted model and judge.
func run(t *testing.T, model, judge []*llm.Response) *agent.Record {
	t.Helper()
	ws := tree(t, base(t))
	role, err := agent.LoadRole(prompts.FS, "align")
	if err != nil {
		t.Fatal(err)
	}
	j := job()
	rs := &curate.Records{Root: ws.Dir}
	brief, err := align.Brief(ws, j)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil, curate.Read(rs), align.Propose(ws, rs, "m2-agent/align@"+role.Version))
	if err != nil {
		t.Fatal(err)
	}
	meter := &agent.Meter{Model: llmtest.New(judge...)}
	rn := agent.Runner{Role: role, Model: llmtest.New(model...), Tools: ts, Meter: meter, Goal: align.Goal(ws, j, rs, st, meter, role.Ladder[0].Model, role.Checks["judge"])}
	rec, err := rn.Run(context.Background(), j, 0, brief)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestRunProposals(t *testing.T) {
	rec := run(t, []*llm.Response{
		llmtest.Call("record", map[string]string{"ref": "wikidata:Q2"}, u),
		propose("same_as", "wikidata:Q2", "the item is the theorem."),
		propose("close_to", "wikidata:Q2", "the item is about the cosets, and the node about the orders."),
	}, []*llm.Response{verdict(curate.Related, "the item states the partition into cosets."), verdict(curate.Related, "the item states the partition into cosets.")})
	if rec.Outcome != agent.Success || len(rec.Turns) != 3 {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	if r := rec.Turns[0].Results[0]; r.IsError || !strings.Contains(r.Content, cosets) {
		t.Errorf("the tool record gives %+v", r)
	}
	for i, want := range []string{"has not proposed an edge", "is related to the record wikidata:Q2, not the same", ""} {
		if got := rec.Turns[i].Why; want == "" && got != "" || !strings.Contains(got, want) {
			t.Errorf("after turn %d the goal says %q, and it must say %q", i+1, got, want)
		}
	}
}

func TestRunMathlib(t *testing.T) {
	rec := run(t, []*llm.Response{
		write(lagrangePath, strings.Replace(lagrange, "msc: [20D60]", "msc: [20D60]\nsame_as: {wikidata: Q1}", 1)),
		write(lagrangePath, bound("Subgroup.index_mul_card", "exact")),
		write(lagrangePath, bound("Subgroup.card_subgroup_dvd_card", "exact")),
	}, nil)
	if rec.Outcome != agent.Success || len(rec.Turns) != 3 {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	if r := rec.Turns[0].Results[0]; !r.IsError || !strings.Contains(r.Content, "does not change the field same_as") {
		t.Errorf("a same_as in the node gives %+v", r)
	}
	if got := rec.Turns[1].Why; !strings.Contains(got, "Align binds Mathlib only when the type hash is identical") {
		t.Errorf("another type hash gives %q", got)
	}
}

func TestRunDifferent(t *testing.T) {
	rec := run(t, []*llm.Response{
		propose("aligned", "arxiv:2401.00001", "the paper states the theorem."),
		propose("none", "arxiv:2401.00001", ""),
		llmtest.Call("report_stuck", map[string]string{"reason": "No record is the node."}, u),
	}, []*llm.Response{verdict(curate.Different, "the paper is about rings.")})
	if rec.Outcome != agent.Failure || !strings.Contains(rec.Reason, "No record is the node") {
		t.Fatalf("the run gives %s, %q", rec.Outcome, rec.Reason)
	}
	if got := rec.Turns[0].Why; !strings.Contains(got, "are about different things: the paper is about rings. Remove the proposal") {
		t.Errorf("a different paper gives %q", got)
	}
	if r := rec.Turns[1].Results[0]; r.IsError || !strings.Contains(r.Content, "Removed your proposal from 3C53TD to arxiv "+paper) {
		t.Errorf("the edge none gives %+v", r)
	}
	if got := rec.Turns[1].Why; !strings.Contains(got, "has not proposed") {
		t.Errorf("after the edge none the goal says %q", got)
	}
}
