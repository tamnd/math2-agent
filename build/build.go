// Package build makes the derived data of a math2 working tree (spec 07.6).
//
// [Run] runs the steps in order. The lean step runs lake build in the lean/ directory of the tree. The extract step runs M2Extract on each formal declaration that a node names and on each declaration with the attribute @[m2 "TAG"], and writes extract.json. The step also runs the degenerate-statement checks of spec 10.4 on each of these declarations that is a theorem. The replay step checks again each binding that passes each check of E5 except the replay, with the comparator and the independent kernels of the toolchain (spec 07.7), and writes replay.json. The battery step runs the machine checks of spec 10.4 on each Lean binding and writes battery.json. The status step computes the evidence level and the fidelity level of each node from the nodes, extract.json, replay.json, battery.json and the reviews, and writes status.json (spec 04.7). The graph step derives the graph from the nodes and writes it as Parquet to graph/ (spec 05.1). The store step writes the serving store, store.db, from the nodes, status.json and the aliases (spec 05.7). The search step writes the search index, search.db, from the nodes, status.json, the aliases and extract.json (spec 14). The inventory step counts the rows of the inventory, with the open count of corpus spec K2.1, and writes inventory.json. A tree with no records/inventory/ directory has no inventory step. The site step comes in a later issue.
//
// Each step has an input hash and an output hash. When the input hash of a step and the hash of its files are the same as in the manifest of the last build, the step does not run again. The manifest records the Lean toolchain, the Mathlib commit and the checkers of the toolchain. The output hash covers the files of the extract, replay, battery, status, graph, store, search and inventory steps. Two builds of the same tree give the same output hash (rule R01).
package build

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/battery"
	"github.com/tamnd/math2-agent/compute"
	"github.com/tamnd/math2-agent/format"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/replay"
	"github.com/tamnd/math2-agent/search"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/store"
)

// The steps, in the order that Run runs them.
const (
	Lean      = "lean"
	Extract   = "extract"
	Replay    = "replay"
	Battery   = "battery"
	Status    = "status"
	Graph     = "graph"
	Store     = "store"
	Search    = "search"
	Inventory = "inventory"
)

// Steps returns the names of the steps in order.
func Steps() []string {
	return []string{Lean, Extract, Replay, Battery, Status, Graph, Store, Search, Inventory}
}

// The files of the build, from the output directory.
const (
	ManifestFile  = "manifest.json"
	ExtractFile   = "extract.json"
	ReplayFile    = "replay.json"
	BatteryFile   = "battery.json"
	StatusFile    = "status.json"
	GraphDir      = "graph"
	StoreFile     = "store.db"
	SearchFile    = "search.db"
	InventoryFile = "inventory.json"
	ComputeDir    = "compute" // the runs of m2 compute, in TAG/ARCH/
)

// Checkers are the programs of the Lean toolchain that check an environment again (spec 07.7). The manifest records the hash of each one that the toolchain has.
var Checkers = []string{"leanchecker", "leanchecker-paranoid", "lean4lean", "nanoda_bin", "con-leche", "con-ron", "leanexport"}

// Options are the options of Run.
type Options struct {
	Root  string // the math2 working tree
	Out   string // the output directory. Empty means build/ in Root.
	Cache string // the directory where M2Extract is built. Empty means m2 in the user cache directory.
	Only  string // run only this step. Empty means all the steps.

	// NoSandbox runs the checkers of the replay step without the bwrap sandbox, which needs Linux. A replay without the sandbox is not trustworthy, thus it does not give E5.
	NoSandbox bool
	Modules   []string  // the modules that the extract step loads. Empty means Mathlib and Math2.
	Day       time.Time // the day of the open count. The zero time means the date of the HEAD commit of the tree, or today when the tree is not a git repository.
	Version   string    // the m2 version, for the manifest
	Log       io.Writer // one line for each step. Nil means no log.
}

// Manifest is the record of a build. Run writes it to manifest.json in the output directory.
type Manifest struct {
	M2          string            `json:"m2"`           // the m2 version
	Lean        string            `json:"lean"`         // the toolchain in lean/lean-toolchain
	LeanGithash string            `json:"lean_githash"` // the commit of the toolchain, as M2Extract gives it
	Mathlib     string            `json:"mathlib"`      // the Mathlib commit in lean/lake-manifest.json
	Checkers    map[string]string `json:"checkers"`     // the hash of each checker program of the toolchain
	Steps       []Step            `json:"steps"`
	Counts      Counts            `json:"counts"`
	Output      string            `json:"output"` // the hash of the files of the steps after the lean step
}

