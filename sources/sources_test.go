package sources

import (
	"errors"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
)

// The records of the tests are made up for the tests. They are not records of the catalogues.
func plane() *Plane {
	p := &Plane{}
	p.Add("zbmath", ingest.Record{ID: "9999.00001", Title: "On the orders of the subgroups of a finite group", Meta: map[string]string{"authors": "Tester, Ada; Müller, Bea", "year": "1901", "doi": "10.9999/Test.1"}})
	p.Add("arxiv", ingest.Record{ID: "9912.99999", Title: "A synthetic paper for the tests of m2", Meta: map[string]string{"authors": "Ada Tester"}})
	p.Add("erdosproblems", ingest.Record{ID: "1", Title: "Problem 1"})
	p.Add("palomar", ingest.Record{ID: "p-1", Title: "A formal proof", Meta: map[string]string{"authors": "Ada Tester|Bea Muller", "isbn": "9780000000002"}})
	return p
}

func TestConfirm(t *testing.T) {
	p := plane()
	for _, c := range []struct {
		name  string
		claim Claim
		want  Verdict
	}{
		{"confirmed", Claim{System: "zbmath", ID: "9999.00001", Title: "On the orders of subgroups of a finite group", Authors: []string{"A. Tester", "Bea Muller"}, Year: "1901"}, Confirmed},
		{"no title", Claim{System: "zbmath", ID: "9999.00001"}, Confirmed},
		{"doi", Claim{System: "doi", ID: "10.9999/test.1", Authors: []string{"Tester"}}, Confirmed},
		{"doi url", Claim{System: "doi", ID: "https://doi.org/10.9999/TEST.1"}, Confirmed},
		{"arxiv version", Claim{System: "arxiv", ID: "9912.99999v2", Year: "1999", Authors: []string{"Tester, Ada"}}, Confirmed},
		{"erdos", Claim{System: "erdos", ID: "1"}, Confirmed},
		{"isbn", Claim{System: "book", ID: "isbn:978-0-00-000000-2", Authors: []string{"B. Muller"}}, Confirmed},
		{"made up", Claim{System: "zbmath", ID: "9999.00002", Title: "On the orders of the subgroups of a finite group"}, Fabricated},
		{"no source", Claim{System: "mr", ID: "1"}, Fabricated},
		{"title", Claim{System: "zbmath", ID: "9999.00001", Title: "Sylow theorems for infinite groups"}, Mismatch},
		{"author", Claim{System: "zbmath", ID: "9999.00001", Authors: []string{"C. Other"}}, Mismatch},
		{"year", Claim{System: "arxiv", ID: "9912.99999", Year: "2001"}, Mismatch},
	} {
		got, err := Confirm(p, c.claim)
		if err != nil {
			t.Fatal(err)
		}
		if got.Verdict != c.want {
			t.Errorf("%s: %s %v, want %s", c.name, got.Verdict, got.Problems, c.want)
		}
		if got.Verdict != Confirmed && len(got.Problems) == 0 {
			t.Errorf("%s: no problems", c.name)
		}
		if got.LocatorConfirmed {
			t.Errorf("%s: the check confirms the locator", c.name)
		}
	}
}

type failing struct{}

func (failing) Resolve(string, string) (Record, bool, error) {
	return Record{}, false, errors.New("down")
}

func TestResolverError(t *testing.T) {
	if _, err := Confirm(failing{}, Claim{System: "zbmath", ID: "1"}); err == nil {
		t.Error("Confirm hides the error of the resolver")
	}
}

func TestArxivYear(t *testing.T) {
	for id, want := range map[string]string{"2312.00752": "2023", "0704.0001v3": "2007", "math/0211159": "2002", "hep-th/9901001": "1999", "math.AG/9205001": "1992", "2313.00001": "", "nonsense": ""} {
		if got := ArxivYear(id); got != want {
			t.Errorf("ArxivYear(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestSurname(t *testing.T) {
	for name, want := range map[string]string{"Tester, Ada": "tester", "Ada Tester": "tester", "É. Galois": "galois", "": ""} {
		if got := Surname(name); got != want {
			t.Errorf("Surname(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestSystems(t *testing.T) {
	got := plane().Systems()
	if len(got) != 4 || got[1] != "erdos" {
		t.Errorf("Systems() = %v", got)
	}
	if _, ok, _ := (&Plane{}).Resolve("zbmath", "1"); ok {
		t.Error("the zero plane has a record")
	}
	for sys, want := range map[string]bool{"zbmath": true, "erdos": true, "doi": true, "book": true, "mr": false, "erdosproblems": true} {
		if got := plane().Knows(sys); got != want {
			t.Errorf("Knows(%q) = %v", sys, got)
		}
	}
	if (&Plane{}).Knows("doi") {
		t.Error("the zero plane knows doi")
	}
}
