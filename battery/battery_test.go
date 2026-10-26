package battery_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/battery"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/replay"
)

// parse makes nodes from front matter. Each item is "ID kind" and then the other lines of the front matter. Each node has a Statement section with the text "S.".
func parse(t testing.TB, items ...string) []*node.Node {
	t.Helper()
	var out []*node.Node
	for _, it := range items {
		head, rest, _ := strings.Cut(it, "\n")
		id, kind, _ := strings.Cut(head, " ")
		src := "---\ntag: " + id + "\nkind: " + kind + "\nname: " + id + "\n" + rest + "\n---\n\n## Statement\n\nS.\n"
		n, err := node.Parse("nodes/"+id[:2]+"/"+id+".md", []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

// bound is the front matter of a Lean binding of decl.
func bound(decl string) string {
	return "formal:\n  - {system: lean4, decl: " + decl + ", match: exact}"
}

// theorem returns the record of a theorem whose statement has the constants.
func theorem(decl string, constants ...string) lean.Record {
	return lean.Record{Decl: decl, Kind: "theorem", StatementPP: "P " + decl, StatementPPAll: "P " + decl, UsesConstantsInStatement: constants, Checks: &lean.Checks{}}
}

// prover gives the results of a table. A pair that is not in the table has no tactic.
type prover map[[2]string]lean.Implication

func (p prover) Implies(_ int, pairs ...[2]string) ([]lean.Implication, error) {
	out := make([]lean.Implication, len(pairs))
	for i, pr := range pairs {
		out[i] = p[pr]
		out[i].From, out[i].To = pr[0], pr[1]
	}
	return out, nil
}

// reader gives the concerns of a table, by declaration.
type reader map[string][]battery.Concern

func (r reader) Read(_ context.Context, p battery.Packet) ([]battery.Concern, error) {
	if p.Statement != "S." {
		return nil, nil
	}
	return r[p.Decl], nil
}

// fixture returns a tree with one binding for each result that a check can give.
func fixture(t testing.TB) (battery.Input, battery.Options) {
	nodes := parse(t,
		"CNCPT1 concept",
		"CNCPT2 concept",
		"CNCPT3 concept",
		"DEFGRP definition\ndefines: CNCPT1\n"+bound("Group"),
		"DEFRNG definition\n"+bound("Ring"),
		"GDSTMT statement\nabout: [CNCPT1]\nuses: [DEFGRP]\n"+bound("Good.t"),
		"EXMPA1 example\ninstance_of: [CNCPT1]\n"+bound("Ex.one"),
		"DEFBAD statement\nuses: [DEFGRP]\n"+bound("Bad.t"),
		"NBND01 statement\n"+bound("Unbound.t"),
		"JNKSTA statement\n"+bound("Junk.t"),
		"NRECD1 statement\n"+bound("Missing.t"),
		"ERRREC statement\n"+bound("Err.t"),
		"VACST1 statement\n"+bound("Vac.t"),
		"TRVSTA statement\n"+bound("Triv.t"),
		"REFST1 statement\n"+bound("Ref.t"),
		"STRNG1 statement\nabout: [CNCPT2]\n"+bound("Strong.t"),
		"EXMPA2 example\ninstance_of: [CNCPT2]\n"+bound("Ex.two"),
		"MSSHYP statement\nabout: [CNCPT3]\n"+bound("Missh.t"),
		"CNTREX counterexample\nabout: [CNCPT3]\n"+bound("Cx.t"),
		"GENPS1 statement\ngeneralizes: [{target: SPECPS}]\n"+bound("Genok.t"),
		"SPECPS statement\n"+bound("Specok.t"),
		"SPCZD1 statement\nspecializes: [GENPS1]\n"+bound("Spc.t"),
		"GENPN1 statement\ngeneralizes: [{target: SPECPN}]\n"+bound("Genopn.t"),
		"SPECPN statement\n"+bound("Specop.t"),
		"REPFA1 statement\n"+bound("Repfl.t"),
		"BACKTR statement\n"+bound("Back.t"),
		"RETRD1 statement\nretired: {date: 2026-10-01, reason: duplicate}\n"+bound("Old.t"),
	)
	records := map[string]lean.Record{}
	for _, r := range []lean.Record{
		{Decl: "Group", Kind: "def"},
		{Decl: "Ring", Kind: "def"},
		theorem("Good.t", "Group", "Eq", "HMul.hMul", "instHMul"),
		theorem("Ex.one"),
		theorem("Bad.t", "Group", "Ring"),
		theorem("Unbound.t", "Nat.factorial"),
		theorem("Err.t"),
		theorem("Vac.t"),
		theorem("Triv.t"),
		theorem("Ref.t"),
		theorem("Strong.t"),
		theorem("Ex.two"),
		theorem("Missh.t"),
		theorem("Cx.t"),
		theorem("Genok.t"),
		theorem("Specok.t"),
		theorem("Spc.t"),
		theorem("Genopn.t"),
		theorem("Specop.t"),
		theorem("Repfl.t"),
		theorem("Back.t"),
	} {
		records[r.Decl] = r
	}
	junk := theorem("Junk.t", "HSub.hSub", "instHSub")
	junk.StatementPPAll = "∀ (n : Nat), @Eq.{1} Nat (@HSub.hSub.{0, 0, 0} Nat Nat Nat (@instHSub.{0} Nat instSubNat) n n) n"
	records["Junk.t"] = junk
	records["Err.t"] = lean.Record{Decl: "Err.t", Error: "unknown declaration"}
	for d, c := range map[string]lean.Checks{"Vac.t": {Vacuous: "omega"}, "Triv.t": {Trivial: "rfl"}, "Ref.t": {Refuted: "decide"}} {
		r := records[d]
		r.Checks = &c
		records[d] = r
	}
	replays := map[string]replay.Result{}
	for d := range records {
		replays[d] = replay.Result{Decl: d, Passed: true}
	}
	replays["Repfl.t"] = replay.Result{Decl: "Repfl.t", Reason: "the statement does not match the pinned statement"}
	delete(replays, "Missing.t")
	in := battery.Input{Nodes: nodes, Records: records, Replays: replays}
	opt := battery.Options{
		Prover: prover{
			{"Good.t", "Ex.one"}:     {Implies: "(intro h; exact h)"},
			{"Strong.t", "Ex.two"}:   {Contradicts: "(intro h hb; simp_all)"},
			{"Cx.t", "Missh.t"}:      {Contradicts: "(intro h hb; grind)"},
			{"Genok.t", "Specok.t"}:  {Implies: "(intro h; simpa using h)"},
			{"Genok.t", "Spc.t"}:     {Implies: "(intro h; exact h)"},
			{"Genopn.t", "Specop.t"}: {},
		},
		BackTranslation: reader{"Back.t": {{Severity: battery.High, Class: battery.Quant, Text: "The quantifiers are in the wrong order."}}},
		Critic:          reader{"Back.t": {{Severity: battery.Medium, Text: "The name of the variable is not the name in the source."}}},
	}
	return in, opt
}

func TestRun(t *testing.T) {
	in, opt := fixture(t)
	reports, err := battery.Run(context.Background(), in, opt)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]battery.Report{}
	for _, r := range reports {
		by[r.Tag] = r
	}
	if _, ok := by["RETRD1"]; ok {
		t.Error("a retired node has a report")
	}
	tests := []struct {
		tag    string
		check  int
		result battery.Result
		class  string
	}{
		{"GDSTMT", 1, battery.Pass, ""},
		{"NRECD1", 1, battery.NotRun, ""},
		{"ERRREC", 1, battery.Fail, ""},
		{"BACKTR", 2, battery.Fail, battery.Quant},
		{"GDSTMT", 2, battery.Pass, ""},
		{"GDSTMT", 3, battery.Pass, ""},
		{"DEFBAD", 3, battery.Fail, battery.Def},
		{"NBND01", 3, battery.Open, ""},
		{"JNKSTA", 3, battery.Flag, battery.Junk},
		{"GDSTMT", 4, battery.Pass, ""},
		{"VACST1", 4, battery.Fail, battery.Vacuous},
		{"DEFGRP", 4, battery.Skip, ""},
		{"TRVSTA", 5, battery.Flag, battery.Junk},
		{"REFST1", 6, battery.Fail, battery.Strong},
		{"GDSTMT", 7, battery.Pass, ""},
		{"STRNG1", 7, battery.Fail, battery.Strong},
		{"MSSHYP", 7, battery.Fail, battery.HypMissing},
		{"NBND01", 7, battery.Skip, ""},
		{"GENPS1", 8, battery.Pass, ""},
		{"SPECPS", 8, battery.Pass, ""},
		{"SPCZD1", 8, battery.Pass, ""},
		{"GENPN1", 8, battery.Open, ""},
		{"GDSTMT", 8, battery.Skip, ""},
		{"BACKTR", 9, battery.Flag, ""},
		{"GDSTMT", 10, battery.Pass, ""},
		{"REPFA1", 10, battery.Fail, battery.Context},
		{"NRECD1", 10, battery.NotRun, ""},
	}
	for _, tt := range tests {
		r, ok := by[tt.tag]
		if !ok {
			t.Errorf("%s has no report", tt.tag)
			continue
		}
		c := r.Check(tt.check)
		if c.Number != tt.check || c.Result != tt.result || c.Class != tt.class {
			t.Errorf("%s, check %d (%s): %s %q, and it must be %s %q. %s", tt.tag, tt.check, c.Name, c.Result, c.Class, tt.result, tt.class, c.Detail)
		}
	}
	var f1 []string
	for _, r := range reports {
		if r.F1 {
			f1 = append(f1, r.Tag)
		}
	}
	// A binding is F1 when it has no fail and no open result. A flag does not block F1, so JNKSTA and TRVSTA are F1, and the flags go to the reviewer.
	if want := []string{"CNTREX", "DEFGRP", "DEFRNG", "EXMPA1", "EXMPA2", "GDSTMT", "GENPS1", "JNKSTA", "SPCZD1", "SPECPS", "TRVSTA"}; !slices.Equal(f1, want) {
		t.Errorf("the F1 bindings are %v, and they must be %v", f1, want)
	}
	if d := by["JNKSTA"].Check(3).Detail; !strings.Contains(d, "natural subtraction") {
		t.Errorf("the constant audit of JNKSTA does not name natural subtraction: %q", d)
	}
}

// TestNoPrograms checks that a battery without a prover and without models does not give F1.
func TestNoPrograms(t *testing.T) {
	in, _ := fixture(t)
	reports, err := battery.Run(context.Background(), in, battery.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reports {
		if r.F1 {
			t.Errorf("%s is F1 without the models", r.Tag)
		}
		for _, n := range []int{2, 9} {
			if c := r.Check(n); c.Result != battery.NotRun {
				t.Errorf("%s, check %d: %s, and it must be %s", r.Tag, n, c.Result, battery.NotRun)
			}
		}
		if r.Tag == "GDSTMT" {
			if c := r.Check(7); c.Result != battery.NotRun {
				t.Errorf("GDSTMT, check 7 without a prover: %s", c.Result)
			}
		}
	}
}

func TestF1(t *testing.T) {
	checks := func(results ...battery.Result) []battery.Check {
		out := make([]battery.Check, len(results))
		for i, r := range results {
			out[i] = battery.Check{Number: i + 1, Result: r}
		}
		return out
	}
	p, f, fl, o, s, n := battery.Pass, battery.Fail, battery.Flag, battery.Open, battery.Skip, battery.NotRun
	tests := []struct {
		name   string
		checks []battery.Check
		want   bool
	}{
		{"all pass", checks(p, p, p, p, p, p, p, p, p, p), true},
		{"check 5 fails", checks(p, p, p, p, f, p, p, p, p, p), true},
		{"flags and skips", checks(p, fl, fl, p, fl, p, s, s, fl, p), true},
		{"open constant audit", checks(p, p, o, p, p, p, p, p, p, p), false},
		{"no critic", checks(p, p, p, p, p, p, p, p, n, p), false},
		{"skipped back-translation", checks(p, s, p, p, p, p, p, p, p, p), false},
		{"no replay", checks(p, p, p, p, p, p, p, p, p, n), false},
		{"too few checks", checks(p, p, p), false},
	}
	for _, tt := range tests {
		if got := battery.F1(tt.checks); got != tt.want {
			t.Errorf("%s: F1 is %v, and it must be %v", tt.name, got, tt.want)
		}
	}
}

func TestNames(t *testing.T) {
	if n := battery.Names(); len(n) != 10 || n[0] != "elaborates" || n[9] != "pinned-statement compile" {
		t.Errorf("Names gives %v", n)
	}
	if c := battery.Classes(); len(c) != 12 || c[0] != battery.HypMissing || c[11] != battery.Tamper {
		t.Errorf("Classes gives %v", c)
	}
}
