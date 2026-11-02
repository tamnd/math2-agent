package pack

import "testing"

func TestWikipediaSource(t *testing.T) {
	if got := sourceTitle("wikipedia", "en:18902@r1250000000"); got != "Wikipedia, page 18902, revision 1250000000" {
		t.Errorf("sourceTitle gave %q", got)
	}
	if got := sourceURL("wikipedia", "en:18902@r1250000000"); got != "https://en.wikipedia.org/w/index.php?oldid=1250000000" {
		t.Errorf("sourceURL gave %q", got)
	}
	if got := sourceURL("wikipedia", "Group theory"); got != "" {
		t.Errorf("sourceURL of a title gave %q", got)
	}
}
