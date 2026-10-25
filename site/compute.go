package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/compute"
)

// runView is the "run this" panel of a computation (spec 08.7): the command that runs the record again, what the record names, and the runs of the build next to the recorded hash.
type runView struct {
	Command  string // the command that runs the record and checks it
	Image    string
	Entry    link // the entry point, with a link to the code
	Inputs   string
	Seed     string
	Output   string // the recorded output hash, or ""
	Summary  string
	Cert     *compute.Certificate
	Checker  link // the checker program, with a link to the code
	Runs     []archRun
	HasBuild bool // the site has the runs of a build
}

// archRun is one run of the build on one architecture.
type archRun struct {
	Arch, Output, Runtime, Problem string
	Same, Verified                 bool // Same: the output hash is the recorded hash
	Seconds                        string
}

// loadRuns reads the runs of the computations from the build, when the site has one.
func (b *builder) loadRuns() error {
	if b.opt.Build == "" {
		return nil
	}
	runs, err := compute.ReadRuns(filepath.Join(b.opt.Build, build.ComputeDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("site: %w", err)
	}
	b.runs = runs
	return nil
}

// runOf returns the panel of a computation node, or nil when the node is not a computation or its record is not valid.
func (b *builder) runOf(v *nodeView) *runView {
	if v.Kind != "computation" || v.Retired != nil {
		return nil
	}
	rec, err := compute.FromNode(v.Node)
	if err != nil {
		return nil
	}
	code := func(rel string) link {
		return link{Text: rel, URL: "https://github.com/" + b.opt.Repo + "/blob/main/" + rec.Dir() + "/" + rel}
	}
	r := &runView{Command: "m2 compute verify " + rec.Tag, Image: rec.Image, Entry: code(rec.Entry), Seed: "none", HasBuild: b.opt.Build != ""}
	if rec.Inputs != nil {
		data, err := json.Marshal(rec.Inputs)
		if err == nil {
			r.Inputs = string(data)
		}
	}
	if rec.Seed != nil {
		r.Seed = strconv.FormatInt(*rec.Seed, 10)
	}
	if rec.Output != nil {
		r.Output, r.Summary = rec.Output.Hash, rec.Output.Summary
	}
	if rec.Certificate != nil {
		r.Cert = rec.Certificate
		if rec.Certificate.CheckerEntry != "" {
			r.Checker = code(rec.Certificate.CheckerEntry)
		}
	}
	for _, arch := range compute.Arches {
		run := b.runs[rec.Tag][arch]
		if run == nil {
			continue
		}
		r.Runs = append(r.Runs, archRun{
			Arch: arch, Output: run.Output, Runtime: run.Runtime, Problem: run.Problem,
			Same: r.Output != "" && run.Output == r.Output, Verified: run.Verified,
			Seconds: strconv.FormatFloat(run.Seconds, 'f', 1, 64),
		})
	}
	return r
}
