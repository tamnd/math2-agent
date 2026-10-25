package mathlib_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/mathlib"
	"github.com/tamnd/math2-agent/lean"
)

func TestRead(t *testing.T) {
	f, err := os.Open("testdata/snapshot.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	src := mathlib.Adapter{}.Source()
	recs, err := ingest.Import(mathlib.Adapter{}, &src, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("the snapshot gives %d records, want 3", len(recs))
	}
	for _, r := range recs {
		if r.Class != ingest.Open || r.Meta["type_hash"] == "" || r.Locator == "" || !strings.HasPrefix(r.URL, mathlib.Docs+"Mathlib/") {
			t.Errorf("the record %s is %+v", r.ID, r)
		}
	}

	short := `{"library":"Mathlib","lean":"4.35.0","total":2}` + "\n" + `{"decl":"Nat.foo","kind":"theorem","module":"Mathlib.Foo","type_hash":"blake3:00"}` + "\n"
	if _, err := ingest.Import(mathlib.Adapter{}, &src, strings.NewReader(short)); err == nil || !strings.Contains(err.Error(), "has 1 declarations, and its header gives 2") {
		t.Errorf("a short snapshot gives %v", err)
	}
	if _, err := ingest.Import(mathlib.Adapter{}, &src, strings.NewReader("{}\n")); err == nil {
		t.Error("a snapshot with no library in the header gives no error")
	}
}

func TestRecord(t *testing.T) {
	r := mathlib.Record(lean.Decl{Decl: "Nat.Prime.two_le", Kind: "theorem", Module: "Mathlib.Data.Nat.Prime.Defs", TypeHash: "blake3:ab", Uses: []string{"Nat.Prime", "Nat.le"}, Doc: "A prime is at least two."})
	if r.URL != "https://leanprover-community.github.io/mathlib4_docs/Mathlib/Data/Nat/Prime/Defs.html#Nat.Prime.two_le" {
		t.Errorf("the URL is %s", r.URL)
	}
	if r.Meta["uses"] != "Nat.Prime Nat.le" || r.Text != "A prime is at least two." || r.Locator != "Mathlib.Data.Nat.Prime.Defs" {
		t.Errorf("the record is %+v", r)
	}
}

func TestCommit(t *testing.T) {
	dir := t.TempDir()
	manifest := `{"version": "1.3.0", "packages": [{"name": "plausible", "rev": "dfa4f38"}, {"name": "mathlib", "rev": "56a0058"}]}`
	if err := os.WriteFile(filepath.Join(dir, "lake-manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, err := mathlib.Commit(dir); err != nil || c != "56a0058" {
		t.Errorf("Commit gives %q, %v", c, err)
	}
	if c, err := mathlib.Commit(filepath.Join("..", "..", "lean")); err != nil || c != "" {
		t.Errorf("a project without Mathlib gives %q, %v", c, err)
	}
}

func TestSource(t *testing.T) {
	s := mathlib.Adapter{}.Source()
	if p := s.Problems(); len(p) > 0 {
		t.Errorf("the catalogue entry has problems: %v", p)
	}
	if a, ok := ingest.Lookup("mathlib"); !ok || a.Version() != "mathlib/1" {
		t.Error("the adapter is not registered")
	}
}
