package understand_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/agent/understand"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/workspace"
)

// objects are the objects of version 1 of 2401.00001: a theorem, its proof, a remark, an object that the extractor could not place, and a definition.
const objects = `{"tag":"THM1","kind":"statement","subkind":"theorem","paper":"2401.00001","version":"1","local":"thm-1","number":"1.1","env":"theorem","body_md":"Every compact metric space is complete.","path":"source","item":"statement/theorem","scope":"paper"}
{"tag":"PRF1","kind":"proof","paper":"2401.00001","version":"1","local":"proof-1","path":"source","item":"proof","scope":"paper","proves":"thm-1"}
{"tag":"RMK1","kind":"remark","paper":"2401.00001","version":"1","local":"remark-1","env":"remark","body_md":"The converse holds by [Ser, Theorem 4.1].","path":"source","item":"remark","scope":"paper"}
{"tag":"UNK1","kind":"unknown","paper":"2401.00001","version":"1","local":"unknown-1","env":"satz","number":"2","body_md":"Let $X$ be a space. Then $X$ is compact.","path":"source","item":"unknown","scope":"paper"}
{"tag":"DEF1","kind":"definition","paper":"2401.00001","version":"1","local":"def-1","env":"definition","body_md":"A space is \\emph{tame} if it is compact.","path":"source","item":"definition","scope":"paper"}
{"kind":"remark","paper":"2401.00001","version":"1","local":"remark-2","path":"source","item":"remark","scope":"paper"}
`

