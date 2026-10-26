package curate_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/curate"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prompts"
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
	lagrangePath = "nodes/3C5/3C53TD.md"
	groupPath    = "nodes/G5S/G5S1M9.md"
	newPath      = "nodes/~/~1a2b3c4d.md"
	ref          = "arxiv_results:2312.00752#3KH2"
	locator      = "ax://paper/2312.00752#3KH2"
	lagrange     = "---\ntag: 3C53TD\nkind: statement\nrole: theorem\nname: Lagrange's theorem\nmsc: [20D60]\nuses: [G5S1M9]\n---\n\n## Statement\n\nLet $G$ be a finite [[group|G5S1M9]] and let $H$ be a subgroup of $G$. Then the order of $H$ divides the order of $G$.\n"
	group        = "---\ntag: G5S1M9\nkind: concept\nname: Group\nmsc: [20A05]\n---\n\n## Summary\n\nA group is a set with an associative operation, an identity and inverses.\n"
	catalogue    = "sources:\n  - id: arxiv_results\n    name: The results of arXiv papers\n    licence: CC-BY-4.0\n    class: open\n  - id: closed\n    name: A closed source\n    licence: LicenseRef-closed\n    class: record\n"
	// recordText is the text of the record in the fixture. It is made up for the test.
	recordText = "Theorem 3. Let K be a finite group in which each element other than the identity has order two. Then K is abelian and the number of elements of K is a power of two."
)

// records returns the records file of arxiv_results.
func records(t *testing.T) string {
	t.Helper()
	recs := []ingest.Record{
		{ID: "2312.00752#3KH2", Kind: "statement", Title: "Theorem 3", URL: "https://arxiv.org/abs/2312.00752v1", Locator: locator, Class: ingest.Open, Text: recordText, Fingerprints: ingest.Fingerprints(recordText), Meta: map[string]string{"paper": "2312.00752", "version": "v1", "role": "theorem", "number": "3"}},
		{ID: "2312.00752#9QQ1", Kind: "proof", Locator: "ax://paper/2312.00752#9QQ1", Class: ingest.Open, Text: "Proof. Easy."},
		{ID: "2312.00752#0NL0", Kind: "statement", Class: ingest.Open, Text: "A result with no locator."},
	}
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
		lagrangePath:                        lagrange,
		groupPath:                           group,
		ingest.CatalogueFile:                catalogue,
		"build/records/arxiv_results.jsonl": records(t),
	}
}

// statement is a new node that states the result of the record.
func statement(body string) string {
	return "---\ntag: ~1a2b3c4d\nkind: statement\nrole: theorem\nname: Groups of exponent two\nmsc: [20D60]\nuses: [G5S1M9]\nsources:\n  - arxiv: 2312.00752v1\n    locator: " + locator + "\n---\n\n## Statement\n\n" + body + "\n"
}

const good = "Let $G$ be a finite [[group|G5S1M9]] such that $g^2 = e$ for each $g$ in $G$. Then $G$ is commutative, and $|G| = 2^n$ for some $n \\ge 0$."

func TestRecords(t *testing.T) {
	ws := tree(t, base(t))
	rs := &curate.Records{Root: ws.Dir}
	r, err := rs.Get(ref)
	if err != nil {
		t.Fatal(err)
	}
	if r.Ref != ref || r.Locator != locator || r.Licence() != "CC-BY-4.0" || r.Source.ID != "arxiv_results" {
		t.Errorf("Get gives %+v", r)
	}
	text := r.Text()
	for _, want := range []string{`<data source="arxiv_results:2312.00752#3KH2" licence="CC-BY-4.0">`, "kind: statement\n", "locator: " + locator, "paper: 2312.00752\n", recordText} {
		if !strings.Contains(text, want) {
			t.Errorf("the text has no %q:\n%s", want, text)
		}
	}
	for ref, want := range map[string]string{
		"arxiv_results":                 "not a record reference",
		"nosource:1":                    "the catalogue has no source nosource",
		"closed:1":                      "the tree has no records of closed. Run m2 ingest first",
		"arxiv_results:2312.00752#ZZZZ": "have no record 2312.00752#ZZZZ",
	} {
		if _, err := rs.Get(ref); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Get(%q) gives %v, want %q", ref, err, want)
		}
	}
	if !curate.IsRecord(ref) || curate.IsRecord("3C53TD") || curate.IsRecord(":x") {
		t.Error("IsRecord is wrong")
	}
}

