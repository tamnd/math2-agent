package site

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
)

const fixtures = "../testdata/nodes/valid"

// buildFixtures builds the site of the fixture tree. With status true, the site reads the store of a build of the tree.
func buildFixtures(t *testing.T, status bool) string {
	t.Helper()
	opt := Options{Content: fixtures, Out: filepath.Join(t.TempDir(), "site"), URL: "https://example.org/math2", Repo: "o/math2", AgentRepo: "o/math2-agent", Version: "test"}
	if status {
		opt.Build = t.TempDir()
		if _, err := build.Run(context.Background(), build.Options{Root: fixtures, Out: opt.Build}); err != nil {
			t.Fatal(err)
		}
	}
	if err := Build(opt); err != nil {
		t.Fatal(err)
	}
	return opt.Out
}

func TestExplore(t *testing.T) {
	out := buildFixtures(t, true)

	// Lagrange's theorem: the closure, the status and the dependents.
	page := readNode(t, out, "Q529ES")
	for _, want := range []string{
		`<h2 id="closure-h">Reading path</h2>`,
		`<svg class="dag" viewBox=`,
		`aria-labelledby="closure-caption"`,
		`<figcaption id="closure-caption">`,
		`class="box ev-e1 root"`,
		`<span class="swatch ev-e1" aria-hidden="true"></span>E1 claimed</li>`,
		`<tr><th>Evidence</th><td>E1 claimed`,
		`sorry_in_closure`,
		`<h2 id="usedby-h">Where this leads</h2>`,
		`Why the build gives this status`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page of Q529ES has no %q", want)
		}
	}

	// Finite groups: the blueprint marks the members of the topic.
	page = readNode(t, out, "ZZZJE6")
	for _, want := range []string{`<h2 id="blueprint-h">Everything in this topic</h2>`, `aria-labelledby="blueprint-caption"`, `class="box ev-e1 member"`, `<span class="kind-word">member</span>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page of ZZZJE6 has no %q", want)
		}
	}

	// The frontier lists the statement with no proof and the open conjectures, and no resolved problem.
	page = readFile(t, out, "explore/frontier/index.html")
	for _, want := range []string{"First Sylow theorem", "Collatz conjecture", "Chromatic number of the plane", "E1 <span class=\"muted\">claimed</span>"} {
		if !strings.Contains(page, want) {
			t.Errorf("the frontier has no %q", want)
		}
	}
	for _, not := range []string{">Lagrange&#39;s theorem<", "Fermat"} {
		if strings.Contains(page, not) {
			t.Errorf("the frontier has %q", not)
		}
	}
}

func TestExploreNoBuild(t *testing.T) {
	out := buildFixtures(t, false)
	page := readNode(t, out, "Q529ES")
	if !strings.Contains(page, "This copy of the site has no build data") {
		t.Error("a site with no build does not say that it has no status")
	}
	if strings.Contains(page, `<tr><th>Evidence</th><td>E1 claimed`) || strings.Contains(page, "ev-e1") {
		t.Error("a site with no build shows an evidence level")
	}
	// The closure comes from the graph, so it needs no build.
	if !strings.Contains(page, `<svg class="dag"`) {
		t.Error("a site with no build has no closure figure")
	}
	if !strings.Contains(readFile(t, out, "explore/frontier/index.html"), "First Sylow theorem") {
		t.Error("a site with no build has no frontier")
	}
}

func TestSVGRows(t *testing.T) {
	var layer []*dagItem
	for i := range 12 {
		layer = append(layer, &dagItem{link: link{Tag: string(rune('A' + i)), Text: "x", URL: "/x"}})
	}
	s := string(svg("c", [][]*dagItem{layer}, 5))
	// 12 boxes in rows of 5 give 3 rows.
	want := `viewBox="0 0 952 218"`
	if !strings.Contains(s, want) {
		t.Errorf("the figure has no %s: %.120s", want, s)
	}
}

func TestClip(t *testing.T) {
	if got := clip("Lagrange's theorem", 30); got != "Lagrange's theorem" {
		t.Errorf("clip of a short name gives %q", got)
	}
	if got := clip("αβγδε", 3); got != "αβ…" {
		t.Errorf("clip gives %q", got)
	}
}
