package wikipedia

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/wpcorpus"
)

const adoptPage = `---
page: 31150
title: Lagrange's theorem (group theory)
revision: 7
---

# Lagrange's theorem (group theory)

::: {#o1 .subject cue=lead name="Lagrange's theorem"}

*Image: Cosets.svg.* A caption with a [link](wikipedia://en/2974).

In [group theory](wikipedia://en/41890), **Lagrange's theorem** states that the order of a subgroup divides the order of the group.[^o3]

:::

::: {#o6 .statement kind=statement/generalization cue=heading name="Extension"}

## Extension {#Extension}

::: {#o8 .statement kind=statement/theorem cue=math-theorem name="Extension of Lagrange's theorem"}

**Extension of Lagrange's theorem.**

If $H$ is a [subgroup](wikipedia://en/28011) of $G$ and $K$ is a subgroup of $H$, then $[G:K] = [G:H]\,[H:K]$, by [index](https://en.wikipedia.org/wiki/Index).

::: {#o9 .proof cue=math-proof}

Each left coset of $H$ is a union of $[H:K]$ left cosets of $K$.

:::

:::

:::

::: {#o10 .statement kind=statement/theorem cue=math-theorem name="Theorem (2)"}

**Theorem (2).**

A text.

:::
`

func adoptFixture(t *testing.T) (string, []inventory.Row, []inventory.Mention) {
	t.Helper()
	dir := t.TempDir()
	write := func(rel string, data []byte) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(wpcorpus.MarkdownPath(31150), []byte(adoptPage))
	objs := []wpcorpus.Object{
		{Ref: "o1", Tag: "SUBJ", Kind: "subject"},
		{Ref: "o6", Tag: "QMSR", Kind: "statement/generalization"},
		{Ref: "o8", Tag: "DCKK", Kind: "statement/theorem"},
		{Ref: "o9", Tag: "PRF1", Kind: "proof"},
		{Ref: "o10", Tag: "GEN2", Kind: "statement/theorem"},
	}
	data, err := wpcorpus.MarshalObjects(31150, 7, "Lagrange's theorem (group theory)", objs)
	if err != nil {
		t.Fatal(err)
	}
	write(wpcorpus.ObjectsPath(31150, 7), data)
	id := func(tag string) string { return wpcorpus.MentionID(31150, tag) }
	m := func(tag, kind, role, cue, name string, flags ...string) inventory.Mention {
		return inventory.Mention{ID: id(tag), Source: Source, Kind: kind, Role: role, Cue: cue, Name: name, Page: 31150, Revision: 7, Section: "Extension", NativeID: "Lagrange's theorem (group theory)#" + tag, Flags: flags}
	}
	subj := inventory.Mention{ID: wpcorpus.MentionID(31150, ""), Source: Source, Kind: "statement", Cue: "lead", Name: "Lagrange's theorem", Page: 31150, Revision: 7, NativeID: "Lagrange's theorem (group theory)"}
	sub := inventory.Mention{ID: wpcorpus.MentionID(28011, ""), Source: Source, Kind: "definition", Cue: "lead", Name: "Subgroup", Page: 28011, Revision: 3, NativeID: "Subgroup"}
	ms := []inventory.Mention{
		subj, sub,
		m("QMSR", "statement", "", "heading", "Extension"),
		m("DCKK", "statement", "theorem", "math-theorem", "Extension of Lagrange's theorem"),
		m("GEN2", "statement", "theorem", "math-theorem", "Theorem (2)"),
		m("FLAG", "statement", "theorem", "math-theorem", "Some theorem", "citation-needed"),
		m("SAME", "statement", "theorem", "math-theorem", "Lagrange's theorem"),
		{ID: id("PRF1"), Source: Source, Kind: "proof", Cue: "math-proof", Page: 31150, Revision: 7, AttachedTo: id("DCKK")},
	}
	rows := []inventory.Row{
		{ID: "R000000001", Mentions: []string{subj.ID}, Outcome: &inventory.Outcome{Value: inventory.OutcomeNode, Target: []string{"3C53TD"}}},
		{ID: "R000000002", Mentions: []string{sub.ID}, Outcome: &inventory.Outcome{Value: inventory.OutcomeNode, Target: []string{"G5S1M9"}}},
		{ID: "R000000003", Mentions: []string{id("QMSR")}},
		{ID: "R000000004", Mentions: []string{id("DCKK")}},
		{ID: "R000000005", Mentions: []string{id("GEN2")}},
		{ID: "R000000006", Mentions: []string{id("FLAG")}},
		{ID: "R000000007", Mentions: []string{id("SAME")}},
	}
	return dir, rows, ms
}