func TestCopiesAndLocates(t *testing.T) {
	ws := tree(t, base(t))
	r, err := (&curate.Records{Root: ws.Dir}).Get(ref)
	if err != nil {
		t.Fatal(err)
	}
	if !curate.Copies(recordText, r) || curate.Copies(good, r) {
		t.Error("Copies is wrong")
	}
	n, err := node.Parse(newPath, []byte(statement(good)))
	if err != nil {
		t.Fatal(err)
	}
	if !curate.Locates(n, locator) || curate.Locates(n, "ax://paper/2312.00752#9QQ1") {
		t.Error("Locates is wrong")
	}
}

var u = llm.Usage{Input: 1000, Output: 200}

func verdict(v, why string) *llm.Response {
	return llmtest.Call("judge", map[string]string{"verdict": v, "why": why}, u)
}

func TestJudge(t *testing.T) {
	ws := tree(t, base(t))
	r, err := (&curate.Records{Root: ws.Dir}).Get(ref)
	if err != nil {
		t.Fatal(err)
	}
	m := llmtest.New(verdict(curate.Specializes, "the node adds a hypothesis."), llmtest.Say("No.", u), llmtest.Call("judge", map[string]string{"verdict": "maybe"}, u))
	v, err := curate.Judge(context.Background(), m, "claude-haiku-5-5", "Judge.", r, newPath, []byte(statement(good)))
	if err != nil || v.Verdict != curate.Specializes || v.Why != "the node adds a hypothesis." {
		t.Errorf("Judge gives %+v, %v", v, err)
	}
	req := m.Requests()[0]
	text := req.Messages[0].Content[0].Text
	if req.Model != "claude-haiku-5-5" || req.System != "Judge." || len(req.Tools) != 1 || !strings.Contains(text, `<data source="nodes/~/~1a2b3c4d.md"`) || !strings.Contains(text, recordText) {
		t.Errorf("the request is %+v\n%s", req, text)
	}
	for range 2 {
		if v, err := curate.Judge(context.Background(), m, "claude-haiku-5-5", "Judge.", r, newPath, []byte(statement(good))); err != nil || v.Verdict != curate.Different {
			t.Errorf("an answer with no verdict gives %+v, %v", v, err)
		}
	}
}

func TestBrief(t *testing.T) {
	ws := tree(t, base(t))
	rs := &curate.Records{Root: ws.Dir}
	brief, err := curate.Brief(ws, &agent.Job{ID: "j-1", Role: "curate", Target: ref, Goal: "a node of the theorem."}, rs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"into a new statement node", "The goal of the job: a node of the theorem.\n", "report_stuck", "{arxiv: 2312.00752v1, locator: " + locator + "}"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief has no %q:\n%s", want, brief)
		}
	}
	if strings.Contains(brief, recordText) {
		t.Error("the brief has the text of the record, which is untrusted")
	}
	brief, err = curate.Brief(ws, &agent.Job{ID: "j-2", Role: "curate", Target: "3C53TD", Goal: "fix the uses"}, rs)
	if err != nil || !strings.Contains(brief, "a statement node") || !strings.Contains(brief, `<data source="nodes/3C5/3C53TD.md"`) {
		t.Errorf("the brief of a node is %q, %v", brief, err)
	}
	for target, want := range map[string]string{
		"arxiv_results:2312.00752#9QQ1": "is a proof",
		"arxiv_results:2312.00752#0NL0": "has no locator",
		"7K3QX2":                        "no such file",
		"lean/X":                        "is not a tag",
	} {
		if _, err := curate.Brief(ws, &agent.Job{ID: "j-3", Role: "curate", Target: target}, rs); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("the brief of %s gives %v, want %q", target, err, want)
		}
	}
}

func TestPropose(t *testing.T) {
	ws := tree(t, base(t))
	p := curate.Propose(ws, "m2-agent/curate@0.1.0")
	job := &agent.Job{ID: "j-1", Role: "curate", Target: "3C53TD"}
	call := func(node, other, why string) (agent.Result, error) {
		in, _ := json.Marshal(map[string]string{"node": node, "other": other, "why": why})
		return p.Call(context.Background(), job, in)
	}
	res, err := call("G5S1M9", "3C53TD", "the map.")
	if err != nil || !strings.Contains(res.Text, "Proposed that 3C53TD and G5S1M9 are the same node") {
		t.Fatalf("propose_same gives %+v, %v", res, err)
	}
	data, err := ws.Read(align.DedupQueue)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := align.ParseQueue(data)
	if err != nil || len(ps) != 1 || ps[0].Node != "3C53TD" || ps[0].By != "m2-agent/curate@0.1.0" || ps[0].Why != "the map." || ps[0].Proposed == "" {
		t.Errorf("the queue is %+v, %v", ps, err)
	}
	if res, err := call("3C53TD", "G5S1M9", "again"); err != nil || !strings.Contains(res.Text, "has the proposal") {
		t.Errorf("a second proposal gives %+v, %v", res, err)
	}
	for _, c := range [][3]string{{"3C53TD", "7K3QX2", "x"}, {"3C53TD", "3C53TD", "x"}, {"3C53TD", "G5S1M9", " "}} {
		if _, err := call(c[0], c[1], c[2]); err == nil {
			t.Errorf("propose_same %v gives no error", c)
		}
	}
}

