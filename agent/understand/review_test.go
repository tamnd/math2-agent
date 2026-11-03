package understand_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent/understand"
)

// reviewTree writes the files to a new tree and returns its root.
func reviewTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const rec = `"ref":"2401.00001v1#x","by":"agent:m2-agent/understand@0.5.0","model":"m","class":"open"`

func unfoldRecord(tag, hash, verdict, queue string) string {
	q := ""
	if queue != "" {
		q = `,"queue":"` + queue + `"`
	}
	return `{"id":"ax://paper/2401.00001#` + tag + `",` + rec + `,"pass":"unfold"` + q + `,"unfold":{"unfolded":"x","hash":"` + hash + `","verdict":{"model":"c","verdict":"` + verdict + `"}}}` + "\n"
}

func TestReviewsAndGold(t *testing.T) {
	root := reviewTree(t, map[string]string{
		"records/understand/arxiv/unfold/AAA.jsonl": unfoldRecord("T1", "h1", "equivalent", "") + unfoldRecord("T2", "h2", "equivalent", "") + unfoldRecord("T3", "h3", "weaker", "unfold-review"),
		"records/understand/arxiv/unfold/BBB.jsonl": unfoldRecord("T4", "h4", "equivalent", "") + unfoldRecord("T5", "h5", "unsure", "unfold-review"),
		"records/understand/arxiv/classify/AAA.jsonl": `{"id":"ax://paper/2401.00001#C1",` + rec + `,"pass":"classify","was":"unknown","kind":"definition"}` + "\n" +
			`{"id":"ax://paper/2401.00001#C2",` + rec + `,"pass":"classify","was":"unknown","kind":"remark"}` + "\n" +
			`{"id":"ax://paper/2401.00001#C3",` + rec + `,"pass":"classify","failed":"model-failed: classify"}` + "\n",
		"records/understand/arxiv/proof/AAA.jsonl": `{"id":"ax://paper/2401.00001#P1",` + rec + `,"pass":"proof","queue":"proof-target","proof":{"steps":[]}}` + "\n",
		understand.GoldUnfoldFile: `{"item":"ax://paper/2401.00001#T1","hash":"h1","verdict":"equivalent","by":"github:a","on":"2026-10-31"}` + "\n" +
			`{"item":"ax://paper/2401.00001#T2","hash":"h2","verdict":"stronger","missing":["H is finite"],"by":"github:a","on":"2026-10-31"}` + "\n" +
			`{"item":"ax://paper/2401.00001#T3","hash":"h3","verdict":"weaker","by":"github:a","on":"2026-10-31"}` + "\n" +
			`{"item":"ax://paper/2401.00001#T5","hash":"h5","verdict":"different","by":"github:a","on":"2026-10-31"}` + "\n" +
			`{"item":"ax://paper/2401.00001#T9","hash":"h9","verdict":"equivalent","by":"github:a","on":"2026-10-31"}` + "\n",
		understand.GoldClassifyFile: `{"item":"ax://paper/2401.00001#C1","kind":"definition","by":"github:a","on":"2026-10-31"}` + "\n" +
			`{"item":"ax://paper/2401.00001#C2","kind":"statement/lemma","by":"github:a","on":"2026-10-31"}` + "\n" +
			`{"item":"ax://paper/2401.00001#C3","kind":"definition","by":"github:a","on":"2026-10-31"}` + "\n",
	})
	qs, err := understand.Reviews(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, q := range qs {
		got = append(got, q.Pass+"/"+q.Name+"="+strings.Join(q.Items, ","))
	}
	want := "classify/model-failed=ax://paper/2401.00001#C3 unfold/unfold-review=ax://paper/2401.00001#T3,ax://paper/2401.00001#T5 proof/proof-target=ax://paper/2401.00001#P1"
	if strings.Join(got, " ") != want {
		t.Errorf("the queues are %s", strings.Join(got, " "))
	}
	gu, err := understand.ReadGoldUnfold(root)
	if err != nil {
		t.Fatal(err)
	}
	ru, err := understand.ReadAll(root, understand.Unfold)
	if err != nil {
		t.Fatal(err)
	}
	s := understand.MeasureUnfold(ru, gu)
	// T1 and T2 passed the check, and people mark only T1 equivalent. T2, T3 and T5 are wrong, and the check rejected T3 and T5.
	if s[0].Matched != 4 || s[0].N != 2 || *s[0].Rate != 0.5 || s[0].Met() || s[1].N != 3 || *s[1].Rate != 2.0/3 {
		t.Errorf("the scores of U are %+v and %+v", s[0], s[1])
	}
	gc, err := understand.ReadGoldClassify(root)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := understand.ReadAll(root, understand.Classify)
	if err != nil {
		t.Fatal(err)
	}
	if c := understand.MeasureClassify(rc, gc); c.Matched != 2 || *c.Rate != 0.5 {
		t.Errorf("the score of C is %+v", c)
	}
}

func TestGoldErrors(t *testing.T) {
	for line, want := range map[string]string{
		`{"item":"a","hash":"h","verdict":"unsure","by":"b","on":"d"}`:            "is not equivalent",
		`{"item":"a","hash":"h","verdict":"equivalent","by":"b"}`:                 "needs item, hash, by and on",
		`{"item":"a","hash":"h","verdict":"weaker","by":"b","on":"d","text":"x"}`: "unknown field",
	} {
		root := reviewTree(t, map[string]string{understand.GoldUnfoldFile: line + "\n"})
		if _, err := understand.ReadGoldUnfold(root); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s gives %v", line, err)
		}
	}
	root := reviewTree(t, map[string]string{understand.GoldClassifyFile: `{"item":"a","kind":"lemma","by":"b","on":"d"}` + "\n"})
	if _, err := understand.ReadGoldClassify(root); err == nil || !strings.Contains(err.Error(), "not one of the kinds") {
		t.Errorf("a kind that classify cannot give gives %v", err)
	}
}

