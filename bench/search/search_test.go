// Package search_test measures the quality of search on labelled queries (spec 14.7).
//
// Each line of queries.tsv is a query with the node or the declaration that it must find:
//
//	benchmark  id  target  query
//
// The benchmarks are these:
//
//   - concept: a description of a concept or a theorem in other words than the node. The metrics are MRR and recall@10.
//   - formula: a formula of a node with other names for the variables. The metric is nDCG@10.
//   - formal: a Lean type, a goal or a list of constants, with the declaration that it states. A result is correct when it is the declaration or a node that binds it. The metrics are recall@1 and recall@10.
//   - paper: the title and the abstract of a paper, with the node that the paper states. The metric is recall@10.
//
// The test adds two benchmarks that it makes from the tree. The identifier benchmark has one query for each same_as id of the nodes, and its metric is recall@1. When two nodes give the same id, each one is a correct result. The premise benchmark takes each declaration that uses other declarations of the index, takes it out of the index and asks Premises for its statement. Its metric is the recall@10 of the declarations that it uses.
//
// Each benchmark runs with all the channels, which is m2, and with each channel alone. A channel alone is the baseline of a tool that does one thing, such as a full text search or a Lean pattern search. The external tools of spec 14.7 are not in the test, because a run sends each query to the service of the tool.
//
// Without flags, the test runs on the nodes of testdata/nodes/valid and the records in testdata/records.tsv. The queries and the records are in our own words. baseline.tsv has the headline metric of each benchmark for m2, and the test fails when a metric is more than 0.02 below it. Give -update to write baseline.tsv again. Run it on another tree with:
//
//	go test ./bench/search -root DIR -queries FILE -v -report search.md
//
// DIR is a math2 working tree. The test reads its nodes, and the records of build/extract.json from m2 build when the file exists.
package search_test

import (
	"bufio"
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/format"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/search"
)

var (
	root    = flag.String("root", "", "a math2 working tree. Empty means the nodes of testdata/nodes/valid and the records of testdata/records.tsv.")
	queries = flag.String("queries", "queries.tsv", "the file of the labelled queries")
	report  = flag.String("report", "", "write the results as Markdown to this file")
	update  = flag.Bool("update", false, "write baseline.tsv again from this run")
	misses  = flag.Bool("misses", false, "log each query that m2 does not find in the top 10, or at the top for the R@1 benchmarks")
)

const baselineFile = "baseline.tsv"

// The benchmarks.
const (
	concept    = "concept"
	formula    = "formula"
	formal     = "formal"
	paper      = "paper"
	identifier = "identifier"
	premise    = "premise"
)

var benchmarks = []string{concept, formula, formal, paper, identifier, premise}

// headline is the metric of each benchmark that baseline.tsv keeps.
var headline = map[string]string{concept: "MRR", formula: "nDCG@10", formal: "R@1", paper: "R@10", identifier: "R@1", premise: "R@10"}

// The systems. m2 runs all the channels. Each other system is one channel alone.
var systems = []string{"m2", search.Lexical, search.Formula, search.Formal, search.Embedding}

type query struct {
	bench, id, target, text string
}

func readQueries(t *testing.T, path string) []query {
	t.Helper()
	f, err := os.Open(path) // #nosec G304 -- the path is a flag of the test.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []query
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.Split(line, "\t")
		if len(p) != 4 {
			t.Fatalf("%s: the line %q has %d fields, and it must have 4", path, line, len(p))
		}
		out = append(out, query{bench: p[0], id: p[1], target: p[2], text: strings.ReplaceAll(p[3], `\n`, "\n")})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// readRecords reads testdata/records.tsv. Each record gets a canonical type form with its heads: a binder for each hypothesis, around the conclusion.
func readRecords(t *testing.T) []lean.Record {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "records.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	var out []lean.Record
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.Split(line, "\t")
		if len(p) != 6 {
			t.Fatalf("records.tsv: the line %q has %d fields, and it must have 6", line, len(p))
		}
		out = append(out, lean.Record{
			Decl: p[0], Kind: "theorem", Type: canon(p[1], split(p[2])), UsesConstantsInStatement: split(p[3]), UsesDecls: split(p[4]), StatementPP: p[5],
		})
	}
	return out
}

