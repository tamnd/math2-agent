package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteRecordsShards(t *testing.T) {
	root := t.TempDir()
	rs := make([]Record, ShardSize+1)
	for i := range rs {
		rs[i] = Record{Source: "s", NativeID: fmt.Sprintf("%07d", len(rs)-i), Level: "work", Kind: "article", Title: "T"}
	}
	if err := WriteRecords(root, "s", SourceFile, rs); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, RecordsDir, "s")
	names := func() []string {
		es, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range es {
			out = append(out, e.Name())
		}
		return out
	}
	if got := fmt.Sprint(names()); got != "[records-0001.md records-0002.md]" {
		t.Fatalf("files %s", got)
	}
	got, err := ReadRecords(root)
	if err != nil || len(got) != len(rs) || got[0].NativeID != "0000001" || got[len(got)-1].NativeID != fmt.Sprintf("%07d", len(rs)) {
		t.Fatalf("read %d records, %v", len(got), err)
	}
	if err := WriteRecords(root, "s", SourceFile, rs[:2]); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(names()); got != "[records.md]" {
		t.Fatalf("files after a small write %s", got)
	}
}