func TestGenericName(t *testing.T) {
	for name, want := range map[string]bool{"Theorem (2)": true, "Theorem A (groups)": true, "Lemma B": true, "Examples": true, "Extension of Lagrange's theorem": false, "Theorem of Lagrange": false, "Uniqueness of identity element": false} {
		if got := genericName.MatchString(name); got != want {
			t.Errorf("%q: got %v, want %v", name, got, want)
		}
	}
}

func TestNames(t *testing.T) {
	m := &inventory.Mention{Cue: "term", Name: "locally cyclic group", Aliases: []string{"Left coset", "Coset representative", "Locally-cyclic group", "Coset  representative"}}
	if got := displayName(m); got != "Locally cyclic group" {
		t.Errorf("got the name %q", got)
	}
	if got := displayName(&inventory.Mention{Cue: "term", Name: "p-group"}); got != "p-group" {
		t.Errorf("got the name %q", got)
	}
	if got := displayName(&inventory.Mention{Cue: "term", Name: "pGroup"}); got != "pGroup" {
		t.Errorf("got the name %q", got)
	}
	got := freeAliases(m, map[string]string{"left coset": "3C53TD"})
	if len(got) != 1 || got[0] != "coset-representative" {
		t.Errorf("got the aliases %q", got)
	}
}

func TestAdopt(t *testing.T) {
	dir, rows, ms := adoptFixture(t)
	n := 0
	opt := AdoptOptions{Corpus: dir, Day: "2026-11-03", Draw: func() (string, error) { n++; return fmt.Sprintf("~0000000%d", n), nil }}
	nodes := []AdoptNode{{ID: "3C53TD", Name: "Lagrange's theorem", MSC: []string{"20D60"}}, {ID: "G5S1M9", Name: "Subgroup"}}
	res, err := Adopt(rows, ms, nodes, opt)
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]string{}
	for _, it := range res.Items {
		state[tagOf(it.Mention)] = it.State + " " + it.Reason
	}
	want := map[string]string{
		"QMSR": "waiting the cue heading names a section and not one item",
		"DCKK": "adopted ",
		"GEN2": "waiting the item has no name of its own, and the write role names it",
		"FLAG": "waiting the item has the flag citation-needed (WP9.2)",
		"SAME": "waiting the item has the name of the subject of its article, and a person decides if they are one item",
		"PRF1": "adopted ",
	}
	for k, w := range want {
		if state[k] != w {
			t.Errorf("%s: got %q, want %q", k, state[k], w)
		}
	}
	if len(res.Drafts) != 2 {
		t.Fatalf("got %d drafts, want 2", len(res.Drafts))
	}
	stmt := string(res.Drafts[0].Data)
	for _, s := range []string{
		"kind: statement\nrole: theorem\nname: Extension of Lagrange's theorem\nmsc: [20D60]\n",
		"  - wikipedia: en:31150@r7\n    locator: Lagrange's theorem (group theory), section Extension\n    note: adopted\n    mention: wikipedia://en/31150#DCKK\n    sections: [Statement]\n",
		"inventory: [R000000004]\n",
		"## Statement\n\nIf $H$ is a [[subgroup|G5S1M9]] of $G$ and $K$ is a subgroup of $H$, then $[G:K] = [G:H]\\,[H:K]$, by index.\n",
	} {
		if !strings.Contains(stmt, s) {
			t.Errorf("the statement node has no %q:\n%s", s, stmt)
		}
	}
	if strings.Contains(stmt, "Each left coset") || strings.Contains(stmt, "**Extension") {
		t.Errorf("the statement node has the proof or the label:\n%s", stmt)
	}
	proof := string(res.Drafts[1].Data)
	for _, s := range []string{"kind: proof\nname: Proof of Extension of Lagrange's theorem\n", "proves: ~00000001\n", "## Proof\n\nEach left coset of $H$ is a union of $[H:K]$ left cosets of $K$.\n"} {
		if !strings.Contains(proof, s) {
			t.Errorf("the proof node has no %q:\n%s", s, proof)
		}
	}
	if strings.Contains(proof, "inventory:") {
		t.Errorf("the proof node has an inventory field:\n%s", proof)
	}
}

