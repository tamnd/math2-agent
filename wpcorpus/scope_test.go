package wpcorpus

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeScopeStore writes the fixture store and gives its path.
func writeScopeStore(t *testing.T, extra string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scope.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	fixture, err := os.ReadFile(filepath.Join("testdata", "scope.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(fixture) + extra); err != nil {
		t.Fatal(err)
	}
	return path
}

const testCategories = `version: 1
roots:
  - {title: "Category:Mathematics", depth: 2}
  - {title: Group theory, depth: 1}
exclude:
  - Mathematics education
exclude_words:
  - cuisine
keywords:
  - mathemat
  - group
`

func writeCategories(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ManifestDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "categories.yaml"), []byte(testCategories), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestTree(t *testing.T) {
	tree, err := LoadCategoryTree(writeCategories(t))
	if err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(writeScopeStore(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	cats, err := st.Tree(tree)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range cats {
		got = append(got, c.Title+":"+string(rune('0'+c.Depth)))
	}
	// Group_theory is at depth 1 from Mathematics and at depth 0 as a root.
	// Finite_groups is at depth 1 below the root Group_theory, and its
	// subcategory is past the depth of both roots.
	want := "Algebra:1 Finite_groups:1 Group_theory:0 Mathematics:0"
	if s := strings.Join(got, " "); s != want {
		t.Errorf("the tree is %s, want %s", s, want)
	}
	if !tree.MathKeyword("Finite_groups") || tree.MathKeyword("French_cuisine") {
		t.Error("MathKeyword")
	}
	if !tree.Excluded("Mathematics_education") || !tree.Excluded("French_cuisine") || tree.Excluded("Algebra") {
		t.Error("Excluded")
	}
}

func TestResolveAndArticles(t *testing.T) {
	st, err := OpenStore(writeScopeStore(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ids, err := st.Resolve([]string{"Groups", "Sylow theorems", "No such page"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids["Groups"] != 1 || ids["Sylow_theorems"] != 5 {
		t.Errorf("resolve: %v", ids)
	}
	arts, err := st.Articles([]int64{1, 3, 4, 5, 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 2 || arts[1] != "Group_(mathematics)" || arts[5] != "Sylow_theorems" {
		t.Errorf("articles: %v", arts)
	}
	if _, err := OpenStore(filepath.Join(t.TempDir(), "none.sqlite")); err == nil {
		t.Error("OpenStore made a new store")
	}
}

func TestSignals(t *testing.T) {
	s := S1 | S6 | S9
	if s.String() != "S1,S6,S9" {
		t.Errorf("String: %s", s)
	}
	if p, err := ParseSignals("S1,S6,S9"); err != nil || p != s {
		t.Errorf("ParseSignals: %v, %v", p, err)
	}
	for _, bad := range []string{"S0", "S10", "X1", "S"} {
		if _, err := ParseSignals(bad); err == nil {
			t.Errorf("ParseSignals(%q) has no error", bad)
		}
	}
	if ProjectSignal("Mathematics") != S1 || ProjectSignal("Cryptography") != S2 || ProjectSignal("Physics") != S3 || ProjectSignal("Chemistry") != 0 {
		t.Error("ProjectSignal")
	}
	if !ListArticle("List of group theory topics") || !ListArticle("Timeline_of_calculus") || ListArticle("Listing") {
		t.Error("ListArticle")
	}
}

func TestClassify(t *testing.T) {
	for _, c := range []struct {
		sig     Signals
		depth   int
		verdict string
		in      bool
	}{
		{S1, -1, VerdictMath, true},
		{S4, -1, VerdictMath, true},
		{S6, 0, VerdictMath, true},
		{S6, 1, VerdictMath, true},
		{S3 | S6, 1, VerdictMath, true},
		{S3 | S6, 3, VerdictPending, false},
		{S5, -1, VerdictPending, false},
		{S6, 2, VerdictPending, false},
		{S8 | S9, -1, VerdictPending, false},
		{S7, -1, VerdictPending, false},
		{S3, -1, VerdictUsesMath, false},
		{0, -1, VerdictNotMath, false},
	} {
		cand := &Candidate{Signals: c.sig, CatDepth: c.depth}
		Classify(cand)
		if cand.Verdict != c.verdict || cand.InScope != c.in {
			t.Errorf("%s depth %d: %s %v, want %s %v", c.sig, c.depth, cand.Verdict, cand.InScope, c.verdict, c.in)
		}
	}
}

func TestVerdicts(t *testing.T) {
	in := verdictsHeader + "\n5\tSylow_theorems\tmath\t0.85\tgpt\ta theorem\n6\tCoset\tnot-math\t0.90\tgpt\t\n"
	vs, err := ReadVerdicts(strings.NewReader(in))
	if err != nil || len(vs) != 2 || vs[5].Confidence != 0.85 {
		t.Fatalf("verdicts %v, %v", vs, err)
	}
	c := &Candidate{PageID: 5, Signals: S5, CatDepth: -1}
	Classify(c)
	ApplyVerdict(c, vs[5])
	if c.Verdict != VerdictMath || !c.InScope || c.Rule != "model" {
		t.Errorf("pending candidate: %+v", c)
	}
	s := &Candidate{PageID: 6, Signals: S1, CatDepth: -1}
	Classify(s)
	ApplyVerdict(s, vs[6])
	if !s.InScope || s.Rule != "strong" {
		t.Errorf("the model changed a verdict of the rules: %+v", s)
	}
	for _, bad := range []string{"x\n", verdictsHeader + "\n1\tA\tmaybe\t0.5\tgpt\t\n", verdictsHeader + "\n1\tA\tmath\n"} {
		if _, err := ReadVerdicts(strings.NewReader(bad)); err == nil {
			t.Errorf("ReadVerdicts(%q) has no error", bad)
		}
	}
}

func TestCandidatesRoundTrip(t *testing.T) {
	cs := []*Candidate{
		{PageID: 5, Title: "Sylow_theorems", Signals: S6, CatDepth: 3, Links: 0},
		{PageID: 1, Title: "Group_(mathematics)", Signals: S1 | S6, Projects: []string{"Mathematics"}, Class: "B", Importance: "Top", CatDepth: 1, Links: 7},
	}
	for _, c := range cs {
		Classify(c)
	}
	var b bytes.Buffer
	if err := WriteCandidates(&b, cs, "20261001", false); err != nil {
		t.Fatal(err)
	}
	back, err := ReadCandidates(&b)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 || back[0].PageID != 1 || back[0].Signals != S1|S6 || back[0].Links != 7 || !back[0].InScope || back[1].InScope || back[1].CatDepth != 3 {
		t.Errorf("round trip: %+v %+v", back[0], back[1])
	}
	b.Reset()
	if err := WriteCandidates(&b, cs, "20261001", true); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(b.String(), "\n"); n != 2 {
		t.Errorf("scope.tsv has %d lines, want 2", n)
	}
	b.Reset()
	if err := ScopeReport(&b, cs, "20261001", []string{"S7"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"It has 2 candidates, and 1 of them are in scope", "| S6 | 2 | 1 | 0 |", "| S1 | 1 | 1 | 0 |", "| weak | 1 | 0 |", "The rules do not decide 1 candidates", "not computed yet: S7"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("the report has no %q:\n%s", want, b.String())
		}
	}
}

func TestReadAssess(t *testing.T) {
	in := "pageid\tns\ttitle\tproject\tclass\timportance\n1\t0\tGroup (mathematics)\tMathematics\tB\tTop\n9\t14\tCategory:Group theory\tMathematics\tCategory\tNA\n"
	rows, err := ReadAssess(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Title != "Group_(mathematics)" || rows[0].Class != "B" {
		t.Errorf("rows %+v", rows)
	}
	if _, err := ReadAssess(strings.NewReader("1\t0\tx\n")); err == nil {
		t.Error("a short row has no error")
	}
}
