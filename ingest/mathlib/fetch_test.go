package mathlib

import (
	"bytes"
	"compress/gzip"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
)

// TestFetch runs fetch on the test fixtures of the lean package, with the root Fixture in place of Mathlib. It needs lake on the PATH.
func TestFetch(t *testing.T) {
	if _, err := exec.LookPath("lake"); err != nil {
		t.Skip("lake is not on the PATH")
	}
	project, err := filepath.Abs(filepath.Join("..", "..", "lean"))
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command("lake", "build", "Fixture")
	build.Dir = project
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("lake build: %v\n%s", err, out)
	}
	var buf bytes.Buffer
	if err := fetch(context.Background(), project, t.TempDir(), "Fixture", "abc", &buf); err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var recs []ingest.Record
	if err := (Adapter{}).Read(zr, func(r ingest.Record) error {
		recs = append(recs, r)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(recs) != 18 {
		t.Fatalf("the snapshot has %d records, want 18", len(recs))
	}
	by := map[string]ingest.Record{}
	for _, r := range recs {
		by[r.ID] = r
	}
	a, b := by["Fixture.add_comm_a"], by["Fixture.add_comm_b"]
	if a.Kind != "theorem" || a.Locator != "Fixture" || a.URL != Docs+"Fixture.html#Fixture.add_comm_a" || !strings.Contains(a.Text, "other binder names") {
		t.Errorf("the record of add_comm_a is %+v", a)
	}
	if a.Meta["type_hash"] == "" || a.Meta["type_hash"] != b.Meta["type_hash"] {
		t.Errorf("the type hashes are %q and %q", a.Meta["type_hash"], b.Meta["type_hash"])
	}
	if !strings.Contains(by["Fixture.uses_a"].Meta["uses"], "Fixture.add_comm_a") {
		t.Errorf("uses_a uses %q", by["Fixture.uses_a"].Meta["uses"])
	}
}
