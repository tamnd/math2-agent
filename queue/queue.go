// Package queue computes the standing queues of the corpus (corpus spec K12.2), puts their items in priority order (K12.3), and estimates the cost of their jobs (K12.6). It calls no model.
//
// A queue is computed from the inventory and the nodes of a tree. Some queues need data that the tree does not have yet, for example the writing levels of the nodes or the recall estimates of the inventory. Such a queue has no items and gives the reason.
package queue

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/inventory"
)

// The queues of corpus spec K12.2.
const (
	Enumerate = "enumerate"
	Cluster   = "cluster"
	Decide    = "decide"
	State     = "state"
	Complete  = "complete"
	Confirm   = "confirm"
	Sanity    = "sanity"
	Source    = "source"
)

// Names returns the names of the queues in the order of corpus spec K12.2.
func Names() []string {
	return []string{Enumerate, Cluster, Decide, State, Complete, Confirm, Sanity, Source}
}

// PriorityVersion is the version of the priority formula. A change of the weights or the terms needs a new version (corpus spec K12.3).
const PriorityVersion = "priority/corpus-1"

// Item is one job target of a queue.
type Item struct {
	Target   string  `json:"target"` // a row id, a mention id or a node tag
	Name     string  `json:"name,omitempty"`
	Kind     string  `json:"kind,omitempty"`
	Goal     string  `json:"goal"`     // the goal of the job, for example "row 103A5C1YC0 has an outcome"
	Priority float64 `json:"priority"` // the value of the priority formula, higher first
}

// Queue is a standing queue and its items in priority order.
type Queue struct {
	Name     string `json:"name"`
	Role     string `json:"role"` // the role that works on the queue
	Goal     string `json:"goal"` // the goal of the queue
	Priority string `json:"priority"`
	Items    []Item `json:"items"`
	Reason   string `json:"reason,omitempty"` // why the queue has no items, when the tree does not have the data for it
}

// Input is what the queues are computed from.
type Input struct {
	Rows     []inventory.Row
	Mentions []inventory.Mention
	Day      time.Time // the day of the open count
}

type def struct {
	role, goal string
	items      func(in Input) []Item
	reason     string // the reason when items is nil
}

var defs = map[string]def{
	Enumerate: {role: "enumerate", goal: "the model lists each item of the area that it knows, with a source for each one", reason: "the inventory has no recall estimates yet (corpus spec K4.7)"},
	Cluster:   {role: "dedup judge, then curators", goal: "each mention belongs to one row", items: clusterItems},
	Decide:    {role: "curate", goal: "the open count goes down", items: decideItems},
	State:     {role: "write, informalize", goal: "W1", reason: "the build does not compute the writing levels yet (corpus spec K08)"},
	Complete:  {role: "write, explain, exercise", goal: "the next writing level", reason: "the build does not compute the writing levels yet (corpus spec K08)"},
	Confirm:   {role: "review-assist, then people", goal: "rule P04 is clean", reason: "no agent-written section is in the tree yet"},
	Sanity:    {role: "explore", goal: "rule X05 is clean", reason: "no agent-written statement is in the tree yet"},
	Source:    {role: "review-assist, then people", goal: "rule P06 is clean", reason: "no source has the mark locator_unconfirmed yet"},
}

// Compute returns the queue name, with its items in priority order: the highest priority first, then the target in byte order.
func Compute(name string, in Input) (*Queue, error) {
	d, ok := defs[name]
	if !ok {
		return nil, fmt.Errorf("queue: %q is not a queue. The queues are %s", name, strings.Join(Names(), ", "))
	}
	q := &Queue{Name: name, Role: d.role, Goal: d.goal, Priority: PriorityVersion, Items: []Item{}}
	if d.items == nil {
		q.Reason = d.reason
		return q, nil
	}
	q.Items = d.items(in)
	slices.SortFunc(q.Items, func(a, b Item) int {
		return cmp.Or(cmp.Compare(b.Priority, a.Priority), cmp.Compare(a.Target, b.Target))
	})
	return q, nil
}

// priority is the formula of corpus spec K12.3 with the terms that the tree has data for. The centrality, demand, unlock, tractability and attempts terms need the graph and the run records of nodes, and a row is not a node yet. The curriculum term needs the volumes of K11. So the formula of this version has the spread term only, with the weight 1.
func priority(spread int) float64 { return float64(spread) }

// decideItems gives each open row (corpus spec K4.4). A row that many sources hold comes first.
func decideItems(in Input) []Item {
	var out []Item
	for i := range in.Rows {
		r := &in.Rows[i]
		if !r.IsOpen(in.Day) {
			continue
		}
		out = append(out, Item{Target: r.ID, Name: r.Name, Kind: r.Kind, Goal: "row " + r.ID + " has an outcome", Priority: priority(r.Spread)})
	}
	return out
}

// clusterItems gives each mention that no row holds (corpus spec K4.3).
func clusterItems(in Input) []Item {
	held := map[string]bool{}
	for i := range in.Rows {
		for _, id := range in.Rows[i].Mentions {
			held[id] = true
		}
	}
	var out []Item
	for i := range in.Mentions {
		m := &in.Mentions[i]
		if held[m.ID] {
			continue
		}
		out = append(out, Item{Target: m.ID, Name: m.Name, Kind: m.Kind, Goal: "mention " + m.ID + " belongs to one row", Priority: priority(1)})
	}
	return out
}