// Step is the record of one step.
type Step struct {
	Name   string `json:"name"`
	Input  string `json:"input"`  // the hash of the inputs
	Output string `json:"output"` // the hash of the files that the step made
	Cached bool   `json:"-"`      // the step did not run, because its inputs and files did not change
}

// Counts are the sizes of the derived data.
type Counts struct {
	Nodes    int `json:"nodes"`
	Edges    int `json:"edges"`
	Records  int `json:"records"`
	Bindings int `json:"bindings"`
	Replays  int `json:"replays"`
	Rows     int `json:"rows,omitempty"` // the rows of the inventory that are not merged
	Open     int `json:"open,omitempty"` // the open count (corpus spec K2.1)
}

// Extraction is the content of extract.json.
type Extraction struct {
	Lean     lean.Version   `json:"lean"`
	Bindings []lean.Binding `json:"bindings"`
	Records  []lean.Record  `json:"records"`
}

// Run builds the tree and returns the manifest. A tree without a lean/ directory has no lean and extract steps.
func Run(ctx context.Context, opt Options) (*Manifest, error) {
	if opt.Only != "" && !slices.Contains(Steps(), opt.Only) {
		return nil, fmt.Errorf("build: %q is not a step. The steps are %s", opt.Only, strings.Join(Steps(), ", "))
	}
	if opt.Out == "" {
		opt.Out = filepath.Join(opt.Root, "build")
	}
	if opt.Cache == "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		opt.Cache = filepath.Join(dir, "m2")
	}
	if len(opt.Modules) == 0 {
		opt.Modules = []string{"Mathlib", "Math2"}
	}
	b := &builder{opt: opt, project: filepath.Join(opt.Root, "lean"), old: map[string]Step{}}
	if old, err := ReadManifest(opt.Out); err == nil {
		b.prev = old
		for _, s := range old.Steps {
			b.old[s.Name] = s
		}
	}
	b.m = &Manifest{M2: opt.Version, Checkers: map[string]string{}}
	if b.prev != nil && opt.Only != "" {
		*b.m = *b.prev
		b.m.M2 = opt.Version
	}

	nodes, err := node.Load(opt.Root)
	if err != nil {
		return nil, err
	}
	// The lean/ directory can be a symbolic link. The hash of a tree reads the files below a directory, thus resolve the link first.
	project, statErr := filepath.EvalSymlinks(b.project)
	hasLean := statErr == nil
	if hasLean {
		b.project = project
	}
	if hasLean {
		if err := b.toolchain(ctx); err != nil {
			return nil, err
		}
	}
	steps := []struct {
		name string
		lean bool
		run  func(context.Context, []*node.Node) (Step, error)
	}{
		{Lean, true, b.lean},
		{Extract, true, b.extract},
		{Replay, true, b.replay},
		{Battery, true, b.battery},
		{Status, false, b.status},
		{Graph, false, b.graph},
		{Store, false, b.store},
		{Search, false, b.search},
		{Inventory, false, b.inventory},
	}
	for _, s := range steps {
		if opt.Only != "" && s.name != opt.Only {
			continue
		}
		if s.lean && !hasLean {
			b.logf("%s: skip, the tree has no lean/ directory", s.name)
			continue
		}
		if s.name == Inventory && !hasInventory(opt.Root) {
			b.logf("%s: skip, the tree has no inventory", s.name)
			continue
		}
		st, err := s.run(ctx, nodes)
		if err != nil {
			return nil, fmt.Errorf("build: %s: %w", s.name, err)
		}
		st.Name = s.name
		if st.Cached {
			b.logf("%s: up to date %s", s.name, st.Output)
		} else {
			b.logf("%s: built %s", s.name, st.Output)
		}
		b.m.Steps = slices.DeleteFunc(b.m.Steps, func(x Step) bool { return x.Name == s.name })
		b.m.Steps = append(b.m.Steps, st)
	}
	slices.SortFunc(b.m.Steps, func(x, y Step) int { return slices.Index(Steps(), x.Name) - slices.Index(Steps(), y.Name) })

	var parts []string
	for _, s := range b.m.Steps {
		if s.Name != Lean {
			parts = append(parts, s.Name+" "+s.Output)
		}
	}
	b.m.Output = hashString(strings.Join(parts, "\n"))
	data, err := json.MarshalIndent(b.m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := write(filepath.Join(opt.Out, ManifestFile), append(data, '\n')); err != nil {
		return nil, err
	}
	return b.m, nil
}

