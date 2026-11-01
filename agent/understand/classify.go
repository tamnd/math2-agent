package understand

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// The pass classify (AX7.2) gives the item kind of each object that the extractor could not place, and of each remark and problem, with a confidence and two flags.

// ReviewBelow is the confidence below which a classification goes to the queue classify-review (AX7.2).
const ReviewBelow = 0.7

// ClassifyReview is the queue of the classifications with a low confidence.
const ClassifyReview = "classify-review"

// The flags of classify.
const (
	ClaimInRemark       = "claim-in-remark"
	ConjectureCandidate = "conjecture-candidate"
)

// Kinds are the item kinds of AX1.3 that classify can give. The enum of schema/understand/classify.v1.json has the same kinds.
var Kinds = []string{
	"statement/theorem", "statement/lemma", "statement/proposition", "statement/corollary", "statement/claim", "statement/fact", "statement/axiom", "statement/criterion",
	"definition", "definition/notation", "definition/terminology", "definition/convention", "definition/construction", "definition/algorithm",
	"example", "example/non-example", "counterexample", "conjecture", "problem", "exercise", "proof",
	"context/hypothesis", "remark", "other",
}

var classifyPass = pass{
	tool:  "classify_items",
	about: "Give the item kind of each item of the paper. Give all the items in one call.",
	other: "Give the item kind of each item. Only for the pass classify.",
	chars: 400, // the start of the body (AX7.2)
	reads: func(item string) bool {
		k, _, _ := strings.Cut(item, "/")
		return k == "unknown" || k == "remark" || k == "problem"
	},
	brief: func(b *strings.Builder, p *Paper) {
		fmt.Fprintf(b, "Give the item kind of each of the %d items below with one call of classify_items. The kinds are: %s. Give a confidence from 0 to 1. Give the flag %s to a remark or an observation that states a claim, and the flag %s to a problem or a question with a yes or no answer.\n\n", len(p.Items), strings.Join(Kinds, ", "), ClaimInRemark, ConjectureCandidate)
		// The pilot of #256 gave remark to most of the items that the gold marks of C give another kind: the model kept the head of the paper.
		b.WriteString("The head of an item, such as Remark, is the guess of the extractor and is often wrong. Give the kind from what the body does: an instance of a notion or a theorem is example, an instance that shows a claim fails is counterexample, a body that fixes symbols is definition/notation, one that fixes a word is definition/terminology or definition, one that fixes a convention is definition/convention, one that builds an object is definition/construction, and one that states a result is a kind of statement. Give remark only to a comment that is none of these.\n\n")
	},
	check: checkClassify,
}

func checkClassify(r *Record, _ *Paper, it Item, raw json.RawMessage) ([]string, string) {
	var in struct {
		Kind       string
		Confidence *float64
		Flags      []string
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Sprintf("the result of %s: %v", it.ID, err)
	}
	switch {
	case !slices.Contains(Kinds, in.Kind):
		return nil, fmt.Sprintf("the kind %q of %s is not one of the kinds", in.Kind, it.ID)
	case in.Confidence == nil || *in.Confidence < 0 || *in.Confidence > 1:
		return nil, fmt.Sprintf("%s has no confidence from 0 to 1", it.ID)
	}
	r.Was, r.Kind, r.Confidence = it.Item, in.Kind, *in.Confidence
	var left []string
	for _, f := range in.Flags {
		k, _, _ := strings.Cut(in.Kind, "/")
		switch {
		case slices.Contains(r.Flags, f):
		case f == ClaimInRemark && (k == "remark" || strings.HasPrefix(it.Item, "remark")):
			r.Flags = append(r.Flags, f)
		case f == ConjectureCandidate && (k == "problem" || k == "conjecture"):
			r.Flags = append(r.Flags, f)
		default:
			left = append(left, fmt.Sprintf("the flag %q, which is not for the kind %s", f, in.Kind))
		}
	}
	if r.Confidence < ReviewBelow {
		r.Queue = ClassifyReview
	}
	return left, ""
}
