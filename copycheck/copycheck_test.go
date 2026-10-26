package copycheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
)

// The reference texts of the tests. The authors of the tests wrote them for the tests.
const (
	refClosed = "A field is a commutative ring in which every nonzero element has a multiplicative inverse, and in which the zero element differs from the unit element. The rational numbers, the real numbers and the complex numbers are the standard examples of fields."
	refOpen   = "Every finite integral domain is a field. To see this, multiply by a fixed nonzero element; the map is injective, hence surjective on a finite set, thus the unit element is in the image."
)

func corpus(phrases ...string) *Corpus {
	return New([]Text{
		{Source: "book", URI: "book:1", Licence: "", Class: ingest.RecordOnly, Text: refClosed},
		{Source: "notes", URI: "https://example.org/notes", Licence: "CC0-1.0", Text: refOpen},
	}, phrases)
}

// TestPlanted50 plants a copy of 50 normalized characters in a body that is otherwise new. The check must find it.
func TestPlanted50(t *testing.T) {
	planted := refClosed[2:52] // "field is a commutative ring in which every nonzero"
	if len(planted) != 50 {
		t.Fatalf("the planted copy has %d characters", len(planted))
	}
	body := "We show the claim by induction on the degree of the polynomial, and at each step we divide by the leading coefficient, which is a unit by the hypothesis. The quotient has a smaller degree, thus the induction goes on until the degree is zero. A constant polynomial that is not zero has no root, which gives the base case. In the step, we take a root, we divide, and we count the roots of the quotient with the same argument. The bound on the number of roots follows. Note that the hypothesis (" + planted + ") is not used in the last step, because the base case needs no inverse."
	r := corpus().Check(Section{Field: "body.proof", Text: body})
	var found *Finding
	for i := range r.Findings {
		if r.Findings[i].Stage == 1 && r.Findings[i].Source == "book" {
			found = &r.Findings[i]
		}
	}
	if found == nil {
		t.Fatalf("the check did not find the planted copy: %+v", r.Findings)
	}
	if got := body[found.Start:found.End]; !strings.Contains(got, planted) {
		t.Errorf("the finding is %q, want it to contain %q", got, planted)
	}
	if found.Length < FlagRun {
		t.Errorf("the run has %d characters, want %d or more", found.Length, FlagRun)
	}
	// A run of 50 characters in a long body is a flag, not a match.
	if found.Match {
		t.Errorf("a run of %d characters in a body of %d is a match, want a flag", found.Length, len(body))
	}
	// A run that is 1 character shorter is not a flag.
	short := strings.Replace(body, planted, planted[:49]+"X", 1)
	if r := corpus().Check(Section{Field: "body.proof", Text: short}); len(r.Findings) != 0 {
		t.Errorf("a run of 49 characters gives %+v, want nothing", r.Findings)
	}
}

func TestNormalize(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"A  *Field*\n\tis   `a` ring.", "a field is a ring."},
		{"Let $x^2$ and $$\\sum_i a_i$$ be \\(y\\) and \\[z\\].", "let \x02 and \x02 be \x02 and \x02."},
		{"See [the book](https://example.org/x) here.", "see the book here."},
		{"  Ünïcode  ", "ünïcode"},
		{"a \\$ sign", "a \\$ sign"},
	} {
		n := normalize(c.in)
		if string(n.b) != c.want {
			t.Errorf("normalize(%q) = %q, want %q", c.in, n.b, c.want)
		}
		if len(n.off) != len(n.b)+1 {
			t.Errorf("normalize(%q): %d offsets for %d bytes", c.in, len(n.off), len(n.b))
		}
	}
}

// TestMatch: a long copy from a closed source is a match that blocks, and a long copy from an open source is a match that needs attribution.
func TestMatch(t *testing.T) {
	r := corpus().Check(
		Section{Field: "body.statement", Text: refClosed[:120] + " Thus we have the claim."},
		Section{Field: "body.proof", Text: "We note this. " + refOpen},
	)
	if !r.Blocks() {
		t.Fatalf("the report does not block: %+v", r.Findings)
	}
	var closed, open bool
	for _, m := range r.Matches() {
		switch {
		case m.Field == "body.statement" && m.Source == "book" && m.Stage == 1:
			closed = m.Blocks()
		case m.Field == "body.proof" && m.Source == "notes":
			open = !m.Blocks()
		}
	}
	if !closed || !open {
		t.Errorf("closed %v, open %v: %+v", closed, open, r.Matches())
	}
	if got := r.Attribution(); len(got) != 1 || got[0] != "https://example.org/notes" {
		t.Errorf("Attribution() = %v", got)
	}
	rec := r.Record("ALG-0001")
	if rec.Node != "ALG-0001" || rec.Version != Version || !rec.Blocks() {
		t.Errorf("Record() = %+v", rec)
	}
}

