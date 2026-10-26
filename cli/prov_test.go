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
	path := filepath.Join(root, "nodes", "G5S", "G5S1M9.md")
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
	path := filepath.Join(root, "nodes", "G5S", "G5S1M9.md")
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
	if code, _, _ := run("prov", "disclose"); code != ExitUsage {
		t.Errorf("disclose with no tag gives the exit code %d", code)
	}
	root := provTree(t)
	if code, _, stderr := run("prov", "record", "-root", root, "-agent", "m2-agent/explain@1.2.0"); code != ExitFailure || !strings.Contains(stderr, "run") {
		t.Errorf("record by an agent with no run: exit code %d, stderr %q", code, stderr)
	}
}

func TestProvDisclose(t *testing.T) {
	root := provTree(t)
	code, stdout, stderr := run("prov", "disclose", "-root", root, "G5S1M9")
	if code != ExitOK || !strings.Contains(stdout, "m2://G5S1M9, Group at the commit ") || !strings.Contains(stdout, "has no provenance log") {
		t.Fatalf("disclose: exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, _, stderr := run("prov", "disclose", "-root", root, "-yaml", "G5S1M9"); code != ExitFailure || !strings.Contains(stderr, "no formal binding") {
		t.Errorf("disclose -yaml of a node with no binding: exit code %d, stderr %q", code, stderr)
	}

	path := filepath.Join(root, "nodes", "G5S", "G5S1M9.md")
	text := strings.Replace(provNode, "name: Group", "name: Group\nformal:\n  - system: lean4\n    library: mathlib\n    decl: Group\n    match: exact", 1)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run("prov", "record", "-root", root, "-agent", "m2-agent/formalize@0.9.1", "-run", "r-2026-10-26-1", "-model", "claude-opus-5-5", "-prompt", "formalize/3", "-at", "2026-10-26T10:00:00Z"); code != ExitOK {
		t.Fatalf("record: exit code %d, stderr %q", code, stderr)
	}
	run1 := &prov.Run{ID: "r-2026-10-26-1", Agent: "m2-agent/formalize@0.9.1", Prompt: "formalize/3", Models: []string{"claude-opus-5-5"}, OnBehalfOf: "github:writer", Started: "2026-10-26T09:50:00Z", Outcome: "success", Seconds: 600, SpendUSD: 1.5}
	if err := prov.WriteRun(filepath.Join(root, "build", prov.RunsDir), run1); err != nil {
		t.Fatal(err)
	}
	st := `[{"tag":"G5S1M9","kind":"definition","name":"Group","evidence":"","fidelity":"F2","flags":[],"items":[],"bindings":[{"decl":"Group","match":"exact","checked":"Group","type_hash":"","axioms":["propext"],"fidelity":"F2","reviews":[{"id":"r-G5S1M9-01","reviewer":"orcid:0000-0002-1825-0097","state":"counts"}],"level":"E5","reason":""}],"reasons":[]}]`
	if err := os.WriteFile(filepath.Join(root, "build", "status.json"), []byte(st), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = run("prov", "disclose", "-root", root, "G5S1M9")
	for _, want := range []string{"on behalf of github:writer", "The runs took 10 minutes and spent 1.50 US dollars in all.", "the evidence level E5 and the fidelity level F2, with reviews by orcid:0000-0002-1825-0097."} {
		if code != ExitOK || !strings.Contains(stdout, want) {
			t.Errorf("disclose: exit code %d, stderr %q, no %q in %q", code, stderr, want, stdout)
		}
	}
	code, stdout, stderr = run("prov", "disclose", "-root", root, "-yaml", "G5S1M9")
	for _, want := range []string{"version: v0.4\n", "- github:writer\n", "method: agent\n", "spend_usd: 1.5\n", "status: peer-reviewed\n", "relationship: formalizes\n"} {
		if code != ExitOK || !strings.Contains(stdout, want) {
			t.Errorf("disclose -yaml: exit code %d, stderr %q, no %q in:\n%s", code, stderr, want, stdout)
		}
	}
}