func write(path, text string) *llm.Response {
	return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
}

// run runs a curate job on a new tree with the scripted model and judge.
func run(t *testing.T, target string, model, judge []*llm.Response) *agent.Record {
	t.Helper()
	ws := tree(t, base(t))
	role, err := agent.LoadRole(prompts.FS, "curate")
	if err != nil {
		t.Fatal(err)
	}
	j := &agent.Job{ID: "j-2026-10-26-4", Role: "curate", Target: target, Goal: "curate the target", OnBehalfOf: "github:octocat"}
	rs := &curate.Records{Root: ws.Dir}
	brief, err := curate.Brief(ws, j, rs)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil, curate.Read(rs), curate.Propose(ws, "m2-agent/curate@"+role.Version))
	if err != nil {
		t.Fatal(err)
	}
	meter := &agent.Meter{Model: llmtest.New(judge...)}
	rn := agent.Runner{Role: role, Model: llmtest.New(model...), Tools: ts, Meter: meter, Goal: curate.Goal(ws, j, rs, meter, role.Ladder[0].Model, role.Checks["judge"])}
	rec, err := rn.Run(context.Background(), j, 0, brief)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestRunRecord(t *testing.T) {
	rec := run(t, ref, []*llm.Response{
		llmtest.Call("record", map[string]string{"ref": ref}, u),
		write(newPath, strings.Replace(statement(good), "## Statement", "## Proof", 1)),
		write(newPath, strings.NewReplacer("kind: statement\nrole: theorem", "kind: concept", "## Statement", "## Summary").Replace(statement(good))),
		write(newPath, statement(recordText)),
		write(newPath, statement("Let $G$ be a finite abelian [[group|G5S1M9]] such that $g^2 = e$ for each $g$ in $G$. Then $|G| = 2^n$ for some $n \\ge 0$.")),
		write(newPath, statement(good)),
	}, []*llm.Response{verdict(curate.Specializes, "the node adds the hypothesis that G is abelian."), verdict(curate.Same, "the map is K to G.")})
	if rec.Outcome != agent.Success || len(rec.Turns) != 6 {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	if r := rec.Turns[0].Results[0]; r.IsError || !strings.Contains(r.Content, recordText) {
		t.Errorf("the tool record gives %+v", r)
	}
	for i, want := range []string{"has not changed the tree", "m2 check finds", "thus the new node ~1a2b3c4d is a statement node", "copies the text of the record", "is specializes to the record, not the same: the node adds the hypothesis that G is abelian", ""} {
		if got := rec.Turns[i].Why; want == "" && got != "" || !strings.Contains(got, want) {
			t.Errorf("after turn %d the goal says %q, and it must say %q", i+1, got, want)
		}
	}
	if rec.Spend.Tokens["claude-sonnet-5-5"].Input != 8000 {
		t.Errorf("the tokens are %v", rec.Spend.Tokens)
	}
}

func TestRunNode(t *testing.T) {
	rec := run(t, "3C53TD", []*llm.Response{
		write(lagrangePath, strings.Replace(lagrange, "Then the order", "Then the size", 1)),
		write(lagrangePath, strings.Replace(lagrange, "msc: [20D60]\n", "msc: [20D60]\nformal: []\n", 1)),
		llmtest.Call("propose_same", map[string]string{"node": "3C53TD", "other": "G5S1M9", "why": "a test."}, u),
	}, nil)
	if rec.Outcome != agent.Success || len(rec.Turns) != 3 {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	for i, want := range []string{"does not change the body", "changes only the fields"} {
		if r := rec.Turns[i].Results[0]; !r.IsError || !strings.Contains(r.Content, want) {
			t.Errorf("turn %d gives %+v, want %q", i+1, r, want)
		}
	}

	rec = run(t, "3C53TD", []*llm.Response{
		write(lagrangePath, strings.Replace(lagrange, "name: Lagrange's theorem\n", "name: Lagrange's theorem\naliases: [lagrange]\n", 1)),
	}, nil)
	if rec.Outcome != agent.Success {
		t.Errorf("a change of the aliases gives %s, %q", rec.Outcome, rec.Reason)
	}
}
