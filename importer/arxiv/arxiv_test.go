package arxiv_test

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/inventory"
)

const objectsV = `{"tag":"S1C1","kind":"section","paper":"2401.00001","version":"VER","local":"sec-1","number":"1","path":"source"}
{"tag":"LEM1","kind":"statement","subkind":"lemma","paper":"2401.00001","version":"VER","local":"lem-1","label":"lem:a","number":"1.1","section":"sec-1","title":"Lemma","name":"Key estimate","path":"source","item":"statement/lemma","scope":"paper","env":"lemma","head":"environment","proof_status":"proved","origin":"own","body_md":"For each $\\varepsilon > 0$ there is a constant $C$ with $|f| \\le C$.","hash":"sha256:aa"}
{"tag":"THM1","kind":"statement","subkind":"theorem","paper":"2401.00001","version":"VER","local":"thm-1","number":"1.2","section":"sec-1","title":"Theorem","path":"source","item":"statement/theorem","scope":"paper","hash":"sha256:bb"}
{"tag":"PRF1","kind":"proof","paper":"2401.00001","version":"VER","local":"proof-1","path":"source","item":"proof","scope":"paper","proves":"thm-1","hash":"sha256:cc"}
{"tag":"CLM1","kind":"statement","subkind":"claim","paper":"2401.00001","version":"VER","local":"claim-1","path":"source","item":"statement/claim","scope":"local","parent":"proof-1","hash":"sha256:dd"}
{"tag":"RMK1","kind":"remark","paper":"2401.00001","version":"VER","local":"remark-1","path":"source","item":"remark","scope":"paper"}
{"tag":"PRF2","kind":"proof","paper":"2401.00001","version":"VER","local":"proof-2","path":"source","item":"proof","scope":"paper","proves":"remark-1"}
{"tag":"THM2","kind":"statement","subkind":"theorem","paper":"2401.00001","version":"VER","local":"thm-2","number":"2.1","title":"Theorem","path":"source","item":"statement/theorem","scope":"paper","origin":"restated","restates":"thm-1","hash":"sha256:bb"}
{"tag":"PRB1","kind":"problem","paper":"2401.00001","version":"VER","local":"problem-1","body_md":"Is every such space compact?","path":"source","item":"problem","scope":"paper"}
`

