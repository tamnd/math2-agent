package site

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, text string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// newContent returns a content tree with the files that each real tree has.
func newContent(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "schema/node.v1.json", "{}\n")
	writeFile(t, root, "macros/m2.katex.json", `{"\\Z": "\\mathbb{Z}"}`)
	return root
}

func buildSite(t *testing.T, content, spec string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "site")
	opt := Options{Content: content, Spec: spec, Out: out, URL: "https://example.org/math2", Repo: "o/math2", AgentRepo: "o/math2-agent", Version: "test", Current: "M0",
		Milestones: []Milestone{{ID: "M0", Title: "the skeleton", URL: "https://example.org/m0"}}}
	if err := Build(opt); err != nil {
		t.Fatal(err)
	}
	return out
}

const statement = `---
tag: 7K3QX2
kind: statement
role: theorem
name: Lagrange's theorem
msc: [20D60]
uses: [1H8D2C]
---

## Statement

Let $G$ be a finite [[group|1H8D2C]], for example $\Z$. See also [[a missing node|ZZZZZZ]].

$$
|G| = [G : H] \cdot |H|
$$
`

const concept = `---
tag: 1H8D2C
kind: concept
name: Group
---

## Summary

A group. $\nosuch{x}$

` + "```markdown\n[[not a link|7K3QX2]]\n```\n"

func TestEmptyTree(t *testing.T) {
	out := buildSite(t, t.TempDir(), "")
	for _, f := range []string{"index.html", "404.html", ".nojekyll", "nodes/index.html", "msc/index.html", "msc/20/index.html", "search/index.html", "agents/index.html", "llms.txt", "llms-full.txt", "sitemap.xml", "robots.txt", "api/nodes.json", "api/search.json", "assets/site.css", "assets/site.js"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("the site has no %s", f)
		}
	}
	if !strings.Contains(readFile(t, out, "index.html"), "The graph has no nodes yet") {
		t.Error("the home page does not say that the graph is empty")
	}
	var index struct{ Count int }
	if err := json.Unmarshal([]byte(readFile(t, out, "api/nodes.json")), &index); err != nil || index.Count != 0 {
		t.Errorf("api/nodes.json: count %d, error %v", index.Count, err)
	}
	if _, err := os.Stat(filepath.Join(out, "curate", "index.html")); err == nil || strings.Contains(readFile(t, out, "llms.txt"), "curate.md") {
		t.Error("a tree with no CURATION.md has a Curation page")
	}
}

func TestDocs(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "CONTRIBUTING.md", "# Contributing\n\nRead [the guide](CURATION.md) and [the readme](README.md).\n")
	writeFile(t, content, "CURATION.md", "# Curation of alignment proposals\n\nRead [CONTRIBUTING.md](./CONTRIBUTING.md) first.\n")
	out := buildSite(t, content, "")
	contribute := readFile(t, out, "contribute/index.html")
	if !strings.Contains(contribute, `href="/math2/curate/"`) || !strings.Contains(contribute, `href="https://github.com/o/math2/blob/main/README.md"`) {
		t.Errorf("the Contribute page does not link to the Curation page and the readme:\n%s", contribute)
	}
	if !strings.Contains(readFile(t, out, "curate/index.html"), `href="/math2/contribute/"`) {
		t.Error("the Curation page does not link to the Contribute page")
	}
	if readFile(t, out, "curate.md") != readFile(t, content, "CURATION.md") {
		t.Error("curate.md is not a copy of CURATION.md")
	}
	if !strings.Contains(readFile(t, out, "llms.txt"), "- [Curation](https://example.org/math2/curate.md)") {
		t.Error("llms.txt does not list the Curation page")
	}
}

