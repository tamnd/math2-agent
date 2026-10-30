package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cat := func(ids ...string) []resource.CatalogueRecord {
		var out []resource.CatalogueRecord
		for _, id := range ids {
			out = append(out, resource.CatalogueRecord{Source: "zbmath", Snapshot: "s1", NativeID: id, Level: "work"})
		}
		return out
	}
	rs := []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "monograph", Title: "A", Scope: &resource.Scope{Value: resource.In, By: "b"}, Catalogue: cat("1", "2")},
		{ID: "8F2BBBBBBT", Kind: "monograph", Title: "B", Scope: &resource.Scope{Value: resource.Deferred, Until: "2026-01-01", Reason: "no class", By: "b"}, Catalogue: cat("3")},
		{ID: "8F2CCCCCCH", Kind: "monograph", Title: "C", Scope: &resource.Scope{Value: resource.Duplicate, Target: "8F2AAAAAA3", By: "b"}},
		{ID: "8F2DDDDDD8", Kind: "monograph", Title: "D", Catalogue: cat("4")},
	}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	var recs []cluster.Record
	for _, id := range []string{"1", "2", "3", "4"} {
		recs = append(recs, cluster.Record{Source: "zbmath", Snapshot: "s1", NativeID: id, Level: "work", Kind: "monograph", Title: "T" + id})
	}
	if err := cluster.WriteRecords(root, "zbmath", "records", recs); err != nil {
		t.Fatal(err)
	}
	if err := cluster.WriteRejects(root, "zbmath", []cluster.Reject{{NativeID: "5", Snapshot: "s1", Reason: "no title"}}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, filepath.FromSlash(resource.CurriculaDir))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	data := "curriculum: a\ntexts:\n  - {name: A, resource: 8F2AAAAAA3, kind: book}\n  - {name: C, resource: 8F2CCCCCCH, kind: book}\n  - {name: Z, resource: 8F2ZZZZZZ3, kind: book}\n  - {name: N, kind: notes}\n"
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestBuild(t *testing.T) {
	root := tree(t)
	rep, err := Build(root, "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Resources != 3 {
		t.Errorf("live resources = %d, want 3", rep.Resources)
	}
	if len(rep.Coverage) != 1 {
		t.Fatalf("coverage = %+v", rep.Coverage)
	}
	// Records 1, 2 and 3 map to a resource with an outcome. Record 4 maps to a resource with no outcome, and record 5 is rejected.
	if c := rep.Coverage[0]; c.Read != 5 || c.Rejected != 1 || c.Mapped != 3 || c.Coverage != 0.6 || c.Reasons["no title"] != 1 || len(c.Missing) != 1 || c.Missing[0] != "4" {
		t.Errorf("coverage = %+v", c)
	}
	if rep.Open.Total != 1 || rep.Open.ByReason["no class"] != 1 {
		t.Errorf("open = %+v", rep.Open)
	}
	if rep.Closure.Texts != 4 || len(rep.Closure.Missing) != 3 {
		t.Fatalf("closure = %+v", rep.Closure)
	}
	for i, want := range []string{"duplicate of 8F2AAAAAA3", "not in the catalogue", "no resource id"} {
		if !strings.Contains(rep.Closure.Missing[i].Reason, want) {
			t.Errorf("missing %d = %+v, want %q", i, rep.Closure.Missing[i], want)
		}
	}
	if early, _ := Build(root, "2025-12-31"); early.Open.Total != 0 {
		t.Errorf("a deferral before its date is open: %+v", early.Open)
	}
}

func TestWrite(t *testing.T) {
	root := tree(t)
	rep, err := Build(root, "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if err := rep.Write(root); err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(filepath.Join(root, ReportFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## 2. Coverage by source", "| zbmath | 5 | 1 | 3 | 60.00% |", "rejected, no title: 1", "## 3. Open decisions", "| no class | 1 |", "## 6. Curriculum closure", "4 texts, and 3 of them", "## Courses (RS5.8)", "| 4. estimate of the courses of the I4 institutions | 0 (0 to 0) |"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("the report has no %q:\n%s", want, md)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, JSONFile))
	if err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(data, &back); err != nil || back.Coverage[0].Mapped != 3 {
		t.Errorf("the JSON report = %v, %+v", err, back)
	}
}

func TestBuildEmpty(t *testing.T) {
	rep, err := Build(t.TempDir(), "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Resources != 0 || len(rep.Coverage) != 0 || rep.Closure.Texts != 0 {
		t.Errorf("report of an empty tree = %+v", rep)
	}
	if !strings.Contains(string(rep.Markdown()), "no source records") {
		t.Error("the report of an empty tree does not say that it has no source records")
	}
}
