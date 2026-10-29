package site

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSplitBody(t *testing.T) {
	body := "## Summary\n\nA short summary with $x$.\n\nMore text.\n\n## Definition\n\nThe definition.\n\n## Explanations\n\n### L1 human\n\nA picture to hold in the head.\n\n### L2 agent\n\nA closer look.\n\n```\n### not a heading\n```\n\n## Examples\n\nAn example.\n"
	r := splitBody(body)
	if !strings.HasPrefix(r.Lead, "## Summary") || strings.Contains(r.Lead, "The definition") {
		t.Errorf("the lead is %q", r.Lead)
	}
	if r.Plain != "A picture to hold in the head." {
		t.Errorf("the plain words are %q", r.Plain)
	}
	for _, want := range []string{"## Definition", "## Examples", "## Going deeper\n\n### For a student\n\nA closer look.", "### not a heading"} {
		if !strings.Contains(r.Rest, want) {
			t.Errorf("the rest has no %q:\n%s", want, r.Rest)
		}
	}
	if strings.Contains(r.Rest, "agent") || strings.Contains(r.Rest, "L1 human") || strings.Contains(r.Rest, "## Explanations") {
		t.Errorf("the rest shows who wrote an explanation:\n%s", r.Rest)
	}
	if r.Excerpt != "A short summary with $x$." {
		t.Errorf("the excerpt is %q", r.Excerpt)
	}
	if got := splitBody("## Statement\n\n$$\nx = y\n$$\n\nSo x is y.\n").Excerpt; got != "So x is y." {
		t.Errorf("the excerpt of a statement that starts with display math is %q", got)
	}
}

