package wikipedia

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if o == nil || o.Value != inventory.OutcomeNode || o.Target[0] != "7K3QX2JW1M" || len(res.Rows[3].History) != 1 {
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
