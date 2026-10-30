package census

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/scope"
)

func TestChapman(t *testing.T) {
	cases := []struct {
		nA, nB, m int
		want      float64
	}{
		{100, 100, 50, 101.0*101.0/51.0 - 1},
		{10, 10, 10, 10},
		{0, 0, 0, 0},
		{50, 40, 0, 51*41 - 1},
	}
	for _, c := range cases {
		if got := Chapman(c.nA, c.nB, c.m); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("Chapman(%d, %d, %d) = %v, want %v", c.nA, c.nB, c.m, got, c.want)
		}
	}
}

func TestRecall(t *testing.T) {
	if got := Recall(50, 200); got != 0.25 {
		t.Errorf("Recall = %v", got)
	}
	if got := Recall(5, 0); got != 0 {
		t.Errorf("Recall with no estimate = %v", got)
	}
}

func TestPair(t *testing.T) {
	e := Pair(400, 300, 120, 7)
	if e.N != Chapman(400, 300, 120) {
		t.Errorf("N = %v", e.N)
	}
	if e.CI.Low >= e.N || e.CI.High <= e.N {
		t.Errorf("the interval %+v does not hold the estimate %v", e.CI, e.N)
	}
	if e.CI.Low < float64(400+300-120) {
		t.Errorf("the low end %v is below the works seen", e.CI.Low)
	}
	if f := Pair(400, 300, 120, 7); f != e {
		t.Errorf("the same seed gives %+v and %+v", e, f)
	}
	if f := Pair(10, 10, 10, 1); f.CI != (Interval{10, 10}) {
		t.Errorf("full overlap gives %+v", f.CI)
	}
	if f := Pair(0, 0, 0, 1); f.CI != (Interval{0, 0}) {
		t.Errorf("no work gives %+v", f.CI)
	}
}

func TestStrata(t *testing.T) {
	kinds := map[string]string{"article": "article", "preprint": "article", "textbook": "book", "thesis": "thesis", "lecture-notes": "report-notes", "course": "course", "software": "other"}
	for k, want := range kinds {
		if got := KindGroup(k); got != want {
			t.Errorf("KindGroup(%q) = %q, want %q", k, got, want)
		}
	}
	dates := map[string]string{"1976": "1970s", "2003-05-01": "2000s", "": "unknown", "19x5": "unknown"}
	for d, want := range dates {
		if got := Decade(d); got != want {
			t.Errorf("Decade(%q) = %q, want %q", d, got, want)
		}
	}
	langs := map[string]string{"en": "en", "ru-Cyrl": "ru", "ZH-Hans": "zh", "": "und"}
	for l, want := range langs {
		if got := Language(l); got != want {
			t.Errorf("Language(%q) = %q, want %q", l, got, want)
		}
	}
	if got := MSC2(&Record{Signals: scope.Record{MSC: []string{"26A42"}}}); got != "26" {
		t.Errorf("MSC2 = %q", got)
	}
	if got := MSC2(&Record{}); got != "none" {
		t.Errorf("MSC2 with no code = %q", got)
	}
}

func TestKeys(t *testing.T) {
	r := Record{
		Title:   "Principles of Mathematical Analysis",
		Authors: []string{"Walter Rudin"},
		IDs:     resource.IDs{"doi": {"https://doi.org/10.1000/XYZ"}, "isbn": {"0-07-054235-X"}, "mr": {"not an id"}},
	}
	got := strings.Join(Keys(&r), " ")
	for _, want := range []string{"doi:10.1000/xyz", "isbn:9780070542358", "ta:principles of mathematical analysis|rudin"} {
		if !strings.Contains(got, want) {
			t.Errorf("Keys = %q, want %q in it", got, want)
		}
	}
	if strings.Contains(got, "mr:") {
		t.Errorf("Keys = %q has a value that does not normalize", got)
	}
	r = Record{Title: "Analysis", Authors: []string{"Rudin, Walter"}}
	if k := Keys(&r); len(k) != 1 || k[0] != "ta:analysis|rudin" {
		t.Errorf("Keys with a family name first = %q", k)
	}
	if k := Keys(&Record{Title: "Analysis"}); len(k) != 0 {
		t.Errorf("Keys with no author = %q", k)
	}
}

