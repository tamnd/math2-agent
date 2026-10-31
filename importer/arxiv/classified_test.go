package arxiv_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/math2-agent/importer/arxiv"
)

// unknownObjects are two objects of version 2 that neither ax nor the titles classify.
const unknownObjects = `{"tag":"UNK1","kind":"unknown","paper":"2401.00001","version":"2","local":"bfhpg-1","number":"2.2","title":"","env":"bfhpg","path":"source","item":"unknown","scope":"paper"}
{"tag":"UNK2","kind":"unknown","paper":"2401.00001","version":"2","local":"zzq-1","number":"2.3","title":"","env":"zzq","path":"source","item":"unknown","scope":"paper"}
`

func TestAClassificationGivesTheKind(t *testing.T) {
	root := checkout(t, "cc-by")
	f := filepath.Join(root, "analysis", "objects", "2401", "2401.00001.jsonl")
	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, append(data, unknownObjects...), 0o644); err != nil {
		t.Fatal(err)
	}
	m2 := t.TempDir()
	dir := filepath.Join(m2, filepath.FromSlash(arxiv.ClassifyDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// UNK1 is a theorem, UNK2 a remark. The classification of LEM1 does not change a kind that code gives, and one in the queue classify-review waits.
	recs := `{"id":"ax://paper/2401.00001#UNK1","pass":"classify","kind":"statement/lemma","confidence":0.5,"queue":"classify-review"}
{"id":"ax://paper/2401.00001#UNK1","pass":"classify","kind":"statement/theorem","confidence":0.9}
{"id":"ax://paper/2401.00001#UNK2","pass":"classify","kind":"remark","confidence":0.8}
{"id":"ax://paper/2401.00001#LEM1","pass":"classify","kind":"remark","confidence":0.8}
{"id":"ax://paper/2401.00001#PRB1","pass":"classify","kind":"exercise","confidence":0.9,"queue":"classify-review"}
`
	if err := os.WriteFile(filepath.Join(dir, "00.jsonl"), []byte(recs), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := arxiv.ReadClassified(m2)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 3 || c["ax://paper/2401.00001#UNK1"] != "statement/theorem" {
		t.Fatalf("the classifications are %v", c)
	}

	plain, err := arxiv.Import(root)
	if err != nil {
		t.Fatal(err)
	}
	if ms := byID(plain); ms["UNK1"].Kind != "unknown" || ms["UNK2"].Kind != "unknown" {
		t.Errorf("with no classifications the kinds are %q and %q, want unknown", ms["UNK1"].Kind, ms["UNK2"].Kind)
	}
	res, err := arxiv.ImportNames(root, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	ms := byID(res)
	if m := ms["UNK1"]; m.Kind != "statement" || m.Role != "theorem" {
		t.Errorf("the classified theorem has the kind %q and the role %q", m.Kind, m.Role)
	}
	if _, ok := ms["UNK2"]; ok {
		t.Error("the classified remark has a mention")
	}
	if ms["LEM1"].Kind != "statement" {
		t.Errorf("the lemma has the kind %q", ms["LEM1"].Kind)
	}
	// Rule D05 expects what the import gives.
	wants, err := arxiv.Expect(root, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wants {
		if w.Mention == "ax://paper/2401.00001#UNK2" {
			t.Error("D05 expects a mention of the classified remark")
		}
	}
	if len(wants) != plainWants(t, root)-1 {
		t.Errorf("D05 expects %d objects, want one less than with no classifications", len(wants))
	}
}

func plainWants(t *testing.T, root string) int {
	t.Helper()
	ws, err := arxiv.Expect(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	return len(ws)
}