// ReadManifest reads the manifest of the build in the output directory out.
func ReadManifest(out string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(out, ManifestFile)) // #nosec G304 -- the caller names the output directory.
	if err != nil {
		return nil, err
	}
	m := &Manifest{}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("build: %s: %w", ManifestFile, err)
	}
	return m, nil
}

// ReadStatus reads status.json of the build in the output directory out. It holds the status of each node, in tag order.
func ReadStatus(out string) ([]status.Status, error) {
	data, err := os.ReadFile(filepath.Join(out, StatusFile)) // #nosec G304 -- the caller names the output directory.
	if err != nil {
		return nil, err
	}
	var st []status.Status
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("build: %s: %w", StatusFile, err)
	}
	return st, nil
}

// ReadReplays reads replay.json of the build in the output directory out. It holds the result of each replay, in order of declaration.
func ReadReplays(out string) ([]replay.Result, error) {
	data, err := os.ReadFile(filepath.Join(out, ReplayFile)) // #nosec G304 -- the caller names the output directory.
	if err != nil {
		return nil, err
	}
	var rs []replay.Result
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("build: %s: %w", ReplayFile, err)
	}
	return rs, nil
}

// ReadBattery reads battery.json of the build in the output directory out. It holds the report of each Lean binding, in order of tag and declaration.
func ReadBattery(out string) ([]battery.Report, error) {
	data, err := os.ReadFile(filepath.Join(out, BatteryFile)) // #nosec G304 -- the caller names the output directory.
	if err != nil {
		return nil, err
	}
	var rs []battery.Report
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("build: %s: %w", BatteryFile, err)
	}
	return rs, nil
}

// ReadExtraction reads extract.json of the build in the output directory out.
func ReadExtraction(out string) (*Extraction, error) {
	data, err := os.ReadFile(filepath.Join(out, ExtractFile)) // #nosec G304 -- the caller names the output directory.
	if err != nil {
		return nil, err
	}
	x := &Extraction{}
	if err := json.Unmarshal(data, x); err != nil {
		return nil, fmt.Errorf("build: %s: %w", ExtractFile, err)
	}
	return x, nil
}

type builder struct {
	opt     Options
	project string // the Lake project of the tree
	m       *Manifest
	prev    *Manifest
	old     map[string]Step // the steps of the last build
}

func (b *builder) logf(format string, args ...any) {
	if b.opt.Log != nil {
		fmt.Fprintf(b.opt.Log, format+"\n", args...)
	}
}

// cached returns the step of the last build when its input is input and its files have the hash output.
func (b *builder) cached(name, input, output string) (Step, bool) {
	s, ok := b.old[name]
	if ok && s.Input == input && s.Output == output {
		s.Cached = true
		return s, true
	}
	return Step{}, false
}

// toolchain records the Lean toolchain, the Mathlib commit and the checkers.
func (b *builder) toolchain(ctx context.Context) error {
	tc, err := lean.Toolchain(b.project)
	if err != nil {
		return err
	}
	b.m.Lean = tc
	if data, err := os.ReadFile(filepath.Join(b.project, "lake-manifest.json")); err == nil { // #nosec G304 -- the path is in the tree.
		var lm struct {
			Packages []struct {
				Name string `json:"name"`
				Rev  string `json:"rev"`
			} `json:"packages"`
		}
		if err := json.Unmarshal(data, &lm); err != nil {
			return fmt.Errorf("lake-manifest.json: %w", err)
		}
		for _, p := range lm.Packages {
			if p.Name == "mathlib" {
				b.m.Mathlib = p.Rev
			}
		}
	}
	out, err := command(ctx, b.project, "lean", "--print-prefix")
	if err != nil {
		return err
	}
	bin := filepath.Join(strings.TrimSpace(out), "bin")
	b.m.Checkers = map[string]string{}
	for _, c := range Checkers {
		h, err := hashFiles(bin, []string{c})
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		b.m.Checkers[c] = h
	}
	return nil
}

// lean runs lake build. The input is the source of the Lake project, and the output is its .olean files.
func (b *builder) lean(ctx context.Context, _ []*node.Node) (Step, error) {
	input, err := HashTree(b.project, func(rel string) bool {
		return strings.HasSuffix(rel, ".lean") || strings.HasPrefix(rel, "lakefile.") || rel == "lean-toolchain" || rel == "lake-manifest.json"
	})
	if err != nil {
		return Step{}, err
	}
	oleans := func() (string, error) {
		return HashTree(filepath.Join(b.project, ".lake", "build", "lib", "lean"), func(rel string) bool { return strings.HasSuffix(rel, ".olean") })
	}
	if out, err := oleans(); err == nil {
		if s, ok := b.cached(Lean, input, out); ok {
			return s, nil
		}
	}
	if b.m.Mathlib != "" {
		b.logf("lean: lake exe cache get")
		if _, err := command(ctx, b.project, "lake", "exe", "cache", "get"); err != nil {
			return Step{}, err
		}
	}
	b.logf("lean: lake build")
	if _, err := command(ctx, b.project, "lake", "build"); err != nil {
		return Step{}, err
	}
	out, err := oleans()
	return Step{Input: input, Output: out}, err
}

