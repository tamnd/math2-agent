// Package build makes the derived data of a math2 working tree (spec 07.6).
//
// [Run] runs the steps in order. The lean step runs lake build in the lean/ directory of the tree. The extract step runs M2Extract on each formal declaration that a node names and on each declaration with the attribute @[m2 "TAG"], and writes extract.json. The status step computes the evidence level and the fidelity level of each node from the nodes, extract.json and the reviews, and writes status.json (spec 04.7). The graph step derives the graph from the nodes and writes it as Parquet to graph/ (spec 05.1). The search and site steps come in later issues.
//
// Each step has an input hash and an output hash. When the input hash of a step and the hash of its files are the same as in the manifest of the last build, the step does not run again. The manifest records the Lean toolchain, the Mathlib commit and the checkers of the toolchain. The output hash covers the files of the extract, status and graph steps. Two builds of the same tree give the same output hash (rule R01).
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
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

// The steps, in the order that Run runs them.
const (
	Lean    = "lean"
	Extract = "extract"
	Status  = "status"
	Graph   = "graph"
)

// Steps returns the names of the steps in order.
func Steps() []string { return []string{Lean, Extract, Status, Graph} }

// The files of the build, from the output directory.
const (
	ManifestFile = "manifest.json"
	ExtractFile  = "extract.json"
	StatusFile   = "status.json"
	GraphDir     = "graph"
)

// Checkers are the programs of the Lean toolchain that check an environment again (spec 07.7). The manifest records the hash of each one that the toolchain has.
var Checkers = []string{"leanchecker", "leanchecker-paranoid", "lean4lean", "nanoda_bin", "con-leche", "con-ron", "leanexport"}

// Options are the options of Run.
type Options struct {
	Root    string    // the math2 working tree
	Out     string    // the output directory. Empty means build/ in Root.
	Cache   string    // the directory where M2Extract is built. Empty means m2 in the user cache directory.
	Only    string    // run only this step. Empty means all the steps.
	Modules []string  // the modules that the extract step loads. Empty means Mathlib and Math2.
	Version string    // the m2 version, for the manifest
	Log     io.Writer // one line for each step. Nil means no log.
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
	Output      string            `json:"output"` // the hash of the files of the extract, status and graph steps
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
	_, statErr := os.Stat(b.project)
	hasLean := statErr == nil
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
		{Status, false, b.status},
		{Graph, false, b.graph},
	}
	for _, s := range steps {
		if opt.Only != "" && s.name != opt.Only {
			continue
		}
		if s.lean && !hasLean {
			b.logf("%s: skip, the tree has no lean/ directory", s.name)
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
	input, err := hashTree(b.project, func(rel string) bool {
		return strings.HasSuffix(rel, ".lean") || strings.HasPrefix(rel, "lakefile.") || rel == "lean-toolchain" || rel == "lake-manifest.json"
	})
	if err != nil {
		return Step{}, err
	}
	oleans := func() (string, error) {
		return hashTree(filepath.Join(b.project, ".lake", "build", "lib", "lean"), func(rel string) bool { return strings.HasSuffix(rel, ".olean") })
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
	x.Records, err = s.Extract(decls, decls)
	return x, err
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

// status computes the status of each node and writes status.json. The step is fast, so it always runs, and the input hash only records the inputs: the output of the extract step, the content hash of each node and the reviews. The step is cached when status.json does not change, so that a new version of the rules always gives a new status.
func (b *builder) status(_ context.Context, nodes []*node.Node) (Step, error) {
	lines := []string{"extract " + b.output(Extract)}
	for _, n := range nodes {
		lines = append(lines, n.ID+" "+node.Hash(n))
	}
	reviews, err := hashTree(filepath.Join(b.opt.Root, "reviews"), func(rel string) bool { return strings.HasSuffix(rel, ".yaml") })
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Step{}, err
	}
	lines = append(lines, "reviews "+reviews)
	input := hashString(strings.Join(lines, "\n"))

	in := status.Input{Nodes: nodes, Records: map[string]lean.Record{}}
	if in.Reviews, err = status.LoadReviews(b.opt.Root); err != nil {
		return Step{}, err
	}
	if b.output(Extract) != "" {
		x, err := ReadExtraction(b.opt.Out)
		if err != nil {
			return Step{}, err
		}
		for _, r := range x.Records {
			in.Records[r.Decl] = r
		}
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
	if out, err := hashTree(dir, all); err == nil {
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
	out, err := hashTree(dir, all)
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

// hashTree hashes the files below dir for which keep is true. It does not go into the .lake directory of a Lake project unless dir is in it. It hashes the path from dir and the bytes of each file, in path order.
func hashTree(dir string, keep func(rel string) bool) (string, error) {
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