// checkout makes a checkout of tamnd/arxiv with one paper of the licence.
func checkout(t *testing.T, licence string) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range map[string]string{
		"analysis/objects/2401/2401.00001.jsonl": objects,
		"metadata/2401.jsonl":                    `{"id":"2401.00001","title":"Compact  spaces","msc_class":"54D30 (Primary)","versions":[{"version":1,"licence":"` + licence + `"}]}` + "\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// tree makes a git repository with one file in one commit and opens it as a workspace.
func tree(t *testing.T) *workspace.Workspace {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("a tree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "base"}} {
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

const by = "agent:m2-agent/understand@0.1.0"

func load(t *testing.T, licence, target string) *understand.Paper {
	t.Helper()
	tg, err := understand.ParseTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	p, err := understand.Load(checkout(t, licence), tg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseTarget(t *testing.T) {
	for s, want := range map[string]understand.Target{
		"arxiv:2401.00001/classify":   {Paper: "2401.00001", Pass: "classify"},
		"arxiv:math/9802134/concepts": {Paper: "math/9802134", Pass: "concepts"},
	} {
		if got, err := understand.ParseTarget(s); err != nil || got != want {
			t.Errorf("ParseTarget(%q) is %+v, %v", s, got, err)
		}
	}
	for _, s := range []string{"2401.00001/classify", "arxiv:2401.00001", "arxiv:2401.00001/explain", "arxiv:x y/classify"} {
		if _, err := understand.ParseTarget(s); err == nil {
			t.Errorf("ParseTarget(%q) gives no error", s)
		}
	}
}

func TestLoad(t *testing.T) {
	p := load(t, "cc-by", "arxiv:2401.00001/classify")
	if p.Version != 1 || p.Title != "Compact spaces" || p.Class != "open" || len(p.MSC) != 1 || p.MSC[0] != "54D30" {
		t.Errorf("the paper is %+v", p)
	}
	var ids []string
	for _, it := range p.Items {
		ids = append(ids, it.ID)
	}
	if got := strings.Join(ids, " "); got != "ax://paper/2401.00001#RMK1 ax://paper/2401.00001#UNK1" {
		t.Errorf("classify reads %s", got)
	}
	p = load(t, "", "arxiv:2401.00001/concepts")
	ids = nil
	for _, it := range p.Items {
		ids = append(ids, strings.TrimPrefix(it.ID, "ax://paper/2401.00001#"))
	}
	if got := strings.Join(ids, " "); got != "THM1 DEF1" || p.Class != "record" {
		t.Errorf("concepts reads %s of the class %s", got, p.Class)
	}
}

func TestBrief(t *testing.T) {
	p := load(t, "", "arxiv:2401.00001/classify")
	b := understand.Brief(&agent.Job{ID: "j-1", Goal: "classify the items."}, p)
	for _, want := range []string{"the pass classify on the arXiv paper 2401.00001v1", "The goal of the job: classify the items.", "each of the 2 items", "statement/lemma", "Item ax://paper/2401.00001#UNK1", "environment: satz", "Then $X$ is compact.", "54D30", "no licence"} {
		if !strings.Contains(b, want) {
			t.Errorf("the brief has no %q:\n%s", want, b)
		}
	}
}

func call(t *testing.T, ws *workspace.Workspace, p *understand.Paper, name string, items ...map[string]any) (agent.Result, error) {
	t.Helper()
	in, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range understand.Tools(ws, p, "claude-haiku-5-5", by) {
		if tool.Spec().Name == name {
			return tool.Call(context.Background(), &agent.Job{ID: "j-1", Role: "understand", Target: "arxiv:" + p.ID + "/" + p.Pass}, in)
		}
	}
	t.Fatalf("no tool %s", name)
	return agent.Result{}, nil
}

func records(t *testing.T, ws *workspace.Workspace, pass, id string) []understand.Record {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(ws.Dir, filepath.FromSlash(understand.Path(pass, id))))
	if err != nil {
		t.Fatal(err)
	}
	rs, err := understand.ParseRecords(data)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestClassify(t *testing.T) {
	ws := tree(t)
	p := load(t, "cc-by", "arxiv:2401.00001/classify")
	res, err := call(t, ws, p, "classify_items",
		map[string]any{"id": "ax://paper/2401.00001#UNK1", "kind": "statement/theorem", "confidence": 0.92, "flags": []string{"claim-in-remark"}},
		map[string]any{"id": "ax://paper/2401.00001#RMK1", "kind": "remark", "confidence": 0.6, "flags": []string{"claim-in-remark"}},
		map[string]any{"id": "ax://paper/2401.00001#THM1", "kind": "remark", "confidence": 1},
		map[string]any{"id": "ax://paper/2401.00001#UNK1", "kind": "remark", "confidence": 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Stored the results of 2 of 4 items.", "left out the flag \"claim-in-remark\"", "item 3: \"ax://paper/2401.00001#THM1\" is not an item", "item 4: ax://paper/2401.00001#UNK1 is in the call twice"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("the result has no %q: %s", want, res.Text)
		}
	}
	for _, r := range append(records(t, ws, "classify", "ax://paper/2401.00001#UNK1"), records(t, ws, "classify", "ax://paper/2401.00001#RMK1")...) {
		switch r.ID {
		case "ax://paper/2401.00001#UNK1":
			if r.Kind != "statement/theorem" || r.Was != "unknown" || r.Queue != "" || len(r.Flags) != 0 || r.Ref != "2401.00001v1#unknown-1" || r.By != by {
				t.Errorf("the record of UNK1 is %+v", r)
			}
		case "ax://paper/2401.00001#RMK1":
			if r.Queue != understand.ClassifyReview || len(r.Flags) != 1 || r.Class != "open" {
				t.Errorf("the record of RMK1 is %+v", r)
			}
		}
	}
	if _, err := call(t, ws, p, "tag_concepts", map[string]any{"id": "x"}); err == nil || !strings.Contains(err.Error(), "Use classify_items") {
		t.Errorf("the tool of the other pass gives %v", err)
	}
	if _, err := call(t, ws, p, "classify_items", map[string]any{"id": "ax://paper/2401.00001#UNK1", "kind": "theorem", "confidence": 0.9}); err == nil || !strings.Contains(err.Error(), "stored no result") {
		t.Errorf("a call with no valid result gives %v", err)
	}
}

func TestConcepts(t *testing.T) {
	ws := tree(t)
	p := load(t, "", "arxiv:2401.00001/concepts")
	res, err := call(t, ws, p, "tag_concepts",
		map[string]any{"id": "ax://paper/2401.00001#DEF1", "msc": []string{"54D30", "54d30", "54E45"}, "concepts": []map[string]any{
			{"name": "compact  space", "role": "uses", "wikidata": "Q381892"},
			{"name": "tame space", "role": "defines"},
			{"name": "x", "role": "names"},
			{"name": "metric space", "role": "uses", "wikidata": "Q180953 "},
			{"name": "y", "role": "uses", "wikidata": "wd:1"},
		}, "terms": []map[string]any{{"term": "tame"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Stored the results of 1 of 1 items.", "left out the code \"54d30\"", "whose role \"names\"", "\"wd:1\" of \"y\", which is not a QID", "no words of a paper of the class record", "have no result yet: ax://paper/2401.00001#THM1"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("the result has no %q: %s", want, res.Text)
		}
	}
	rs := records(t, ws, "concepts", "ax://paper/2401.00001#DEF1")
	if len(rs) != 1 {
		t.Fatalf("the records are %+v", rs)
	}
	r := rs[0]
	a, _ := json.Marshal(r.Concepts)
	if string(a) != `[{"wikidata":"Q381892","name":"compact space","role":"uses"},{"name":"tame space","role":"defines","flags":["new-concept"]},{"wikidata":"Q180953","name":"metric space","role":"uses"},{"name":"y","role":"uses","flags":["new-concept"]}]` ||
		strings.Join(r.MSC, " ") != "54D30 54E45" || len(r.Terms) != 0 || r.Class != "record" {
		t.Errorf("the record is %+v with the concepts %s", r, a)
	}
}

func TestRun(t *testing.T) {
	ws := tree(t)
	role, err := agent.LoadRole(prompts.FS, "understand")
	if err != nil {
		t.Fatal(err)
	}
	j := &agent.Job{ID: "j-2026-10-27-1", Role: "understand", Target: "arxiv:2401.00001/classify", Goal: "classify the items", OnBehalfOf: "github:octocat"}
	p := load(t, "", j.Target)
	b := "agent:m2-agent/understand@" + role.Version
	ts, err := tools.Profile(role, nil, ws, nil, understand.Tools(ws, p, role.Ladder[0].Model, b)...)
	if err != nil {
		t.Fatal(err)
	}
	u := llm.Usage{Input: 1000, Output: 200}
	rn := agent.Runner{Role: role, Model: llmtest.New(
		llmtest.Call("classify_items", map[string]any{"items": []any{map[string]any{"id": "ax://paper/2401.00001#UNK1", "kind": "statement/theorem", "confidence": 0.9}}}, u),
		llmtest.Call("classify_items", map[string]any{"items": []any{map[string]any{"id": "ax://paper/2401.00001#RMK1", "kind": "remark", "confidence": 0.8, "flags": []string{"claim-in-remark"}}}}, u),
	), Tools: ts, Goal: understand.Goal(ws, j, p, b)}
	rec, err := rn.Run(context.Background(), j, 0, understand.Brief(j, p))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Outcome != agent.Success || len(rec.Turns) != 2 {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	if got := rec.Turns[0].Why; !strings.Contains(got, "1 of the 2 items have no result yet: ax://paper/2401.00001#RMK1") {
		t.Errorf("after the first turn the goal says %q", got)
	}
}