// extract runs M2Extract. The input is the output of the lean step, the program and the formal declarations of the nodes.
func (b *builder) extract(ctx context.Context, nodes []*node.Node) (Step, error) {
	exe, err := lean.Install(ctx, b.project, b.opt.Cache)
	if err != nil {
		return Step{}, err
	}
	prog, err := filepath.Rel(b.opt.Cache, exe)
	if err != nil {
		return Step{}, err
	}
	decls := formalDecls(nodes)
	olean := b.output(Lean)
	input := hashString(strings.Join(append([]string{olean, filepath.ToSlash(prog), strings.Join(b.opt.Modules, " ")}, decls...), "\n"))
	path := filepath.Join(b.opt.Out, ExtractFile)
	if out, err := hashFiles(b.opt.Out, []string{ExtractFile}); err == nil {
		if s, ok := b.cached(Extract, input, out); ok {
			if x, err := ReadExtraction(b.opt.Out); err == nil {
				b.count(x)
				return s, nil
			}
		}
	}

	sess, err := lean.Start(ctx, exe, b.project)
	if err != nil {
		return Step{}, err
	}
	x, err := run(sess, b.opt.Modules, decls)
	if cerr := sess.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Step{}, err
	}
	data, err := json.MarshalIndent(x, "", "  ")
	if err != nil {
		return Step{}, err
	}
	if err := write(path, append(data, '\n')); err != nil {
		return Step{}, err
	}
	b.count(x)
	out, err := hashFiles(b.opt.Out, []string{ExtractFile})
	return Step{Input: input, Output: out}, err
}

func (b *builder) count(x *Extraction) {
	b.m.LeanGithash = x.Lean.Githash
	b.m.Counts.Records = len(x.Records)
	b.m.Counts.Bindings = len(x.Bindings)
}

func run(s *lean.Session, modules, decls []string) (*Extraction, error) {
	x := &Extraction{}
	var err error
	if x.Lean, err = s.Version(); err != nil {
		return nil, err
	}
	if err := s.Import(modules...); err != nil {
		return nil, err
	}
	if x.Bindings, err = s.Bindings(); err != nil {
		return nil, err
	}
	slices.SortFunc(x.Bindings, func(p, q lean.Binding) int { return strings.Compare(p.Decl, q.Decl) })
	for _, bd := range x.Bindings {
		decls = append(decls, bd.Decl)
	}
	slices.Sort(decls)
	decls = slices.Compact(decls)
	if x.Records, err = s.Extract(decls, decls); err != nil {
		return nil, err
	}
	var statements []string
	for _, r := range x.Records {
		if r.Statement() {
			statements = append(statements, r.Decl)
		}
	}
	checks, err := s.Checks(lean.CheckHeartbeats, statements...)
	if err != nil {
		return nil, err
	}
	for i, r := range x.Records {
		if c, ok := checks[r.Decl]; ok {
			x.Records[i].Checks = &c
		}
	}
	return x, nil
}

