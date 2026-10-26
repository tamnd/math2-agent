package sample

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

var (
	agent  = Stratum{Kind: "theorem", Writer: "prove", Model: "model-a", Prompt: "prove@1", Layer: "L2"}
	person = Stratum{Kind: "definition", Writer: "person"}
)

func members() []Member {
	var out []Member
	for i := range 250 {
		out = append(out, Member{Tag: fmt.Sprintf("A%04d", i), Stratum: agent})
	}
	for i := range 30 {
		out = append(out, Member{Tag: fmt.Sprintf("P%04d", i), Stratum: person})
	}
	return out
}

func TestDraw(t *testing.T) {
	a, err := Draw(members(), "2026-10", "s1", 10)
	if err != nil {
		t.Fatal(err)
	}
	count := map[Stratum]int{}
	for _, x := range a {
		count[x.Stratum]++
		if x.Done || x.Month != "2026-10" || x.Seed != "s1" {
			t.Errorf("bad audit %+v", x)
		}
	}
	if count[agent] != MinSample || count[person] != 30 {
		t.Errorf("the draw has %v, want %d and 30", count, MinSample)
	}
	b, _ := Draw(members(), "2026-10", "s1", 0)
	c, _ := Draw(members(), "2026-10", "s2", 0)
	same, other := true, 0
	for i := range a {
		same = same && a[i].Tag == b[i].Tag
		if a[i].Tag != c[i].Tag {
			other++
		}
	}
	if !same {
		t.Error("the same seed gives a different draw")
	}
	if other == 0 {
		t.Error("a different seed gives the same draw")
	}
	if all, _ := Draw(members(), "2026-10", "s1", 300); len(all) != 280 {
		t.Errorf("a draw of 300 has %d audits, want 280", len(all))
	}
	for _, bad := range [][2]string{{"2026-13", "s"}, {"2026-10", ""}, {"2026-10", "a b"}} {
		if _, err := Draw(nil, bad[0], bad[1], 0); err == nil {
			t.Errorf("Draw(%q, %q) has no error", bad[0], bad[1])
		}
	}
}

func TestUpper(t *testing.T) {
	if got := Upper(0, 300); got != 0.01 {
		t.Errorf("Upper(0, 300) = %v", got)
	}
	if got := Upper(0, 0); got != 1 {
		t.Errorf("Upper(0, 0) = %v", got)
	}
	lo, hi := Wilson(1, 100)
	if math.Abs(lo-0.00177) > 1e-4 || math.Abs(hi-0.05449) > 1e-4 {
		t.Errorf("Wilson(1, 100) = %v, %v", lo, hi)
	}
	if Upper(1, 100) != hi {
		t.Error("Upper with errors is not the Wilson upper end")
	}
	if Need(True) != 300 || Need(Readable) != 60 {
		t.Errorf("Need = %d, %d", Need(True), Need(Readable))
	}
}

func TestRates(t *testing.T) {
	var audits []Audit
	for i := range 1000 {
		a := Audit{Month: "2026-10", Seed: "s", Tag: fmt.Sprintf("A%04d", i), Stratum: agent, Done: true}
		switch {
		case i < 9:
			a.Errors = []string{"wrong_source", "stronger_than_source"} // one faithful error each
		case i < 49:
			a.Errors = []string{"style"}
		}
		audits = append(audits, a)
	}
	audits = append(audits, Audit{Month: "2026-10", Tag: "P0001", Stratum: person}) // not done
	got := map[Property]Rate{}
	for _, r := range Rates(audits) {
		got[r.Property] = r
	}
	if len(got) != 3 {
		t.Fatalf("Rates = %+v", got)
	}
	if r := got[True]; r.Audits != 1000 || r.Errors != 0 || r.State != Below {
		t.Errorf("true: %+v", r)
	}
	if r := got[Faithful]; r.Errors != 9 || r.State != Near {
		t.Errorf("faithful: %+v", r)
	}
	if r := got[Readable]; r.Errors != 40 || r.State != Above {
		t.Errorf("readable: %+v", r)
	}
	short := Rates([]Audit{{Month: "2026-10", Tag: "A", Stratum: agent, Done: true}})
	if short[0].State != Short {
		t.Errorf("one clean audit: %+v", short[0])
	}
}

func TestRegister(t *testing.T) {
	root := t.TempDir()
	if a, err := Load(root); err != nil || a != nil {
		t.Fatalf("Load of an empty tree = %v, %v", a, err)
	}
	drawn, _ := Draw(members()[:2], "2026-10", "s1", 0)
	if err := Append(root, drawn); err != nil {
		t.Fatal(err)
	}
	done := drawn[0]
	done.Auditor, done.Done, done.Errors = "github:octocat", true, []string{"wrong_definition", "style"}
	clean := drawn[1]
	clean.Auditor, clean.Done = "github:octocat", true
	if err := Append(root, []Audit{done, clean}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Auditor != "github:octocat" || len(got[0].Errors) != 2 || !got[1].Done || got[1].Errors != nil || got[0].Stratum != agent {
		t.Errorf("Load = %+v", got)
	}
	for _, bad := range []string{
		"2026-10 s A0001 theorem|prove|-|-|L2 -\n",
		"2026-1 s A0001 theorem|prove|-|-|L2 - open\n",
		"2026-10 s A0001 theorem|prove - open\n",
		"2026-10 s A0001 theorem|prove|-|-|L2 - typo\n",
	} {
		if err := os.WriteFile(filepath.Join(root, File), []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(root); err == nil {
			t.Errorf("Load reads %q", bad)
		}
	}
}

func TestStratum(t *testing.T) {
	if s := person.String(); s != "definition|person|-|-|-" {
		t.Errorf("String() = %q", s)
	}
	if s, err := ParseStratum(agent.String()); err != nil || s != agent {
		t.Errorf("ParseStratum = %+v, %v", s, err)
	}
	for c, p := range Classes {
		if p.Threshold() == 0 {
			t.Errorf("the class %s has no property", c)
		}
	}
}
