package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/inventory"
)

func TestCorpusStatusOfTheResearchLayer(t *testing.T) {
	root, corpus := t.TempDir(), t.TempDir()
	if code, _, stderr := run("corpus", "status", "-root", root, "-layer", "research"); code != ExitUsage || !strings.Contains(stderr, "-corpus") {
		t.Fatalf("no checkout gave %d, stderr %q", code, stderr)
	}
	if code, _, _ := run("corpus", "status", "-root", root, "-layer", "stacks", "-corpus", corpus); code != ExitUsage {
		t.Fatalf("an unknown layer gave %d", code)
	}
	code, stdout, stderr := run("corpus", "status", "-root", root, "-layer", "research", "-corpus", corpus)
	if code != ExitOK || !strings.Contains(stdout, "papers with no state  not known") || !strings.Contains(stdout, "census gaps           not known") || !strings.Contains(stdout, "complete              false") || !strings.Contains(stderr, CensusFile) {
		t.Fatalf("no census gave %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	rows := []inventory.Row{
		{ID: "inv:a", Mentions: []string{"ax://paper/2401.00001#T1"}, Kind: "statement", Layer: inventory.Research, Spread: 1, Outcome: &inventory.Outcome{Value: inventory.OutcomeNode, Target: []string{"T0001", "T0002"}, By: "t"}},
		{ID: "inv:b", Mentions: []string{"ax://paper/2401.00001#T2"}, Kind: "statement", Layer: inventory.Research, Spread: 1, Outcome: &inventory.Outcome{Value: inventory.OutcomeRecord, By: "t"}},
		{ID: "inv:c", Mentions: []string{"ax://paper/2401.00001#T3"}, Kind: "statement", Layer: inventory.Research, Spread: 1},
		{ID: "inv:d", Mentions: []string{"stacks://tag/0001"}, Kind: "statement", Layer: "core", Spread: 1},
	}
	if err := inventory.WriteRows(root, rows); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(corpus, CensusFile), `{"generated":"2026-10-27","scope_counted":true,"scope":10,"extracted":7,"stopped":1,"no_state":2,"gaps":0}`)
	write(filepath.Join(root, "build", "writing.json"), `{"cards":"embedded","levels":{"W1":1},"nodes":[{"tag":"T0001","kind":"theorem","level":"W1"}]}`)

	code, stdout, stderr = run("corpus", "status", "-root", root, "-layer", "research", "-corpus", corpus, "-json")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	var st ResearchStatus
	if err := json.Unmarshal([]byte(stdout), &st); err != nil {
		t.Fatal(err)
	}
	if st.NoState != 2 || st.Scope != 10 || st.Rows != 3 || st.Open != 1 || st.Outcomes["node"] != 1 || st.Outcomes["record"] != 1 || st.Outcomes["open"] != 1 {
		t.Errorf("status %+v", st)
	}
	if st.Nodes != 2 || st.Levels["W1"] != 1 || st.Unbuilt != 1 || st.Complete {
		t.Errorf("levels %+v", st)
	}
}

func TestTheReportOfTheResearchLayer(t *testing.T) {
	root, corpus := t.TempDir(), t.TempDir()
	rows := []inventory.Row{
		{ID: "inv:a", Mentions: []string{"ax://paper/2401.00001#T1"}, Kind: "statement", Layer: inventory.Research, Spread: 1, MSC: []string{"14H10"}, Outcome: &inventory.Outcome{Value: inventory.OutcomeNode, Target: []string{"T0001"}, By: "t"}},
		{ID: "inv:b", Mentions: []string{"ax://paper/2401.00002#T1"}, Kind: "statement", Layer: inventory.Research, Spread: 1, MSC: []string{"11F03"}, Outcome: &inventory.Outcome{Value: inventory.OutcomeNode, Target: []string{"T0002"}, By: "t"}},
		{ID: "inv:c", Mentions: []string{"ax://paper/2401.00002#T2"}, Kind: "statement", Layer: inventory.Research, Spread: 1, MSC: []string{"11F03"}},
	}
	if err := inventory.WriteRows(root, rows); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(corpus, CensusFile), `{"generated":"2026-10-27","scope_counted":true,"scope":5,"extracted":4,"stopped":0,"no_state":1,"gaps":1,
"areas":{"14":{"scope":2,"extracted":2,"no_state":0,"gaps":0},"11":{"scope":3,"extracted":2,"no_state":1,"gaps":1}}}`)
	write(filepath.Join(root, "build", "writing.json"), `{"cards":"embedded","levels":{"W1":2},"nodes":[{"tag":"T0001","kind":"theorem","level":"W1"},{"tag":"T0002","kind":"theorem","level":"W1"}]}`)
	write(filepath.Join(root, "build", "formal.json"), `{"outcomes":{"bound":1,"out_of_reach":1},"nodes":[{"tag":"T0001","kind":"statement","outcome":"bound"},{"tag":"T0002","kind":"statement","outcome":"out_of_reach","reason":"needs-a-large-library"}]}`)

	code, stdout, stderr := run("corpus", "status", "-root", root, "-layer", "research", "-corpus", corpus, "-json", "-write")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	var st ResearchStatus
	if err := json.Unmarshal([]byte(stdout), &st); err != nil {
		t.Fatal(err)
	}
	if st.Formal["bound"] != 1 || st.Formal["out_of_reach"] != 1 || len(st.Areas) != 2 {
		t.Fatalf("status %+v", st)
	}
	a11, a14 := st.Areas[0], st.Areas[1]
	if a11.Code != "11" || a11.Scope != 3 || a11.NoState != 1 || a11.Gaps != 1 || a11.Rows != 2 || a11.Open != 1 || a11.Nodes != 1 || a11.Formal["out_of_reach"] != 1 || a11.Complete {
		t.Errorf("area 11 %+v", a11)
	}
	if a14.Code != "14" || a14.Rows != 1 || a14.Open != 0 || a14.Levels["W1"] != 1 || a14.Formal["bound"] != 1 || !a14.Complete {
		t.Errorf("area 14 %+v", a14)
	}
	md, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ResearchReport)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- Papers with no state: 1\n", "| formal out_of_reach | 1 |", "| 11 | 3 | 1 | 1 | 2 | 1 | 1 | W1 1 | out_of_reach 1 | no |", "| 14 | 2 | 0 | 0 | 1 | 0 | 1 | W1 1 | bound 1 | yes |"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("the report has no %q:\n%s", want, md)
		}
	}
}
