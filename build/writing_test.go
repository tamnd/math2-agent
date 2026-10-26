package build_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/card"
)

// TestWriting checks that each node gets a writing level, and that a row of the inventory with the outcome node gives the node its row.
func TestWriting(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "build")
	if _, err := build.Run(ctx, build.Options{Root: root, Only: build.Writing}); err != nil {
		t.Fatal(err)
	}
	read := func() build.Levels {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(out, build.WritingFile))
		if err != nil {
			t.Fatal(err)
		}
		var w build.Levels
		if err := json.Unmarshal(data, &w); err != nil {
			t.Fatal(err)
		}
		return w
	}
	w := read()
	if len(w.Nodes) == 0 {
		t.Fatalf("writing.json has %d nodes", len(w.Nodes))
	}
	total := 0
	for _, n := range w.Levels {
		total += n
	}
	if total != len(w.Nodes) || w.Cards != "embedded" {
		t.Errorf("the levels are %v from %s, for %d nodes", w.Levels, w.Cards, len(w.Nodes))
	}
	first := w.Nodes[0]
	if first.Level != card.W0 || len(first.Next) == 0 || first.Next[0].What != "an inventory row" {
		t.Fatalf("the node %s is %s with %+v, and it has no row", first.Tag, first.Level, first.Next)
	}

	row := `{"id":"9DK2RX7Q4H","mentions":[],"kind":"concept","layer":"textbook","spread":1,"outcome":{"value":"node","target":["` + first.Tag + `"]}}` + "\n"
	if err := os.MkdirAll(filepath.Join(root, "records", "inventory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "records", "inventory", "9DK.jsonl"), []byte(row), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := build.Run(ctx, build.Options{Root: root, Only: build.Writing})
	if err != nil {
		t.Fatal(err)
	}
	if again.Steps[len(again.Steps)-1].Cached {
		t.Error("a new row does not run the writing step again")
	}
	for _, r := range read().Nodes {
		if r.Tag == first.Tag && len(r.Next) > 0 && r.Next[0].What == "an inventory row" {
			t.Errorf("the node %s has the row 9DK2RX7Q4H, and its work list is %+v", r.Tag, r.Next)
		}
	}
}
