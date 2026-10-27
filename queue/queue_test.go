package queue

import (
	"slices"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/inventory"
)

var day = time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)

func input() Input {
	return Input{
		Day: day,
		Rows: []inventory.Row{
			{ID: "103A5C1YC0", Mentions: []string{"m2://rec/wikidata/Q1#1"}, Name: "a", Spread: 1},
			{ID: "104B6D2ZD1", Mentions: []string{"m2://rec/wikidata/Q2#1", "m2://rec/oeis/A1#1"}, Name: "b", Spread: 2},
			{ID: "105C7E3AE2", Mentions: []string{"m2://rec/wikidata/Q3#1"}, Spread: 1, Outcome: &inventory.Outcome{Value: inventory.OutcomeOutOfScope}},
			{ID: "106D8F4BF3", Mentions: []string{"m2://rec/wikidata/Q4#1"}, Spread: 1, Outcome: &inventory.Outcome{Value: inventory.OutcomeDeferred, Until: "2026-10-01"}},
		},
		Mentions: []inventory.Mention{{ID: "m2://rec/wikidata/Q1#1"}, {ID: "m2://rec/wikidata/Q9#1", Name: "loose"}},
	}
}

func targets(q *Queue) []string {
	var out []string
	for _, it := range q.Items {
		out = append(out, it.Target)
	}
	return out
}

func TestDecide(t *testing.T) {
	q, err := Compute(Decide, input())
	if err != nil {
		t.Fatal(err)
	}
	// The row with two sources comes first. The deferred row whose date has passed is open again.
	if want := []string{"104B6D2ZD1", "103A5C1YC0", "106D8F4BF3"}; !slices.Equal(targets(q), want) {
		t.Errorf("the queue is %v, and it must be %v", targets(q), want)
	}
	if q.Items[0].Goal != "row 104B6D2ZD1 has an outcome" || q.Reason != "" {
		t.Errorf("the queue is %+v", q)
	}
}

func TestCluster(t *testing.T) {
	q, err := Compute(Cluster, input())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"m2://rec/wikidata/Q9#1"}; !slices.Equal(targets(q), want) {
		t.Errorf("the queue is %v, and it must be %v", targets(q), want)
	}
}

func TestEachQueue(t *testing.T) {
	for _, name := range Names() {
		q, err := Compute(name, input())
		if err != nil {
			t.Fatal(err)
		}
		if q.Items == nil || len(q.Items) == 0 && q.Reason == "" {
			t.Errorf("%s has no items and no reason", name)
		}
		if _, ok := Estimates[name]; !ok {
			t.Errorf("%s has no estimate", name)
		}
	}
	if _, err := Compute("prove", input()); err == nil {
		t.Error("Compute accepted a queue that is not in corpus spec K12.2")
	}
}

func TestPlan(t *testing.T) {
	q, err := Compute(Decide, input())
	if err != nil {
		t.Fatal(err)
	}
	p := MakePlan(q, 2)
	if p.Jobs != 2 || p.ReviewHigh != 2.0/60 || p.CostHigh != 0 || len(p.Notes) != 1 {
		t.Errorf("the plan is %+v", p)
	}
	if all := MakePlan(q, -1); all.Jobs != 3 {
		t.Errorf("the plan of each job has %d jobs", all.Jobs)
	}
	q = &Queue{Name: State, Items: make([]Item, 100)}
	if p := MakePlan(q, 1000); p.Jobs != 100 || p.TokensIn != 1_000_000 || p.CostLow != 2 || p.CostHigh != 4 {
		t.Errorf("the plan is %+v", p)
	}
}

func TestAlign(t *testing.T) {
	node := func(tags ...string) *inventory.Outcome {
		return &inventory.Outcome{Value: inventory.OutcomeNode, Target: tags}
	}
	in := Input{Day: day, Rows: []inventory.Row{
		{ID: "201A1A1A10", Layer: inventory.Textbook, MSC: []string{"11A41"}, Spread: 3, Outcome: node("PRIME0000A")},
		{ID: "202A1A1A11", Layer: inventory.Textbook, MSC: []string{"11N05", "05A15"}, Spread: 1, Outcome: node("PNT000000A", "PRIME0000A")},
		{ID: "203A1A1A12", Layer: inventory.Textbook, MSC: []string{"05A15"}, Spread: 5, Outcome: node("CATALAN00A")},
		{ID: "204A1A1A13", Layer: inventory.Textbook, MSC: []string{"14H52"}, Spread: 5, Outcome: node("ELLIPTIC0A")},
		{ID: "205A1A1A14", Layer: inventory.Research, MSC: []string{"11N05"}, Outcome: &inventory.Outcome{Value: inventory.OutcomeRecord}},
		{ID: "206A1A1A15", Layer: inventory.Research, MSC: []string{"05A15"}},
		{ID: "207A1A1A16", Layer: inventory.Research, MSC: []string{"14H52"}, Outcome: &inventory.Outcome{Value: inventory.OutcomeAligned}},
	}}
	q, err := Compute(Align, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Items) != 0 || q.Reason == "" {
		t.Errorf("with no area asked, the queue is %+v", q)
	}
	// Area 11 has a record and area 05 has an open result. Area 14 has only an aligned result, so its node waits for nothing.
	in.Asked = []string{"11", "14H52", "05"}
	if q, err = Compute(Align, in); err != nil {
		t.Fatal(err)
	}
	if want := []string{"CATALAN00A", "PRIME0000A", "PNT000000A"}; !slices.Equal(targets(q), want) {
		t.Errorf("the queue is %v, and it must be %v", targets(q), want)
	}
	if q.Items[1].Priority != 3 || q.Items[1].Goal != "node PRIME0000A has an aligned edge to each research result that states it" || q.Reason != "" {
		t.Errorf("the queue is %+v", q)
	}
	in.Asked = []string{"05"}
	if q, _ = Compute(Align, in); !slices.Equal(targets(q), []string{"CATALAN00A", "PNT000000A", "PRIME0000A"}) {
		t.Errorf("with area 05 asked, the queue is %v", targets(q))
	}
}
