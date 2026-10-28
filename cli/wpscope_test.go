package cli

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/wpcorpus"
)

const scopeCategories = `version: 1
roots:
  - {title: Mathematics, depth: 2}
  - {title: Group theory, depth: 1}
exclude: [Mathematics education]
keywords: [mathemat, group]
`

const scopeAssess = "pageid\tns\ttitle\tproject\tclass\timportance\n" +
	"1\t0\tGroup (mathematics)\tMathematics\tB\tTop\n" +
	"3\t0\tGroups\tMathematics\tRedirect\tNA\n" +
	"8\t0\tList of group theory topics\tMathematics\tList\tLow\n" +
	"9\t0\tCoset\tMathematics\tC\tMid\n" +
	"10\t0\tAbelian group\tMathematics\tB\tHigh\n" +
	"11\t0\tÉvariste Galois\tMathematics\tB\tTop\n" +
	"6\t0\tMonster group\tPhysics\tC\tLow\n"

// The tables that the scope build asks for, in order.
var scopeLoads = []struct{ table, sql string }{
	{"cat_pages", `CREATE TABLE cat_pages (cl_from INTEGER, cl_target_id INTEGER);
INSERT INTO cat_pages VALUES (5,503),(7,504);`},
	{"s1_links", `CREATE TABLE s1_links (pl_from INTEGER, pl_target_id INTEGER);
INSERT INTO s1_links VALUES (1,600),(9,600),(10,600),(10,600),(8,601),(1,601);`},
	{"link_targets", `CREATE TABLE link_targets (lt_id INTEGER, lt_namespace INTEGER, lt_title TEXT);
INSERT INTO link_targets VALUES (600,0,'Lagrange''s_theorem_(group_theory)'),(601,0,'Monster_group');`},
	{"page_cats", `CREATE TABLE page_cats (cl_from INTEGER, cl_target_id INTEGER);
INSERT INTO page_cats VALUES (2,502),(6,506);`},
}

func TestWPScope(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, wpcorpus.ManifestDir), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, text string) string {
		p := filepath.Join(root, rel)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write(filepath.Join(wpcorpus.ManifestDir, "categories.yaml"), scopeCategories)
	assess := write("assess.tsv", scopeAssess)
	cited := write("cited.txt", "# the titles of math2\nGroups\nNo such page\n")
	fixture, err := os.ReadFile(filepath.Join("..", "wpcorpus", "testdata", "scope.sql"))
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(root, "scope.sqlite")
	db, err := sql.Open("sqlite", store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(string(fixture)); err != nil {
		t.Fatal(err)
	}
	args := []string{"scope", "-corpus", root, "-store", store, "-dump", "20261001", "-assess", assess, "-cited", cited, "-work", filepath.Join(root, "work")}

	// Each run stops at the first missing table, and names its load.
	for _, l := range scopeLoads {
		var out, errb bytes.Buffer
		if code := runWP(args, &out, &errb); code != ExitFailure {
			t.Fatalf("before %s: exit %d, %s", l.table, code, errb.String())
		}
		if !strings.Contains(errb.String(), "the store has no table "+l.table) || !strings.Contains(errb.String(), "wiki dump load --db "+store+" --date 20261001") {
			t.Fatalf("before %s: %s", l.table, errb.String())
		}
		if _, err := db.Exec(l.sql); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := os.ReadFile(filepath.Join(root, "work", "tree-ids.txt"))
	if err != nil || string(ids) != "501\n503\n502\n500\n" {
		t.Errorf("tree-ids.txt is %q, %v", ids, err)
	}
	if ids, err := os.ReadFile(filepath.Join(root, "work", "s1-target-ids.txt")); err != nil || string(ids) != "600\n601\n" {
		t.Errorf("s1-target-ids.txt is %q, %v", ids, err)
	}

	var out, errb bytes.Buffer
	if code := runWP(args, &out, &errb); code != ExitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "8 candidates, 8 in scope") {
		t.Errorf("stdout: %s", out.String())
	}
	f, err := os.Open(filepath.Join(root, wpcorpus.ManifestDir, "scope.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	cs, err := wpcorpus.ReadCandidates(f)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range cs {
		if c.Title == "Group_(mathematics)" && c.Class != "B" {
			t.Errorf("Group_(mathematics) has the class %q from the tag of a redirect, want B", c.Class)
		}
		got[c.Title] = c.Signals.String() + " " + c.Rule
	}
	want := map[string]string{
		"Group_(mathematics)":               "S1,S4 strong",
		"Lagrange's_theorem_(group_theory)": "S9 S9",
		"Sylow_theorems":                    "S6 S6 near",
		"Monster_group":                     "S3,S8 two weak",
		"List_of_group_theory_topics":       "S1 strong",
		"Coset":                             "S1 strong",
		"Abelian_group":                     "S1 strong",
		"Évariste_Galois":                   "S1 strong",
	}
	for title, w := range want {
		if got[title] != w {
			t.Errorf("%s: %q, want %q", title, got[title], w)
		}
	}
	report, err := os.ReadFile(filepath.Join(root, "reports", "scope.md"))
	if err != nil || !strings.Contains(string(report), "It has 8 candidates, and 8 of them are in scope") || !strings.Contains(string(report), "not computed yet: S5, S7") {
		t.Errorf("scope.md: %s, %v", report, err)
	}
}

func TestWPScopeUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runWP([]string{"scope", "-store", "x.sqlite"}, &out, &errb); code != ExitUsage {
		t.Errorf("exit %d", code)
	}
}
