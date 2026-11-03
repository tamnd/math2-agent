package axrecords_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/importer/arxiv/axrecords"
	"github.com/tamnd/math2-agent/inventory"
)

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func read(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// tree is a synthetic math2 tree with a textbook row, two research rows, the mentions of arXiv and of a book, and a record of understand.
func tree(t *testing.T) string {
	root := t.TempDir()
	write(t, root, map[string]string{
		"records/mentions/arxiv/AAA.jsonl":             `{"id":"ax://paper/2401.00001#R1","source":"arxiv","kind":"statement"}` + "\n",
		"records/mentions/book/BBB.jsonl":              `{"id":"book://x#1","source":"book","kind":"definition"}` + "\n",
		"records/understand/arxiv/classify/AAA.jsonl":  `{"id":"ax://paper/2401.00001#R1","pass":"classify"}` + "\n",
		"records/understand/arxiv/prose/sub/AAA.jsonl": `{"id":"ax://paper/2401.00001#S1","pass":"prose"}` + "\n",
		"records/gold/unfolding.jsonl":                 `{"item":"ax://paper/2401.00001#R1"}` + "\n",
		"records/inventory/100.jsonl":                  `{"id":"100AAAAAAA","mentions":["book://x#1"],"kind":"definition","layer":"textbook","spread":1}` + "\n",
		"records/inventory/200.jsonl":                  `{"id":"200AAAAAAA","mentions":["ax://paper/2401.00001#R1"],"kind":"statement","layer":"research","spread":1}` + "\n",
		"records/inventory/300.jsonl":                  `{"id":"300AAAAAAA","mentions":["ax://paper/2401.00002#R1"],"kind":"statement","layer":"research","spread":1}` + "\n",
	})
	return root
}

func TestExportKeepUnpack(t *testing.T) {
	root, out := tree(t), t.TempDir()
	rows0, err := inventory.ReadRows(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := axrecords.Keep(root); err == nil {
		t.Error("Keep runs on a tree with no pin")
	}
	shards, err := axrecords.Export(root, out)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range shards {
		names = append(names, s.File)
	}
	want := "records/arxiv/mentions/AAA.jsonl.zst records/arxiv/rows/200.jsonl.zst records/arxiv/rows/300.jsonl.zst records/arxiv/understand/classify/AAA.jsonl.zst records/arxiv/understand/prose/sub/AAA.jsonl.zst"
	if strings.Join(names, " ") != want {
		t.Errorf("the shards are %s", strings.Join(names, " "))
	}
	if _, err := axrecords.WritePin(root, out, "0123abcd"); err != nil {
		t.Fatal(err)
	}
	files, rows, err := axrecords.Keep(root)
	if err != nil || files != 3 || rows != 2 {
		t.Fatalf("Keep gives %d files, %d rows, %v", files, rows, err)
	}
	if p := inventory.Packed(root); len(p) != 1 || p[0] != "arxiv" {
		t.Errorf("Packed gives %v", p)
	}
	for _, gone := range []string{"records/mentions/arxiv", "records/understand/arxiv", "records/inventory/200.jsonl"} {
		if _, err := os.Stat(filepath.Join(root, gone)); err == nil {
			t.Errorf("%s is in the tree after Keep", gone)
		}
	}
	if read(t, root, "records/gold/unfolding.jsonl") == "" || read(t, root, "records/mentions/book/BBB.jsonl") == "" {
		t.Error("Keep removes what is not an arXiv record")
	}
	if err := inventory.WriteRows(root, rows0); err == nil {
		t.Error("WriteRows writes a packed tree")
	}
	if _, err := axrecords.Export(root, out); err == nil {
		t.Error("Export runs on a packed tree")
	}
	// A records tree at another commit has another index.
	other := t.TempDir()
	write(t, other, map[string]string{"records/arxiv/index.md": "# other\n"})
	if _, _, err := axrecords.Unpack(root, other); err == nil {
		t.Error("Unpack takes an index that the pin does not give")
	}
	files, rows, err = axrecords.Unpack(root, out)
	if err != nil || files != 3 || rows != 2 {
		t.Fatalf("Unpack gives %d files, %d rows, %v", files, rows, err)
	}
	rows1, err := inventory.ReadRows(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows1) != 3 || len(inventory.Packed(root)) != 0 {
		t.Errorf("after Unpack the tree has %d rows and Packed gives %v", len(rows1), inventory.Packed(root))
	}
	if got := read(t, root, "records/understand/arxiv/prose/sub/AAA.jsonl"); !strings.Contains(got, "S1") {
		t.Errorf("the prose record is %q", got)
	}
	if files, rows, err := axrecords.Unpack(root, out); err != nil || files != 0 || rows != 0 {
		t.Errorf("a second Unpack gives %d, %d, %v", files, rows, err)
	}
	if err := inventory.WriteRows(root, rows1); err != nil {
		t.Errorf("WriteRows refuses an unpacked tree: %v", err)
	}
	// An export after a change drops the shards that the tree no longer has.
	if err := os.Remove(filepath.Join(root, "records/understand/arxiv/classify/AAA.jsonl")); err != nil {
		t.Fatal(err)
	}
	if shards, err = axrecords.Export(root, out); err != nil || len(shards) != 4 {
		t.Fatalf("the second export gives %d shards, %v", len(shards), err)
	}
	if _, err := os.Stat(filepath.Join(out, "records/arxiv/understand/classify/AAA.jsonl.zst")); err == nil {
		t.Error("the second export keeps a shard that the tree no longer has")
	}
}
