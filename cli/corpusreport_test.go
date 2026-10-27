package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/stage"
)

// TestCorpusReport runs m2 corpus report on the fixtures, with run records that a runner made with a scripted model.
func TestCorpusReport(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(valid)); err != nil {
		t.Fatal(err)
	}
	write := func(rel, text string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fsys := fstest.MapFS{
		"write/role.yaml":    {Data: []byte("role: write\nversion: 0.1.0\ntools: []\nladder: [{model: claude-sonnet-5-5}]\nbudget: {usd: 0.50}\nmax_tokens: 4000\nturns: 2\nstuck: 2\n")},
		"write/system.md":    {Data: []byte("You write the sections of a node.")},
		"write/CHANGELOG.md": {Data: []byte("# 0.1.0\n")},
	}
	role, err := agent.LoadRole(fsys, "write")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	store := filepath.Join(root, "build")
	var ids []string
	for i, target := range []string{"GY1XDB", "Q529ES"} {
		resp := &llm.Response{Content: []llm.Block{{Type: llm.Text, Text: "The proof section is written."}}, Usage: llm.Usage{Input: 10000, Output: 1500}}
		model := llmtest.New(resp, resp)
		done := i == 0
		r := agent.Runner{Role: role, Model: model, Goal: func(context.Context) (bool, string, error) { return done, "the node is below W2", nil }, Now: func() time.Time { return day }}
		job := &agent.Job{ID: "j-2026-11-02-" + strings.ToLower(target), Role: "write", Target: target, Goal: "node reaches W2", OnBehalfOf: "github:octocat"}
		rec, err := r.Run(context.Background(), job, 0, "Write the proof of "+target+".")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := agent.WriteRecord(store, rec); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rec.ID)
	}
	write(prov.Path("GY1XDB"), `{"field":"body.proof","op":"add","agent":"m2-agent/write@0.1.0","human":null,"run":"`+ids[0]+`","at":"2026-11-02T09:10:00Z"}`+"\n"+
		`{"field":"body.proof","op":"confirm","agent":null,"human":"github:octocat","at":"2026-11-03T09:00:00Z"}`+"\n")
	write(review.File("Q529ES"), "reviews:\n  - id: r-Q529ES-01\n    reviewer: github:octocat\n    date: 2026-11-04\n    decl: Math2.lagrange\n    statement_hash: blake3:"+strings.Repeat("0", 64)+"\n    type_hash: blake3:"+strings.Repeat("1", 64)+"\n    verdict: faithful\n    minutes: 5\n")
	row, err := json.Marshal(inventory.Row{ID: "9DK2RX7Q4H", Kind: "theorem", Layer: "textbook", History: []inventory.Event{{Op: "outcome", At: "2026-11-05", By: "github:octocat"}}})
	if err != nil {
		t.Fatal(err)
	}
	write(inventory.RowsDir+"/20.jsonl", string(row)+"\n")
	reportNow = func() time.Time { return day.AddDate(0, 0, 7) }
	t.Cleanup(func() { reportNow = time.Now })

	code, stdout, stderr := run("corpus", "report", "-root", root, "-from", "2026-11-01", "-to", "2026-11-30")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstderr: %s", code, stderr)
	}
	for _, want := range []string{"# The cost of each stage of the corpus", "from 2026-11-01 to 2026-11-30", "| usable | write | 2 | 2 | 1 | 50% |", "| reviewed | 1 | 1 | 0.08 | 0.08 | 3 |", "| decided | 1 | 0 | 0.00 | 0.02 | 1 |", "| confirmed | 1 | 0 |"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the report has no %q:\n%s", want, stdout)
		}
	}

	code, stdout, stderr = run("corpus", "report", "-root", root, "-json", "-from", "2026-12-01")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstderr: %s", code, stderr)
	}
	var rep stage.Report
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.From != "2026-12-01" || len(rep.Stages) != 0 {
		t.Errorf("the report of December is %s", stdout)
	}

	for _, args := range [][]string{{"-from", "2026-13-01"}, {"extra"}} {
		if code, _, _ := run(append([]string{"corpus", "report", "-root", root}, args...)...); code != ExitUsage {
			t.Errorf("%v gives the exit code %d", args, code)
		}
	}
}
