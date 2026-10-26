package style

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
)

func parse(t *testing.T, kind, body string) *node.Node {
	t.Helper()
	data := "---\ntag: 3C53TD\nkind: " + kind + "\nname: A node\n---\n\n" + body
	n, err := node.Parse("nodes/3C5/3C53TD.md", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// words returns a sentence of n words.
func words(n int) string {
	return strings.TrimSpace(strings.Repeat("word ", n-1)) + " end."
}

func TestNode(t *testing.T) {
	cases := []struct {
		name, kind, body string
		want             []string // rule, line and column of each finding
	}{
		{"short", "statement", "## Statement\n\n" + words(25) + "\n", nil},
		{"long main", "statement", "## Statement\n\n" + words(26) + "\n", []string{"Y01:9:1"}},
		{"long other", "statement", "## Statement\n\nA short one.\n\n## Remarks\n\n" + words(30) + " " + words(31) + "\n", []string{"Y01:13:151"}},
		{"main of kind", "concept", "## Summary\n\n" + words(26) + "\n\n## Remarks\n\n" + words(26) + "\n", []string{"Y01:9:1"}},
		{"formula is one word", "statement", "## Statement\n\nLet $G = \\langle a, b \\mid a^2 = b^3 = 1 \\rangle$ be a group, with `x y z` and \\(a + b + c\\). " + words(15) + "\n", nil},
		{"word touches a formula", "statement", "## Statement\n\n" + strings.Repeat("the $n$th ", 12) + "end.\n", nil},
		{"display formula ends", "statement", "## Statement\n\n" + strings.Repeat("word ", 20) + "\n$$x = 1 + 2 + 3.$$\n" + words(20) + "\n", nil},
		{"display formula goes on", "statement", "## Statement\n\n" + strings.Repeat("word ", 20) + "\n$$x = 1$$\n" + words(20) + "\n", []string{"Y01:9:1"}},
		{"blank line ends", "statement", "## Statement\n\n" + strings.Repeat("word ", 20) + "\n\n" + words(20) + "\n", nil},
		{"list item ends", "proof", "## Proof\n\n1. " + strings.Repeat("word ", 20) + "\n2. " + words(20) + "\n", nil},
		{"abbreviation", "statement", "## Statement\n\n" + strings.Repeat("word ", 12) + "e.g. J. Smith " + words(12) + "\n", []string{"Y01:9:1"}},
		{"code block and table", "example", "## Example\n\n```\n" + strings.Repeat("word ", 40) + "\n```\n\n| " + strings.Repeat("a | ", 40) + "\n", nil},
		{"link reads as text", "statement", "## Statement\n\n[[" + strings.Repeat("word ", 20) + "|3C53TD]] [x](https://example.org/a/b/c) " + words(4) + "\n", nil},
		{"filler", "proof", "## Proof\n\nClearly $x = 1$. It is easy\nto see that $y = 2$. The step is unclear.\n", []string{"Y02:9:1", "Y02:9:18"}},
		{"we in statement", "statement", "## Statement\n\nIf we let $x = 1$, then our claim holds. Let us go.\n\n## Remarks\n\nWe can also say it.\n", []string{"Y03:9:4", "Y03:9:25", "Y03:9:46"}},
		{"we in proof", "proof", "## Proof\n\nWe apply the lemma.\n", nil},
		{"place", "statement", "## Statement\n\nThe theorem in Section 3 holds.\n\n## Remarks\n\nSee the previous chapter, and § 4, and this section. The table shows it.\n", []string{"Y04:9:16", "Y04:13:5", "Y04:13:31", "Y04:13:40"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, f := range Node(parse(t, c.kind, c.body)) {
				got = append(got, fmt.Sprintf("%s:%d:%d", f.Rule, f.Line, f.Col))
			}
			if strings.Join(got, " ") != strings.Join(c.want, " ") {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestLint(t *testing.T) {
	long := parse(t, "statement", "## Statement\n\nClearly "+words(30)+"\n")
	res, fs := Lint([]*node.Node{long}, nil)
	if len(res) != len(Rules()) || len(fs) != 2 {
		t.Fatalf("got %d results and %d findings", len(res), len(fs))
	}
	want := map[string]check.State{"Y01": check.Found, "Y02": check.Found, "Y03": check.Pass, "Y04": check.Pass}
	for _, r := range res {
		if r.State != want[r.Rule] {
			t.Errorf("%s: state %s, want %s", r.Rule, r.State, want[r.Rule])
		}
	}
	res, _ = Lint([]*node.Node{long}, func(tag string) bool { return tag == "3C53TD" })
	if res[0].State != check.Fail || res[1].State != check.Fail || res[2].State != check.Pass {
		t.Errorf("strict: got %v", res)
	}
	res, fs = Lint(nil, nil)
	if len(fs) != 0 || res[0].State != check.NA {
		t.Errorf("no nodes: got %v and %v", res, fs)
	}
}

func TestMessages(t *testing.T) {
	fs := Node(parse(t, "concept", "## Summary\n\n"+words(26)+"\n\n## Remarks\n\nObviously, see Chapter 2.\n"))
	want := []string{
		"nodes/3C5/3C53TD.md:9:1: Y01: the sentence has 26 words, and the limit in the section Summary is 25",
		`nodes/3C5/3C53TD.md:13:1: Y02: "Obviously" hides a step: give the step, or make it a lemma`,
		`nodes/3C5/3C53TD.md:13:16: Y04: "Chapter 2" is a reference to place, and a node has no place: link to the node`,
	}
	if len(fs) != len(want) {
		t.Fatalf("got %v", fs)
	}
	for i, f := range fs {
		if f.String() != want[i] {
			t.Errorf("got %s\nwant %s", f, want[i])
		}
	}
}
