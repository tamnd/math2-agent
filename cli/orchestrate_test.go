package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/orchestrator"
	"github.com/tamnd/math2-agent/prompts"
)

func TestOrchestrate(t *testing.T) {
	t.Setenv("M2_LEAN_IMAGE", "")
	root, out := agentTree(t)
	proveModel(t)
	store, work := t.TempDir(), filepath.Join(t.TempDir(), "w")
	queue := filepath.Join(store, "queue")
	files := map[string]string{
		"config.yaml":     "roles: {prove: 10}\non_behalf_of: {schedule:standing-prove: 5}\n",
		"queue/a1.yaml":   "job: j-2026-11-20-a1\nrole: prove\ntarget: 7K3QX2\ngoal: the statement of the node has a Lean proof\non_behalf_of: schedule:standing-prove\nbudget: {usd: 2}\n",
		"queue/a2.yaml":   "job: j-2026-11-20-a2\nrole: prove\ntarget: 7K3QX2\ngoal: the statement of the node has a Lean proof\non_behalf_of: github:octocat\n",
		"queue/notes.txt": "not a job",
	}
	for name, text := range files {
		p := filepath.Join(store, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr := run("orchestrate", "-root", root, "-out", out, "-store", store, "-config", filepath.Join(store, "config.yaml"), "-work", work, "-once", "-no-sandbox", "-json")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	merge, report, _ := strings.Cut(stdout, "\n")
	if merge != "the job j-2026-11-20-a2 merges into j-2026-11-20-a1" {
		t.Errorf("the first line is %q", merge)
	}
	var rep orchestrator.Report
	if err := json.Unmarshal([]byte(report), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Runs) != 1 || rep.Runs[0].Outcome != agent.Success || rep.Runs[0].Budget != 2 || len(rep.Done) != 1 || len(rep.Waiting) != 0 {
		t.Errorf("the report is %+v", rep)
	}
	for id, want := range map[string]string{"j-2026-11-20-a1": agent.Success, "j-2026-11-20-a2": "merged"} {
		var d orchestrator.Done
		data, err := os.ReadFile(filepath.Join(queue, "done", id+".json"))
		if err == nil {
			err = json.Unmarshal(data, &d)
		}
		if err != nil || d.Outcome != want {
			t.Errorf("the end of %s is %+v, %v", id, d, err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(queue, "*.yaml")); len(left) != 0 {
		t.Errorf("the queue has %v", left)
	}
	if _, err := os.Stat(filepath.Join(work, "j-2026-11-20-a1-0", "lean", "Math2", "T.lean")); err != nil {
		t.Errorf("the work tree of the run that reached the goal: %v", err)
	}
	as, err := orchestrator.ReadAttempts(store)
	if err != nil || len(as) != 1 || as[0].Run != rep.Runs[0].Run {
		t.Errorf("the attempts in the store are %+v, %v", as, err)
	}

	// In the next pass, orchestrator.yaml gives no budget to prove, thus the new job waits in the queue.
	if err := os.WriteFile(filepath.Join(queue, "b1.yaml"), []byte(strings.ReplaceAll(files["queue/a1.yaml"], "a1", "b1")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "config.yaml"), []byte("roles: {formalize: 10}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ = run("orchestrate", "-root", root, "-out", out, "-store", store, "-config", filepath.Join(store, "config.yaml"), "-work", work, "-once", "-no-sandbox")
	if code != ExitOK || stdout != "job j-2026-11-20-b1 waits: orchestrator.yaml gives no budget to the role prove\n" {
		t.Errorf("exit code %d, stdout %q", code, stdout)
	}

	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"orchestrate", "extra"}, ExitUsage, "Usage: m2 orchestrate"},
		{[]string{"orchestrate", "-root", root, "-out", out, "-store", store, "-config", filepath.Join(store, "none.yaml"), "-once"}, ExitFailure, "none.yaml"},
		{[]string{"orchestrate", "-root", root, "-out", out, "-store", store, "-config", filepath.Join(store, "config.yaml"), "-once"}, ExitFailure, "give the image of the Lean sandbox"},
	} {
		code, _, stderr := run(c.args...)
		if code != c.code || !strings.Contains(stderr, c.want) {
			t.Errorf("m2 %s: exit code %d, stderr %q", strings.Join(c.args, " "), code, stderr)
		}
	}
}

func TestOrchestrateShadow(t *testing.T) {
	t.Setenv("M2_LEAN_IMAGE", "")
	root, out := agentTree(t)
	proveModel(t)
	u := llm.Usage{Input: 1000, Output: 200}
	write := func(path, text string) *llm.Response {
		return llmtest.Call("write_file", map[string]string{"path": path, "text": text}, u)
	}
	proved := "module\n\ndef Math2.T.s : Prop := ∀ n : Nat, n = n\n\n@[m2 \"7K3QX2\"]\ntheorem Math2.T.t : Math2.T.s := fun _ => rfl\n"
	newModel = func() (llm.Model, error) {
		return llmtest.New(write("lean/Math2/T.lean", proved), write("nodes/7K3/7K3QX2.md", agentNode("Math2.T.t")), write("lean/Math2/T.lean", proved), write("nodes/7K3/7K3QX2.md", agentNode("Math2.T.t"))), nil
	}

	// The candidate is the prove role of prompts/ at the version 1.0.0.
	cand := t.TempDir()
	err := fs.WalkDir(prompts.FS, "prove", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(prompts.FS, p)
		if err != nil {
			return err
		}
		if p == "prove/role.yaml" {
			data = regexp.MustCompile(`(?m)^version: .*$`).ReplaceAll(data, []byte("version: 1.0.0"))
		}
		if err := os.MkdirAll(filepath.Join(cand, filepath.Dir(p)), 0o750); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(cand, p), data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	store, work := t.TempDir(), filepath.Join(t.TempDir(), "w")
	config := filepath.Join(store, "config.yaml")
	if err := os.WriteFile(config, []byte("roles: {prove: 10}\nshadow: {prove: "+cand+"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store, "queue"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "queue", "a1.yaml"), []byte("job: j-2026-11-20-a1\nrole: prove\ntarget: 7K3QX2\ngoal: the statement of the node has a Lean proof\non_behalf_of: github:octocat\nbudget: {usd: 2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("orchestrate", "-root", root, "-out", out, "-store", store, "-config", config, "-work", work, "-once", "-no-sandbox", "-json")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var rep orchestrator.Report
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Runs) != 2 || rep.Runs[0].Shadow || !rep.Runs[1].Shadow || rep.Runs[1].Outcome != agent.Success || len(rep.Shadows) != 0 {
		t.Fatalf("the report is %+v", rep)
	}
	recs, err := agent.ReadRecords(store)
	if err != nil || len(recs) != 2 {
		t.Fatalf("the records are %v, %v", recs, err)
	}
	for _, r := range recs {
		if r.ID == rep.Runs[1].Run && (!r.Shadow || r.Agent != "m2-agent/prove@1.0.0") {
			t.Errorf("the shadow record is of %s, shadow %v", r.Agent, r.Shadow)
		}
	}
	if _, err := os.Stat(filepath.Join(store, "runs", rep.Runs[1].Run+".json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the shadow run has a summary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "j-2026-11-20-a1-0-shadow")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the work tree of the shadow run is not removed: %v", err)
	}

	// A candidate at the version of the role is not a candidate.
	data, err := fs.ReadFile(prompts.FS, "prove/role.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cand, "prove", "role.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run("orchestrate", "-root", root, "-out", out, "-store", store, "-config", config, "-work", work, "-once", "-no-sandbox"); code != ExitFailure || !strings.Contains(stderr, "the shadow week of prove: the candidate in "+cand+" has the version") {
		t.Errorf("exit code %d, stderr %q", code, stderr)
	}
}
