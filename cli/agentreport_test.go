package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/prov"
)

func TestAgentReport(t *testing.T) {
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
	write(tools.NodePath("7K3QX2"), "---\ntag: 7K3QX2\nkind: theorem\nname: A theorem\nmsc: [11A41]\n---\n\n## Statement\n\nA statement.\n")
	write(prov.Path("7K3QX2"), `{"field":"formal[0].decl","op":"change","agent":"m2-agent/prove@0.1.1","run":"r-2026-10-20-0001","at":"2026-10-20T10:00:00Z"}`+"\n")
	for _, r := range []*agent.Record{
		{ID: "r-2026-10-20-0001", Job: agent.Job{ID: "j-2026-10-20-1", Role: "prove", Target: "7K3QX2"}, Rung: agent.Rung{Model: "model-a"}, Outcome: agent.Success, Started: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC), Spend: agent.Spend{USD: 0.75}},
		{ID: "r-2026-10-27-0002", Job: agent.Job{ID: "j-2026-10-27-1", Role: "prove", Target: "7K3QX2"}, Rung: agent.Rung{Model: "model-a"}, Outcome: agent.Failure, Started: time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC), Spend: agent.Spend{USD: 0.5}},
	} {
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		write("build/"+agent.RecordsDir+"/"+r.ID+".json", string(data))
	}
	write("reports/agents/2026-W42.json", `{"week":"2026-W42","roles":[{"role":"prove","usd_per_accepted":1.5}]}`)
	reportNow = func() time.Time { return time.Date(2026, 10, 28, 6, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { reportNow = time.Now })

	code, stdout, stderr := run("agent", "report", "-root", root)
	if code != ExitOK {
		t.Fatalf("exit code %d\nstderr: %s", code, stderr)
	}
	for _, want := range []string{"# The agent report of 2026-W43", "| prove | 1 | 1 | 1 | 0 | 0 | 0 |", "| prove | $0.75 | 1 | $0.75 | $1.50 |", "| prove | 11 | 1 | 1 | $0.75 | $0.75 |"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the report has no %q:\n%s", want, stdout)
		}
	}
	for _, name := range []string{"2026-W43.md", "2026-W43.json"} {
		if _, err := os.Stat(filepath.Join(root, "reports", "agents", name)); err != nil {
			t.Error(err)
		}
	}

	code, stdout, stderr = run("agent", "report", "-root", root, "-week", "2026-W44", "-json")
	if code != ExitOK {
		t.Fatalf("exit code %d\nstderr: %s", code, stderr)
	}
	var rep struct {
		Week  string `json:"week"`
		Roles []struct {
			Runs     int `json:"runs"`
			Accepted int `json:"accepted"`
		} `json:"roles"`
	}
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Week != "2026-W44" || len(rep.Roles) != 1 || rep.Roles[0].Runs != 1 || rep.Roles[0].Accepted != 0 {
		t.Errorf("the report is %s", stdout)
	}

	for _, args := range [][]string{{"-week", "2026-W60"}, {"extra"}} {
		if code, _, _ := run(append([]string{"agent", "report", "-root", root}, args...)...); code != ExitUsage {
			t.Errorf("%v gives the exit code %d", args, code)
		}
	}
}