func checkout(t *testing.T, licence string) string {
	t.Helper()
	root := t.TempDir()
	write := func(name, data string) {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v1 := strings.ReplaceAll(objectsV, `"VER"`, `"1"`)
	// Version 2 drops the problem, so its mention is retired.
	var v2 []string
	for _, l := range strings.Split(strings.TrimSpace(strings.ReplaceAll(objectsV, `"VER"`, `"2"`)), "\n") {
		if !strings.Contains(l, "PRB1") {
			v2 = append(v2, strings.Replace(l, `"number":"1.2"`, `"number":"1.3"`, 1))
		}
	}
	write("analysis/objects/2401/2401.00001.jsonl", v1+strings.Join(v2, "\n")+"\n")
	write("analysis/edges/2401/2401.00001.jsonl", `{"from":"2401.00001#proof-1","to":"2401.00001#thm-1","predicate":"proves","method":"attach","confidence":"certain","evidence":{"version":2}}
{"from":"2401.00001#proof-1","to":"2401.00001#lem-1","predicate":"uses","method":"ref","confidence":"certain","evidence":{"version":2}}
{"from":"2401.00001#thm-1","to":"bib:Ser","predicate":"cites-result","part":"Theorem 4.1","method":"cite","confidence":"high","evidence":{"version":2}}
`)
	write("analysis/census/2401/2401.00001v2.json", `{"strength":"normal"}`)
	write("metadata/2401.jsonl", `{"id":"2401.00001","categories":["math.AG"],"msc_class":"14J60 (Primary)","versions":[{"version":1,"licence":"`+licence+`"},{"version":2}]}`+"\n")
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "c"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return root
}

func byID(res *arxiv.Result) map[string]inventory.Mention {
	out := map[string]inventory.Mention{}
	for _, m := range res.Mentions {
		out[strings.TrimPrefix(m.ID, "ax://paper/2401.00001#")] = m
	}
	return out
}

func TestEachItemGetsAMention(t *testing.T) {
	res, err := arxiv.Import(checkout(t, "cc-by"))
	if err != nil {
		t.Fatal(err)
	}
	ms := byID(res)
	var got []string
	for k := range ms {
		got = append(got, k)
	}
	slices.Sort(got)
	// The section and the remark get no mention, and the restated theorem is one mention.
	if want := []string{"CLM1", "LEM1", "PRB1", "PRF1", "PRF2", "THM1"}; !slices.Equal(got, want) {
		t.Fatalf("mentions %v, want %v", got, want)
	}
	// A proof of a remark has no target that is a mention.
	if p := ms["PRF2"]; p.AttachedTo != "unknown" || !slices.Equal(p.Hints, []string{"target-not-an-item"}) {
		t.Errorf("proof of the remark: attached_to %q, hints %v", p.AttachedTo, p.Hints)
	}
	lem := ms["LEM1"]
	if lem.Kind != "statement" || lem.Role != "lemma" || lem.Version != 2 || lem.Class != "open" || lem.Name != "Key estimate" || lem.Census != "normal" {
		t.Errorf("lemma %+v", lem)
	}
	if !slices.Equal(lem.MSC, []string{"14J60"}) || lem.TextRef == "" || lem.Text != "" {
		t.Errorf("lemma msc %v, text_ref %q", lem.MSC, lem.TextRef)
	}
	thm := ms["THM1"]
	if thm.Number != "1.3" || len(thm.Versions) != 1 || thm.Versions[0] != (inventory.VersionRef{Version: 1, Number: "1.2"}) {
		t.Errorf("theorem number %q, versions %v", thm.Number, thm.Versions)
	}
	if len(thm.Also) != 1 || !strings.Contains(thm.Also[0], "2.1") {
		t.Errorf("theorem also %v", thm.Also)
	}
	if len(thm.CitesResult) != 1 || thm.CitesResult[0] != (inventory.CitedResult{To: "bib:Ser", Locator: "Theorem 4.1", Confidence: "high"}) {
		t.Errorf("theorem cites %v", thm.CitesResult)
	}
	if thm.Structure == nil || !slices.Equal(thm.Structure.ProvedBy, []string{"ax://paper/2401.00001#PRF1"}) || thm.Structure.In != "ax://paper/2401.00001#S1C1" {
		t.Errorf("theorem structure %+v", thm.Structure)
	}
	prf := ms["PRF1"]
	if prf.AttachedTo != "ax://paper/2401.00001#THM1" || prf.Structure == nil || !slices.Equal(prf.Structure.Uses, []string{"ax://paper/2401.00001#LEM1"}) {
		t.Errorf("proof %+v", prf)
	}
	if clm := ms["CLM1"]; clm.AttachedTo != "ax://paper/2401.00001#PRF1" || clm.Role != "lemma" {
		t.Errorf("claim %+v", clm)
	}
	if prb := ms["PRB1"]; prb.Retired != 2 || prb.Version != 1 || !slices.Contains(prb.Hints, "conjecture-candidate") {
		t.Errorf("problem %+v", prb)
	}
	if res.Attached != 3 || res.Retired != 1 || res.Restated != 1 || len(res.Untagged) != 0 {
		t.Errorf("result %+v", res)
	}
}

func TestARecordVersionGivesNoText(t *testing.T) {
	res, err := arxiv.Import(checkout(t, "arxiv-1.0"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res.Mentions {
		if m.Class != "record" || m.Name != "" || m.TextRef != "" || m.Text != "" || strings.Contains(m.Locator, " ") {
			t.Errorf("mention %s of a record version: class %s, name %q, text_ref %q, locator %q", m.ID, m.Class, m.Name, m.TextRef, m.Locator)
		}
	}
}

func TestAttachedMentionsGoInTheRowOfTheirTarget(t *testing.T) {
	res, err := arxiv.Import(checkout(t, "cc-by"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	out, err := inventory.Cluster(nil, res.Mentions, inventory.Options{Mint: func(func(string) bool) (string, error) {
		n++
		return "R" + strings.Repeat("0", 4) + string(rune('0'+n)), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 3 || out.Orphans != 1 {
		t.Fatalf("%d rows, %d orphans", len(out.Rows), out.Orphans)
	}
	for _, r := range out.Rows {
		if slices.Contains(r.Mentions, "ax://paper/2401.00001#THM1") {
			if !slices.Equal(r.Proofs, []string{"ax://paper/2401.00001#PRF1"}) || !slices.Equal(r.Local, []string{"ax://paper/2401.00001#CLM1"}) {
				t.Errorf("row of the theorem: proofs %v, local %v", r.Proofs, r.Local)
			}
		}
	}
}

func TestAShortBodyGivesNoHash(t *testing.T) {
	res, err := arxiv.Import(checkout(t, "cc-by"))
	if err != nil {
		t.Fatal(err)
	}
	ms := byID(res)
	if ms["LEM1"].Hash != "sha256:aa" || ms["THM1"].Hash != "" {
		t.Errorf("the hashes are %q and %q, want sha256:aa and none", ms["LEM1"].Hash, ms["THM1"].Hash)
	}
}

func TestYear(t *testing.T) {
	for id, want := range map[string]string{"2401.00001": "2024", "0704.0001": "2007", "math/9802134": "1998", "math/0301148": "2003"} {
		if got := arxiv.Year(id); got != want {
			t.Errorf("Year(%q) is %q, want %q", id, got, want)
		}
	}
}

func TestCandidatesAndReport(t *testing.T) {
	ms := []inventory.Mention{
		{ID: "ax://paper/2401.00001#T1", Source: "arxiv", Kind: "statement", Name: "Banach fixed point theorem", MSC: []string{"47H10"}},
		{ID: "ax://paper/2401.00002#T1", Source: "arxiv", Kind: "statement", Name: "The Banach fixed-point theorem", MSC: []string{"54E50"}},
		{ID: "ax://paper/2401.00003#T1", Source: "arxiv", Kind: "statement", Name: "Key lemma", MSC: []string{"47H10"}},
		{ID: "ax://paper/2401.00004#T1", Source: "arxiv", Kind: "unknown", MSC: []string{"47H10"}},
		{ID: "ax://paper/2401.00004#P1", Source: "arxiv", Kind: "proof", AttachedTo: "ax://paper/2401.00004#T1"},
		{ID: "m2://rec/wikipedia/Banach fixed-point theorem#1", Source: "wikipedia", Kind: "statement", Name: "Banach fixed-point theorem", Aliases: []string{"Contraction mapping theorem"}},
	}
	rows := []inventory.Row{
		{ID: "R1", Mentions: []string{ms[0].ID}, Kind: "statement", MSC: []string{"47H10"}, Spread: 1, Outcome: &inventory.Outcome{Value: "record"}},
		{ID: "R2", Mentions: []string{ms[1].ID}, Kind: "statement", MSC: []string{"54E50"}, Spread: 1, Outcome: &inventory.Outcome{Value: "record"}},
		{ID: "R3", Mentions: []string{ms[2].ID}, Kind: "statement", MSC: []string{"47H10"}, Spread: 1, Outcome: &inventory.Outcome{Value: "record"}},
		{ID: "W1", Mentions: []string{ms[5].ID}, Kind: "statement", MSC: []string{"47H10", "54E50"}, Spread: 1},
	}
	cands := arxiv.Candidates(rows, ms, "")
	if len(cands) != 2 || cands[0].Target != "W1" || cands[0].Row != "R1" || cands[1].Mention != ms[1].ID {
		t.Fatalf("the candidates are %+v", cands)
	}
	if got := arxiv.Candidates(rows, ms, "54E35"); len(got) != 1 || got[0].Mention != ms[1].ID {
		t.Errorf("the candidates of the block 54 are %+v", got)
	}
	r := arxiv.Report(rows, ms, cands, time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC))
	for _, want := range []string{"4 arXiv mentions that are not attached to another item, in 3 rows", "1 mention waits for a row", "found 2 candidates", "| record | 3 |", "| 47 | 3 | 2 | 2 | 0 | 0 | 1.00 | 0 | 0 | 1 | 1 |", "| 2024 | 4 | 3 | 3 |"} {
		if !strings.Contains(r, want) {
			t.Errorf("the report has no %q:\n%s", want, r)
		}
	}
}

func TestPlanUnderstand(t *testing.T) {
	ms := []inventory.Mention{
		{ID: "ax://paper/2401.00001#T1", Source: "arxiv", Kind: "statement", Class: "open", MSC: []string{"47H10"}},
		{ID: "ax://paper/2401.00001#P1", Source: "arxiv", Kind: "proof", Class: "open", MSC: []string{"47H10"}, AttachedTo: "ax://paper/2401.00001#T1"},
		{ID: "ax://paper/2401.00002#T1", Source: "arxiv", Kind: "statement", Class: "record", MSC: []string{"54E50"}},
		{ID: "ax://paper/2401.00003#D1", Source: "arxiv", Kind: "definition", Class: "record", MSC: []string{"47H09"}},
	}
	rows := []inventory.Row{
		{ID: "R1", Mentions: []string{ms[0].ID, ms[2].ID}, Kind: "statement", Spread: 1},
		{ID: "R2", Mentions: []string{ms[3].ID}, Kind: "definition", Spread: 1},
	}
	for _, c := range []struct {
		tier, msc string
		jobs      int
	}{{"T1", "", 3}, {"T1", "47", 2}, {"T2", "", 1}, {"T3", "", 2}, {"T3", "54", 1}, {"T4", "", 1}} {
		p, err := arxiv.PlanUnderstand(c.tier, rows, ms, c.msc)
		if err != nil || p.Jobs != c.jobs {
			t.Errorf("tier %s, msc %q: %d jobs, %v; want %d", c.tier, c.msc, p.Jobs, err, c.jobs)
		}
	}
	p, _ := arxiv.PlanUnderstand("T1", rows, ms, "")
	if p.TokensIn != 75000 || math.Abs(p.CostLow-0.006) > 1e-9 || math.Abs(p.CostHigh-0.018) > 1e-9 {
		t.Errorf("the plan of T1 is %+v", p)
	}
	if _, err := arxiv.PlanUnderstand("T0", rows, ms, ""); err == nil {
		t.Error("T0 has a plan")
	}
}