// TestCover: flags that cover 10% of a short section are matches, even when each run is shorter than MatchRun.
func TestCover(t *testing.T) {
	body := "Note that a " + refClosed[2:62] + " holds."
	r := corpus().Check(Section{Field: "body.statement", Text: body})
	if len(r.Findings) != 1 || !r.Findings[0].Match {
		t.Fatalf("got %+v, want 1 match", r.Findings)
	}
}

// TestPhrase: a standard phrase does not count, and no run goes through it.
func TestPhrase(t *testing.T) {
	ref := "Let $G$ be a group and let $H$ be a subgroup of finite index in the group."
	body := "Let $K$ be a group and let $L$ be a subgroup of finite index in the group."
	c := New([]Text{{Source: "book", Class: ingest.RecordOnly, Text: ref}}, nil)
	if r := c.Check(Section{Field: "body.statement", Text: body}); len(r.Findings) == 0 {
		t.Fatalf("with no phrase list, the check finds nothing")
	}
	c = New([]Text{{Source: "book", Class: ingest.RecordOnly, Text: ref}}, []string{"let $G$ be a group", "be a subgroup of finite index"})
	if r := c.Check(Section{Field: "body.statement", Text: body}); len(r.Findings) != 0 {
		t.Errorf("the standard phrases count: %+v", r.Findings)
	}
}

// TestNear: a paragraph with a few words changed is a near copy.
func TestNear(t *testing.T) {
	ref := "Suppose that the sequence is bounded and monotone. Then the sequence converges, because the least upper bound of its terms is a limit of the sequence, and each tail of the sequence stays close to that bound after a finite number of terms."
	body := "Suppose that the sequence is bounded and increasing. Then the sequence converges, because the least upper bound of its terms is a limit of the sequence, and each tail of the sequence stays close to that bound after a finite number of terms."
	c := New([]Text{{Source: "book", Class: ingest.RecordOnly, Text: ref}}, nil)
	r := c.Check(Section{Field: "body.proof", Text: body})
	near := false
	for _, f := range r.Findings {
		if f.Stage == 2 && f.Match {
			near = true
			if f.Start != 0 || f.End != len(body) {
				t.Errorf("the near copy is at %d to %d, want 0 to %d", f.Start, f.End, len(body))
			}
		}
	}
	if !near {
		t.Errorf("the check does not find the near copy: %+v", r.Findings)
	}
	other := "A compact metric space is complete and totally bounded, and each open cover of it has a finite subcover, as the proof by bisection shows for the closed unit interval."
	if r := c.Check(Section{Field: "body.proof", Text: other}); len(r.Findings) != 0 {
		t.Errorf("a new paragraph gives %+v", r.Findings)
	}
}

func TestSame(t *testing.T) {
	body := refClosed + "\n\n" + refOpen
	a := corpus().Check(Section{Field: "body.proof", Text: body})
	b := corpus().Check(Section{Field: "body.proof", Text: body})
	if len(a.Findings) == 0 || len(a.Findings) != len(b.Findings) {
		t.Fatalf("%d and %d findings", len(a.Findings), len(b.Findings))
	}
	for i := range a.Findings {
		if a.Findings[i] != b.Findings[i] {
			t.Errorf("finding %d: %+v and %+v", i, a.Findings[i], b.Findings[i])
		}
	}
}

func TestReadDir(t *testing.T) {
	dir := t.TempDir()
	data := `{"source":"book","uri":"book:1","licence":"LicenseRef-proprietary","text":"one"}` + "\n\n" + `{"source":"notes","licence":"CC0-1.0","text":"two"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	texts, err := ReadDir(dir)
	if err != nil || len(texts) != 2 || texts[1].Source != "notes" {
		t.Fatalf("ReadDir = %+v, %v", texts, err)
	}
	c := New(texts, nil)
	if c.texts[1].Class != ingest.Open {
		t.Errorf("the class of CC0-1.0 is %q", c.texts[1].Class)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.jsonl"), []byte("{\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDir(dir); err == nil {
		t.Error("ReadDir reads a bad line")
	}
	if err := os.WriteFile(filepath.Join(dir, "phrases.txt"), []byte("# the list\nlet $G$ be a group\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p, err := ReadPhrases(filepath.Join(dir, "phrases.txt")); err != nil || len(p) != 1 {
		t.Errorf("ReadPhrases = %v, %v", p, err)
	}
}

func TestParagraphs(t *testing.T) {
	s := "one\ntwo\n\n  \nthree\n\nfour"
	var got []string
	for _, p := range paragraphs(s) {
		if s[p.start:p.end] != p.text {
			t.Errorf("the place of %q is wrong", p.text)
		}
		got = append(got, p.text)
	}
	if strings.Join(got, "|") != "one\ntwo|three|four" {
		t.Errorf("paragraphs = %q", got)
	}
}
