package understand

import (
	"fmt"
	"slices"
	"strconv"
)

// The decisions of the review queues (AX7.4 to AX7.6, rule D11). A person who works a queue writes one line for each record or prose item that they decide, in records/review/decisions.jsonl. A decision holds ids and marks, never text of a paper (AX11.1), so it can name a record of any class. A record leaves its queue once each of its queued parts has a decision; the import then applies the confirmed proof targets (AX5.2).

// DecisionsFile is the register of the decisions of the review queues, relative to the root of the tree.
const DecisionsFile = "records/review/decisions.jsonl"

// The decisions of the queue proof-target: the target that the pass gives is right, the proof proves another item, or it proves no item of the paper.
const (
	TargetConfirm = "confirm"
	TargetOther   = "other"
	TargetNone    = "none"
)

// The decisions of the queue prose-review: the span is a prose item of the kind of the record, or it is not.
const (
	ProseAccept = "accept"
	ProseReject = "reject"
)

// Decision is what a person decides for one record of a review queue.
type Decision struct {
	Record   string `json:"record"`           // the record, ax://paper/ID#TAG
	Queue    string `json:"queue"`            // proof-target, unfold-review or prose-review
	Hash     string `json:"hash,omitempty"`   // for unfold-review, the hash of the unfolded form that the person read
	Item     string `json:"item,omitempty"`   // for prose-review, the sentences of the prose item, as FROM-TO
	Decision string `json:"decision"`         // for proof-target confirm, other or none; for unfold-review a verdict; for prose-review accept or reject
	Kind     string `json:"kind,omitempty"`   // for an accepted prose item, the kind when it is not the kind of the record
	Target   string `json:"target,omitempty"` // for proof-target confirm and other, the item that the proof proves, ax://paper/ID#TAG
	By       string `json:"by"`
	On       string `json:"on"`
}

// key is the part of a record that the decision decides.
func (d Decision) key() string { return d.Queue + " " + d.Record + " " + d.Hash + " " + d.Item }

// ReadDecisions reads the register of the decisions from the tree at root. A tree with no register gives none and no error. A decision with a missing field, a decision that its queue does not take, or a second decision of one part is an error.
func ReadDecisions(root string) ([]Decision, error) {
	out, err := readGold[Decision](root, DecisionsFile)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i, d := range out {
		if err := d.valid(); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", DecisionsFile, i+1, err)
		}
		if seen[d.key()] {
			return nil, fmt.Errorf("%s:%d: %s of the queue %s has a decision already", DecisionsFile, i+1, d.Record, d.Queue)
		}
		seen[d.key()] = true
	}
	return out, nil
}

func (d Decision) valid() error {
	if d.Record == "" || d.Queue == "" || d.Decision == "" || d.By == "" || d.On == "" {
		return fmt.Errorf("a decision needs record, queue, decision, by and on")
	}
	switch d.Queue {
	case ProofTarget:
		if !slices.Contains([]string{TargetConfirm, TargetOther, TargetNone}, d.Decision) {
			return fmt.Errorf("the decision %q of proof-target is not confirm, other or none", d.Decision)
		}
		if (d.Decision == TargetNone) != (d.Target == "") {
			return fmt.Errorf("a decision confirm or other of proof-target needs its target, and none takes no target")
		}
	case UnfoldReview:
		if d.Hash == "" || d.Decision == "unsure" || !slices.Contains(Verdicts, d.Decision) {
			return fmt.Errorf("a decision of unfold-review needs the hash and a verdict equivalent, stronger, weaker or different")
		}
	case ProseReview:
		if _, _, ok := span(d.Item); !ok || d.Decision != ProseAccept && d.Decision != ProseReject {
			return fmt.Errorf("a decision of prose-review needs the item as FROM-TO and accept or reject")
		}
	default:
		return fmt.Errorf("the queue %q takes no decision", d.Queue)
	}
	return nil
}

// span parses FROM-TO.
func span(s string) (int, int, bool) {
	for i := range len(s) {
		if s[i] == '-' {
			from, err1 := strconv.Atoi(s[:i])
			to, err2 := strconv.Atoi(s[i+1:])
			return from, to, err1 == nil && err2 == nil && from > 0 && from <= to
		}
	}
	return 0, 0, false
}

// decided reports whether the decisions decide each queued part of the record.
func decided(r Record, queue string, by map[string]bool) bool {
	switch queue {
	case ProofTarget:
		return by[Decision{Queue: ProofTarget, Record: r.ID}.key()]
	case UnfoldReview:
		return r.Unfold != nil && by[Decision{Queue: UnfoldReview, Record: r.ID, Hash: r.Unfold.Hash}.key()]
	case ProseReview:
		n := 0
		for _, p := range r.Prose {
			if p.Queue == "" {
				continue
			}
			n++
			if !by[Decision{Queue: ProseReview, Record: r.ID, Item: strconv.Itoa(p.From) + "-" + strconv.Itoa(p.To)}.key()] {
				return false
			}
		}
		return n > 0
	}
	return false
}

// ProofTargets returns the item that each decided orphan proof proves, by the id of the proof: the target of a decision confirm or other.
func ProofTargets(ds []Decision) map[string]string {
	out := map[string]string{}
	for _, d := range ds {
		if d.Queue == ProofTarget && d.Target != "" {
			out[d.Record] = d.Target
		}
	}
	return out
}