// formalDecls returns the declarations and the adapters of the Lean bindings of the nodes, sorted and without duplicates.
func formalDecls(nodes []*node.Node) []string {
	var out []string
	for _, n := range nodes {
		list, _ := n.Meta["formal"].([]any)
		for _, item := range list {
			m, _ := item.(map[string]any)
			if m["system"] != "lean4" {
				continue
			}
			for _, k := range []string{"decl", "adapter"} {
				if s, _ := m[k].(string); s != "" {
					out = append(out, s)
				}
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// input returns the evidence of the build for the status: the nodes, the records of extract.json, the reviews, the reviewers, the authors of the bindings and the runs of the computations. It has the results of replay.json and battery.json when replays is set.
func (b *builder) input(nodes []*node.Node, replays bool) (status.Input, error) {
	in := status.Input{Nodes: nodes, Records: map[string]lean.Record{}, Replays: map[string]status.Replay{}}
	var err error
	if in.Reviews, err = status.LoadReviews(b.opt.Root); err != nil {
		return in, err
	}
	if in.Reviewers, err = status.LoadReviewers(b.opt.Root); err != nil {
		return in, err
	}
	if in.Authors, err = status.LoadAuthors(b.opt.Root); err != nil {
		return in, err
	}
	if b.output(Extract) != "" {
		x, err := ReadExtraction(b.opt.Out)
		if err != nil {
			return in, err
		}
		for _, r := range x.Records {
			in.Records[r.Decl] = r
		}
	}
	if in.Runs, err = compute.ReadRuns(filepath.Join(b.opt.Out, ComputeDir)); err != nil {
		return in, err
	}
	if replays && b.output(Replay) != "" {
		rs, err := ReadReplays(b.opt.Out)
		if err != nil {
			return in, err
		}
		for _, r := range rs {
			in.Replays[r.Decl] = b.verdict(r)
		}
	}
	if replays && b.output(Battery) != "" {
		reports, err := ReadBattery(b.opt.Out)
		if err != nil {
			return in, err
		}
		in.Battery = map[status.Key]bool{}
		for _, r := range reports {
			in.Battery[status.Key{Tag: r.Tag, Decl: r.Decl}] = r.F1
		}
	}
	return in, nil
}

// verdict turns a replay result into the replay of the status. A replay without the sandbox, or with fewer than two kernels, does not pass. The checkers are the kernels that accept the proof, with the hash of each program in the manifest.
func (b *builder) verdict(r replay.Result) status.Replay {
	v := status.Replay{Passed: r.Passed, Reason: r.Reason, Checkers: map[string]string{}}
	for _, k := range r.Kernels {
		v.Checkers[k] = b.m.Checkers[k]
	}
	switch {
	case r.Passed && !r.Sandboxed:
		v.Passed, v.Reason = false, "the checkers ran without the sandbox, thus the result is not trustworthy"
	case r.Passed && len(r.Kernels) < 2:
		v.Passed, v.Reason = false, fmt.Sprintf("only %d kernel accepts the proof, and E5 needs two independent kernels", len(r.Kernels))
	}
	return v
}

// replay checks again each binding that needs a replay, and writes replay.json. The input is the output of the lean and extract steps, the declarations to check with their hashes, the checkers and the sandbox option.
func (b *builder) replay(ctx context.Context, nodes []*node.Node) (Step, error) {
	in, err := b.input(nodes, false)
	if err != nil {
		return Step{}, err
	}
	var decls []string
	for _, s := range status.Compute(in) {
		for _, bd := range s.Bindings {
			if bd.NeedsReplay() {
				decls = append(decls, bd.Checked)
			}
		}
	}
	slices.Sort(decls)
	decls = slices.Compact(decls)
	lines := []string{"lean " + b.output(Lean), "extract " + b.output(Extract), fmt.Sprintf("sandbox %v", !b.opt.NoSandbox)}
	for _, c := range slices.Sorted(maps.Keys(b.m.Checkers)) {
		lines = append(lines, "checker "+c+" "+b.m.Checkers[c])
	}
	for _, d := range decls {
		r := in.Records[d]
		lines = append(lines, "decl "+d+" "+r.TypeHash+" "+r.ValueHash)
	}
	input := hashString(strings.Join(lines, "\n"))
	b.m.Counts.Replays = len(decls)
	if out, err := hashFiles(b.opt.Out, []string{ReplayFile}); err == nil {
		if s, ok := b.cached(Replay, input, out); ok {
			return s, nil
		}
	}

	results := []replay.Result{}
	if len(decls) > 0 {
		if runtime.GOOS != "linux" && !b.opt.NoSandbox {
			return Step{}, fmt.Errorf("%d declarations need a replay, and the sandbox of the checkers needs Linux. Build on Linux, or set the option to run without the sandbox. A replay without the sandbox does not give E5", len(decls))
		}
		c, err := replay.New(ctx, b.project, replay.Options{Paranoid: true, NoSandbox: b.opt.NoSandbox})
		if err != nil {
			return Step{}, err
		}
		for _, d := range decls {
			b.logf("replay: %s", d)
			r, err := c.Check(ctx, replay.TargetOf(in.Records[d]))
			if err != nil {
				return Step{}, err
			}
			results = append(results, r)
		}
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return Step{}, err
	}
	if err := write(filepath.Join(b.opt.Out, ReplayFile), append(data, '\n')); err != nil {
		return Step{}, err
	}
	out, err := hashFiles(b.opt.Out, []string{ReplayFile})
	return Step{Input: input, Output: out}, err
}

// battery runs the machine checks of spec 10.4 on each Lean binding, and writes battery.json. The input is the output of the extract and replay steps and the content hash of each node. The checks 7 and 8 use a Lean session, which starts only when a check has a pair of statements to compare. The back-translation and the critic need models, and the step does not run them, thus no binding is F1 yet.
func (b *builder) battery(ctx context.Context, nodes []*node.Node) (Step, error) {
	lines := []string{"extract " + b.output(Extract), "replay " + b.output(Replay), fmt.Sprintf("heartbeats %d", lean.CheckHeartbeats)}
	for _, n := range nodes {
		lines = append(lines, n.ID+" "+node.Hash(n))
	}
	input := hashString(strings.Join(lines, "\n"))
	if out, err := hashFiles(b.opt.Out, []string{BatteryFile}); err == nil {
		if s, ok := b.cached(Battery, input, out); ok {
			return s, nil
		}
	}
	x, err := ReadExtraction(b.opt.Out)
	if err != nil {
		return Step{}, err
	}
	in := battery.Input{Nodes: nodes, Records: map[string]lean.Record{}, Bindings: x.Bindings, Replays: map[string]replay.Result{}}
	for _, r := range x.Records {
		in.Records[r.Decl] = r
	}
	if b.output(Replay) != "" {
		rs, err := ReadReplays(b.opt.Out)
		if err != nil {
			return Step{}, err
		}
		for _, r := range rs {
			in.Replays[r.Decl] = r
		}
	}
	p := &prover{ctx: ctx, b: b}
	reports, err := battery.Run(ctx, in, battery.Options{Prover: p})
	if cerr := p.close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Step{}, err
	}
	if reports == nil {
		reports = []battery.Report{}
	}
	data, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return Step{}, err
	}
	if err := write(filepath.Join(b.opt.Out, BatteryFile), append(data, '\n')); err != nil {
		return Step{}, err
	}
	out, err := hashFiles(b.opt.Out, []string{BatteryFile})
	return Step{Input: input, Output: out}, err
}

// prover is the Prover of the battery step. It starts a Lean session with the modules of the build at the first call.
type prover struct {
	ctx  context.Context
	b    *builder
	sess *lean.Session
}

func (p *prover) Implies(heartbeats int, pairs ...[2]string) ([]lean.Implication, error) {
	if p.sess == nil {
		exe, err := lean.Install(p.ctx, p.b.project, p.b.opt.Cache)
		if err != nil {
			return nil, err
		}
		sess, err := lean.Start(p.ctx, exe, p.b.project)
		if err != nil {
			return nil, err
		}
		if err := sess.Import(p.b.opt.Modules...); err != nil {
			_ = sess.Close()
			return nil, err
		}
		p.sess = sess
	}
	return p.sess.Implies(heartbeats, pairs...)
}

func (p *prover) close() error {
	if p.sess == nil {
		return nil
	}
	return p.sess.Close()
}

// status computes the status of each node and writes status.json. The step is fast, so it always runs, and the input hash only records the inputs: the output of the extract and replay steps, the content hash of each node, the reviews and the registers of the reviewers and the bindings. The step is cached when status.json does not change, so that a new version of the rules always gives a new status.
func (b *builder) status(_ context.Context, nodes []*node.Node) (Step, error) {
	lines := []string{"extract " + b.output(Extract), "replay " + b.output(Replay), "battery " + b.output(Battery)}
	for _, n := range nodes {
		lines = append(lines, n.ID+" "+node.Hash(n))
	}
	reviews, err := HashTree(filepath.Join(b.opt.Root, "reviews"), func(rel string) bool { return strings.HasSuffix(rel, ".yaml") })
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Step{}, err
	}
	lines = append(lines, "reviews "+reviews)
	runs, err := HashTree(filepath.Join(b.opt.Out, ComputeDir), func(rel string) bool { return strings.HasSuffix(rel, "/"+compute.RunFile) })
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Step{}, err
	}
	lines = append(lines, "compute "+runs)
	for _, reg := range []string{"reviewers.reg", "bindings.reg"} {
		data, err := os.ReadFile(filepath.Join(b.opt.Root, "registers", reg)) // #nosec G304 -- the caller names the root of the tree.
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Step{}, err
		}
		lines = append(lines, reg+" "+hashString(string(data)))
	}
	input := hashString(strings.Join(lines, "\n"))

	in, err := b.input(nodes, true)
	if err != nil {
		return Step{}, err
	}
	data, err := json.MarshalIndent(status.Compute(in), "", "  ")
	if err != nil {
		return Step{}, err
	}
	data = append(data, '\n')
	path := filepath.Join(b.opt.Out, StatusFile)
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) { // #nosec G304 -- the caller names the output directory.
		if out, err := hashFiles(b.opt.Out, []string{StatusFile}); err == nil {
			if s, ok := b.cached(Status, input, out); ok {
				return s, nil
			}
		}
	}
	if err := write(path, data); err != nil {
		return Step{}, err
	}
	out, err := hashFiles(b.opt.Out, []string{StatusFile})
	return Step{Input: input, Output: out}, err
}