func TestReservoir(t *testing.T) {
	s := NewReservoir[int](10, 3)
	for i := range 1000 {
		s.Add(i)
	}
	if len(s.Items()) != 10 || s.Seen() != 1000 {
		t.Fatalf("items %d, seen %d", len(s.Items()), s.Seen())
	}
	small := NewReservoir[int](10, 3)
	for i := range 4 {
		small.Add(i)
	}
	if len(small.Items()) != 4 {
		t.Errorf("a short stream gives %d items", len(small.Items()))
	}
}

func TestOffsets(t *testing.T) {
	o := Offsets(1000, 50, 9)
	if len(o) != 50 {
		t.Fatalf("len %d", len(o))
	}
	for i := 1; i < len(o); i++ {
		if o[i] <= o[i-1] || o[i] >= 1000 {
			t.Fatalf("offsets are not distinct and in order: %v", o)
		}
	}
	if o := Offsets(3, 10, 9); len(o) != 3 || o[2] != 2 {
		t.Errorf("Offsets(3, 10) = %v", o)
	}
}

// article gives an in article of the 1990s with a DOI from the number.
func article(n int) Record {
	return Record{
		NativeID: fmt.Sprint(n),
		Kind:     "article",
		Date:     "1995",
		Language: "en",
		IDs:      resource.IDs{"doi": {fmt.Sprintf("10.1000/%d", n)}},
		Signals:  scope.Record{MSC: []string{"11A41"}},
	}
}

func TestCensus(t *testing.T) {
	c := New(scope.Env{}, 500, 1)
	// A has the even works and B the works that 3 divides, of the works 0 to 999. The two are independent, the universe of the stratum is 1000 works, and 167 are in both.
	for i := range 1000 {
		if i%2 == 0 {
			c.Add("a", article(i))
		}
		if i%3 == 0 {
			c.Add("b", article(i))
		}
	}
	c.Add("a", Record{Kind: "article", Signals: scope.Record{MSC: []string{"62F10"}}})
	c.Add("a", Record{Kind: "article", Title: "No class"})
	a := c.Catalogue("a")
	if a.All["in"] != 500 || a.All["border"] != 1 || a.All["deferred"] != 1 {
		t.Fatalf("counts of a: %v", a.All)
	}
	if a.By[DimDecade]["1990s"]["in"] != 500 || a.By[DimMSC2]["62"]["border"] != 1 {
		t.Fatalf("counts by dimension of a: %v", a.By)
	}
	var ab Overlap
	for _, o := range c.Overlaps() {
		if o.A == "a" && o.B == "b" {
			ab = o
		}
	}
	if ab.Sampled != 500 {
		t.Fatalf("sampled %d", ab.Sampled)
	}
	if f := ab.Fraction(1); f < 0.25 || f > 0.42 {
		t.Errorf("the share of a in b is %v, want about 1/3", f)
	}
	est := c.Estimates([][2]string{{"a", "b"}})
	if len(est) != 1 {
		t.Fatalf("estimates: %+v", est)
	}
	if e := est[0]; e.N < 850 || e.N > 1150 || e.CI.Low > e.N || e.CI.High < e.N {
		t.Errorf("estimate %+v, want about 1000", e)
	}
	if un := c.Uncovered(est); len(un) != 0 {
		t.Errorf("uncovered %v", un)
	}
	if un := c.Uncovered(nil); len(un) != 1 || un[0] != (Stratum{"article", "1990s"}) {
		t.Errorf("uncovered with no pair %v", un)
	}
}

