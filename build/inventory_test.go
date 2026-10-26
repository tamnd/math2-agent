package build_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/inventory"
)

// TestInventory checks that the build writes the open count, and that a deferred row is open again when its date passes.
func TestInventory(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	rows := []inventory.Row{
		{ID: "103A5C1YC0", Mentions: []string{"m2://rec/wikidata/Q1#1"}, Kind: "statement", Layer: inventory.Textbook, Spread: 1},
		{ID: "104B6D2ZD1", Mentions: []string{"m2://rec/wikidata/Q2#1"}, Kind: "statement", Layer: inventory.Textbook, Spread: 1, Outcome: &inventory.Outcome{Value: inventory.OutcomeDeferred, Until: "2026-11-01"}},
		{ID: "105C7E3AE2", Mentions: []string{"m2://rec/wikidata/Q3#1"}, Kind: "unknown", Layer: inventory.Textbook, Spread: 1, Outcome: &inventory.Outcome{Value: inventory.OutcomeOutOfScope, Reason: "a person"}},
	}
	if err := inventory.WriteRows(root, rows); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	out := t.TempDir()
	day := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	m, err := build.Run(ctx, build.Options{Root: root, Out: out, Only: build.Inventory, Day: day})
	if err != nil {
		t.Fatal(err)
	}
	if m.Counts.Rows != 3 || m.Counts.Open != 1 {
		t.Errorf("the counts are %+v, and there must be 3 rows and 1 open", m.Counts)
	}
	data, err := os.ReadFile(filepath.Join(out, build.InventoryFile))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Day string `json:"day"`
		inventory.Summary
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Day != "2026-10-26" || got.Open != 1 || got.Outcomes[inventory.OutcomeDeferred] != 1 {
		t.Errorf("%s has %+v", build.InventoryFile, got)
	}

	again, err := build.Run(ctx, build.Options{Root: root, Out: out, Only: build.Inventory, Day: day})
	if err != nil {
		t.Fatal(err)
	}
	if !again.Steps[len(again.Steps)-1].Cached || again.Counts.Open != 1 {
		t.Errorf("the second build is %+v with the counts %+v, and it must be cached with 1 open", again.Steps, again.Counts)
	}

	later, err := build.Run(ctx, build.Options{Root: root, Out: out, Only: build.Inventory, Day: day.AddDate(0, 0, 7)})
	if err != nil {
		t.Fatal(err)
	}
	if later.Steps[len(later.Steps)-1].Cached || later.Counts.Open != 2 {
		t.Errorf("a week later the build is %+v with the counts %+v, and the deferred row must be open", later.Steps, later.Counts)
	}
}
