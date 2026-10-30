package arxiv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource/cluster"
)

func TestNotes(t *testing.T) {
	for _, c := range []struct {
		title, comments string
		categories      []string
		rule            string
	}{
		{"Lecture Notes on Gradient Flows and Optimal Transport", "", []string{"math.AP"}, "lecture-notes"},
		{"Conformal Field Theory", "Lectures notes, 66 pages", []string{"math-ph"}, "lecture-notes"},
		{"Lectures on Differential Topology", "416 pages", []string{"math.GT"}, "title-lectures"},
		{"Edinburgh Lectures on Geometry, Analysis and Physics", "", []string{"math.AG"}, "title-lectures"},
		{"Representations of Affine Lie Algebras", "Notes for the summer school on Geometric representation theory, 2009", []string{"math.RT"}, "notes-for-course"},
		{"A lecture on Invariant Random Subgroups", "Based on a mini course given at Oberwolfach", []string{"math.GR"}, "minicourse"},
		{"Lectures on Quantum Field Theory", "Lecture course given by R. E. Borcherds, Fall 2001", []string{"math-ph"}, "title-lectures"},
		{"Classical and quantum polyhedra", "Lectures given at Karpacz winter school (February 2001)", []string{"math.QA"}, "lectures-given"},
		{"Polyhedra", "Contribution to the summer school on polyhedra, with a tutorial", []string{"math.CO"}, "school"},
		{"Calculus for engineers", "Notes written for my students in the first semester", []string{"math.HO"}, "math-ho"},
		{"Handouts on calculus", "For undergraduates", []string{"math.HO"}, "math-ho"},
		{"Induced Subgraphs", "Will appear in the proceedings of WG 2005 (Lecture Notes in Computer Science, Springer)", []string{"math.CO"}, ""},
		{"Frontiers of open quantum system dynamics", `Contribution to "Quantum Physics and Geometry", Lecture Notes of the Unione Matematica Italiana 25`, []string{"math-ph"}, ""},
		{"Lie Methods in Growth of Groups", "to appear in volume 275 of the London Mathematical Society Lecture Notes series", []string{"math.GR"}, ""},
		{"Accardi contra Bell", "IMS Lecture Notes-Monograph series", []string{"math.PR"}, ""},
		{"Notes on the multiplicity conjecture", "", []string{"math.AC"}, ""},
		{"Stochastic SIR epidemics with households and schools", "", []string{"math.PR"}, ""},
		{"Firoozbakht conjecture", "6 pages. Endnotes added", []string{"math.NT"}, ""},
		{"Male Under-performance in Undergraduate Mathematical Courses", "", []string{"math.HO"}, ""},
		{"Handouts on calculus", "For undergraduates", []string{"math.CA"}, ""},
		{"Combinatorics of Alternating Tangles", "proceedings European Summer School St-Petersburg 2001", []string{"math-ph"}, ""},
	} {
		rule, ok := Notes(c.title, c.comments, c.categories)
		if rule != c.rule || ok != (c.rule != "") {
			t.Errorf("Notes(%q, %q) = %q, %v, want %q", c.title, c.comments, rule, ok, c.rule)
		}
	}
}

func TestMeasure(t *testing.T) {
	sample := []Labelled{
		{ID: "a", Title: "Lecture notes on sheaves", Notes: true},
		{ID: "b", Title: "Lectures on sheaves", Notes: false},
		{ID: "c", Title: "Sheaves", Notes: true},
		{ID: "d", Title: "Sheaves", Notes: false},
		{ID: "e", Title: "Sheaves", Comments: "Notes of a minicourse", Notes: true},
	}
	s := Measure(sample)
	if s.Rules != NotesRules || s.TP != 2 || s.FP != 1 || s.FN != 1 || s.TN != 1 {
		t.Fatalf("score = %+v", s)
	}
	if s.Precision != 2.0/3 || s.Recall != 2.0/3 {
		t.Errorf("precision = %v, recall = %v, want 2/3 and 2/3", s.Precision, s.Recall)
	}
	if strings.Join(s.Wrong, " ") != "b c" {
		t.Errorf("wrong = %v, want b and c", s.Wrong)
	}
	if s := Measure(nil); s.Precision != 0 || s.Recall != 0 {
		t.Errorf("empty sample: %+v", s)
	}
}

// TestSample measures the rule set on the labelled sample of testdata. The sample has 94 papers of the metadata plane of tamnd/arxiv, from twelve months, labelled by a person. A change to the rules that lowers the score fails here, and then the floors change only with a new version of NotesRules.
func TestSample(t *testing.T) {
	sample, err := ReadLabelled("testdata/notes-sample.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(sample) != 94 {
		t.Fatalf("the sample has %d papers, want 94", len(sample))
	}
	s := Measure(sample)
	t.Logf("%s: precision %.2f, recall %.2f, TP %d, FP %d, FN %d, TN %d, wrong %v", s.Rules, s.Precision, s.Recall, s.TP, s.FP, s.FN, s.TN, s.Wrong)
	if s.Precision < 0.95 || s.Recall < 0.75 {
		t.Errorf("precision %.2f and recall %.2f are under the floors 0.95 and 0.75", s.Precision, s.Recall)
	}
}

func TestReadLabelled(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(bad, []byte("---\nrecords:\n    - id: \"1\"\n      title: T\n      label: true\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLabelled(bad); err == nil {
		t.Error("a record with an unknown field gives no error")
	}
	empty := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(empty, []byte("---\nrecords:\n    - title: T\n      notes: true\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLabelled(empty); err == nil {
		t.Error("a record with no id gives no error")
	}
}

func TestRecordsNotes(t *testing.T) {
	in := `{"id": "1009.0843", "title": "Lecture Notes on Quantum Brownian Motion", "categories": ["math-ph"], "comments": "92 pages, 28 figures"}
{"id": "0505.0001", "title": "Induced subgraphs", "categories": ["math.CO"], "comments": "Lecture Notes in Computer Science"}
`
	var rs []cluster.Record
	if err := (Adapter{}).Records(strings.NewReader(in), func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Kind != "lecture-notes" || rs[1].Kind != "preprint" {
		t.Errorf("records = %+v", rs)
	}
}
