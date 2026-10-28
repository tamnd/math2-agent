package wpcorpus

import (
	"math/rand/v2"
	"testing"
)

func TestSystematic(t *testing.T) {
	xs := make([]int, 1000)
	for i := range xs {
		xs[i] = i
	}
	got := systematic(xs, 50, rand.New(rand.NewPCG(1, 1)))
	if len(got) != 50 {
		t.Fatalf("len = %d", len(got))
	}
	// The steps are equal, so each block of 20 gives one element.
	for i, x := range got {
		if x/20 != i {
			t.Errorf("element %d is %d, not in the block %d", i, x, i)
		}
	}
	if got := systematic(xs[:10], 50, rand.New(rand.NewPCG(1, 1))); len(got) != 10 {
		t.Errorf("a small pool gives %d elements", len(got))
	}
}

func TestGoldMatches(t *testing.T) {
	g := &GoldItem{Kind: "statement", Span: [2]int{100, 200}}
	for _, tt := range []struct {
		o    Object
		want bool
	}{
		{Object{Kind: "statement/theorem", Span: [2]int{100, 200}}, true},
		{Object{Kind: "statement", Span: [2]int{150, 400}}, true},
		{Object{Kind: "statement", Span: [2]int{170, 400}}, false},
		{Object{Kind: "definition", Span: [2]int{100, 200}}, false},
	} {
		if got := matches(g, &tt.o); got != tt.want {
			t.Errorf("matches(%s %v) = %v", tt.o.Kind, tt.o.Span, got)
		}
	}
}

func TestExcerpt(t *testing.T) {
	if got := excerpt("a  b\nc", 10); got != "a b c" {
		t.Errorf("excerpt = %q", got)
	}
	if got := excerpt("abcdef ghij", 7); got != "abcdef..." {
		t.Errorf("excerpt = %q", got)
	}
}
