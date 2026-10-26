package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/dataset"
	"github.com/tamnd/math2-agent/prov"
)

func TestAgentDataset(t *testing.T) {
	root := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(prov.Path("7K3QX2"), `{"field":"formal[0].decl","op":"change","agent":"m2-agent/prove@0.1.1","run":"r-2026-07-20-0001","at":"2026-07-20T10:00:00Z"}`+"\n")
	for _, r := range []*agent.Record{
		{ID: "r-2026-07-20-0001", Job: agent.Job{ID: "j-2026-07-20-1", Role: "prove", Target: "7K3QX2", OnBehalfOf: "github:octocat"}, Rung: agent.Rung{Model: "model-a"}, Outcome: agent.Success, Started: time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC), Spend: agent.Spend{USD: 0.75}},
		{ID: "r-2026-09-27-0002", Job: agent.Job{ID: "j-2026-09-27-1", Role: "prove", Target: "7K3QX2", OnBehalfOf: "project:ug-core"}, Rung: agent.Rung{Model: "model-a"}, Outcome: agent.Failure, Reason: "no key sk-ant-xyz", Started: time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC), Spend: agent.Spend{USD: 0.5}},
		{ID: "r-2026-10-27-0003", Job: agent.Job{ID: "j-2026-10-27-1", Role: "prove", Target: "7K3QX2", OnBehalfOf: "project:ug-core"}, Rung: agent.Rung{Model: "model-a"}, Outcome: agent.Failure, Started: time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)},
	} {
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		write("build/"+agent.RecordsDir+"/"+r.ID+".json", string(data))
	}
	reportNow = func() time.Time { return time.Date(2026, 10, 28, 6, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { reportNow = time.Now })

	code, stdout, stderr := run("agent", "dataset", "-root", root)
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %s", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("the dataset of 2026-Q3 is\n%s", stdout)
	}
	var rows []dataset.Row
	for _, l := range lines {
		var r dataset.Row
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, r)
	}
	if !rows[0].Merged || rows[0].OnBehalfOf != dataset.Person || rows[1].Merged || rows[1].Reason != "no key [key]" {
		t.Errorf("the rows are %+v", rows)
	}
	if strings.Contains(stdout, "octocat") || strings.Contains(stdout, "sk-ant") {
		t.Errorf("the dataset has personal data or a key:\n%s", stdout)
	}

	file := filepath.Join(t.TempDir(), "runs.jsonl")
	if code, stdout, stderr := run("agent", "dataset", "-root", root, "-quarter", "2026-Q4", "-o", file); code != ExitOK || !strings.Contains(stdout, "wrote 1 runs of 2026-Q4") {
		t.Fatalf("exit code %d, stdout %s, stderr %s", code, stdout, stderr)
	}
	if code, _, stderr := run("agent", "dataset", "-quarter", "2026-W43"); code != ExitUsage || !strings.Contains(stderr, "YYYY-Qn") {
		t.Errorf("a week gives the exit code %d and %s", code, stderr)
	}
}