func TestAdoptLinksMintedNodes(t *testing.T) {
	dir, rows, ms := adoptFixture(t)
	opt := AdoptOptions{Corpus: dir, Day: "2026-11-03", Draw: func() (string, error) { return "", fmt.Errorf("no draw") }, Hold: map[string]string{wpcorpus.MentionID(31150, "PRF1"): "the proof needs a check"}}
	nodes := []AdoptNode{
		{ID: "3C53TD", Name: "Lagrange's theorem"},
		{ID: "7K3QX2JW1M", Name: "Extension of Lagrange's theorem", Inventory: []string{"R000000004"}, Mentions: []string{wpcorpus.MentionID(31150, "DCKK")}},
	}
	res, err := Adopt(rows, ms, nodes, opt)
	if err != nil {
		t.Fatal(err)
	}
	if res.Linked != 1 || len(res.Drafts) != 0 {
		t.Fatalf("got %d linked and %d drafts, want 1 and 0", res.Linked, len(res.Drafts))
	}
	o := res.Rows[3].Outcome
	if o == nil || o.Value != inventory.OutcomeNode || o.Rule != NodeRule || o.Target[0] != "7K3QX2JW1M" || len(res.Rows[3].History) != 1 {
		t.Fatalf("the row has the outcome %+v and the history %+v", o, res.Rows[3].History)
	}
	for _, it := range res.Items {
		if tagOf(it.Mention) == "PRF1" && it.Reason != "a person holds the item: the proof needs a check" {
			t.Errorf("the proof has the reason %q", it.Reason)
		}
	}
	rep := string(AdoptReport(res, "2026-11-03"))
	for _, s := range []string{"linked: 1\n", "| a person holds the item | 1 |", "## Lagrange's theorem (group theory)"} {
		if !strings.Contains(rep, s) {
			t.Errorf("the report has no %q:\n%s", s, rep)
		}
	}
}

