package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArticleViews(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/7K3/7K3QX2.md", statement)
	writeFile(t, content, "records/parity/wikipedia/AAA.jsonl", strings.Join([]string{
		`{"article":"wikipedia://en/31150","title":"Lagrange's theorem (group theory)","revision":7,"mention":"wikipedia://en/31150","kind":"statement","name":"Lagrange's theorem","row":"R1","outcome":"node","target":["7K3QX2"],"assessed":"B/High"}`,
		`{"article":"wikipedia://en/31150","title":"Lagrange's theorem (group theory)","revision":7,"mention":"wikipedia://en/31150#EXM1","kind":"example","name":"Example","row":"R2","outcome":"deferred","reason":"the item has no name of its own","until":"2027-05-01"}`,
		`{"article":"wikipedia://en/31150","title":"Lagrange's theorem (group theory)","revision":7,"mention":"wikipedia://en/31150#PRF1","kind":"proof","outcome":"open"}`,
		`{"article":"wikipedia://en/1","title":"A","revision":3,"mention":"wikipedia://en/1","kind":"topic","name":"A","outcome":"out_of_scope","reason":"a letter"}`,
		"",
	}, "\n"))
	out := buildSite(t, content, "")
	view := readFile(t, out, "wikipedia/lagranges-theorem-group-theory/index.html")
	for _, want := range []string{
		`<a href="https://en.wikipedia.org/wiki/Lagrange%27s_theorem_%28group_theory%29">`,
		`<a href="https://en.wikipedia.org/w/index.php?oldid=7">revision 7</a>, with the assessment B/High`,
		`<td>subject</td><td>statement</td><td>Lagrange&#39;s theorem</td><td>node</td><td><a href="/math2/wiki/lagranges-theorem/">Lagrange&#39;s theorem</a></td>`,
		`<td>#EXM1</td><td>example</td><td>Example</td><td>deferred until 2027-05-01</td><td></td><td>the item has no name of its own</td>`,
		"1 items have no outcome yet.",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the article view has no %s:\n%s", want, view)
		}
	}
	// The slug a is a letter of the lists, so the view of the article A gets the page id.
	if _, err := os.Stat(filepath.Join(out, "wikipedia", "a-1", "index.html")); err != nil {
		t.Errorf("the article A has no view at wikipedia/a-1/: %v", err)
	}
	list := readFile(t, out, "wikipedia/articles/index.html")
	for _, want := range []string{"The 2 articles have 4 items, and 1 items have no outcome yet.", `<a href="/math2/wikipedia/a-1/">A</a>`} {
		if !strings.Contains(list, want) {
			t.Errorf("the list of the views has no %s:\n%s", want, list)
		}
	}
	if strings.Contains(list, "Wikipedia scope") {
		t.Error("the list of the views links to the scope, and the site has no corpus")
	}
}
