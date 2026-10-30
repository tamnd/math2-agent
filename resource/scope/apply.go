package scope

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
)

// DecisionsFile holds the decisions of people (RS9.6). A decision of a person wins over each test, and a build never changes it.
const DecisionsFile = "records/resources/decisions.md"

// PersonRule is the rule of a scope that a person set.
const PersonRule = "person"

// ClassifierRule is the rule of a scope that the classifier of RS9.3 gave. RS9.2 calls it test S8.
const ClassifierRule = "S8"

// DeferDays is the time from a deferred decision to the next decision.
const DeferDays = 90

// PersonDecision is one record of DecisionsFile. Kind is scope for a scope outcome. The kinds merge and split are for clustering (RS10.6).
type PersonDecision struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Value  string `json:"value,omitempty"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason"`
	By     string `json:"by"`
	At     string `json:"at"`
}

// ReadDecisions reads DecisionsFile in the tree at root. A tree with no file has no decisions. A scope decision with no reason is an error.
func ReadDecisions(root string) ([]PersonDecision, error) {
	recs, err := mdrec.Read(filepath.Join(root, DecisionsFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []PersonDecision
	for n, rec := range recs {
		var d PersonDecision
		dec := json.NewDecoder(bytes.NewReader(rec))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&d); err != nil {
			return nil, fmt.Errorf("%s: record %d: %w", DecisionsFile, n+1, err)
		}
		if d.ID == "" || d.Kind == "" || d.Reason == "" || d.By == "" {
			return nil, fmt.Errorf("%s: record %d: a decision needs an id, a kind, a reason and a person", DecisionsFile, n+1)
		}
		out = append(out, d)
	}
	return out, nil
}

// A Classifier decides the scope of a resource that no test decides (RS9.3). Value is in, border or out, or empty in the middle band. Class is the MSC2 class of a work in scope, or empty.
type Classifier interface {
	Classify(r *resource.Resource) (value string, score float64, class string)
	Version() string
}

// Input is what Apply reads.
type Input struct {
	Env        Env
	Decisions  []PersonDecision
	Classifier Classifier // nil means no model
	// Signals adds the signals that the resource does not have, as the curricula and the units of institutions. Nil means none.
	Signals func(r *resource.Resource, rec *Record)
	By      string // the build and its version
	Day     string // YYYY-MM-DD
}

// Item is one item of the queue res-scope.
type Item struct {
	ID     string  `json:"id"`
	Reason string  `json:"reason"`
	Score  float64 `json:"score,omitempty"`
}

// Counts are the outcomes of one run.
type Counts struct {
	ByValue map[string]int // the live resources of each outcome after the run
	Changed int            // the resources whose outcome changed
	Open    int            // the decision open count of RS9.6
}

// Apply sets the scope of each resource: the decision of a person first, then the tests S1 to S7 and the border rules, then the classifier. A record that nothing decides is deferred for DeferDays. It gives the items of the queue res-scope: the records in the middle band of the classifier and the deferred records past their date.
func Apply(rs []resource.Resource, in Input) (Counts, []Item, error) {
	day, err := time.Parse(time.DateOnly, in.Day)
	if err != nil {
		return Counts{}, nil, fmt.Errorf("scope: the day %q is not YYYY-MM-DD", in.Day)
	}
	until := day.AddDate(0, 0, DeferDays).Format(time.DateOnly)
	person := map[string]PersonDecision{}
	for _, d := range in.Decisions {
		if d.Kind == "scope" {
			person[d.ID] = d
		}
	}
	c := Counts{ByValue: map[string]int{}}
	var items []Item
	for i := range rs {
		r := &rs[i]
		old := r.Scope
		next := decide(r, in, person, until)
		if next.Value == resource.Deferred && old != nil && old.Value == resource.Deferred && old.Until != "" && old.Rule == next.Rule {
			next.Until = old.Until // the date of the first deferral stays
		}
		if next.Value == resource.Deferred && next.Until <= in.Day {
			items = append(items, Item{ID: r.ID, Reason: "the record is deferred past its date"})
		} else if next.Value == resource.Deferred && next.Rule == ClassifierRule {
			items = append(items, Item{ID: r.ID, Reason: next.Reason})
		}
		if old == nil || old.Value != next.Value || old.Rule != next.Rule || old.Target != next.Target {
			c.Changed++
			note := "to " + next.Value
			if old != nil {
				note = "from " + old.Value + " to " + next.Value
			}
			r.History = append(r.History, resource.Event{Op: "scope", At: in.Day, By: next.By, Note: note})
		}
		r.Scope = next
		if r.Live() {
			c.ByValue[next.Value]++
			if next.Value == resource.Deferred && next.Until <= in.Day {
				c.Open++
			}
		}
	}
	return c, items, nil
}

func decide(r *resource.Resource, in Input, person map[string]PersonDecision, until string) *resource.Scope {
	if d, ok := person[r.ID]; ok {
		return &resource.Scope{Value: d.Value, Target: d.Target, Reason: d.Reason, Rule: PersonRule, By: d.By}
	}
	if r.Scope != nil && (r.Scope.Value == resource.Duplicate || r.Scope.Rule == PersonRule) {
		s := *r.Scope
		return &s
	}
	rec := FromResource(r)
	if in.Signals != nil {
		in.Signals(r, &rec)
	}
	d := Decide(rec, in.Env)
	if d.Value != resource.Deferred {
		return &resource.Scope{Value: d.Value, Rule: d.Rule, By: in.By}
	}
	if in.Classifier == nil {
		return &resource.Scope{Value: resource.Deferred, Until: until, Reason: "no test decides the record, and the classifier has no model", By: in.By}
	}
	v, score, class := in.Classifier.Classify(r)
	if v == "" {
		return &resource.Scope{Value: resource.Deferred, Until: until, Rule: ClassifierRule, Reason: fmt.Sprintf("the classifier score %.3f is in the middle band", score), By: in.Classifier.Version()}
	}
	if v == resource.In && class != "" && len(r.MSC) == 0 {
		r.MSC = []resource.Code{{Code: class + "-XX", From: in.Classifier.Version()}}
	}
	return &resource.Scope{Value: v, Rule: ClassifierRule, Reason: fmt.Sprintf("the classifier score of in is %.3f", score), By: in.Classifier.Version()}
}
