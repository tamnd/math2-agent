package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// alignTree writes a tree with two nodes and the Wikidata records of one of them.
func alignTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"nodes/G5S/G5S1M9.md":          "---\ntag: G5S1M9\nkind: definition\nname: Group\n---\n\n## Statement\n\nA set with an operation.\n",
		"nodes/H7A/H7AAAA.md":          "---\ntag: H7AAAA\nkind: definition\nname: Ring\n---\n\n## Statement\n\nA set with two operations.\n",
		"build/records/wikidata.jsonl": `{"id":"Q2","title":"group","class":"open"}` + "\n" + `{"id":"Q9","title":"field","class":"open"}` + "\n",
	}
	for p, data := range files {
		path := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestAlign(t *testing.T) {
	root := alignTree(t)
	code, stdout, stderr := run("align", "wikidata", "-root", root, "-date", "2026-10-25", "-n")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "wikidata: 1 same_as, 0 close_to, 0 aligned and 0 duplicate proposals for 2 nodes") || !strings.Contains(stdout, "G5S1M9 same_as wikidata:Q2  group") {
		t.Errorf("stdout is:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, "queues")); err == nil {
		t.Error("align -n writes the queue")
	}

	if code, _, stderr := run("align", "-root", root, "-date", "2026-10-25", "wikidata"); code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	code, stdout, stderr = run("align", "queue", "-root", root, "-system", "wikidata")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	id, _, _ := strings.Cut(stdout, " ")
	if !strings.Contains(stdout, "G5S1M9 same_as wikidata:Q2") || len(id) != 12 {
		t.Fatalf("the queue is:\n%s", stdout)
	}
	if _, stdout, _ := run("align", "queue", "-root", root, "-json"); !strings.Contains(stdout, `"id":"`+id+`"`) {
		t.Errorf("the queue in JSON is:\n%s", stdout)
	}

	if code, _, stderr := run("align", "accept", "-root", root, "-by", "agent:align", id); code != ExitFailure || stderr == "" {
		t.Errorf("an agent accepts a proposal: exit code %d, stderr %q", code, stderr)
	}
	code, stdout, stderr = run("align", "accept", "-root", root, "-by", "github:curator", "-date", "2026-10-26", id)
	if code != ExitOK || !strings.Contains(stdout, "accepted "+id) {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "nodes/G5S/G5S1M9.md")); !strings.Contains(string(b), "same_as:\n  wikidata: Q2\n") {
		t.Errorf("the node is:\n%s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "registers/crosswalk.reg")); !strings.Contains(string(b), "G5S1M9 same_as wikidata Q2 accepted github:curator 2026-10-26 blake3:") {
		t.Errorf("the register is:\n%s", b)
	}
	if code, _, _ := run("align", "confirm", "-root", root, "-by", "github:curator", "G5S1M9", "wikidata"); code != ExitOK {
		t.Errorf("confirm gives the exit code %d", code)
	}
	if code, stdout, _ := run("align", "queue", "-root", root); code != ExitOK || stdout != "" {
		t.Errorf("the queue after the decision is %q", stdout)
	}
}

func TestAlignUsage(t *testing.T) {
	root := alignTree(t)
	for name, args := range map[string][]string{
		"no source":    {"align"},
		"two sources":  {"align", "wikidata", "oeis"},
		"bad flag":     {"align", "-zz"},
		"no id":        {"align", "accept", "-by", "github:curator"},
		"one argument": {"align", "confirm", "-by", "github:curator", "G5S1M9"},
		"queue args":   {"align", "queue", "x"},
	} {
		if code, _, _ := run(args...); code != ExitUsage {
			t.Errorf("%s: exit code %d, want %d", name, code, ExitUsage)
		}
	}
	if code, _, stderr := run("align", "-root", root, "oeis"); code != ExitFailure || !strings.Contains(stderr, "m2 ingest oeis") {
		t.Errorf("a source with no records: exit code %d, stderr %q", code, stderr)
	}
}
