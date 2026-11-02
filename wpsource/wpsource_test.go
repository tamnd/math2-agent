package wpsource

import "testing"

func TestParse(t *testing.T) {
	r, err := Parse("en:18902@r1250000000")
	if err != nil {
		t.Fatal(err)
	}
	if r.Page != 18902 || r.Revision != 1250000000 {
		t.Fatalf("Parse gave %+v", r)
	}
	if got := r.String(); got != "en:18902@r1250000000" {
		t.Errorf("String gave %q", got)
	}
	if got := r.URL(); got != "https://en.wikipedia.org/w/index.php?oldid=1250000000" {
		t.Errorf("URL gave %q", got)
	}
	if got := r.HistoryURL(); got != "https://en.wikipedia.org/w/index.php?curid=18902&action=history" {
		t.Errorf("HistoryURL gave %q", got)
	}
	for _, bad := range []string{"", "18902@r1", "en:18902", "en:0@r1", "de:1@r1", "en:1@r", "en:1@r1x", "Lagrange's theorem"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) gave no error", bad)
		}
	}
}
