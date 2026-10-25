package status

import (
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/compute"
	"github.com/tamnd/math2-agent/node"
)

// computed gives the evidence level of a computation (spec 08.3). A computation is E1. It is E4 when it has a certificate, it is not an experiment, and the verified runs on the two runner architectures agree with each other and with the record (rule C02).
func computed(in Input, n *node.Node, s *Status) {
	rec, err := compute.FromNode(n)
	if err != nil {
		s.Reasons = append(s.Reasons, fmt.Sprintf("The record is not valid, so the level stays at E1: %s.", strings.TrimPrefix(err.Error(), "compute: ")))
		return
	}
	switch {
	case rec.Role == "experiment":
		s.Reasons = append(s.Reasons, "An experiment is an observation, so the level stays at E1.")
		return
	case rec.Certificate == nil:
		s.Reasons = append(s.Reasons, "The computation has no certificate, so the level stays at E1.")
		return
	}
	item := Item{Kind: "computation", Ref: rec.Tag, Detail: rec.Certificate.Kind, Level: Claimed}
	if why := certified(rec, in.Runs[rec.Tag]); why != "" {
		s.Items = append(s.Items, item)
		s.Reasons = append(s.Reasons, fmt.Sprintf("The %s certificate is not certified, because %s. The level stays at E1.", rec.Certificate.Kind, why))
		return
	}
	item.Level = Certified
	s.Items = append(s.Items, item)
	s.Evidence = Certified
	s.Reasons = append(s.Reasons, fmt.Sprintf("The runs on %s give the same output and the same certificate, and the fixed checker accepts the %s certificate, so the level is E4.", strings.Join(compute.Arches, " and "), rec.Certificate.Kind))
}

// certified returns why the runs do not certify the record, or "" when they do.
func certified(rec *compute.Record, runs map[string]*compute.Run) string {
	var first *compute.Run
	for _, arch := range compute.Arches {
		run := runs[arch]
		switch {
		case run == nil:
			return "the computation has no run on " + arch
		case run.Image != rec.Image || run.Entry != rec.Entry:
			return "the run on " + arch + " is of another record"
		case !run.Verified:
			if run.Problem != "" {
				return "the run on " + arch + " is not verified: " + run.Problem
			}
			return "the run on " + arch + " is not verified"
		case rec.Output == nil || run.Output != rec.Output.Hash || run.Certificate != rec.Certificate.Artefact:
			return "the run on " + arch + " gives other hashes than the record"
		case run.Check == nil || !run.Check.Accepted:
			return "the checker did not accept the certificate on " + arch
		case first != nil && (run.Output != first.Output || run.Certificate != first.Certificate):
			return "the runs on the two architectures do not agree"
		}
		first = run
	}
	return ""
}