// output returns the output hash of the step name in this build, or in the last build when the step did not run in this build.
func (b *builder) output(name string) string {
	for _, s := range b.m.Steps {
		if s.Name == name {
			return s.Output
		}
	}
	return b.old[name].Output
}

// graph derives the graph and writes it as Parquet. The input is the content hash of each node.
func (b *builder) graph(_ context.Context, nodes []*node.Node) (Step, error) {
	var lines []string
	for _, n := range nodes {
		lines = append(lines, n.ID+" "+node.Hash(n))
	}
	input := hashString(strings.Join(lines, "\n"))
	dir := filepath.Join(b.opt.Out, GraphDir)
	all := func(string) bool { return true }
	g := graph.Build(nodes)
	b.m.Counts.Nodes, b.m.Counts.Edges = len(g.Nodes), len(g.Edges)
	if out, err := HashTree(dir, all); err == nil {
		if s, ok := b.cached(Graph, input, out); ok {
			return s, nil
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return Step{}, err
	}
	if _, err := g.WriteParquet(dir); err != nil {
		return Step{}, err
	}
	out, err := HashTree(dir, all)
	return Step{Input: input, Output: out}, err
}

// store writes the serving store from the nodes, status.json and the aliases. The input is the version of the store, the output of the status and graph steps and the hash of registers/aliases.reg. The aliases of the nodes are in the node hashes, thus in the output of the graph step.
func (b *builder) store(_ context.Context, nodes []*node.Node) (Step, error) {
	reg, err := os.ReadFile(filepath.Join(b.opt.Root, filepath.FromSlash(format.AliasRegister))) // #nosec G304 -- the caller names the root of the tree.
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Step{}, err
	}
	input := hashString(strings.Join([]string{"store " + store.Version, "status " + b.output(Status), "graph " + b.output(Graph), "aliases " + hashString(string(reg))}, "\n"))
	if out, err := hashFiles(b.opt.Out, []string{StoreFile}); err == nil {
		if s, ok := b.cached(Store, input, out); ok {
			return s, nil
		}
	}
	st, err := ReadStatus(b.opt.Out)
	if err != nil {
		return Step{}, err
	}
	aliases, err := format.ReadAliases(b.opt.Root, nodes)
	if err != nil {
		return Step{}, err
	}
	if err := store.Write(filepath.Join(b.opt.Out, StoreFile), store.Input{Nodes: nodes, Status: st, Aliases: aliases}); err != nil {
		return Step{}, err
	}
	out, err := hashFiles(b.opt.Out, []string{StoreFile})
	return Step{Input: input, Output: out}, err
}

