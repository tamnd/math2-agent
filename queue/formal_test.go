package queue

import (
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/inventory"
)

func TestFormalResearch(t *testing.T) {
	m := func(paper, tag, kind string, uses ...string) inventory.Mention {
		x := inventory.Mention{ID: "ax://paper/" + paper + "#" + tag, Source: "arxiv", NativeID: paper + "#" + tag, Kind: kind}
		if len(uses) > 0 {
			x.Structure = &inventory.Structure{Uses: uses}
		}
		return x
	}
	ms := []inventory.Mention{
		// The main theorem MT of 2401.00001 has a proof that uses the lemma LE and the definition DE.
		m("2401.00001", "MT", "statement"),
		m("2401.00001", "PR", "proof", "ax://paper/2401.00001#LE", "ax://paper/2401.00001#DE"),
		m("2401.00001", "LE", "statement"),
		m("2401.00001", "DE", "definition"),
		// The definition DU is used by the statement of the node MT2.
		m("2401.00002", "DU", "definition"),
		m("2401.00002", "MT2", "statement", "ax://paper/2401.00002#DU"),
		// The conjecture CO has a spread of 2, and the row FS has a formal statement.
		m("2401.00003", "CO", "conjecture"),
		m("2401.00004", "FS", "statement"),
		{ID: "m2://rec/formal_conjectures/x#1", Source: "formal_conjectures", Kind: "statement"},
		// The row RE has the outcome record.
		m("2401.00005", "RE", "statement"),
	}
	node := func(tag string) *inventory.Outcome {
		return &inventory.Outcome{Value: inventory.OutcomeNode, Target: []string{tag}}
	}
	row := func(id, kind string, out *inventory.Outcome, spread int, mentions ...string) inventory.Row {
		return inventory.Row{ID: id, Kind: kind, Layer: inventory.Research, Outcome: out, Spread: spread, Mentions: mentions}
	}
	mt := row("MT", "statement", node("T-MT"), 1, ms[0].ID)
	mt.Proofs = []string{ms[1].ID}
	in := Input{Mentions: ms, Rows: []inventory.Row{
		mt,
		row("LE", "statement", node("T-LE"), 1, ms[2].ID),
		row("DE", "definition", node("T-DE"), 1, ms[3].ID),
		row("DU", "definition", node("T-DU"), 1, ms[4].ID),
		row("M2", "statement", node("T-M2"), 1, ms[5].ID),
		row("CO", "conjecture", node("T-CO"), 2, ms[6].ID),
		row("FS", "statement", node("T-FS"), 2, ms[7].ID, ms[8].ID),
		row("RE", "statement", &inventory.Outcome{Value: inventory.OutcomeRecord}, 1, ms[9].ID),
	}}
	q, err := Compute(FormalResearch, in)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"T-FS", "T-CO", "T-DE", "T-DU", "T-LE", "T-M2", "T-MT"}; !slices.Equal(targets(q), want) {
		t.Errorf("the queue is %v, and it must be %v", targets(q), want)
	}
	if !strings.Contains(q.Items[0].Goal, "AX10.2 reason 1, a formal source states it, thus an align run") {
		t.Errorf("the goal of FS is %q", q.Items[0].Goal)
	}
}
