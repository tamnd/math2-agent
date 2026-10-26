package agent

import (
	"cmp"
	"context"
	"fmt"
	"math"

	"github.com/tamnd/math2-agent/llm"
)

// A Meter is a model that the goal of a run calls, for example the grounding check of explain (spec 12.6). The calls of the meter count in the spend of the run and keep its budget. Before each goal check, [Runner.Run] gives the meter the money that is left. After the check, it adds the spend of the meter to the record and puts the responses in the turn. A Meter is not safe for concurrent use.
type Meter struct {
	Model llm.Model // the model service

	prices    map[string]llm.Price
	left      float64 // the money that the calls can spend, when limited
	limited   bool
	usd       float64
	tokens    map[string]llm.Usage
	responses []*llm.Response
}

// Complete sends the request to the model service. It sets the limit of the output tokens so that the call costs at most the money that is left to the run. Outside a run, the meter has no limit and uses llm.Prices.
func (m *Meter) Complete(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	prices := m.prices
	if prices == nil {
		prices = llm.Prices
	}
	price, ok := prices[req.Model]
	if !ok {
		return nil, fmt.Errorf("agent: no price for the model %s, thus the check cannot keep the budget", req.Model)
	}
	if m.limited {
		r := *req
		left := int(math.Floor((m.left - m.usd - inputBound(&r, price)) / outputPrice(price) * 1e6))
		r.MaxTokens = min(r.MaxTokens, left)
		if r.MaxTokens < 1 {
			return nil, fmt.Errorf("agent: the budget of the run is used up before the check")
		}
		req = &r
	}
	resp, err := m.Model.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	resp.Model = cmp.Or(resp.Model, req.Model)
	usd, _ := llm.Cost(prices, resp.Model, resp.Usage)
	m.usd += usd
	if m.tokens == nil {
		m.tokens = map[string]llm.Usage{}
	}
	m.tokens[resp.Model] = m.tokens[resp.Model].Add(resp.Usage)
	m.responses = append(m.responses, resp)
	return resp, nil
}

// start makes the meter ready for one goal check. A budget of zero dollars has no limit.
func (m *Meter) start(prices map[string]llm.Price, b Budget, spent float64) {
	m.prices, m.usd, m.tokens, m.responses = prices, 0, nil, nil
	m.limited, m.left = b.USD > 0, b.USD-spent
}

// charge adds the spend of the last goal check to the record and to the turn.
func (m *Meter) charge(rec *Record, t *Turn) {
	rec.Spend.USD += m.usd
	for model, u := range m.tokens {
		rec.Spend.Tokens[model] = rec.Spend.Tokens[model].Add(u)
	}
	t.USD += m.usd
	t.Checks = append(t.Checks, m.responses...)
}