// git runs git in dir as a fixed person at a fixed time, so that the history of the test does not depend on the machine.
func git(t *testing.T, dir, at string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=test@example.org", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+at, "GIT_COMMITTER_DATE="+at)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

const feedGroup = `---
tag: 1H8D2C
kind: concept
name: Group
msc: [20A05]
---

## Summary

A set with an operation that can be undone.
`

const feedGroupDef = `---
tag: 2B4C6D
kind: definition
name: Definition of a group
defines: 1H8D2C
---

## Definition

A set with an associative operation, a unit and inverses.
`

const feedCoset = `---
tag: 3C5D7E
kind: concept
name: Coset
msc: [20A05]
uses: [1H8D2C]
---

## Summary

A subgroup moved by one element.
`

func TestFeeds(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	content := newContent(t)
	git(t, content, "2026-09-01T10:00:00Z", "init", "-q")
	writeFile(t, content, "nodes/1H8/1H8D2C.md", feedGroup)
	writeFile(t, content, "nodes/2B4/2B4C6D.md", feedGroupDef)
	git(t, content, "2026-09-01T10:00:00Z", "add", ".")
	git(t, content, "2026-09-01T10:00:00Z", "commit", "-q", "-m", "a group")
	writeFile(t, content, "nodes/3C5/3C5D7E.md", feedCoset)
	git(t, content, "2026-09-03T10:00:00Z", "add", ".")
	git(t, content, "2026-09-03T10:00:00Z", "commit", "-q", "-m", "a coset")
	writeFile(t, content, "nodes/1H8/1H8D2C.md", strings.Replace(feedGroup, "undone.", "undone, as a turn of a square.", 1))
	git(t, content, "2026-09-05T10:00:00Z", "commit", "-q", "-am", "a better group")
	out := buildSite(t, content, "")

	latest := readFile(t, out, "latest/index.html")
	g, c := strings.Index(latest, ">Group</a>"), strings.Index(latest, ">Coset</a>")
	if g < 0 || c < 0 || g > c {
		t.Fatalf("Latest does not show the group, changed last, before the coset:\n%s", latest)
	}
	for _, want := range []string{`<h2 class="day">5 September 2026</h2>`, `<h2 class="day">3 September 2026</h2>`, `<time datetime="2026-09-05T10:00:00Z">`, `<span class="tag-rev">Revised</span>`, `<span class="tag-new">New</span>`, "as a turn of a square", `Builds on <a href="/math2/wiki/group/">Group</a>`} {
		if !strings.Contains(latest, want) {
			t.Errorf("Latest has no %s", want)
		}
	}
	if strings.Contains(latest, "Definition of a group") {
		t.Error("Latest has a card for the definition of a concept that has its own card")
	}
	if strings.Contains(latest, "no history of changes") {
		t.Error("Latest says that the build has no history")
	}

	atom := readFile(t, out, "latest/feed.xml")
	for _, want := range []string{`<feed xmlns="http://www.w3.org/2005/Atom">`, "<title>Group</title>", "<updated>2026-09-05T10:00:00Z</updated>", `<link href="https://example.org/math2/latest/feed.xml" rel="self"></link>`} {
		if !strings.Contains(atom, want) {
			t.Errorf("the Atom feed has no %s:\n%s", want, atom)
		}
	}

	discover := readFile(t, out, "discover/index.html")
	if strings.Count(discover, `<article class="slide" `) != 2 || !strings.Contains(discover, `class="s-discover bare"`) || strings.Contains(discover, `<footer`) {
		t.Errorf("Discover does not have one full card for each of the two nodes:\n%s", discover)
	}

	page := readNode(t, out, "1H8D2C")
	if !strings.Contains(page, "<tr><th>Changed</th><td>5 September 2026, first written 1 September 2026</td></tr>") {
		t.Error("the page of the group does not say when it changed")
	}
	home := readFile(t, out, "index.html")
	if !strings.Contains(home, "Just added") || !strings.Contains(home, `href="/math2/latest/"`) {
		t.Error("the home page does not show the latest nodes")
	}
}

func TestFeedsNoHistory(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/1H8/1H8D2C.md", feedGroup)
	out := buildSite(t, content, "")
	latest := readFile(t, out, "latest/index.html")
	if !strings.Contains(latest, "no history of changes") || strings.Contains(latest, "<time") || strings.Contains(latest, "tag-new") {
		t.Errorf("Latest of a tree with no git history shows dates:\n%s", latest)
	}
	if strings.Contains(readNode(t, out, "1H8D2C"), "<th>Changed</th>") {
		t.Error("a page of a tree with no git history says when it changed")
	}
}

func TestSubjectOrder(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/1H8/1H8D2C.md", feedGroup)
	writeFile(t, content, "nodes/3C5/3C5D7E.md", feedCoset)
	// A concept has no uses of its own: it needs what its definition needs. The name sorts first, and the depth puts it last.
	writeFile(t, content, "nodes/4D6/4D6E8F.md", "---\ntag: 4D6E8F\nkind: concept\nname: Abelian group\nmsc: [20K99]\n---\n\n## Summary\n\nA group whose operation commutes.\n")
	writeFile(t, content, "nodes/5E7/5E7F9G.md", "---\ntag: 5E7F9G\nkind: definition\nname: Definition of an abelian group\ndefines: 4D6E8F\nuses: [3C5D7E]\n---\n\n## Definition\n\nA group in which each two elements commute.\n")
	out := buildSite(t, content, "")
	subject := readFile(t, out, "msc/20/index.html")
	if a, g := strings.Index(subject, ">Abelian group</a>"), strings.Index(subject, ">Group</a>"); a < 0 || a < g {
		t.Error("the page of the subject does not put a concept after what its definition needs")
	}
	g, c := strings.Index(subject, ">Group</a>"), strings.Index(subject, ">Coset</a>")
	if g < 0 || c < 0 || g > c {
		t.Errorf("the page of the subject does not put the group, which the coset needs, first")
	}
	if !strings.Contains(subject, "Start here") || !strings.Contains(subject, "<h2>Concepts <span class=\"count\">3</span></h2>") {
		t.Errorf("the page of the subject does not group its nodes for a learner:\n%s", subject)
	}
	if strings.Contains(subject, "1H8D2C") {
		t.Error("the page of the subject shows a tag")
	}
}
