package stage_test

import (
	"fmt"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/stage"
	"github.com/tamnd/math2-agent/status"
)

func ExampleMeasure() {
	day := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	rep := stage.Measure(stage.Input{
		Records: []*agent.Record{
			{ID: "r-2026-11-02-0001", Job: agent.Job{ID: "j-2026-11-02-1", Role: "write", Target: "7K3QX2", Goal: "node reaches W2"}, Outcome: agent.Success, Started: day, Spend: agent.Spend{USD: 0.05}},
			{ID: "r-2026-11-02-0002", Job: agent.Job{ID: "j-2026-11-02-2", Role: "write", Target: "3C53TD", Goal: "node reaches W2"}, Outcome: agent.Failure, Started: day, Spend: agent.Spend{USD: 0.03}},
		},
		Logs:    map[string][]prov.Line{"7K3QX2": {{Field: "body.proof", Op: prov.Add, Agent: "m2-agent/write@0.1.0", Run: "r-2026-11-02-0001", At: "2026-11-02T09:05:00Z"}}},
		Reviews: map[string]status.Review{"r-7K3QX2-01": {Reviewer: "github:octocat", Date: "2026-11-03", Minutes: 4}},
		Made:    day.AddDate(0, 0, 7),
	})
	for _, s := range rep.Stages {
		fmt.Printf("%s: %d jobs, %.0f%% accepted, $%.2f for each accepted run, %d review, %.2f hours\n", s.Stage, s.Jobs, 100*s.AcceptanceRate(), s.PerAccepted(), s.Reviews, s.ReviewHigh)
	}
	// Output:
	// usable: 2 jobs, 50% accepted, $0.08 for each accepted run, 0 review, 0.00 hours
	// reviewed: 0 jobs, 0% accepted, $0.00 for each accepted run, 1 review, 0.07 hours
}
