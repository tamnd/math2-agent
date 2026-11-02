package site

import "testing"

func TestWikipediaSourceLink(t *testing.T) {
	got := sourceLinks([]any{map[string]any{"wikipedia": "en:18902@r1250000000", "locator": "Group (mathematics), Definition", "note": "adopted"}})
	if len(got) != 1 {
		t.Fatalf("sourceLinks gave %v", got)
	}
	l := got[0]
	if l.Text != "Wikipedia, page 18902, revision 1250000000" || l.URL != "https://en.wikipedia.org/w/index.php?oldid=1250000000" || l.Note != "Group (mathematics), Definition. adopted" {
		t.Errorf("sourceLinks gave %+v", l)
	}
}
