package queue_test

import (
	"fmt"
	"time"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/queue"
)

func ExampleCompute() {
	in := queue.Input{
		Day: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
		Rows: []inventory.Row{
			{ID: "103A5C1YC0", Name: "Lagrange's theorem", Spread: 1},
			{ID: "104B6D2ZD1", Name: "Sylow theorems", Spread: 3},
		},
	}
	q, err := queue.Compute(queue.Decide, in)
	if err != nil {
		panic(err)
	}
	for _, it := range q.Items {
		fmt.Println(it.Target, it.Name, it.Priority)
	}
	// Output:
	// 104B6D2ZD1 Sylow theorems 3
	// 103A5C1YC0 Lagrange's theorem 1
}

func ExampleMakePlan() {
	q := &queue.Queue{Name: queue.State, Items: make([]queue.Item, 500)}
	p := queue.MakePlan(q, 100)
	fmt.Printf("%d jobs, %d tokens in, $%.2f to $%.2f\n", p.Jobs, p.TokensIn, p.CostLow, p.CostHigh)
	// Output:
	// 100 jobs, 1000000 tokens in, $2.00 to $4.00
}