func TestNodes(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/7K3/7K3QX2.md", statement)
	writeFile(t, content, "nodes/1H8/1H8D2C.md", concept)
	out := buildSite(t, content, "")

	page := readFile(t, out, "node/7K3QX2/index.html")
	for _, want := range []string{
		`<a href="/math2/node/1H8D2C/">group</a>`,
		`class="missing"`,
		`<span class="math"><span class="katex"><span class="katex-mathml"><math xmlns="http://www.w3.org/1998/Math/MathML">`,
		`<annotation encoding="application/x-tex">G</annotation>`,
		`<mi mathvariant="double-struck">Z</mi>`,
		`<div class="math display"><span class="katex-display">`,
		`<link rel="stylesheet" href="/math2/assets/katex/katex.min.css">`,
		`m2://7K3QX2`,
		`<a href="/math2/msc/20/"`,
		`<link rel="canonical" href="https://example.org/math2/node/7K3QX2/">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the node page does not contain %s", want)
		}
	}

	group := readFile(t, out, "node/1H8D2C/index.html")
	if !strings.Contains(group, "What links here") || !strings.Contains(group, `<a href="/math2/node/7K3QX2/">Lagrange&#39;s theorem</a>`) {
		t.Error("the backlink from 7K3QX2 is not on the page of 1H8D2C")
	}
	if !strings.Contains(group, `<code class="tex-error" title="KaTeX parse error: Undefined control sequence: \nosuch`) {
		t.Error("a formula that KaTeX cannot parse does not show as TeX")
	}
	for _, f := range []string{"assets/katex/katex.min.css", "assets/katex/fonts/KaTeX_Main-Regular.woff2", "assets/katex/LICENSE"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Error(err)
		}
	}
	if strings.Contains(page, "cdn.jsdelivr.net") {
		t.Error("the node page loads a file from a CDN")
	}
	if !strings.Contains(group, "[[not a link|7K3QX2]]") {
		t.Error("a wiki link in fenced code changed")
	}

	if got := readFile(t, out, "node/7K3QX2.md"); got != statement {
		t.Error("node/7K3QX2.md is not the node file byte for byte")
	}
	var node struct {
		Kind      string
		Links     []jsonLink
		Backlinks []jsonLink
	}
	if err := json.Unmarshal([]byte(readFile(t, out, "api/node/1H8D2C.json")), &node); err != nil {
		t.Fatal(err)
	}
	if node.Kind != "concept" || len(node.Backlinks) != 1 || node.Backlinks[0] != (jsonLink{Field: "uses", Target: "7K3QX2"}) {
		t.Errorf("api/node/1H8D2C.json: %+v", node)
	}
	if !strings.Contains(readFile(t, out, "llms.txt"), "- [Group](https://example.org/math2/node/1H8D2C.md): concept 1H8D2C") {
		t.Error("llms.txt does not list the node")
	}
	checkLinks(t, out)
}

func TestSpec(t *testing.T) {
	spec := t.TempDir()
	writeFile(t, spec, "00-README.md", "# Overview\n\nRead [the goal](02-the-goal.md#scope) and [a file](../README.md).\n")
	writeFile(t, spec, "02-the-goal.md", "# 02. The goal\n\n## Scope\n\nAll of it.\n")
	out := buildSite(t, newContent(t), spec)
	page := readFile(t, out, "spec/00-README/index.html")
	if !strings.Contains(page, `href="/math2/spec/02-the-goal/#scope"`) {
		t.Error("the link to another spec document is not rewritten")
	}
	if !strings.Contains(page, `href="https://github.com/o/math2-agent/blob/main/spec/../README.md"`) {
		t.Error("the link to a file is not a GitHub link")
	}
	if !strings.Contains(page, "Next: 02. The goal") {
		t.Error("the page has no link to the next document")
	}
	checkLinks(t, out)
}

func TestSpecParts(t *testing.T) {
	spec := t.TempDir()
	writeFile(t, spec, "00-README.md", "# Overview\n\nThe [corpus spec](corpus/00-README.md).\n")
	writeFile(t, spec, "corpus/00-README.md", "# The corpus\n\nRead [part A](01a-reference-works.md#k) and [the main goal](../02-the-goal.md).\n")
	writeFile(t, spec, "corpus/01a-reference-works.md", "# K01, part A\n\n## K\n\nText.\n")
	writeFile(t, spec, "02-the-goal.md", "# 02. The goal\n")
	out := buildSite(t, newContent(t), spec)
	page := readFile(t, out, "spec/corpus/00-README/index.html")
	for _, want := range []string{`href="/math2/spec/corpus/01a-reference-works/#k"`, `href="/math2/spec/02-the-goal/"`, "Next: K01, part A", "blob/main/spec/corpus/00-README.md"} {
		if !strings.Contains(page, want) {
			t.Errorf("the corpus README has no %s", want)
		}
	}
	if strings.Contains(page, "Next: 02. The goal") {
		t.Error("the pager of a part goes to the main spec")
	}
	if page := readFile(t, out, "spec/00-README/index.html"); !strings.Contains(page, `href="/math2/spec/corpus/00-README/"`) {
		t.Error("the link from the main spec to the corpus spec is not rewritten")
	}
	if index := readFile(t, out, "spec/index.html"); !strings.Contains(index, "<h2>The corpus</h2>") {
		t.Error("the spec index has no heading for the corpus part")
	}
	readFile(t, out, "spec/corpus/01a-reference-works.md")
	checkLinks(t, out)
}

var href = regexp.MustCompile(`(?:href|src)="(/math2/[^"#?]*)`)

// checkLinks checks that each link inside the site goes to a file of the site.
func checkLinks(t *testing.T, out string) {
	t.Helper()
	err := filepath.WalkDir(out, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range href.FindAllStringSubmatch(string(data), -1) {
			target := strings.TrimPrefix(m[1], "/math2/")
			if target == "" || strings.HasSuffix(target, "/") {
				target += "index.html"
			}
			if target == "node/ZZZZZZ/index.html" {
				continue // the missing node of the test
			}
			if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(target))); err != nil {
				rel, _ := filepath.Rel(out, p)
				t.Errorf("%s: the link %s goes to no file", rel, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