func TestAdoptDefersAndParity(t *testing.T) {
	dir, rows, ms := adoptFixture(t)
	hold := wpcorpus.MentionID(31150, "PRF1")
	opt := AdoptOptions{Corpus: dir, Day: "2026-11-03", Until: "2027-05-01", Draw: func() (string, error) { return "", fmt.Errorf("no draw") }, Hold: map[string]string{hold: "the proof needs a check"}}
	nodes := []AdoptNode{
		{ID: "3C53TD", Name: "Lagrange's theorem"},
		{ID: "7K3QX2JW1M", Name: "Extension of Lagrange's theorem", Inventory: []string{"R000000004"}, Mentions: []string{wpcorpus.MentionID(31150, "DCKK")}},
	}
	res, err := Adopt(rows, ms, nodes, opt)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deferred == 0 {
		t.Fatal("the run deferred no row")
	}
	for _, r := range res.Rows {
		if r.Outcome != nil && r.Outcome.Value == inventory.OutcomeDeferred && (r.Outcome.Rule != DeferRule || r.Outcome.Until != "2027-05-01" || r.Outcome.Reason == "" || len(r.History) == 0) {
			t.Errorf("the deferred row %s has the outcome %+v and the history %+v", r.ID, r.Outcome, r.History)
		}
	}
	again, err := Adopt(res.Rows, ms, nodes, opt)
	if err != nil {
		t.Fatal(err)
	}
	if again.Deferred != 0 || again.Linked != 0 {
		t.Errorf("a second run deferred %d rows and linked %d rows, want 0 and 0", again.Deferred, again.Linked)
	}

	pn := []ParityNode{{ID: "7K3QX2JW1M", Mentions: map[string]string{wpcorpus.MentionID(31150, "DCKK"): "adopted"}}}
	items := Parity(res.Rows, ms, pn, nil, "2026-11-03")
	if len(items) == 0 || strings.Contains(items[0].Mention, "#") {
		t.Fatalf("the parity data does not start with the article: %+v", items)
	}
	for _, it := range items {
		if it.Missing() {
			t.Errorf("the item %s has no outcome after the run deferred the rows", it.Mention)
		}
		if tagOf(it.Mention) == "DCKK" && (it.Outcome != inventory.OutcomeNode || it.Origin != "adopted") {
			t.Errorf("the item DCKK has the outcome %s and the origin %q", it.Outcome, it.Origin)
		}
	}
	if late := Parity(res.Rows, ms, pn, nil, "2027-05-02"); !slices.ContainsFunc(late, func(it ParityItem) bool { return it.Missing() }) {
		t.Error("the rows are deferred until 2027-05-01, and the parity data of 2027-05-02 has no open item")
	}

	root := t.TempDir()
	if err := WriteParity(root, items); err != nil {
		t.Fatal(err)
	}
	back, err := ReadParity(root)
	if err != nil || len(back) != len(items) {
		t.Fatalf("read %d items back (%v), want %d", len(back), err, len(items))
	}
	if err := WriteParity(root, items[:1]); err != nil {
		t.Fatal(err)
	}
	back, _ = ReadParity(root)
	if n := len(slices.DeleteFunc(back, func(it ParityItem) bool { return it.Article != items[0].Article })); n != 1 {
		t.Errorf("a second write of the article kept %d of its items, want 1", n)
	}
	rep := string(ParityReport("group-theory", items, "2026-11-03"))
	for _, s := range []string{"area: group-theory\n", "without_outcome: 0\n", "## Lagrange's theorem (group theory)", "| Reason of a deferred item |"} {
		if !strings.Contains(rep, s) {
			t.Errorf("the report has no %q:\n%s", s, rep)
		}
	}
}

func TestAdoptSkipsMintedProof(t *testing.T) {
	dir, rows, ms := adoptFixture(t)
	opt := AdoptOptions{Corpus: dir, Day: "2026-11-03", Draw: func() (string, error) { return "", fmt.Errorf("no draw") }}
	proof := wpcorpus.MentionID(31150, "PRF1")
	for _, id := range []string{"GN6WHHGBAE", "~0000000a"} {
		nodes := []AdoptNode{
			{ID: "3C53TD", Name: "Lagrange's theorem"},
			{ID: "7K3QX2JW1M", Name: "Extension of Lagrange's theorem", Inventory: []string{"R000000004"}, Mentions: []string{wpcorpus.MentionID(31150, "DCKK")}},
			{ID: id, Name: "Proof of Extension of Lagrange's theorem", Proves: "7K3QX2JW1M", Mentions: []string{proof}},
		}
		res, err := Adopt(rows, ms, nodes, opt)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(res.Items, func(it AdoptItem) bool { return it.Mention == proof })
		switch {
		case id[0] != '~' && i >= 0:
			t.Errorf("the minted node %s has the proof, and the run lists it as %s", id, res.Items[i].State)
		case id[0] == '~' && (i < 0 || res.Items[i].State != Drafted):
			t.Errorf("the draft %s has the proof, and the run does not list it as drafted", id)
		}
	}
}