func TestWeight(t *testing.T) {
	c := New(scope.Env{}, 1000, 1)
	for i := range 400 {
		c.Add("full", article(i))
	}
	// The sample has each second work of 0 to 399, and each record stands for two.
	for i := 0; i < 400; i += 2 {
		r := article(i)
		r.Weight = 2
		c.Add("sample", r)
	}
	s := c.Catalogue("sample")
	if s.All["in"] != 400 || s.Weight != 2 {
		t.Fatalf("weighted count %v, weight %v", s.All["in"], s.Weight)
	}
	for _, o := range c.Overlaps() {
		if o.A == "full" && o.Fraction(s.Weight) != 1 {
			t.Errorf("the share of full in sample is %v, want 1 after the weight", o.Fraction(s.Weight))
		}
	}
}

func TestReport(t *testing.T) {
	c := New(scope.Env{}, 100, 1)
	for i := range 50 {
		c.Add("a", article(i))
		c.Add("b", article(i+25))
	}
	c.Add("b", Record{Kind: "textbook", Date: "1950", Signals: scope.Record{LCC: []string{"QA300"}}})
	var b strings.Builder
	if err := c.WriteReport(&b, [][2]string{{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"# Census of the catalogues of resources", "| a | 50 | 1.00 | 50 | 0 | 0 | 0 | 50 |", "## Counts by MSC2", "| a | b | records | 50 | 25 |", "| a | b | article | 1990s |", "## Strata with no estimate", "| book | 1950s |"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not have %q:\n%s", want, out)
		}
	}
	var none strings.Builder
	if err := c.WriteReport(&none, nil); err != nil || !strings.Contains(none.String(), "gives no estimate") {
		t.Errorf("the report with no pair: %v\n%s", err, none.String())
	}
}

func TestLookupNeedsWhole(t *testing.T) {
	c := New(scope.Env{}, 1000, 1)
	// A has the works 0 to 299, and a lookup finds each one in B. B has the same works, but only the works below 100 are in. B also has the in works 300 to 599.
	for i := range 300 {
		c.Add("b", article(300+i))
		r := article(i)
		r.Found = map[string]bool{"b": true}
		c.Add("a", r)
		b := article(i)
		if i >= 100 {
			b.Signals = scope.Record{MSC: []string{"62F10"}}
		}
		c.Add("b", b)
	}
	est := c.Estimates([][2]string{{"a", "b"}})
	if len(est) != 1 || est[0].M != 100 {
		t.Fatalf("m = %+v, want the 100 in works of b, not the 300 that the lookup finds", est)
	}
	c.Whole = map[string]bool{"b": true}
	if est := c.Estimates([][2]string{{"a", "b"}}); len(est) != 1 || est[0].M != 300 {
		t.Fatalf("m = %+v, want the 300 works that the lookup finds in a whole catalogue", est)
	}
}

func TestThinStratum(t *testing.T) {
	c := New(scope.Env{}, 1000, 1)
	for i := range 300 {
		c.Add("a", article(i))
	}
	for i := range 5 {
		c.Add("b", article(i))
	}
	if est := c.Estimates([][2]string{{"a", "b"}}); len(est) != 0 {
		t.Fatalf("a stratum with %d matches has the estimate %+v", 5, est)
	}
}

func TestPairSample(t *testing.T) {
	full := PairSample(Sample{NA: 400, NB: 300, Sampled: 400, Matched: 120, WB: 1}, 7)
	if full.N != Chapman(400, 300, 120) || full.CI.Low > full.N || full.CI.High < full.N {
		t.Fatalf("estimate %+v", full)
	}
	sampled := PairSample(Sample{NA: 400, NB: 300, KA: 40, KB: 30, Sampled: 40, Matched: 12, WB: 1}, 7)
	if sampled.N != full.N || sampled.CI.High-sampled.CI.Low <= full.CI.High-full.CI.Low {
		t.Errorf("the interval of small samples %v is not wider than %v", sampled.CI, full.CI)
	}
	if again := PairSample(Sample{NA: 400, NB: 300, KA: 40, KB: 30, Sampled: 40, Matched: 12, WB: 1}, 7); again != sampled {
		t.Error("the same seed gives another interval")
	}
}