// canon writes a canonical type form, as M2Extract writes it, with a binder for each hypothesis and the conclusion as the body.
func canon(concl string, hyps []string) string {
	var ls []string
	add := func(l string) int { ls = append(ls, l); return len(ls) - 1 }
	add("s s(u0)")
	x := add("b 0")
	var hs []int
	for _, h := range hyps {
		hs = append(hs, add("a "+strconv.Itoa(add("c "+h+" u0"))+" "+strconv.Itoa(x)))
	}
	body := add("a " + strconv.Itoa(add("c "+concl+" u0")) + " " + strconv.Itoa(x))
	for i := len(hs) - 1; i >= 0; i-- {
		body = add(fmt.Sprintf("p c %d %d", hs[i], body))
	}
	add(fmt.Sprintf("p i 0 %d", body))
	return strings.Join(ls, "\n") + "\n"
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// corpus is the input of the index and the facts that the benchmarks need.
type corpus struct {
	in    search.Input
	bound map[string][]string // the tags that bind each declaration
}

func load(t *testing.T) corpus {
	t.Helper()
	dir := *root
	if dir == "" {
		dir = filepath.Join("..", "..", "testdata", "nodes", "valid")
	}
	nodes, err := node.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := format.ReadAliases(dir, nodes)
	if err != nil {
		t.Fatal(err)
	}
	c := corpus{in: search.Input{Nodes: nodes, Aliases: aliases}, bound: map[string][]string{}}
	if *root == "" {
		c.in.Records = readRecords(t)
	} else if x, err := build.ReadExtraction(filepath.Join(dir, "build")); err == nil {
		c.in.Records, c.in.Bindings = x.Records, x.Bindings
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	for _, e := range graph.Build(nodes).Edges {
		if e.Type == graph.Formalizes {
			c.bound[e.To] = append(c.bound[e.To], e.From)
		}
	}
	for _, b := range c.in.Bindings {
		c.bound[b.Decl] = append(c.bound[b.Decl], b.Tag)
	}
	return c
}

func index(t *testing.T, in search.Input) *search.Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "search.db")
	if err := search.Write(path, in); err != nil {
		t.Fatal(err)
	}
	ix, err := search.Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	return ix
}

// scores holds the rank of the target of each query, or 0 when the target is not in the results.
type scores struct {
	ranks []int
}

func (s scores) recall(k int) float64 {
	return s.mean(func(r int) float64 {
		if r > 0 && r <= k {
			return 1
		}
		return 0
	})
}

func (s scores) mrr() float64 {
	return s.mean(func(r int) float64 {
		if r == 0 {
			return 0
		}
		return 1 / float64(r)
	})
}

// ndcg is nDCG@10 with one relevant result.
func (s scores) ndcg() float64 {
	return s.mean(func(r int) float64 {
		if r == 0 || r > 10 {
			return 0
		}
		return 1 / math.Log2(float64(r)+1)
	})
}

func (s scores) mean(f func(int) float64) float64 {
	if len(s.ranks) == 0 {
		return 0
	}
	var sum float64
	for _, r := range s.ranks {
		sum += f(r)
	}
	return sum / float64(len(s.ranks))
}

func (s scores) metric(name string) float64 {
	switch name {
	case "MRR":
		return s.mrr()
	case "R@1":
		return s.recall(1)
	case "R@10":
		return s.recall(10)
	case "nDCG@10":
		return s.ndcg()
	}
	panic("unknown metric " + name)
}

// depth is the number of results that the test asks for. A target below it has the rank 0.
const depth = 20

