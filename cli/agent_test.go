package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/sandbox"
)

// agentLean reads each line "def NAME : TYPE := VALUE" or "theorem NAME : TYPE := PROOF" of lean/Math2/T.lean of the tree as a declaration.
type agentLean struct{ tree string }

var agentDeclRe = regexp.MustCompile(`^(def|theorem) (\S+) : (.+?) := (.+)$`)

func (f agentLean) Check(context.Context, string) (*lean.Elaboration, error) {
	return &lean.Elaboration{Messages: []lean.Message{}}, nil
}

func (f agentLean) Extract(_ context.Context, decls []string, _ bool) ([]lean.Record, error) {
	data, err := os.ReadFile(filepath.Join(f.tree, "lean", "Math2", "T.lean"))
	if err != nil {
		return nil, err
	}
	all := map[string]lean.Record{}
	for _, line := range strings.Split(string(data), "\n") {
		if m := agentDeclRe.FindStringSubmatch(line); m != nil {
			r := lean.Record{Decl: m[2], Kind: m[1], Module: "Math2.T", Type: "c " + m[3] + "\n", Axioms: []string{}}
			if m[3] == "Prop" {
				r.Type, r.ValueHash = "s 0\n", lean.Hash(m[4])
			}
			all[r.Decl] = r
		}
	}
	var out []lean.Record
	for _, d := range decls {
		r, ok := all[d]
		if !ok {
			r = lean.Record{Decl: d, Error: "unknown declaration"}
		}
		out = append(out, r)
	}
	return out, nil
}

func agentNode(decl string) string {
	return "---\ntag: 7K3QX2\nkind: statement\nname: Reflexivity\nformal:\n  - system: lean4\n    library: math2\n    decl: " + decl + "\n    match: exact\n---\n\n## Statement\n\nEach natural number is equal to itself.\n"
}

const agentLeanFile = "module\n\n@[m2 \"7K3QX2\"]\ndef Math2.T.s : Prop := ∀ n : Nat, n = n\n"

// agentTree makes a git tree of math2 with the node 7K3QX2 and its statement in Lean, and a build of the valid tree.
func agentTree(t *testing.T) (root, out string) {
	t.Helper()
	out = t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: valid, Out: out}); err != nil {
		t.Fatal(err)
	}
	root = t.TempDir()
	for name, text := range map[string]string{
		"nodes/7K/7K3QX2.md": agentNode("Math2.T.s"),
		"lean/Math2.lean":    "module\n\npublic import Math2.T\n",
		"lean/Math2/T.lean":  agentLeanFile,
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
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
		if b, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}
	return root, out
}

// proveModel replaces the model and the Lean environment with a scripted model that proves 7K3QX2 in two turns and the fake agentLean.
func proveModel(t *testing.T) {
	t.Helper()
	u := llm.Usage{Input: 1000, Output: 200}
	write := func(path, text string) *llm.Response {
		return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
	}
	proved := "module\n\ndef Math2.T.s : Prop := ∀ n : Nat, n = n\n\n@[m2 \"7K3QX2\"]\ntheorem Math2.T.t : Math2.T.s := fun _ => rfl\n"
	oldModel, oldLean := newModel, newLean
	newModel = func() (llm.Model, error) {
		return llmtest.New(write("lean/Math2/T.lean", proved), write("nodes/7K/7K3QX2.md", agentNode("Math2.T.t"))), nil
	}
	newLean = func(_ context.Context, tree string, _ *tools.Box) (tools.Lean, error) { return agentLean{tree}, nil }
	t.Cleanup(func() { newModel, newLean = oldModel, oldLean })
}

