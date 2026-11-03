package release

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
)

func TestAttribution(t *testing.T) {
	files := map[string]string{
		"nodes/VAF/VAF6B01Q8W.md": "---\ntag: VAF6B01Q8W\nkind: statement\nname: Extension\nsources:\n  - wikipedia: en:31150@r7\n    locator: Lagrange's theorem (group theory), section Extension\n    note: adopted\n---\n\n## Statement\n\nText.\n",
		"nodes/GN6/GN6WHHGBAE.md": "---\ntag: GN6WHHGBAE\nkind: proof\nname: Proof\nsources:\n  - wikipedia: en:31150@r9\n    locator: Lagrange's theorem (group theory), section Extension\n    note: adapted\n  - wikipedia: en:98759@r3\n    locator: Coset\n    note: adopted\n---\n\n## Proof\n\nText.\n",
		"nodes/564/564VVAQTNR.md": "---\ntag: 564VVAQTNR\nkind: concept\nname: Coset\nsources:\n  - wikipedia: en:98759@r3\n    locator: Coset, section Definition\n    note: adopted\n---\n\n## Summary\n\nText.\n",
		"nodes/RXW/RXWA8TF2RM.md": "---\ntag: RXWA8TF2RM\nkind: concept\nname: Cited\nsources:\n  - wikipedia: en:5555@r1\n    note: cited\n---\n\n## Summary\n\nText.\n",
		"nodes/T0B/T0BWYYR508.md": "---\ntag: T0BWYYR508\nkind: concept\nname: Gone\nretired: {reason: duplicate}\nsources:\n  - wikipedia: en:6666@r1\n    note: adopted\n---\n\n## Summary\n\nText.\n",
		"nodes/~/~1cb5b3fa.md":    "---\ntag: ~1cb5b3fa\nkind: concept\nname: Draft\nsources:\n  - wikipedia: en:7777@r1\n    note: adopted\n---\n\n## Summary\n\nText.\n",
	}
	var nodes []*node.Node
	for p, s := range files {
		n, err := node.Parse(p, []byte(s))
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, n)
	}
	arts := Attribution(nodes)
	if len(arts) != 2 || arts[0].Title != "Coset" || arts[1].Title != "Lagrange's theorem (group theory)" {
		t.Fatalf("got the articles %+v", arts)
	}
	if got := arts[0].Revisions; len(got) != 1 || strings.Join(got[0].Nodes, " ") != "564VVAQTNR GN6WHHGBAE" {
		t.Errorf("Coset has the revisions %+v", got)
	}
	if got := arts[1].Revisions; len(got) != 2 || got[0].Revision != 7 || got[1].Nodes[0] != "GN6WHHGBAE" || got[0].URL != "https://en.wikipedia.org/w/index.php?oldid=7" {
		t.Errorf("Lagrange's theorem has the revisions %+v", got)
	}
	if arts[1].History != "https://en.wikipedia.org/w/index.php?curid=31150&action=history" {
		t.Errorf("got the history %q", arts[1].History)
	}
	data, err := AttributionMarkdown(arts)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	head, body, ok := strings.Cut(strings.TrimPrefix(text, "---\n"), "---\n")
	if !ok {
		t.Fatalf("the file has no YAML header:\n%s", text)
	}
	var back struct {
		Licence  string    `yaml:"licence"`
		Articles int       `yaml:"articles"`
		Nodes    int       `yaml:"nodes"`
		List     []Article `yaml:"attribution"`
	}
	if err := yaml.Unmarshal([]byte(head), &back); err != nil {
		t.Fatal(err)
	}
	if back.Licence != AttributionLicence || back.Articles != 2 || back.Nodes != 3 || len(back.List) != 2 || back.List[1].Revisions[1].Revision != 9 {
		t.Errorf("the header reads back as %+v", back)
	}
	for _, s := range []string{"## Coset\n", "- Revision 7 (https://en.wikipedia.org/w/index.php?oldid=7): VAF6B01Q8W\n", "2 articles give text to 3 nodes."} {
		if !strings.Contains(body, s) {
			t.Errorf("the body has no %q:\n%s", s, body)
		}
	}
	if empty, err := AttributionMarkdown(nil); err != nil || !strings.Contains(string(empty), "attribution: []\n") {
		t.Errorf("the file of no article is %q (%v)", empty, err)
	}
}