// search writes the search index from the nodes, status.json, the aliases and extract.json. The input is the version of the index, the output of the extract, status and graph steps and the hash of registers/aliases.reg. A tree without a lean/ directory has no extract.json, and its index has no declarations.
func (b *builder) search(_ context.Context, nodes []*node.Node) (Step, error) {
	reg, err := os.ReadFile(filepath.Join(b.opt.Root, filepath.FromSlash(format.AliasRegister))) // #nosec G304 -- the caller names the root of the tree.
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Step{}, err
	}
	input := hashString(strings.Join([]string{"search " + search.Version, "extract " + b.output(Extract), "status " + b.output(Status), "graph " + b.output(Graph), "aliases " + hashString(string(reg))}, "\n"))
	if out, err := hashFiles(b.opt.Out, []string{SearchFile}); err == nil {
		if s, ok := b.cached(Search, input, out); ok {
			return s, nil
		}
	}
	st, err := ReadStatus(b.opt.Out)
	if err != nil {
		return Step{}, err
	}
	aliases, err := format.ReadAliases(b.opt.Root, nodes)
	if err != nil {
		return Step{}, err
	}
	in := search.Input{Nodes: nodes, Status: st, Aliases: aliases}
	if x, err := ReadExtraction(b.opt.Out); err == nil {
		in.Records, in.Bindings = x.Records, x.Bindings
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Step{}, err
	}
	if err := search.Write(filepath.Join(b.opt.Out, SearchFile), in); err != nil {
		return Step{}, err
	}
	out, err := hashFiles(b.opt.Out, []string{SearchFile})
	return Step{Input: input, Output: out}, err
}

