package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/prov"
)

const provNode = "---\ntag: G5S1M9\nkind: definition\nname: Group\n---\n\n## Statement\n\nA set with an operation.\n"

// provTree writes a git tree with one committed node.
func provTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "nodes", "G5", "G5S1M9.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(provNode), 0o600); err != nil {
		t.Fatal(err)
	}
	gitT(t, root, "init", "-q")
	gitT(t, root, "add", ".")
	gitT(t, root, "commit", "-q", "-m", "seed")
	return root
}

func TestProvRecordShowConfirm(t *testing.T) {
	root := provTree(t)
	path := filepath.Join(root, "nodes", "G5", "G5S1M9.md")
	text := strings.Replace(provNode, "name: Group", "name: Group\nmsc: [20A05]", 1) + "\n## Explanations\n\n### L1 agent\n\nA group is a way to combine things.\n"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("prov", "record", "-root", root, "-agent", "m2-agent/explain@1.2.0", "-run", "r-2026-10-26-1", "-model", "claude-opus-5-5", "-at", "2026-10-26T10:00:00Z", "-n")
	if code != ExitOK || stdout != "G5S1M9 add body.explanations.L1\nG5S1M9 add msc\n" {
		t.Fatalf("record -n: exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "prov")); err == nil {
		t.Error("record -n writes the log")
	}
	if code, _, stderr := run("prov", "record", "-root", root, "-agent", "m2-agent/explain@1.2.0", "-run", "r-2026-10-26-1", "-model", "claude-opus-5-5", "-at", "2026-10-26T10:00:00Z"); code != ExitOK {
		t.Fatalf("record: exit code %d, stderr %q", code, stderr)
	}
	// The fields have lines now, so a second record writes nothing.
	if _, stdout, _ := run("prov", "record", "-root", root, "-by", "github:writer"); !strings.Contains(stdout, "no field changed since HEAD has no line") {
		t.Errorf("the second record writes %q", stdout)
	}

	code, stdout, stderr = run("prov", "show", "-root", root, "G5S1M9")
	if code != ExitOK || !strings.Contains(stdout, "body.explanations.L1") || !strings.Contains(stdout, "m2-agent/explain@1.2.0") {
		t.Fatalf("show: exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if _, stdout, _ := run("prov", "show", "-root", root, "-field", "msc", "-json", "G5S1M9"); strings.Count(stdout, `"field"`) != 1 || !strings.Contains(stdout, `"field": "msc"`) {
		t.Errorf("show -field msc -json writes %q", stdout)
	}

	code, stdout, _ = run("prov", "quarantine", "-root", root, "-model", "claude-opus-5-5", "-by", "github:curator")
	if code != ExitOK || !strings.Contains(stdout, "2 fields are from") {
		t.Fatalf("quarantine: exit code %d, stdout %q", code, stdout)
	}
	if _, stdout, _ := run("prov", "show", "-root", root, "G5S1M9"); !strings.Contains(stdout, "body.explanations.L1 is in quarantine") {
		t.Errorf("show after the quarantine writes %q", stdout)
	}
	if code, _, stderr := run("prov", "confirm", "-root", root, "-by", "github:curator", "G5S1M9", "name"); code != ExitFailure || stderr == "" {
		t.Errorf("confirm on a field with no agent text: exit code %d, stderr %q", code, stderr)
	}
	if code, stdout, _ := run("prov", "confirm", "-root", root, "-by", "github:curator", "G5S1M9", "body.explanations.L1"); code != ExitOK || stdout != "G5S1M9 confirm body.explanations.L1\n" {
		t.Errorf("confirm: exit code %d, stdout %q", code, stdout)
	}
	log, err := prov.Read(root, "G5S1M9")
	if err != nil {
		t.Fatal(err)
	}
	if q := prov.Quarantined(log); len(q) != 1 {
		t.Errorf("after the confirm the fields in quarantine are %v", q)
	}

	code, stdout, _ = run("prov", "credit", "-root", root, "-from", "2026-01-01", "-to", "2026-12-31", "github:curator")
	if code != ExitOK || !strings.Contains(stdout, "Contributions of github:curator") || !strings.Contains(stdout, "m2://G5S1M9") {
		t.Errorf("credit: exit code %d, stdout %q", code, stdout)
	}
}

func TestProvUsage(t *testing.T) {
	for name, args := range map[string][]string{
		"no subcommand":     {"prov"},
		"unknown":           {"prov", "edit"},
		"show no tag":       {"prov", "show"},
		"confirm no field":  {"prov", "confirm", "-by", "github:curator", "G5S1M9"},
		"confirm no person": {"prov", "confirm", "G5S1M9", "name"},
		"quarantine model":  {"prov", "quarantine"},
		"credit no person":  {"prov", "credit"},
	} {
		if code, _, _ := run(args...); code != ExitUsage {
			t.Errorf("%s: exit code %d, want %d", name, code, ExitUsage)
		}
	}
	if code, _, _ := run("prov", "disclose"); code != ExitNotWritten {
		t.Errorf("disclose gives the exit code %d", code)
	}
	root := provTree(t)
	if code, _, stderr := run("prov", "record", "-root", root, "-agent", "m2-agent/explain@1.2.0"); code != ExitFailure || !strings.Contains(stderr, "run") {
		t.Errorf("record by an agent with no run: exit code %d, stderr %q", code, stderr)
	}
}
