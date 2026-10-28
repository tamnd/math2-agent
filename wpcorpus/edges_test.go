package wpcorpus

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func edgeContext() EdgeContext {
	scope := map[string]int{"Subgroup": 200, "Widget theorem": 300, "Order (group theory)": 400, "Lagrange's theorem (group theory)": 500, "Coset": 600, "Gizmo": 700, "Gadget": 800, "Group theory": 900, "Gadget theorem": 1000}
	all := map[string]int{"Ada Lovelace": 50, "Euler": 51}
	types := map[int]string{900: TypeField, 500: TypeStatement, 400: TypeConcept, 200: TypeConcept, 1000: TypeStatement}
	return EdgeContext{
		Scope: func(t string) int { return scope[t] },
		Page: func(t string) (int, bool) {
			id, ok := all[t]
			return id, ok
		},
		Objects: func(page int) []Object {
			if page == 500 {
				return []Object{
					{Kind: "section", Cue: "section", SectionAnchor: "Statement", Tag: "S000"},
					{Kind: "statement/theorem", Cue: "heading", SectionAnchor: "Statement", Tag: "AB12"},
				}
			}
			if page == 600 {
				return []Object{{Kind: "section", Cue: "section", SectionAnchor: "Definition", Tag: "S000"}}
			}
			return nil
		},
		Type: func(page int) string { return types[page] },
	}
}

func TestBuildEdges(t *testing.T) {
	r := extractFile(t, "edges.wiki", "Widget theorem (group theory)")
	for i := range r.Objects {
		r.Objects[i].Tag = fmt.Sprintf("T%03d", i)
	}
	tag := func(cue, name string) string {
		o := find(r, cue, name)
		if o == nil {
			t.Fatalf("no object %s %s", cue, name)
		}
		return MentionID(12345, o.Tag)
	}
	art := MentionID(12345, "")
	lemma := tag("math-theorem", "Widget lemma")
	proof := tag("heading", "Proof")
	f := BuildEdges(r, edgeContext())
	want := []string{
		art + " wikipedia://en/900 in-field infobox high",
		art + " 50 proved-by infobox high",
		art + " wikipedia://en/1000 specializes infobox high",
		art + " wikipedia://en/900 in-field link-in-lead high",
		art + " wikipedia://en/200 uses link-in-lead medium",
		art + " Missing page uses link-in-lead medium red-link",
		art + " wikipedia://en/700 specializes link-in-lead medium",
		tag("heading", "Statement") + " wikipedia://en/300 restates hatnote high",
		lemma + " wikipedia://en/400 uses link-in-item high",
		lemma + " " + proof + " uses anchor certain",
		lemma + " " + art + " uses anchor certain broken-anchor",
		proof + " wikipedia://en/500#AB12 uses named-link high",
		proof + " wikipedia://en/600 uses link-in-proof medium broken-anchor",
		proof + " " + proof + " uses anchor certain",
		art + " wikipedia://en/800 related hatnote medium",
		art + " 51 related link-in-text low",
		art + " " + art + " uses anchor certain",
		art + " wikipedia://en/800 related see-also low",
		art + " Template:Group theory sidebar in-nav nav low",
		art + " Category:Theorems in group theory kind-of category medium",
		art + " Category:Group theory in-field category medium",
	}
	var got []string
	for _, e := range f.Edges {
		to := e.To
		if e.Page != 0 {
			to = fmt.Sprint(e.Page)
		}
		if e.Target != "" {
			to = e.Target
		}
		s := strings.TrimSpace(fmt.Sprintf("%s %s %s %s %s %s", e.From, to, e.Predicate, e.Method, e.Confidence, e.Finding))
		got = append(got, s)
		if e.Revision != 678 {
			t.Errorf("edge %s has revision %d", s, e.Revision)
		}
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("edges:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, e := range f.Edges {
		if e.Method == MethodLinkInLead && e.To == "wikipedia://en/200" && e.Label != "" {
			t.Errorf("label %q: a label that is the title with a lower case letter is not kept", e.Label)
		}
	}

	data, err := MarshalEdges(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("| infobox | in-field | high | 1 |")) || !bytes.Contains(data, []byte("1 red link")) {
		t.Errorf("edge file:\n%s", data)
	}
	root := t.TempDir()
	if err := WriteEdges(root, f); err != nil {
		t.Fatal(err)
	}
	back, err := ReadEdges(root, 12345)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Edges) != len(f.Edges) || back.Edges[0] != f.Edges[0] {
		t.Errorf("read back %d edges, first %+v", len(back.Edges), back.Edges[0])
	}

	var s EdgeSummary
	s.Add(f)
	s.Add(f)
	s.Add(f)
	var b bytes.Buffer
	if err := EdgeReport(&b, s, map[int]string{50: "Ada Lovelace"}); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"| Ada Lovelace (50) | 3 |", "| 51 | 3 |", "| Missing page | 3 |", "| link-in-lead | 12 |"} {
		if !strings.Contains(b.String(), w) {
			t.Errorf("the report has no %q:\n%s", w, b.String())
		}
	}
}

func TestEdgeWithoutContext(t *testing.T) {
	r := extractFile(t, "edges.wiki", "Widget theorem (group theory)")
	f := BuildEdges(r, EdgeContext{})
	for _, e := range f.Edges {
		if e.Finding == FindingRedLink || e.Page != 0 {
			t.Errorf("edge %+v: with no context, no edge has a page id or a red link", e)
		}
		if e.Method == "" || e.Confidence == "" || e.Predicate == "" {
			t.Errorf("edge %+v has no method, predicate or confidence", e)
		}
	}
}

func TestEdgePhrasesManifest(t *testing.T) {
	m, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.EdgePhrases()) != len(defaultEdgePhrases) || m.EdgesVersion != 0 {
		t.Errorf("phrases %d, version %d", len(m.EdgePhrases()), m.EdgesVersion)
	}
}

func TestGlossaryTermLink(t *testing.T) {
	m, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	src := "A glossary.\n\n== A ==\n{{glossary}}\n{{term|1=abelian group}}\n{{defn|1=A [[Group (mathematics)|group]] is [[Abelian group|abelian]] if it commutes.}}\n{{term|1=automorphism}}\n{{defn|1=An [[group automorphism|automorphism]] is an [[isomorphism]] to itself.}}\n{{term|1=coset}}\n{{defn|1=A set of the form [[Subset|gH]].}}\n{{glossary end}}\n"
	r, err := Extract(&Page{ID: 1, Revision: 2, Title: "Glossary of group theory", Wikitext: src}, m, "wpcorpus-test")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range r.Links {
		if u.Method == MethodGlossary {
			got = append(got, u.Title)
		}
	}
	if strings.Join(got, ", ") != "Abelian group, Group automorphism" {
		t.Errorf("glossary links %q", got)
	}
}
