package arxiv_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/inventory"
)

func TestParseLocator(t *testing.T) {
	for _, c := range []struct{ in, word, number string }{
		{"Theorem 4.1", "theorem", "4.1"},
		{"Thm. 4.1", "thm", "4.1"},
		{"Prop 2", "prop", "2"},
		{"Lemma~A.3 (ii)", "lemma", "A.3"},
		{"Theorem B", "theorem", "B"},
		{"4.1", "", "4.1"},
	} {
		w, n, ok := arxiv.ParseLocator(c.in)
		if !ok || w != c.word || n != c.number {
			t.Errorf("ParseLocator(%q) = %q, %q, %v, want %q, %q", c.in, w, n, ok, c.word, c.number)
		}
	}
	for _, in := range []string{"§3", "p. 12", "(3.2)", "Section 2", "Main Theorem", "A", "Theorem 1 and Lemma 2"} {
		if _, _, ok := arxiv.ParseLocator(in); ok {
			t.Errorf("ParseLocator(%q) is ok, want no item", in)
		}
	}
}

func TestWork(t *testing.T) {
	for _, c := range []struct {
		b    arxiv.Bib
		want string
	}{
		{arxiv.Bib{ArXiv: "2401.00001", DOI: "10.1/x"}, "arxiv:2401.00001"},
		{arxiv.Bib{ArXiv: "math.AG/0003117"}, "arxiv:math/0003117"},
		{arxiv.Bib{DOI: "10.1/x"}, "doi:10.1/x"},
		{arxiv.Bib{}, ""},
	} {
		if got := arxiv.Work(c.b); got != c.want {
			t.Errorf("Work(%+v) = %q, want %q", c.b, got, c.want)
		}
	}
}

// TestCitedResults cites the items of paper 2401.00001 of the checkout from a second paper.
func TestCitedResults(t *testing.T) {
	root := checkout(t, "cc-by")
	write := func(name, data string) {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("analysis/objects/2402/2402.00002.jsonl", `{"tag":"T1","kind":"statement","subkind":"theorem","paper":"2402.00002","version":"1","local":"thm-1","number":"1","path":"source","item":"statement/theorem","scope":"paper"}
{"tag":"T2","kind":"statement","subkind":"theorem","paper":"2402.00002","version":"1","local":"thm-2","number":"2","path":"source","item":"statement/theorem","scope":"paper"}
{"tag":"T3","kind":"statement","subkind":"theorem","paper":"2402.00002","version":"1","local":"thm-3","number":"3","path":"source","item":"statement/theorem","scope":"paper"}
{"tag":"T4","kind":"statement","subkind":"theorem","paper":"2402.00002","version":"1","local":"thm-4","number":"4","path":"source","item":"statement/theorem","scope":"paper"}
`)
	write("analysis/edges/2402/2402.00002.jsonl", `{"from":"2402.00002#thm-1","to":"bib:A","predicate":"cites-result","part":"Thm. 1.3","confidence":"high","evidence":{"version":1}}
{"from":"2402.00002#thm-2","to":"bib:A","predicate":"cites-result","part":"1.1","confidence":"high","evidence":{"version":1}}
{"from":"2402.00002#thm-3","to":"bib:A","predicate":"cites-result","part":"Theorem 1.2","confidence":"high","evidence":{"version":1}}
{"from":"2402.00002#thm-4","to":"bib:A","predicate":"cites-result","part":"Section 2","confidence":"high","evidence":{"version":1}}
{"from":"2402.00002#thm-4","to":"bib:B","predicate":"cites-result","part":"Theorem 3","confidence":"high","evidence":{"version":1}}
{"from":"2402.00002#thm-4","to":"bib:C","predicate":"cites-result","part":"Lemma 5","confidence":"high","evidence":{"version":1}}
`)
	write("analysis/bib/2402/2402.00002.jsonl", `{"paper":"2402.00002","version":1,"key":"A","arxiv":"2401.00001","from":"bibitem"}
{"paper":"2402.00002","version":1,"key":"B","doi":"10.1/x","from":"bibitem"}
`)
	for _, args := range [][]string{{"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "c2"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	res, err := arxiv.Import(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]inventory.CitedResult{}
	for _, m := range res.Mentions {
		got[m.ID] = m.CitesResult
	}
	a := "ax://paper/2401.00001#"
	want := map[string][]inventory.CitedResult{
		// The number of the latest version and the kind agree.
		"ax://paper/2402.00002#T1": {{To: a + "THM1", Locator: "Thm. 1.3", Confidence: "high"}},
		// The locator has no kind.
		"ax://paper/2402.00002#T2": {{To: a + "LEM1", Locator: "1.1", Confidence: "medium"}},
		// The number is the number of version 1.
		"ax://paper/2402.00002#T3": {{To: a + "THM1", Locator: "Theorem 1.2", Confidence: "high"}},
		// A section is no item, a DOI is a work, and key C is not in the bibliography.
		"ax://paper/2402.00002#T4": {
			{To: "arxiv:2401.00001", Locator: "Section 2", Confidence: "high"},
			{To: "doi:10.1/x", Locator: "Theorem 3", Confidence: "high"},
			{To: "bib:C", Locator: "Lemma 5", Confidence: "high"},
		},
	}
	for id, w := range want {
		g := got[id]
		if len(g) != len(w) {
			t.Errorf("%s cites %v, want %v", id, g, w)
			continue
		}
		for i := range w {
			if g[i] != w[i] {
				t.Errorf("%s cites %v, want %v", id, g, w)
				break
			}
		}
	}
	if res.CitedMentions != 3 || res.CitedWorks != 2 {
		t.Errorf("cited results: %d to a mention and %d to a work, want 3 and 2", res.CitedMentions, res.CitedWorks)
	}
}