func TestDecisions(t *testing.T) {
	prose := `{"id":"ax://paper/2401.00001#S1",` + rec + `,"pass":"prose","prose":[{"from":1,"to":2,"line":1,"file":"a.tex","kind":"definition","queue":"prose-review"},{"from":4,"to":4,"line":3,"file":"a.tex","kind":"claim","queue":"prose-review"}]}` + "\n"
	files := map[string]string{
		"records/understand/arxiv/unfold/AAA.jsonl": unfoldRecord("T3", "h3", "weaker", "unfold-review") + unfoldRecord("T5", "h5", "unsure", "unfold-review"),
		"records/understand/arxiv/proof/AAA.jsonl": `{"id":"ax://paper/2401.00001#P1",` + rec + `,"pass":"proof","queue":"proof-target","proof":{"steps":[]}}` + "\n" +
			`{"id":"ax://paper/2401.00001#P2",` + rec + `,"pass":"proof","queue":"proof-target","proof":{"steps":[]}}` + "\n",
		"records/understand/arxiv/prose/AAA.jsonl": prose,
		understand.DecisionsFile: `{"record":"ax://paper/2401.00001#P1","queue":"proof-target","decision":"confirm","target":"ax://paper/2401.00001#R1","by":"github:a","on":"2026-11-03"}` + "\n" +
			`{"record":"ax://paper/2401.00001#T3","queue":"unfold-review","hash":"h3","decision":"weaker","by":"github:a","on":"2026-11-03"}` + "\n" +
			`{"record":"ax://paper/2401.00001#T5","queue":"unfold-review","hash":"old","decision":"equivalent","by":"github:a","on":"2026-11-03"}` + "\n" +
			`{"record":"ax://paper/2401.00001#S1","queue":"prose-review","item":"1-2","decision":"accept","by":"github:a","on":"2026-11-03"}` + "\n",
	}
	qs, err := understand.Reviews(reviewTree(t, files))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, q := range qs {
		got = append(got, q.Pass+"/"+q.Name+"="+strings.Join(q.Items, ","))
	}
	// P1 and T3 are decided. The decision of T5 is of another form, and S1 has one item with no decision.
	want := "prose/prose-review=ax://paper/2401.00001#S1 unfold/unfold-review=ax://paper/2401.00001#T5 proof/proof-target=ax://paper/2401.00001#P2"
	if strings.Join(got, " ") != want {
		t.Errorf("the queues are %s", strings.Join(got, " "))
	}
	files[understand.DecisionsFile] += `{"record":"ax://paper/2401.00001#S1","queue":"prose-review","item":"4-4","decision":"reject","by":"github:a","on":"2026-11-03"}` + "\n"
	root := reviewTree(t, files)
	qs, err = understand.Reviews(root)
	if err != nil || len(qs) != 2 {
		t.Errorf("with each prose item decided the queues are %v, %v", qs, err)
	}
	ds, err := understand.ReadDecisions(root)
	if err != nil {
		t.Fatal(err)
	}
	if pt := understand.ProofTargets(ds); len(pt) != 1 || pt["ax://paper/2401.00001#P1"] != "ax://paper/2401.00001#R1" {
		t.Errorf("the proof targets are %v", pt)
	}
	for _, bad := range []string{
		`{"record":"ax://paper/2401.00001#P1","queue":"proof-target","decision":"confirm","by":"github:a","on":"2026-11-03"}`,
		`{"record":"ax://paper/2401.00001#P1","queue":"proof-target","decision":"none","target":"ax://paper/2401.00001#R1","by":"github:a","on":"2026-11-03"}`,
		`{"record":"ax://paper/2401.00001#T3","queue":"unfold-review","decision":"weaker","by":"github:a","on":"2026-11-03"}`,
		`{"record":"ax://paper/2401.00001#T3","queue":"unfold-review","hash":"h3","decision":"unsure","by":"github:a","on":"2026-11-03"}`,
		`{"record":"ax://paper/2401.00001#S1","queue":"prose-review","item":"2-1","decision":"accept","by":"github:a","on":"2026-11-03"}`,
		`{"record":"ax://paper/2401.00001#S1","queue":"classify-review","decision":"accept","by":"github:a","on":"2026-11-03"}`,
		`{"record":"ax://paper/2401.00001#P1","queue":"proof-target","decision":"none","on":"2026-11-03"}`,
		`{"record":"ax://paper/2401.00001#P1","queue":"proof-target","decision":"none","by":"github:a","on":"2026-11-03","why":"text"}`,
		`{"record":"ax://paper/2401.00001#P1","queue":"proof-target","decision":"none","by":"github:a","on":"2026-11-03"}` + "\n" + `{"record":"ax://paper/2401.00001#P1","queue":"proof-target","decision":"none","by":"github:b","on":"2026-11-03"}`,
	} {
		if _, err := understand.ReadDecisions(reviewTree(t, map[string]string{understand.DecisionsFile: bad + "\n"})); err == nil {
			t.Errorf("ReadDecisions takes %s", bad)
		}
	}
}