func TestWhySame(t *testing.T) {
	bySame := map[string]string{"wikidata:Q7111377": "7Z5ZS7KNXS", "wikipedia:Riesz potential": "1V2N8MFSHC"}
	for _, c := range []struct {
		m     inventory.Mention
		title string
		want  string
	}{
		{inventory.Mention{ID: "wikipedia://en/12747184", Same: map[string][]string{"wikidata": {"Q7111377"}}}, "Out(Fn)", "node 7Z5ZS7KNXS has the same Wikidata item Q7111377, and a person decides if they are one item"},
		{inventory.Mention{ID: "wikipedia://en/5"}, "Riesz potential", "node 1V2N8MFSHC has the same article in same_as, and a person decides if they are one item"},
		{inventory.Mention{ID: "wikipedia://en/6", Same: map[string][]string{"wikidata": {"Q1"}}}, "Group", ""},
		{inventory.Mention{ID: "wikipedia://en/12747184#AB12", Same: map[string][]string{"wikidata": {"Q7111377"}}}, "Out(Fn)", ""},
	} {
		if got := whySame(&c.m, c.title, bySame); got != c.want {
			t.Errorf("%s: got %q, want %q", c.m.ID, got, c.want)
		}
	}
}

func TestWhyName(t *testing.T) {
	for _, c := range []struct {
		cue, name string
		wait      bool
	}{
		{"named-heading", "Grushko decomposition theorem", false},
		{"named-heading", "A useful corollary", true},
		{"inline-head", "Problem 1 (Schmidt 1968)", true},
		{"list-entry", "Lattices", true},
		{"named-heading", "Identity / neutral elements", true},
		{"named-heading", "Variants of Wilson's theorem", true},
		{"named-heading", "Characterization by a convolution identity", true},
		{"named-heading", "Equivalent formulations of locality axiom", true},
		{"list-entry", "Root (or zero) of a polynomial", true},
		{"named-heading", "Bhargava's approach to the composition law", true},
		{"named-heading", "Interpretation via the Chinese Remainder Theorem", true},
		{"named-heading", "Extension to include the Goldbach conjecture", true},
		{"named-heading", "Conjectures on the boundedness of ranks", true},
		{"named-heading", "Generalized Riemann hypothesis (GRH)", true},
		{"inline-head", "'Problem #6 at IMO 1988'", true},
		{"math-theorem", "Lemma A.", true},
		{"named-heading", "Hensel's lemma for p-adic numbers", false},
		{"named-heading", "Skowronski–Weyman theorem", false},
		{"named-heading", "Distributivity laws for complete lattices", true},
		{"named-heading", "Dual of Dilworth's theorem (Mirsky's theorem)", true},
		{"named-heading", "Adjoint functor theorem for posets", false},
		{"named-heading", "Theorem two", true},
		{"heading", "Proof of Theorem two", true},
		{"named-heading", "Theorem for vertex-weighted graphs", true},
		{"named-heading", "The stacking theorem", true},
		{"named-heading", "Importance of inequality", true},
		{"named-heading", "Graphon version of counting lemma", true},
		{"named-heading", "Bandwidth formulas for some graphs", true},
		{"named-heading", "Formula for partition size", true},
		{"named-heading", "Lovász \"sandwich theorem\"", true},
		{"math-theorem", "Dirac's Theorem (1952)", true},
		{"list-entry", "How many unit distances", true},
		{"list-entry", "Cullen primes", true},
		{"list-entry", "AEC", true},
		{"list-entry", "Lyapunov function: Lyapunov's second method for stability", true},
		{"list-entry", "Erdős–Oler conjecture", false},
		{"term", "Alpha", true},
		{"term", "Co-", true},
		{"term", "Acyclic", false},
		{"named-heading", "Nordhaus–Gaddum inequality", false},
		{"named-heading", "Clifford's theorem", false},
		{"named-heading", "Weil's conjecture on Tamagawa numbers", false},
		{"named-heading", "History: the lemma that is not Burnside's", true},
		{"named-heading", "Precise statement of the Adyan–Rabin theorem", true},
		{"named-heading", "Examples of applications of the Švarc–Milnor lemma", true},
		{"named-heading", "Definition (Class functions)", true},
		{"named-heading", "Characters, Schur's lemma and the inner product", true},
		{"named-heading", "Freudenthal-Hopf theorems", true},
		{"named-heading", "Some theorems", true},
		{"named-heading", "Structure theorem", true},
		{"named-heading", "Volume formula", true},
		{"math-theorem", "Corollary (Maschke's theorem)", true},
		{"named-heading", "Involving the binomial theorem", true},
		{"lead", "Centralizer and normalizer", false},
		{"lead", "Vector algebra relations", false},
		{"list-entry", "-adic numbers", true},
		{"list-entry", "the Heisenberg group modulo p", true},
		{"list-entry", "2-groups", false},
		{"named-heading", "Principle", true},
		{"named-heading", "Principles", true},
		{"named-heading", "Primitive propositions", true},
		{"named-heading", "Relationship with the axiom of choice", true},
		{"named-heading", "Validity of the law of excluded middle", true},
		{"named-heading", "Theorems proved with the help of computer programs", true},
		{"named-heading", "An introduction to the notation of \"Section A Mathematical Logic\"", true},
		{"inline-head", "polygon of 65537 sides", true},
		{"term", "coanalytic", false},
		{"term", "GCH", true},
		{"list-entry", "Approximations of", true},
		{"list-entry", "Proof that  is irrational", true},
		{"named-heading", "1. Axiom of extensionality", true},
		{"named-heading", "Axiom of pairing - 4", true},
		{"named-heading", "Theorem 1. Every valid formula is provable.", true},
		{"named-heading", "2 theorem", true},
		{"named-heading", "Theorem 2. Every formula &phi; is refutable", true},
		{"named-heading", "Using the Chinese remainder theorem", true},
		{"named-heading", "As a theorem of arithmetic", true},
		{"named-heading", "Second example (with variables)", true},
		{"named-heading", "The second theorem", true},
		{"named-heading", "Laws", true},
		{"named-heading", "Cases where the axiom of choice is not needed", true},
		{"named-heading", "Hume's principle", false},
		{"named-heading", "The Fundamental Theorem of Proper Forcing", false},
		{"named-heading", "Ramified forcing", false},
		{"lead", "block", false},
		{"named-heading", "Principle of least action", false},
		{"named-heading", "Wilf's conjecture", false},
		{"term", "π", true},
		{"lead", "π", false},
		{"named-heading", "Accuracy of the formula", true},
		{"named-heading", "An approximation formula given in Wucao suanjing", true},
		{"named-heading", "Another formula for the circumference of a circle", true},
		{"named-heading", "Connection to the Pythagorean theorem", true},
		{"named-heading", "Derivation of the formula", true},
		{"named-heading", "Equivalent forms of the formula", true},
		{"named-heading", "From formula mathematics to tensors", true},
		{"math-theorem", "Theorem (Brahmagupta)", true},
		{"math-theorem", "Lemma (Brahmagupta)", true},
		{"term", "Ζ", true},
		{"named-heading", "Gou Gu (Pythagorean) Theorem", false},
		{"named-heading", "Fermat's little theorem", false},
	} {
		got := whyName(&inventory.Mention{Cue: c.cue, Name: c.name}) != ""
		if got != c.wait {
			t.Errorf("%s %q: wait %v, want %v", c.cue, c.name, got, c.wait)
		}
	}
	for name, want := range map[string]bool{"Axiom 1": true, "Claim 2": true, "Lemma 3.1": true, "Theorem IV": true, "Axiom of choice": false, "Lemma": false} {
		if got := labelName.MatchString(name); got != want {
			t.Errorf("labelName %q: %v, want %v", name, got, want)
		}
	}
}

