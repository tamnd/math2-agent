package report_test

import (
	"fmt"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/report"
	"github.com/tamnd/math2-agent/prov"
)

func ExampleMake() {
	w, _ := report.ParseWeek("2026-W43")
	start := w.Start()
	rep := report.Make(w, report.Input{
		Records: []*agent.Record{
			{ID: "r-2026-10-19-0001", Job: agent.Job{ID: "j-2026-10-19-1", Role: "prove", Target: "7K3QX2"}, Outcome: agent.Success, Started: start.Add(time.Hour), Spend: agent.Spend{USD: 0.6}},
			{ID: "r-2026-10-20-0002", Job: agent.Job{ID: "j-2026-10-19-1", Role: "prove", Target: "7K3QX2"}, Outcome: agent.Failure, Started: start.Add(25 * time.Hour), Spend: agent.Spend{USD: 0.3}},
		},
		Logs: map[string][]prov.Line{"7K3QX2": {{Field: "formal[0].decl", Op: prov.Change, Agent: "m2-agent/prove@0.1.1", Run: "r-2026-10-19-0001"}}},
		Made: start.AddDate(0, 0, 7),
	})
	fmt.Println(rep.Week, rep.From, rep.To)
	for _, r := range rep.Roles {
		fmt.Printf("%s: %d job, %d runs, %d accepted, $%.2f for each accepted run\n", r.Role, r.Jobs, r.Runs, r.Accepted, r.PerOutput)
	}
	// Output:
	// 2026-W43 2026-10-19 2026-10-25
	// prove: 1 job, 2 runs, 1 accepted, $0.90 for each accepted run
}