func TestSearch(t *testing.T) {
	c := load(t)
	ix := index(t, c.in)
	qs := readQueries(t, *queries)
	// An id that more than one node gives is a query with more than one target.
	ids := map[string][]string{}
	for _, e := range graph.Build(c.in.Nodes).Edges {
		if e.Type == graph.SameAs && e.To != "" {
			ids[e.System+":"+e.To] = append(ids[e.System+":"+e.To], e.From)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(ids)) {
		_, text, _ := strings.Cut(id, ":")
		qs = append(qs, query{bench: identifier, id: id, target: strings.Join(ids[id], ","), text: text})
	}
	results := map[string]map[string]*scores{} // benchmark, system
	for _, b := range benchmarks {
		results[b] = map[string]*scores{}
		for _, s := range systems {
			results[b][s] = &scores{}
		}
	}
	for _, q := range qs {
		if !slices.Contains(benchmarks, q.bench) {
			t.Fatalf("%s: %q is not a benchmark", q.id, q.bench)
		}
		for _, sys := range systems {
			sq := search.Query{Text: q.text, K: depth}
			if sys != "m2" {
				sq.Channels = []string{sys}
			}
			rs, err := ix.Search(sq)
			if err != nil {
				t.Fatalf("%s: %v", q.id, err)
			}
			r := rank(rs, q, c.bound)
			results[q.bench][sys].ranks = append(results[q.bench][sys].ranks, r)
			if *misses && sys == "m2" && (r == 0 || r > cutoff(q.bench)) {
				t.Logf("%s %s: %q gives rank %d, top %v", q.bench, q.id, q.text, r, top(rs, 3))
			}
		}
	}
	results[premise]["m2"].ranks = premises(t, c)

	var b strings.Builder
	b.WriteString("| Benchmark | Queries | Metric | m2 | lexical | formula | formal | embedding | best channel alone |\n|---|---|---|---|---|---|---|---|---|\n")
	base := readBaseline(t)
	next := map[string]float64{}
	for _, bench := range benchmarks {
		n := len(results[bench]["m2"].ranks)
		if n == 0 {
			continue
		}
		for _, m := range metrics(bench) {
			fmt.Fprintf(&b, "| %s | %d | %s |", bench, n, m)
			best := ""
			var bestV float64
			for _, sys := range systems {
				s := results[bench][sys]
				if len(s.ranks) == 0 {
					b.WriteString(" |")
					continue
				}
				v := s.metric(m)
				fmt.Fprintf(&b, " %.3f |", v)
				if sys != "m2" && (best == "" || v > bestV) {
					best, bestV = sys, v
				}
			}
			if best == "" {
				b.WriteString(" |\n")
			} else {
				fmt.Fprintf(&b, " %s %.3f |\n", best, bestV)
			}
		}
		m := headline[bench]
		v := results[bench]["m2"].metric(m)
		next[bench] = v
		if old, ok := base[bench]; ok && v < old-0.02 && !*update {
			t.Errorf("%s: %s is %.3f, and baseline.tsv has %.3f. A drop of more than 0.02 blocks the release", bench, m, v, old)
		}
	}
	t.Logf("\n%s", b.String())
	if *report != "" {
		if err := os.WriteFile(*report, []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if *update {
		writeBaseline(t, next)
	}
}

// cutoff is the rank that the headline metric of the benchmark counts.
func cutoff(bench string) int {
	if headline[bench] == "R@1" {
		return 1
	}
	return 10
}

func metrics(bench string) []string {
	switch bench {
	case concept:
		return []string{"MRR", "R@10"}
	case formal:
		return []string{"R@1", "R@10"}
	}
	return []string{headline[bench]}
}

// rank returns the rank of the first target of the query in the results, from 1, or 0. A target can be a list of tags with commas.
func rank(rs []search.Result, q query, bound map[string][]string) int {
	for i, r := range rs {
		if q.bench == formal {
			if r.Decl == q.target || slices.Contains(bound[q.target], r.Tag) {
				return i + 1
			}
			continue
		}
		for _, tg := range strings.Split(q.target, ",") {
			if r.Tag == tg || slices.Contains(r.Folded, tg) {
				return i + 1
			}
		}
	}
	return 0
}

func top(rs []search.Result, k int) []string {
	var out []string
	for i, r := range rs {
		if i == k {
			break
		}
		out = append(out, cmp.Or(r.Tag, "lean:"+r.Decl))
	}
	return out
}

// premises runs the premise benchmark. For each declaration whose uses are in the index, it takes the declaration out of the index, asks for the premises of its statement, and gives the rank of each declaration that it uses.
func premises(t *testing.T, c corpus) []int {
	t.Helper()
	known := map[string]bool{}
	for _, r := range c.in.Records {
		known[r.Decl] = true
		for _, u := range r.UsesDecls {
			known[u] = true
		}
	}
	var ranks []int
	for i, r := range c.in.Records {
		if r.StatementPP == "" || len(r.UsesDecls) == 0 {
			continue
		}
		in := c.in
		in.Records = slices.Delete(slices.Clone(c.in.Records), i, i+1)
		var gold []string
		for _, u := range r.UsesDecls {
			for _, o := range in.Records {
				if o.Decl == u || slices.Contains(o.UsesDecls, u) {
					gold = append(gold, u)
					break
				}
			}
		}
		if len(gold) == 0 {
			continue
		}
		ix := index(t, in)
		tag := ""
		if tags := c.bound[r.Decl]; len(tags) > 0 {
			tag = tags[0]
		}
		ps, err := ix.Premises(search.Goal{Goal: r.StatementPP, Tag: tag, K: depth})
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range gold {
			ranks = append(ranks, slices.IndexFunc(ps, func(p search.Premise) bool { return p.Decl == g })+1)
		}
	}
	return ranks
}

func readBaseline(t *testing.T) map[string]float64 {
	t.Helper()
	out := map[string]float64{}
	data, err := os.ReadFile(baselineFile)
	if errors.Is(err, fs.ErrNotExist) || *root != "" {
		return out
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		p := strings.Split(line, "\t")
		if len(p) != 3 || strings.HasPrefix(line, "#") {
			continue
		}
		v, err := strconv.ParseFloat(p[2], 64)
		if err != nil {
			t.Fatalf("%s: %v", baselineFile, err)
		}
		out[p[0]] = v
	}
	return out
}

func writeBaseline(t *testing.T, next map[string]float64) {
	t.Helper()
	if *root != "" {
		t.Fatal("-update writes the baseline of the testdata, thus give no -root")
	}
	var b strings.Builder
	b.WriteString("# benchmark\tmetric\tm2\n# The headline metric of each benchmark for m2 on the testdata. go test ./bench/search -update writes this file.\n")
	for _, bench := range benchmarks {
		if v, ok := next[bench]; ok {
			fmt.Fprintf(&b, "%s\t%s\t%.3f\n", bench, headline[bench], v)
		}
	}
	if err := os.WriteFile(baselineFile, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}