func TestDisplayNameQualifier(t *testing.T) {
	m := &inventory.Mention{ID: "wikipedia://en/1", Cue: "lead", Name: "block", NativeID: "Block (permutation group theory)"}
	if got := displayName(m); got != "Block (permutation group theory)" {
		t.Errorf("got %q", got)
	}
	m = &inventory.Mention{ID: "wikipedia://en/2", Cue: "lead", Name: "Metacyclic group", NativeID: "Metacyclic group"}
	if got := displayName(m); got != "Metacyclic group" {
		t.Errorf("got %q", got)
	}
}

func TestDisplayNameThe(t *testing.T) {
	m := &inventory.Mention{ID: "wikipedia://en/3#o4", Cue: "named-heading", Name: "The Routh criterion"}
	if got := displayName(m); got != "Routh criterion" {
		t.Errorf("got %q", got)
	}
	m = &inventory.Mention{ID: "wikipedia://en/4", Cue: "lead", Name: "The Pirates of Penzance"}
	if got := displayName(m); got != "The Pirates of Penzance" {
		t.Errorf("got %q", got)
	}
}

func TestDropNotes(t *testing.T) {
	notes := map[string]string{"n3": "Serre (1977).", "o6": "See Logic programming."}
	for in, want := range map[string]string{
		"A proof may be found in [^n3].":       "A proof may be found in Serre (1977).",
		"The group is simple.[^o6] It is big.": "The group is simple. It is big.",
		"See [^n3], page 5.":                   "See Serre (1977), page 5.",
		"It is in [^n3] and more.":             "It is in  and more.",
		"A proof is in [^zz].":                 "A proof is in .",
	} {
		if got := dropNotes(in, notes); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestProofName(t *testing.T) {
	if got := proofName("The normal forms theorem"); got != "Proof of the normal forms theorem" {
		t.Errorf("got %q", got)
	}
	if got := proofName("Burnside's lemma"); got != "Proof of Burnside's lemma" {
		t.Errorf("got %q", got)
	}
}

func TestSentences(t *testing.T) {
	for text, want := range map[string]int{
		"Generalized continuum hypothesis":                  0,
		"- Brauer's theorem (*number theory*)":              0,
		"René-Louis Baire\nA subset has the Baire property": 0,
		"Named for [Paul Gordan](wikipedia://en/1059850).":  1,
		"A relation R is Euclidean. It is a relation.":      2,
	} {
		if got := sentences(text); got != want {
			t.Errorf("sentences(%q) = %d, want %d", text, got, want)
		}
	}
}

func TestListsItems(t *testing.T) {
	for _, c := range []struct {
		cue, text string
		want      bool
	}{
		{"term", "Oswald Veblen\n\nThe Veblen hierarchy is a family of ordinal valued functions.", true},
		{"term", "sharp\n\nNumber of elements: the cardinality of a set.\n\nsharp\n\nPrimorial.", true},
		{"term", "Relating to types; for example, typically ambiguous.", false},
		{"named-heading", "$$x = y$$\n\nThe equation holds.", false},
	} {
		if got := listsItems(c.cue, c.text); got != c.want {
			t.Errorf("listsItems(%q, %q) = %v, want %v", c.cue, c.text, got, c.want)
		}
	}
}

func TestSeeEntry(t *testing.T) {
	if !seeEntry.MatchString("For strong connectivity and strongly connected components of directed graphs, see connected and component.") {
		t.Error("an entry that points to other entries is not found")
	}
	if seeEntry.MatchString("For a graph $G$, the girth is the length of its shortest cycle.") {
		t.Error("a definition is an entry that points to other entries")
	}
	for _, e := range []string{"See transpose.", "Synonym for *degree*.", "A synonym for degeneracy."} {
		if !seeEntry.MatchString(e) {
			t.Errorf("%q is not found as an entry that points to other entries", e)
		}
	}
	if seeEntry.MatchString("Seed a graph with one vertex.") {
		t.Error("a word that starts with see is a pointer")
	}
	if !personEntry.MatchString("Julius Petersen (1839–1910), Danish graph theorist.") || !personEntry.MatchString("Henri Meyniel, French graph theorist.") {
		t.Error("an entry about a person is not found")
	}
	if personEntry.MatchString("A source, in a directed graph, is a vertex with no incoming edges.") {
		t.Error("a definition is an entry about a person")
	}
}

func TestModifierEntry(t *testing.T) {
	for _, c := range [][2]string{
		{"Unitary", "A unitary representation of a group *G* is a representation."},
		{"Brauer", "Brauer's theorem on induced characters states that a character is a sum."},
		{"Adams", "Adams operations."},
		{"Artin", "Emil Artin."},
		{"Faithful", "A [[faithful representation|5FW1MD0HNV]] is a representation that is injective."},
		{"Primitive", "The term \"primitive element\" is an old term for a vector."},
		{"Artin", "Emil Artin.\n\nArtin's theorem on induced characters states a result."},
		{"Positive-energy representation", "positive-energy representation."},
		{"Dominant", "The representations are indexed by their highest weight. These *dominant weights* form a lattice."},
		{"Admissible", "A representation is called admissible if it has finite multiplicities."},
		{"Defined over", "A representation is said to be defined over *F* if it comes from F."},
	} {
		if !modifierEntry(c[0], c[1]) {
			t.Errorf("%q with %q is not found as a modifier entry", c[0], c[1])
		}
	}
	for _, c := range [][2]string{
		{"Ring", "A ring is a set with two operations."},
		{"Girth", "The girth of a graph is the length of its shortest cycle."},
		{"Source", "A vertex with no incoming edges."},
		{"Ideal", "An ideal I of a ring R is a subgroup."},
	} {
		if modifierEntry(c[0], c[1]) {
			t.Errorf("%q with %q is found as a modifier entry", c[0], c[1])
		}
	}
}

func TestCaseOf(t *testing.T) {
	subjectOf := map[string]int{inventory.NameKey("Local Langlands conjectures"): 7, inventory.NameKey("Ramanujan–Petersson conjecture"): 8}
	if !caseOf("Local Langlands conjectures for GL2", 7, subjectOf) || !caseOf("The Ramanujan–Petersson conjecture over global function fields", 8, subjectOf) {
		t.Error("a case of the subject of the article is not found")
	}
	if caseOf("Local Langlands conjectures for GL2", 9, subjectOf) || caseOf("Prime number theorem for arithmetic progressions", 7, subjectOf) {
		t.Error("a heading of another article is a case of the subject")
	}
	if !fieldName.MatchString("Classical differential geometry") || fieldName.MatchString("Lie algebra") {
		t.Error("fieldName does not find a field of study")
	}
	if !personEntry.MatchString("- Akṣapada Gautama(c. 600 BCE–200 CE)") || personEntry.MatchString("See the equations (1–2) above.") {
		t.Error("personEntry does not find a person with an old date")
	}
}

func TestURLText(t *testing.T) {
	if !urlText.MatchString("Graph Theory, <http://www.math.ucsd.edu/~fan/research/revised.html>):") {
		t.Error("the URL of a citation is not found")
	}
	if urlText.MatchString("See [[cardinality|0G0ASKNA6Z]].") {
		t.Error("a node link is a URL")
	}
	if !slices.Contains(check.StatementSections(), "Statement") {
		t.Error("Statement is not a statement section")
	}
}
