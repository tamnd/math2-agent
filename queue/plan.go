package queue

// Estimate is the cost of one job of a queue, from the planning numbers of corpus spec K12.6 and K12.5. A zero field with Known false means that the spec gives no number.
type Estimate struct {
	Tier      int     `json:"tier,omitempty"`       // the model tier of spec 12.3
	TokensIn  int     `json:"tokens_in,omitempty"`  // the tokens in for each job
	TokensOut int     `json:"tokens_out,omitempty"` // the tokens out for each job
	CostLow   float64 `json:"cost_low"`             // the model cost of each job in US dollars, at batch prices, the low end
	CostHigh  float64 `json:"cost_high"`            // the high end
	CostKnown bool    `json:"cost_known"`           // the spec gives the model cost
	// The minutes of review for each job, the low end and the high end (K12.5).
	ReviewLow   float64 `json:"review_low"`
	ReviewHigh  float64 `json:"review_high"`
	ReviewKnown bool    `json:"review_known"` // the spec gives the review time
}

// Estimates are the planning numbers of each queue. The run records replace them with measured numbers after the first thousand jobs of each role (K12.6).
var Estimates = map[string]Estimate{
	Enumerate: {Tier: 1, TokensIn: 500, TokensOut: 100, CostHigh: 0.001, CostKnown: true},
	Cluster:   {Tier: 0, TokensIn: 2000, TokensOut: 100, CostHigh: 0.001, CostKnown: true, ReviewLow: 0, ReviewHigh: 1, ReviewKnown: true},
	Decide:    {ReviewLow: 1, ReviewHigh: 1, ReviewKnown: true},
	State:     {Tier: 1, TokensIn: 10000, TokensOut: 1500, CostLow: 0.02, CostHigh: 0.02, CostKnown: true},
	Complete:  {Tier: 1, TokensIn: 30000, TokensOut: 8000, CostLow: 0.06, CostHigh: 0.14, CostKnown: true},
	Confirm:   {ReviewLow: 1, ReviewHigh: 2, ReviewKnown: true},
	Sanity:    {},
	Source:    {ReviewLow: 2, ReviewHigh: 2, ReviewKnown: true},
	Align:     {},
	// AX13.3: $10,000 to $60,000 for 100,000 to 500,000 research nodes, before the acceptance rate. The spec gives no token count for one job.
	WriteResearch: {Tier: 2, CostLow: 0.02, CostHigh: 0.6, CostKnown: true},
	// AX10 gives no cost for a formalize job on a research node.
	FormalResearch: {},
}

// Plan is the estimate for the first jobs of a queue. It is a plan only: no model run starts without a budget that a person approved (K12.6).
type Plan struct {
	Queue      string   `json:"queue"`
	Jobs       int      `json:"jobs"`
	Per        Estimate `json:"per_job"`
	TokensIn   int      `json:"tokens_in"`
	TokensOut  int      `json:"tokens_out"`
	CostLow    float64  `json:"cost_low"`  // US dollars
	CostHigh   float64  `json:"cost_high"` // US dollars, with retries and escalation that double the cost
	ReviewLow  float64  `json:"review_hours_low"`
	ReviewHigh float64  `json:"review_hours_high"`
	Notes      []string `json:"notes,omitempty"`
}

// MakePlan estimates the cost and the review hours of the first n jobs of q. A value of n below zero means each job of the queue.
func MakePlan(q *Queue, n int) Plan {
	if n < 0 || n > len(q.Items) {
		n = len(q.Items)
	}
	e := Estimates[q.Name]
	p := Plan{Queue: q.Name, Jobs: n, Per: e}
	p.TokensIn, p.TokensOut = n*e.TokensIn, n*e.TokensOut
	p.CostLow, p.CostHigh = float64(n)*e.CostLow, 2*float64(n)*e.CostHigh
	p.ReviewLow, p.ReviewHigh = float64(n)*e.ReviewLow/60, float64(n)*e.ReviewHigh/60
	if !e.CostKnown {
		p.Notes = append(p.Notes, "corpus spec K12.6 gives no model cost for this queue")
	}
	if !e.ReviewKnown {
		p.Notes = append(p.Notes, "corpus spec K12.5 gives no review time for this queue")
	}
	if q.Reason != "" {
		p.Notes = append(p.Notes, q.Reason)
	}
	return p
}