func TestAgentRun(t *testing.T) {
	t.Setenv("M2_LEAN_IMAGE", "")
	root, out := agentTree(t)
	proveModel(t)

	work := filepath.Join(t.TempDir(), "w")
	store := t.TempDir()
	code, stdout, stderr := run("agent", "run", "-root", root, "-out", out, "-store", store, "-work", work, "-target", "7K3QX2", "-on-behalf-of", "github:octocat", "-no-sandbox", "-json", "prove")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var sum struct {
		Run      string   `json:"run"`
		Outcome  string   `json:"outcome"`
		WorkTree string   `json:"work_tree"`
		Changes  []string `json:"changes"`
		Turns    int      `json:"turns"`
	}
	if err := json.Unmarshal([]byte(stdout), &sum); err != nil {
		t.Fatal(err)
	}
	if sum.Outcome != agent.Success || sum.Turns != 2 || sum.WorkTree != work || strings.Join(sum.Changes, " ") != "lean/Math2/T.lean nodes/7K/7K3QX2.md" {
		t.Errorf("the summary is %+v", sum)
	}
	if _, err := os.Stat(filepath.Join(store, agent.RecordsDir, sum.Run+".json")); err != nil {
		t.Errorf("the run record: %v", err)
	}

	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"agent"}, ExitUsage, "Usage: m2 agent run"},
		{[]string{"agent", "run", "explore"}, ExitUsage, "the role explore is not written yet"},
		{[]string{"agent", "run", "-root", root, "-out", out, "-target", "7K3QX2", "-on-behalf-of", "", "prove"}, ExitFailure, "not a person, a project or a schedule"},
		{[]string{"agent", "run", "-root", root, "-out", t.TempDir(), "-target", "7K3QX2", "-on-behalf-of", "github:octocat", "-no-sandbox", "prove"}, ExitFailure, "Run m2 build first"},
		{[]string{"agent", "run", "-root", root, "-out", out, "-target", "7K3QX2", "-on-behalf-of", "github:octocat", "prove"}, ExitFailure, "give the image of the Lean sandbox"},
		{[]string{"agent", "run", "-root", root, "-out", out, "-target", "7K3QX2", "-on-behalf-of", "github:octocat", "-sandbox", "m2-lean:latest", "prove"}, ExitFailure, "has no digest"},
		{[]string{"agent", "run", "-root", root, "-out", out, "-target", "7K3QX2", "-on-behalf-of", "github:octocat", "-sandbox", "m2-lean@sha256:" + strings.Repeat("0", 64), "prove"}, ExitFailure, "needs the Lake packages"},
	} {
		code, _, stderr := run(c.args...)
		if code != c.code || !strings.Contains(stderr, c.want) {
			t.Errorf("m2 %s: exit code %d, stderr %q", strings.Join(c.args, " "), code, stderr)
		}
	}
}

func TestLinkPackagesBoxed(t *testing.T) {
	root, tree := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(tree, "lean", "Math2"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := linkPackages(root, tree, true); err != nil {
		t.Fatal(err)
	}
	link, err := os.Readlink(filepath.Join(tree, "lean", ".lake", "packages"))
	if err != nil || link != sandbox.WorkDir {
		t.Errorf("the link is %q, %v", link, err)
	}
	for _, dir := range []string{"lean", "lean/.lake", "lean/Math2"} {
		fi, err := os.Stat(filepath.Join(tree, dir))
		if err != nil || fi.Mode().Perm() != 0o777 {
			t.Errorf("%s: %v %v", dir, fi.Mode(), err)
		}
	}
}

func TestAgentRunExplain(t *testing.T) {
	t.Setenv("M2_LEAN_IMAGE", "")
	root, out := agentTree(t)
	u := llm.Usage{Input: 1000, Output: 200}
	text := agentNode("Math2.T.s") + "\n## Explanations\n\n### L2 agent\n\nThe [[statement|7K3QX2]] says that each natural number is equal to itself.\n"
	labels := map[string]any{"labels": []map[string]any{{"sentence": 1, "label": "supported", "why": "The node states it."}}}
	oldModel, oldLean := newModel, newLean
	newModel = func() (llm.Model, error) {
		return llmtest.New(llmtest.Call("write_file", map[string]string{"path": "nodes/7K/7K3QX2.md", "text": text}, u), llmtest.Call("label", labels, u)), nil
	}
	newLean = func(context.Context, string, *tools.Box) (tools.Lean, error) {
		return nil, errors.New("explain needs no Lean")
	}
	t.Cleanup(func() { newModel, newLean = oldModel, oldLean })

	store := t.TempDir()
	code, stdout, stderr := run("agent", "run", "-root", root, "-out", out, "-store", store, "-work", filepath.Join(t.TempDir(), "w"), "-target", "7K3QX2", "-on-behalf-of", "github:octocat", "explain")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "success, the goal holds") || !strings.Contains(stdout, "m2-agent/explain@0.1.0, 1 turns") || !strings.Contains(stdout, "nodes/7K/7K3QX2.md") {
		t.Errorf("the output is\n%s", stdout)
	}
}

