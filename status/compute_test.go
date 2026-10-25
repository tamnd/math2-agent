package status

import (
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/compute"
)

func TestComputation(t *testing.T) {
	const (
		img  = "m2-python@sha256:7777777777777777777777777777777777777777777777777777777777777777"
		out  = "blake3:1111111111111111111111111111111111111111111111111111111111111111"
		cert = "blake3:2222222222222222222222222222222222222222222222222222222222222222"
	)
	record := "claim: C.\nengine:\n  image: " + img + "\n  entry: main.py\noutput:\n  hash: " + out + "\n"
	withCert := record + "certificate:\n  kind: witness\n  artefact: " + cert + "\n  checker: " + img + "\n  checker_entry: check.py\n"
	good := func(arch string) *compute.Run {
		return &compute.Run{Tag: "1A0000", Image: img, Entry: "main.py", Arch: arch, Network: "none", Output: out, Certificate: cert, Verified: true, Check: &compute.Check{Accepted: true}}
	}
	both := func(change func(amd, arm *compute.Run)) map[string]map[string]*compute.Run {
		amd, arm := good("amd64"), good("arm64")
		if change != nil {
			change(amd, arm)
		}
		runs := map[string]*compute.Run{}
		if amd != nil && amd.Tag != "" {
			runs["amd64"] = amd
		}
		if arm != nil && arm.Tag != "" {
			runs["arm64"] = arm
		}
		return map[string]map[string]*compute.Run{"1A0000": runs}
	}
	tests := []struct {
		name   string
		front  string
		runs   map[string]map[string]*compute.Run
		want   Level
		reason string
	}{
		{"no certificate", record, both(nil), Claimed, "no certificate"},
		{"an experiment", "role: experiment\n" + withCert, both(nil), Claimed, "an observation"},
		{"two runs that agree", withCert, both(nil), Certified, "is E4"},
		{"no runs", withCert, nil, Claimed, "no run on amd64"},
		{"one run", withCert, both(func(_, arm *compute.Run) { arm.Tag = "" }), Claimed, "no run on arm64"},
		{"a run that is not verified", withCert, both(func(amd, _ *compute.Run) { amd.Verified, amd.Problem = false, "the run exits with the code 1" }), Claimed, "exits with the code 1"},
		{"a run of another image", withCert, both(func(amd, _ *compute.Run) { amd.Image = "m2-python@sha256:" + strings.Repeat("6", 64) }), Claimed, "another record"},
		{"a checker that rejects", withCert, both(func(_, arm *compute.Run) { arm.Check.Accepted = false }), Claimed, "did not accept"},
		{"another output hash", withCert, both(func(_, arm *compute.Run) { arm.Output = "blake3:" + strings.Repeat("3", 64) }), Claimed, "other hashes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := find(t, Compute(Input{Nodes: parse(t, "1A0000 computation\n"+tt.front), Runs: tt.runs}), "1A0000")
			if s.Evidence != tt.want {
				t.Errorf("evidence %s, want %s. Reasons: %q", s.Evidence, tt.want, s.Reasons)
			}
			if r := strings.Join(s.Reasons, " "); !strings.Contains(r, tt.reason) {
				t.Errorf("the reasons %q do not say %q", r, tt.reason)
			}
		})
	}
}
