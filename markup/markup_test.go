package markup

import (
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
)

func TestParse(t *testing.T) {
	src := "A [[group|GY1XDB]] has $e$.\n\n$$\nab = ba\n$$\n\nThe end, $$x$$."
	doc, source := Parse(src)
	var inline, display []string
	var links []string
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *Math:
			if n.Display {
				display = append(display, string(n.TeX))
			} else {
				inline = append(inline, string(n.TeX))
			}
		case *ast.Link:
			if t, ok := n.FirstChild().(*ast.Text); ok {
				links = append(links, string(n.Destination)+" "+string(t.Value(source)))
			}
		}
		return ast.WalkContinue, nil
	})
	if strings.Join(inline, ",") != "e" {
		t.Errorf("the inline math is %q", inline)
	}
	if len(display) != 2 || strings.TrimSpace(display[0]) != "ab = ba" || display[1] != "x" {
		t.Errorf("the display math is %q", display)
	}
	if strings.Join(links, ",") != NodeScheme+"GY1XDB group" {
		t.Errorf("the links are %q", links)
	}
	if doc.ChildCount() != 3 {
		t.Errorf("the document has %d blocks", doc.ChildCount())
	}
}