// hasInventory reports whether the tree at root has the rows of an inventory.
func hasInventory(root string) bool {
	_, err := os.Stat(filepath.Join(root, inventory.RowsDir))
	return err == nil
}

// inventory counts the rows of the inventory on the day of the build and writes inventory.json (corpus spec K4.9). The input is the hash of the rows, the hash of the mentions and the day, because a deferred row is open again when its date passes.
func (b *builder) inventory(ctx context.Context, _ []*node.Node) (Step, error) {
	day := b.opt.Day
	if day.IsZero() {
		day = time.Now().UTC()
		if out, err := command(ctx, b.opt.Root, "git", "log", "-1", "--format=%cs"); err == nil {
			if t, err := time.Parse(time.DateOnly, strings.TrimSpace(out)); err == nil {
				day = t
			}
		}
	}
	lines := []string{"day " + day.Format(time.DateOnly)}
	for _, dir := range []string{inventory.RowsDir, inventory.MentionsDir} {
		h, err := HashTree(filepath.Join(b.opt.Root, dir), func(rel string) bool { return strings.HasSuffix(rel, ".jsonl") })
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Step{}, err
		}
		lines = append(lines, dir+" "+h)
	}
	input := hashString(strings.Join(lines, "\n"))
	if out, err := hashFiles(b.opt.Out, []string{InventoryFile}); err == nil {
		if s, ok := b.cached(Inventory, input, out); ok {
			b.m.Counts.Rows, b.m.Counts.Open = b.prev.Counts.Rows, b.prev.Counts.Open
			return s, nil
		}
	}
	rows, err := inventory.ReadRows(b.opt.Root)
	if err != nil {
		return Step{}, err
	}
	ms, err := inventory.ReadMentions(b.opt.Root)
	if err != nil {
		return Step{}, err
	}
	sum := inventory.Summarize(rows, ms, day)
	data, err := json.MarshalIndent(struct {
		Day string `json:"day"`
		*inventory.Summary
	}{day.Format(time.DateOnly), sum}, "", "  ")
	if err != nil {
		return Step{}, err
	}
	if err := write(filepath.Join(b.opt.Out, InventoryFile), append(data, '\n')); err != nil {
		return Step{}, err
	}
	b.m.Counts.Rows, b.m.Counts.Open = sum.Rows, sum.Open
	out, err := hashFiles(b.opt.Out, []string{InventoryFile})
	return Step{Input: input, Output: out}, err
}

// command runs a program in dir and returns its standard output. The error has the output of the program.
func command(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- the programs and the arguments are fixed in this package.
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String() + "\n" + string(out))
		if lines := strings.Split(msg, "\n"); len(lines) > 40 {
			msg = strings.Join(lines[len(lines)-40:], "\n")
		}
		return "", fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, msg)
	}
	return string(out), nil
}

func hashString(s string) string {
	sum := blake3.Sum256([]byte(s))
	return node.HashPrefix + hex.EncodeToString(sum[:])
}

// HashTree returns the hash of the files below dir for which keep is true. The graph step and the release manifest use it. It does not go into the .lake directory of a Lake project unless dir is in it. It hashes the path from dir and the bytes of each file, in path order.
func HashTree(dir string, keep func(rel string) bool) (string, error) {
	var rels []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".lake" && path != dir {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil || d.IsDir() {
			return err
		}
		if rel = filepath.ToSlash(rel); keep(rel) {
			rels = append(rels, rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hashFiles(dir, rels)
}

// hashFiles hashes the path and the bytes of each file, in the order of rels.
func hashFiles(dir string, rels []string) (string, error) {
	slices.Sort(rels)
	h := blake3.New()
	for _, rel := range rels {
		f, err := os.Open(filepath.Join(dir, filepath.FromSlash(rel))) // #nosec G304 -- the paths come from the tree or from this package.
		if err != nil {
			return "", err
		}
		info, err := f.Stat()
		if err == nil {
			_, _ = fmt.Fprintf(h, "%s %d\n", rel, info.Size())
			_, err = io.Copy(h, f)
		}
		_ = f.Close()
		if err != nil {
			return "", err
		}
	}
	return node.HashPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

func write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the build output is public.
		return err
	}
	return os.WriteFile(path, data, 0o644) // #nosec G306 -- the build output is public.
}
