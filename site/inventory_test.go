package site

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/inventory"
)

func TestInventory(t *testing.T) {
	root := newContent(t)
	ms := []inventory.Mention{
		{ID: "m2://rec/wikidata/Q11518#1", Source: "wikidata", NativeID: "Q11518", Kind: "statement", Name: "Pythagorean theorem", Same: map[string][]string{"mathworld": {"PythagoreanTheorem"}}},
		{ID: "m2://rec/wikidata/Q1#1", Source: "wikidata", NativeID: "Q1", Name: "2-group"},
	}
	rows := []inventory.Row{
		{ID: "7K3QX2M9R2", Mentions: []string{ms[0].ID}, Kind: "statement", Name: "Pythagorean theorem", Layer: inventory.Textbook, Spread: 1},
		{ID: "3C53TD", Mentions: []string{ms[1].ID}, Kind: inventory.Unknown, Name: "2-group", Layer: inventory.Textbook, Spread: 1,
			Outcome: &inventory.Outcome{Value: inventory.OutcomeOutOfScope, Reason: "a test"}},
	}
	if err := inventory.WriteMentions(root, "wikidata", ms); err != nil {
		t.Fatal(err)
	}
	if err := inventory.WriteRows(root, rows); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "nodes/1H8/1H8D2C.md", "---\ntag: 1H8D2C\nkind: statement\nrole: theorem\nname: Pythagorean theorem\nmsc: [51M04]\nsources:\n  - url: https://en.wikipedia.org/wiki/Pythagorean_theorem\n    locator: lead section\nsame_as:\n  wikidata: Q11518\n---\n\n## Statement\n\nIn a right triangle, $a^2 + b^2 = c^2$.\n")
	out := buildSite(t, root, "")
	index := readFile(t, out, "inventory/index.html")
	for _, want := range []string{"Open rows", `href="/math2/inventory/p/"`, `href="/math2/inventory/other/"`, "out_of_scope"} {
		if !strings.Contains(index, want) {
			t.Errorf("inventory/index.html has no %q", want)
		}
	}
	p := readFile(t, out, "inventory/p/index.html")
	for _, want := range []string{`<tr id="7K3QX2M9R2">`, "https://www.wikidata.org/wiki/Q11518", "https://mathworld.wolfram.com/PythagoreanTheorem.html"} {
		if !strings.Contains(p, want) {
			t.Errorf("inventory/p/index.html has no %q", want)
		}
	}
	if !strings.Contains(p, `<a href="/math2/wiki/pythagorean-theorem/">Pythagorean theorem</a>`) || !strings.Contains(p, `<a class="tag" href="/math2/inventory/row/7K/#7K3QX2M9R2">`) {
		t.Error("the row of the Pythagorean theorem does not link to its node and its card")
	}
	card := readFile(t, out, "inventory/row/7K/index.html")
	for _, want := range []string{`<section class="card inv-card" id="7K3QX2M9R2">`, `<a href="https://www.wikidata.org/wiki/Q11518">`, `<a href="/math2/wiki/pythagorean-theorem/">Pythagorean theorem</a>`} {
		if !strings.Contains(card, want) {
			t.Errorf("inventory/row/7K/index.html has no %q", want)
		}
	}
	if card := readFile(t, out, "inventory/row/3C/index.html"); !strings.Contains(card, "None yet.") {
		t.Error("the card of a row with no node does not say so")
	}
	if other := readFile(t, out, "inventory/other/index.html"); !strings.Contains(other, "a test") {
		t.Error("inventory/other/index.html does not give the reason of the outcome")
	}
	var s inventory.Summary
	if err := json.Unmarshal([]byte(readFile(t, out, "api/inventory.json")), &s); err != nil || s.Rows != 2 || s.Open != 1 {
		t.Errorf("api/inventory.json: %+v, error %v", s, err)
	}
	checkLinks(t, out)
}

func TestLetterOf(t *testing.T) {
	for name, want := range map[string]string{"group": "G", "Ébauche": "#", "2-group": "#", "": "#", "z": "Z"} {
		if got := letterOf(name); got != want {
			t.Errorf("letterOf(%q) = %q, want %q", name, got, want)
		}
	}
}
