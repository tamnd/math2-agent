package site

import (
	"strings"
	"testing"
)

func TestAreas(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/AAA/AAAAAA.md", "---\ntag: AAAAAA\nkind: topic\nname: Group theory\nmsc: [\"20-XX\"]\n---\n## Summary\n\nGroup theory is the study of groups.\n\n## Remarks\n\n1. [[Subgroups|BBBBBB]]\n1. [[Groups|CCCCCC]]\n")
	writeFile(t, content, "nodes/BBB/BBBBBB.md", "---\ntag: BBBBBB\nkind: topic\nname: Subgroups\nmsc: [\"20A05\"]\ntopics: [AAAAAA]\n---\n## Summary\n\nSubgroups.\n")
	writeFile(t, content, "nodes/CCC/CCCCCC.md", "---\ntag: CCCCCC\nkind: topic\nname: Groups\nmsc: [\"20A05\"]\ntopics: [AAAAAA]\n---\n## Summary\n\nGroups.\n")
	writeFile(t, content, "nodes/DDD/DDDDDD.md", "---\ntag: DDDDDD\nkind: topic\nname: Proof methods\nmsc: [\"00A35\"]\n---\n## Summary\n\nMethods.\n")
	writeFile(t, content, "nodes/EEE/EEEEEE.md", "---\ntag: EEEEEE\nkind: concept\nname: Subgroup\nmsc: [\"20A05\"]\ntopics: [BBBBBB]\n---\n## Summary\n\nA subgroup.\n")
	out := buildSite(t, content, "")

	page := readFile(t, out, "areas/index.html")
	sub, grp := strings.Index(page, ">Subgroups</a>"), strings.Index(page, ">Groups</a>")
	if sub < 0 || grp < 0 || sub > grp {
		t.Error("the sub-areas are not on the Areas page in the order of the body of the area")
	}
	for _, want := range []string{"2 sub-areas, 1 idea", "Other topics", ">Proof methods</a>", `href="/math2/areas/" aria-current="page">Areas</a>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the Areas page does not contain %s", want)
		}
	}
	node := readNode(t, out, "EEEEEE")
	if !strings.Contains(node, `<a href="/math2/areas/">Areas</a> <span aria-hidden="true">/</span> <a href="/math2/wiki/group-theory/">Group theory</a> <span aria-hidden="true">/</span> <a href="/math2/wiki/subgroups/">Subgroups</a>`) {
		t.Error("the crumbs of a node do not show its area and sub-area")
	}
}

func TestAreaOrder(t *testing.T) {
	content := newContent(t)
	area := func(tag, sub, name, msc string) {
		writeFile(t, content, "nodes/"+tag[:3]+"/"+tag+".md", "---\ntag: "+tag+"\nkind: topic\nname: "+name+"\nmsc: [\""+msc+"\"]\n---\n## Summary\n\n"+name+" is an area.\n\n## Remarks\n\n1. [[Part of "+name+"|"+sub+"]]\n")
		writeFile(t, content, "nodes/"+sub[:3]+"/"+sub+".md", "---\ntag: "+sub+"\nkind: topic\nname: Part of "+name+"\nmsc: [\""+msc+"\"]\ntopics: ["+tag+"]\n---\n## Summary\n\nA part.\n")
	}
	area("AAAAAA", "BBBBBB", "Algebra", "20-XX")
	area("CCCCCC", "DDDDDD", "Logic", "03-XX")
	area("EEEEEE", "FFFFFF", "Analysis", "26-XX")
	writeFile(t, content, "topics/areas.yaml", "areas:\n  - AAAAAA # Algebra\n  - EEEEEE # Analysis\n")
	out := buildSite(t, content, "")

	page := readFile(t, out, "areas/index.html")
	alg, ana, log := strings.Index(page, ">Algebra</a>"), strings.Index(page, ">Analysis</a>"), strings.Index(page, ">Logic</a>")
	if alg < 0 || ana < 0 || log < 0 || alg > ana || ana > log {
		t.Errorf("the areas are not in the order of topics/areas.yaml, then by MSC: Algebra %d, Analysis %d, Logic %d", alg, ana, log)
	}
}
