package understand_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/agent/understand"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prompts"
	uschema "github.com/tamnd/math2-agent/schema/understand"
	"github.com/tamnd/math2-agent/workspace"
)

// objects are the objects of version 1 of 2401.00001: a theorem, its proof, a remark, an object that the extractor could not place, and a definition.
const objects = `{"tag":"THM1","kind":"statement","subkind":"theorem","paper":"2401.00001","version":"1","local":"thm-1","number":"1.1","env":"theorem","body_md":"Every compact metric space is complete.","path":"source","item":"statement/theorem","scope":"paper"}
{"tag":"PRF1","kind":"proof","paper":"2401.00001","version":"1","local":"proof-1","path":"source","item":"proof","scope":"paper","proves":"thm-1"}
{"tag":"RMK1","kind":"remark","paper":"2401.00001","version":"1","local":"remark-1","env":"remark","body_md":"The converse holds by [Ser, Theorem 4.1].","path":"source","item":"remark","scope":"paper"}
{"tag":"UNK1","kind":"unknown","paper":"2401.00001","version":"1","local":"unknown-1","env":"point","number":"2","body_md":"Let $X$ be a space. Then $X$ is compact.","path":"source","item":"unknown","scope":"paper"}
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
	b := understand.Brief(&agent.Job{ID: "j-1", Goal: "classify the items."}, p, "The prompt of the pass.")
	for _, want := range []string{"the pass classify on the arXiv paper 2401.00001v1", "classify the items.\n\nThe prompt of the pass.\n\nGive the item kind", "The goal of the job: classify the items.", "each of the 2 items", "statement/lemma", "Item ax://paper/2401.00001#UNK1", "environment: point", "Then $X$ is compact.", "54D30", "no licence"} {
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

// TestModelFailed checks that the second result of an item that is not valid in a run gives the item the mark model-failed, and that a valid result is not replaced by the mark (AX7.1).
func TestModelFailed(t *testing.T) {
	ws := tree(t)
	p := load(t, "cc-by", "arxiv:2401.00001/classify")
	ts := understand.Tools(ws, p, "claude-haiku-5-5", by)
	job := &agent.Job{ID: "j-1", Role: "understand", Target: "arxiv:2401.00001/classify"}
	give := func(items ...map[string]any) (agent.Result, error) {
		in, _ := json.Marshal(map[string]any{"items": items})
		return ts[0].Call(context.Background(), job, in)
	}
	rmk := map[string]any{"id": "ax://paper/2401.00001#RMK1", "kind": "remark", "confidence": 0.9}
	if _, err := give(rmk, map[string]any{"id": "ax://paper/2401.00001#UNK1", "kind": "theorem", "confidence": 0.9}); err != nil {
		t.Fatal(err)
	}
	if rs := records(t, ws, "classify", "ax://paper/2401.00001#RMK1"); len(rs) != 1 || rs[0].Failed != "" {
		t.Fatalf("after one result that is not valid the records are %+v", rs)
	}
	res, err := give(map[string]any{"id": "ax://paper/2401.00001#UNK1", "kind": "statement/theorem"}, map[string]any{"id": "ax://paper/2401.00001#RMK1", "kind": "lemma", "confidence": 0.9}, map[string]any{"id": "ax://paper/2401.00001#RMK1", "kind": "lemma", "confidence": 0.9})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "stored as model-failed: classify") {
		t.Errorf("the result is %s", res.Text)
	}
	for _, id := range []string{"ax://paper/2401.00001#UNK1", "ax://paper/2401.00001#RMK1"} {
		for _, r := range records(t, ws, "classify", id) {
			switch {
			case r.ID == "ax://paper/2401.00001#UNK1" && (r.Failed != "model-failed: classify" || r.Kind != ""):
				t.Errorf("the record of UNK1 is %+v", r)
			case r.ID == "ax://paper/2401.00001#RMK1" && (r.Failed != "" || r.Kind != "remark"):
				t.Errorf("the valid record of RMK1 is %+v", r)
			}
		}
	}
	ok, why, err := understand.Goal(ws, job, p, by)(context.Background())
	if err != nil || !ok {
		t.Errorf("the goal gives %v, %q, %v", ok, why, err)
	}
}

// TestSchemas checks that the tool of each pass gives the schema of schema/understand, and that the kinds of the schema of classify are Kinds.
func TestSchemas(t *testing.T) {
	for _, pass := range understand.Passes {
		p := &understand.Paper{ID: "2401.00001", Pass: pass}
		tool := understand.Tool(tree(t), p, "m", by)
		if string(tool.Spec().Schema) != string(uschema.Of(pass)) {
			t.Errorf("the tool of %s has another schema than %s", pass, uschema.File(pass))
		}
	}
	var s struct {
		Properties struct {
			Items struct {
				Items struct {
					Properties struct{ Kind struct{ Enum []string } }
				}
			}
		}
	}
	if err := json.Unmarshal(uschema.Of(understand.Classify), &s); err != nil {
		t.Fatal(err)
	}
	if got := s.Properties.Items.Items.Properties.Kind.Enum; !slices.Equal(got, understand.Kinds) {
		t.Errorf("the kinds of the schema are %v", got)
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
	rec, err := rn.Run(context.Background(), j, 0, understand.Brief(j, p, role.Passes[p.Pass]))
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

// text is the running text of 2401.00001v1, as ax prose prints it: two sections with synthetic sentences.
const text = `{"paper":"2401.00001","version":1,"text":true,"sections":[
{"section":"sec-1","sentences":[
 {"n":1,"file":"main.tex","line":5,"text":"We say that a space is \\emph{tame} if it is compact.","cues":[{"kind":"definition","cue":"we say that"}]},
 {"n":2,"file":"main.tex","line":6,"text":"This holds for each finite space."},
 {"n":3,"file":"main.tex","line":7,"text":"It is not known whether each tame space is metrizable.","cues":[{"kind":"problem","cue":"it is not known whether"}]}]},
{"section":"sec-2","sentences":[
 {"n":4,"file":"main.tex","line":12,"text":"The proof of the theorem is in the next section."}]}]}`

func prosePaper(t *testing.T, licence string) *understand.Paper {
	t.Helper()
	p := load(t, licence, "arxiv:2401.00001/prose")
	var tx understand.Text
	if err := json.Unmarshal([]byte(text), &tx); err != nil {
		t.Fatal(err)
	}
	if err := p.SetText(&tx); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProseBrief(t *testing.T) {
	p := prosePaper(t, "cc-by")
	if len(p.Items) != 2 || p.Items[0].ID != "ax://paper/2401.00001#sec-1" || p.Items[1].Ref != "2401.00001v1#sec-2" {
		t.Fatalf("the items are %+v", p.Items)
	}
	b := understand.Brief(&agent.Job{ID: "j-1", Goal: "find the prose items."}, p, "")
	for _, want := range []string{"each of the 2 sections below with one call of give_prose", "the running text of sec-1", "[1] (cue definition: we say that) We say that", "[4] The proof"} {
		if !strings.Contains(b, want) {
			t.Errorf("the brief has no %q:\n%s", want, b)
		}
	}
	var other understand.Text
	_ = json.Unmarshal([]byte(strings.Replace(text, `"version":1`, `"version":2`, 1)), &other)
	if err := p.SetText(&other); err == nil {
		t.Error("the text of another version is set")
	}
}

func TestProse(t *testing.T) {
	ws := tree(t)
	p := prosePaper(t, "cc-by")
	res, err := call(t, ws, p, "give_prose",
		map[string]any{"id": "ax://paper/2401.00001#sec-1", "prose": []map[string]any{
			{"from": 1, "to": 2, "kind": "definition", "term": "tame"},
			{"from": 3, "to": 3, "kind": "problem", "term": "x"},
			{"from": 2, "to": 2, "kind": "claim"},
			{"from": 4, "to": 4, "kind": "claim"},
		}},
		map[string]any{"id": "ax://paper/2401.00001#sec-2", "prose": []any{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Stored the results of 2 of 2 items.", "the term of the item 3 to 3, which is not a definition", "the item 2 to 2, which overlaps", "the item 4 to 4, whose sentences are not of the section"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("the result has no %q: %s", want, res.Text)
		}
	}
	rs := records(t, ws, "prose", "ax://paper/2401.00001#sec-1")
	var got []string
	for _, r := range rs {
		for _, pi := range r.Prose {
			got = append(got, fmt.Sprintf("%s %s %s:%d %s %s %v", r.ID, pi, pi.File, pi.Line, pi.Term, pi.Queue, pi.Accepted()))
		}
	}
	if want := "ax://paper/2401.00001#sec-1 [1-2] definition main.tex:5 tame prose-review false|ax://paper/2401.00001#sec-1 [3-3] problem main.tex:7  prose-review false"; strings.Join(got, "|") != want {
		t.Errorf("the prose items are\n%s\nwant\n%s", strings.Join(got, "|"), want)
	}
	if _, err := call(t, ws, p, "give_prose", map[string]any{"id": "ax://paper/2401.00001#sec-2"}); err == nil || !strings.Contains(err.Error(), "has no list prose") {
		t.Errorf("a result with no list gives %v", err)
	}
	// A paper of the class record keeps no term.
	ws = tree(t)
	p = prosePaper(t, "")
	res, err = call(t, ws, p, "give_prose", map[string]any{"id": "ax://paper/2401.00001#sec-1", "prose": []map[string]any{{"from": 1, "to": 1, "kind": "definition", "term": "tame"}}})
	if err != nil || !strings.Contains(res.Text, "no words of a paper of the class record") {
		t.Errorf("the result is %v, %v", res.Text, err)
	}
}

// TestProseCheck checks that the check of another family accepts an item only when it gives the same kind and finds the span complete (AX7.4 step 3).
func TestProseCheck(t *testing.T) {
	ws := tree(t)
	p := prosePaper(t, "cc-by")
	m := llmtest.New(
		llmtest.Call("check_prose", map[string]any{"kind": "definition", "complete": true}, llm.Usage{}),
		llmtest.Call("check_prose", map[string]any{"kind": "conjecture", "complete": true}, llm.Usage{}),
	)
	p.Check = understand.CheckWith(m, "other-model", "the check prompt")
	if _, err := call(t, ws, p, "give_prose", map[string]any{"id": "ax://paper/2401.00001#sec-1", "prose": []map[string]any{
		{"from": 1, "to": 1, "kind": "definition", "term": "tame"},
		{"from": 3, "to": 3, "kind": "problem"},
	}}); err != nil {
		t.Fatal(err)
	}
	rs := records(t, ws, "prose", "ax://paper/2401.00001#sec-1")
	if len(rs) != 1 || len(rs[0].Prose) != 2 {
		t.Fatalf("the records are %+v", rs)
	}
	a, b := rs[0].Prose[0], rs[0].Prose[1]
	if !a.Accepted() || a.Check.Model != "other-model" || b.Accepted() || b.Queue != understand.ProseReview || b.Check.Kind != "conjecture" {
		t.Errorf("the items are %+v %+v, %+v %+v", a, a.Check, b, b.Check)
	}
	reqs := m.Requests()
	if len(reqs) != 2 || reqs[0].System != "the check prompt" || !strings.Contains(reqs[0].Messages[0].Content[0].Text, "After the span:") || strings.Contains(reqs[0].Messages[0].Content[0].Text, "Before the span:") {
		t.Errorf("the first request of the check is %+v", reqs[0])
	}
}