func TestAgentRunCurate(t *testing.T) {
	t.Setenv("M2_LEAN_IMAGE", "")
	root, out := agentTree(t)
	// The records are in build/records, which git does not track. The run reads them from the tree and not from the work tree.
	recordText := "Proposition 1. For every natural number n we have n = n."
	rec, err := json.Marshal(ingest.Record{ID: "2401.00001#1AB2", Kind: "statement", Locator: "ax://paper/2401.00001#1AB2", Class: ingest.Open, Text: recordText, Fingerprints: ingest.Fingerprints(recordText), Meta: map[string]string{"paper": "2401.00001", "version": "v2"}})
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		ingest.CatalogueFile:                "sources:\n  - id: arxiv_results\n    name: The results of arXiv papers\n    licence: CC-BY-4.0\n    class: open\n",
		"build/records/arxiv_results.jsonl": string(rec) + "\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	u := llm.Usage{Input: 1000, Output: 200}
	text := "---\ntag: ~0a1b2c3d\nkind: statement\nrole: theorem\nname: Reflexivity of equality\nsources:\n  - arxiv: 2401.00001v2\n    locator: ax://paper/2401.00001#1AB2\n---\n\n## Statement\n\nEach natural number is equal to itself.\n"
	oldModel, oldLean := newModel, newLean
	newModel = func() (llm.Model, error) {
		return llmtest.New(
			llmtest.Call("record", map[string]string{"ref": "arxiv_results:2401.00001#1AB2"}, u),
			llmtest.Call("write_file", map[string]string{"path": "nodes/~/~0a1b2c3d.md", "text": text}, u),
			llmtest.Call("judge", map[string]string{"verdict": "same", "why": "n is the number."}, u),
		), nil
	}
	newLean = func(context.Context, string, *tools.Box) (tools.Lean, error) {
		return nil, errors.New("curate needs no Lean")
	}
	t.Cleanup(func() { newModel, newLean = oldModel, oldLean })

	code, stdout, stderr := run("agent", "run", "-root", root, "-out", out, "-store", t.TempDir(), "-work", filepath.Join(t.TempDir(), "w"), "-target", "arxiv_results:2401.00001#1AB2", "-on-behalf-of", "github:octocat", "curate")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "success, the goal holds") || !strings.Contains(stdout, "m2-agent/curate@0.1.0, 2 turns") || !strings.Contains(stdout, "nodes/~/~0a1b2c3d.md") {
		t.Errorf("the output is\n%s", stdout)
	}
}

func TestAgentRunAlign(t *testing.T) {
	t.Setenv("M2_LEAN_IMAGE", "")
	root, out := agentTree(t)
	rec, err := json.Marshal(ingest.Record{ID: "Q7", Kind: "item", Title: "reflexivity", URL: "https://www.wikidata.org/wiki/Q7", Text: "The statement that each natural number is equal to itself."})
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		ingest.CatalogueFile:           "sources:\n  - id: wikidata\n    name: Wikidata\n    licence: CC0-1.0\n    class: open\n",
		"build/records/wikidata.jsonl": string(rec) + "\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	u := llm.Usage{Input: 1000, Output: 200}
	oldModel, oldLean := newModel, newLean
	newModel = func() (llm.Model, error) {
		return llmtest.New(
			llmtest.Call("propose_edge", map[string]string{"edge": "same_as", "ref": "wikidata:Q7", "why": "the item is the statement."}, u),
			llmtest.Call("judge", map[string]string{"verdict": "same", "why": "n is the number."}, u),
		), nil
	}
	newLean = func(context.Context, string, *tools.Box) (tools.Lean, error) {
		return nil, errors.New("align needs no Lean")
	}
	t.Cleanup(func() { newModel, newLean = oldModel, oldLean })

	code, stdout, stderr := run("agent", "run", "-root", root, "-out", out, "-store", t.TempDir(), "-work", filepath.Join(t.TempDir(), "w"), "-target", "7K3QX2", "-on-behalf-of", "github:octocat", "align")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "success, the goal holds") || !strings.Contains(stdout, "m2-agent/align@0.1.0, 1 turns") || !strings.Contains(stdout, "queues/align/wikidata.jsonl") {
		t.Errorf("the output is\n%s", stdout)
	}
}

func TestAgentRunBump(t *testing.T) {
	t.Setenv("M2_LEAN_IMAGE", "")
	root, out := agentTree(t)
	if err := os.WriteFile(filepath.Join(root, "nodes", "7K", "7K3QX2.md"), []byte(agentNode("Math2.T.r")), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command("git", "-C", root, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-am", "the bump").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, b)
	}
	u := llm.Usage{Input: 1000, Output: 200}
	oldModel, oldLean := newModel, newLean
	newModel = func() (llm.Model, error) {
		return llmtest.New(llmtest.Call("rebind", map[string]string{"decl": "Math2.T.r", "new": "Math2.T.s", "why": "the library renamed r to s."}, u)), nil
	}
	newLean = func(_ context.Context, tree string, _ *tools.Box) (tools.Lean, error) { return agentLean{tree}, nil }
	t.Cleanup(func() { newModel, newLean = oldModel, oldLean })

	code, stdout, stderr := run("agent", "run", "-root", root, "-out", out, "-store", t.TempDir(), "-work", filepath.Join(t.TempDir(), "w"), "-target", "7K3QX2", "-on-behalf-of", "github:octocat", "-no-sandbox", "bump")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "success, the goal holds") || !strings.Contains(stdout, "m2-agent/bump@0.1.0, 1 turns") || !strings.Contains(stdout, "nodes/7K/7K3QX2.md") {
		t.Errorf("the output is\n%s", stdout)
	}
}
