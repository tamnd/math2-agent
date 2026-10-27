package queue

import (
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/inventory"
)

func TestWriteResearch(t *testing.T) {
	rec := &inventory.Outcome{Value: inventory.OutcomeRecord, Rule: "research-default"}
	mention := func(paper, tag, kind, name string, names ...string) inventory.Mention {
		m := inventory.Mention{ID: "ax://paper/" + paper + "#" + tag, Source: "arxiv", NativeID: paper + "#" + tag, Kind: kind, Name: name}
		if len(names) > 0 {
			m.Structure = &inventory.Structure{Names: names}
		}
		return m
	}
	ms := []inventory.Mention{
		// The row KK is named the same in two papers.
		mention("2401.00001", "A", "statement", "Kahn-Kalai conjecture"),
		mention("2401.00002", "A", "statement", "Kahn-Kalai conjecture"),
		// The row SP is a lemma that three other papers name.
		mention("2401.00003", "A", "statement", "Key lemma"),
		mention("2401.00004", "A", "statement", "", "SP"),
		mention("2401.00005", "A", "statement", "", "SP"),
		mention("2401.00006", "A", "statement", "", "SP", "OP"),
		// The row OP is a problem that two papers name.
		mention("2401.00007", "A", "problem", ""),
		mention("2401.00008", "A", "statement", "", "OP"),
		// The row FO has a formal statement.
		mention("2401.00009", "A", "statement", ""),
		{ID: "m2://rec/mathlib_1000/x#1", Source: "mathlib_1000", Kind: "statement"},
		// The row NO has no reason, and the row ND is a node already.
		mention("2401.00010", "A", "statement", "Small case"),
		mention("2401.00011", "A", "statement", ""),
		// The row OW names itself, which does not count.
		mention("2401.00012", "A", "statement", "", "OW"),
		mention("2401.00012", "B", "statement", "", "OW"),
	}
	row := func(id, kind, name string, out *inventory.Outcome, mentions ...string) inventory.Row {
		return inventory.Row{ID: id, Kind: kind, Name: name, Layer: inventory.Research, Outcome: out, Mentions: mentions}
	}
	in := Input{Mentions: ms, Rows: []inventory.Row{
		row("KK", "statement", "Kahn-Kalai conjecture", rec, ms[0].ID, ms[1].ID),
		row("SP", "statement", "Key lemma", rec, ms[2].ID),
		row("OP", "problem", "", nil, ms[6].ID),
		row("FO", "statement", "", rec, ms[8].ID, ms[9].ID),
		row("NO", "statement", "Small case", rec, ms[10].ID),
		row("ND", "statement", "", &inventory.Outcome{Value: inventory.OutcomeNode, Target: []string{"x"}}, ms[11].ID),
		row("OW", "statement", "", rec, ms[12].ID),
	}}
	q, err := Compute(WriteResearch, in)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"KK", "SP", "OP", "FO"}; !slices.Equal(targets(q), want) {
		t.Errorf("the queue is %v, and it must be %v", targets(q), want)
	}
	if len(q.Items) > 1 && !strings.Contains(q.Items[1].Goal, "reason 4, spread, 3 papers cite it") {
		t.Errorf("the goal of SP is %q", q.Items[1].Goal)
	}
	in.Asked = []string{"05"}
	in.Rows[4].MSC = []string{"05C80"}
	q, _ = Compute(WriteResearch, in)
	if targets(q)[0] != "NO" {
		t.Errorf("with the area 05 asked, the queue is %v, and NO must be first", targets(q))
	}
}
