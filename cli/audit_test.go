package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// auditTree copies the audit fixture at name to a new directory, so that the reports of a run do not go into the testdata.
func auditTree(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("..", "audit", "testdata", name))); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestAuditGoodTree(t *testing.T) {
	root := auditTree(t, filepath.Join("T05", "good"))
	code, stdout, stderr := run("audit", "-root", root, "-plane", "nodes")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q\n%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "T05   pass") || !strings.Contains(stdout, "nothing failed") {
		t.Errorf("stdout has no T05 pass and no summary:\n%s", stdout)
	}
	if strings.Contains(stdout, "G01") {
		t.Errorf("the report of the nodes plane has a graph rule:\n%s", stdout)
	}
	text, err := os.ReadFile(filepath.Join(root, "reports", "audit", "nodes.txt"))
	if err != nil || string(text) != stdout {
		t.Errorf("reports/audit/nodes.txt is not the output: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "reports", "audit", "nodes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Results []struct{ Rule, State string }
	}
	if err := json.Unmarshal(data, &rep); err != nil || len(rep.Results) != 19 {
		t.Errorf("reports/audit/nodes.json has %d results, and the nodes plane has 19 rules: %v", len(rep.Results), err)
	}
}

func TestAuditBadTree(t *testing.T) {
	root := auditTree(t, filepath.Join("T05", "bad"))
	code, stdout, _ := run("audit", "-root", root, "-rule", "t05,t06", "-q")
	if code != ExitFailure {
		t.Fatalf("exit code is %d, want %d", code, ExitFailure)
	}
	if strings.Contains(stdout, ": T05: ") || !strings.Contains(stdout, "T05   fail") {
		t.Errorf("-q must write only the table:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, "reports", "audit", "T05-T06.txt")); err != nil {
		t.Error(err)
	}
}

func TestAuditRelease(t *testing.T) {
	root := auditTree(t, filepath.Join("T05", "good"))
	if code, _, stderr := run("audit", "-root", root, "-release", "2027.03"); code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "reports", "releases", "2027.03.txt")); err != nil {
		t.Error(err)
	}
}

func TestAuditUsage(t *testing.T) {
	root := auditTree(t, filepath.Join("T05", "good"))
	for _, args := range [][]string{
		{"-release", "2027-03"},
		{"-release", "2027.03", "-plane", "nodes"},
		{"-release", "2027.03", "-changed"},
		{"extra"},
		{"-plane", "trust"},
		{"-rule", "Z99"},
	} {
		if code, _, _ := run(append([]string{"audit", "-root", root}, args...)...); code != ExitUsage {
			t.Errorf("%v: exit code is %d, want %d", args, code, ExitUsage)
		}
	}
	if code, _, _ := run("audit", "-root", root, "-plane", "graph", "-rule", "T05"); code != ExitFailure {
		t.Errorf("a plane with none of the rules: exit code is %d, want %d", code, ExitFailure)
	}
}
