package orchestrator_test

import (
	"context"
	"fmt"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/orchestrator"
)

// The first run of the job fails on the cheapest rung. The second run, on the next rung, reaches the goal with the money that is left.
func ExampleOrchestrator_Drain() {
	role := &agent.Role{
		Name: "prove", Version: "0.1.1",
		Ladder: []agent.Rung{{Model: "claude-sonnet-5-5", Effort: "medium"}, {Model: "claude-opus-5-5", Effort: "high"}},
		Budget: agent.Budget{USD: 5},
	}
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	n := 0
	dispatch := func(_ context.Context, job *agent.Job, rung int) (*agent.Record, error) {
		n++
		rec := &agent.Record{
			ID: fmt.Sprintf("r-2026-11-02-%04d", n), Job: *job, Rung: role.Ladder[rung], Started: start,
			Outcome: agent.Failure, Reason: "no progress in 6 turns", Spend: agent.Spend{Tokens: map[string]llm.Usage{}, USD: 1.25},
		}
		if rung == 1 {
			rec.Outcome, rec.Reason = agent.Success, "the goal holds"
		}
		fmt.Printf("%s on %s with $%.2f: %s\n", job.Target, rec.Rung.Model, job.Budget.USD, rec.Outcome)
		return rec, nil
	}
	config := &orchestrator.Config{Roles: map[string]float64{"prove": 200}}
	o := orchestrator.New(config, []*agent.Role{role}, dispatch, nil, func() time.Time { return start })
	_, err := o.Add(&agent.Job{ID: "j-2026-11-02-41a7", Role: "prove", Target: "7K3QX2", Goal: "the statement has a proof", OnBehalfOf: "schedule:standing-prove"})
	if err != nil {
		fmt.Println(err)
		return
	}
	rep := o.Drain(context.Background())
	fmt.Println(rep.Done[0].Outcome, len(rep.Runs), len(rep.Over()))
	// Output:
	// 7K3QX2 on claude-sonnet-5-5 with $5.00: failure
	// 7K3QX2 on claude-opus-5-5 with $3.75: success
	// success 2 0
}
