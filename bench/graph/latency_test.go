// Package graph_test measures the latency of the graph queries Q1 to Q7 of spec 05.6 on a synthetic graph, and compares the 95th percentile with the target of each query.
//
// Run it at reference scale with:
//
//	go test ./bench/graph -run TestLatency -nodes 5000000 -timeout 2h -v
//
// The default size is small, so that the test runs on each pull request. Give -report FILE to write the results as a Markdown table, for the release notes. Writing the store takes most of the time at reference scale, so -store FILE keeps the store file for the next run, and -query Q3,Q4 runs only some queries, for example to profile them with -cpuprofile.
package graph_test

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/store"
	"github.com/tamnd/math2-agent/synth"
)

var (
	nodes   = flag.Int("nodes", 20000, "the number of synthetic nodes")
	samples = flag.Int("samples", 200, "the number of runs of each query")
	report  = flag.String("report", "", "write the results as Markdown to this file")
	strict  = flag.Bool("strict", true, "fail when a query misses its target")
	keep    = flag.String("store", "", "keep the store file at this path, and use the file when it exists. The file must come from the same -nodes.")
	only    = flag.String("query", "", "run only these queries, for example Q3,Q4")
)

// query is one query of spec 05.6 with its target p95.
type query struct {
	id, name string
	target   time.Duration
	run      func(s *store.Store, r *rand.Rand) (int, error) // the size of the result
}

func TestLatency(t *testing.T) {
	start := time.Now()
	d := synth.Make(synth.Options{Nodes: *nodes, Seed: 1})
	made := time.Since(start)
	g := d.Graph
	path := filepath.Join(t.TempDir(), "store.db")
	if *keep != "" {
		path = *keep
	}
	start = time.Now()
	write := "Write " // the time of Write, or a note that the run used the store file of an earlier run
	if _, err := os.Stat(path); err == nil {
		write = "the store file came from an earlier run"
	} else {
		if err := store.Write(path, store.Input{Graph: g, Status: d.Status, MSC: d.MSC, Aliases: d.Aliases}); err != nil {
			t.Fatal(err)
		}
		write += time.Since(start).Round(time.Millisecond).String()
	}
	info, _ := os.Stat(path)
	var statements, top []string
	for _, n := range g.Nodes {
		switch n.Kind {
		case "statement":
			statements = append(statements, n.Tag)
		}
	}
	// The statements of the top level have the deepest closures.
	top = statements[len(statements)*(synth.Levels-1)/synth.Levels:]
	pick := func(r *rand.Rand, tags []string) string { return tags[r.IntN(len(tags))] }
	var formal []string
	for _, e := range g.Edges {
		if e.Type == graph.Formalizes {
			formal = append(formal, e.From)
		}
	}
	// Each pair of Q5 has a path: from the concept that is farthest from the statement to the statement.
	pr := rand.New(rand.NewPCG(3, 0)) // #nosec G404 -- the samples must repeat.
	var pairs [][2]string
	for range *samples {
		to := pick(pr, top)
		if from := farthest(g, to); from != "" {
			pairs = append(pairs, [2]string{from, to})
		}
	}
	count, edges := len(g.Nodes), len(g.Edges)
	// The synthetic graph is not live after this point, so the heap after Open is the heap of the store.
	runtime.GC()
	start = time.Now()
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	opened := time.Since(start)
	var mem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&mem)

	queries := []query{
		{"Q1", "resolve an identifier", 5 * time.Millisecond, func(s *store.Store, r *rand.Rand) (int, error) {
			refs := []string{pick(r, statements), "m2://lean/" + pick(r, formal), fmt.Sprintf("concept-%d", r.IntN(*nodes)), "m2://ext/wikidata/Q" + fmt.Sprint(1000+r.IntN(*nodes))}
			tags, err := s.Resolve(refs[r.IntN(len(refs))])
			if err != nil && !strings.HasPrefix(err.Error(), "no node has") {
				return 0, err
			}
			return len(tags), nil
		}},
		{"Q2", "neighbourhood, 2 hops", 20 * time.Millisecond, func(s *store.Store, r *rand.Rand) (int, error) {
			nb, err := s.Neighbourhood(pick(r, statements), 2)
			return len(nb.Nodes), err
		}},
		{"Q3", "dependency closure", 100 * time.Millisecond, func(s *store.Store, r *rand.Rand) (int, error) {
			c, err := s.Closure(pick(r, top), graph.MinDepth)
			return len(c.Nodes), err
		}},
		{"Q4", "reverse dependencies", 200 * time.Millisecond, func(s *store.Store, r *rand.Rand) (int, error) {
			d, err := s.Dependents(pick(r, statements))
			return len(d.Uses), err
		}},
		{"Q5", "prerequisite path", 100 * time.Millisecond, func(s *store.Store, r *rand.Rand) (int, error) {
			pair := pairs[r.IntN(len(pairs))]
			p, err := s.Path(pair[0], pair[1])
			return len(p), err
		}},
		{"Q6", "status slice", 500 * time.Millisecond, func(s *store.Store, r *rand.Rand) (int, error) {
			es, err := s.Frontier(store.Filter{MSC: fmt.Sprintf("%02d", 3+r.IntN(synth.Classes)), Evidence: status.Claimed})
			return len(es), err
		}},
		{"Q7", "open-problem frontier", 200 * time.Millisecond, func(s *store.Store, r *rand.Rand) (int, error) {
			es, err := s.Frontier(store.Filter{Open: true, Topic: synth.Tag(r.IntN(synth.Classes))})
			return len(es), err
		}},
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Graph queries on %d synthetic nodes and %d edges, %s, %d cores, Go %s. Make took %s, %s and Open %s. The store file has %d MB, and the heap after Open has %d MB.\n\n",
		count, edges, runtime.GOOS+"/"+runtime.GOARCH, runtime.NumCPU(), runtime.Version(), made.Round(time.Millisecond), write, opened.Round(time.Millisecond), info.Size()>>20, mem.HeapAlloc>>20)
	b.WriteString("| Query | | median size | p50 | p95 | max | target p95 | |\n|---|---|---|---|---|---|---|---|\n")
	for k, q := range queries {
		if *only != "" && !slices.Contains(strings.Split(*only, ","), q.id) {
			continue
		}
		// Each query has its own random numbers, so -query does not change the samples.
		measure := func() ([]time.Duration, []int) {
			r := rand.New(rand.NewPCG(2, uint64(k))) // #nosec G404,G115 -- the samples must repeat, and k is not negative.
			times, sizes := make([]time.Duration, *samples), make([]int, *samples)
			for i := range times {
				start := time.Now()
				n, err := q.run(s, r)
				if err != nil {
					t.Fatalf("%s: %v", q.id, err)
				}
				times[i], sizes[i] = time.Since(start), n
			}
			slices.Sort(times)
			slices.Sort(sizes)
			return times, sizes
		}
		times, sizes := measure()
		p95 := times[len(times)*95/100]
		if p95 > q.target {
			// The other test packages share the machine, so one slow pass can be noise. The query misses its target only when a second pass misses it too.
			times, sizes = measure()
			p95 = times[len(times)*95/100]
		}
		verdict := "pass"
		if p95 > q.target {
			verdict = "miss"
			if *strict {
				t.Errorf("%s (%s) has the p95 %s, and the target is %s", q.id, q.name, p95, q.target)
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %s | %s |\n", q.id, q.name, sizes[len(sizes)/2], round(times[len(times)/2]), round(p95), round(times[len(times)-1]), q.target, verdict)
	}
	t.Log("\n" + b.String())
	if *report != "" {
		if err := os.WriteFile(*report, []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// farthest returns the concept that is the farthest from the statement on a learning path, or "".
func farthest(g *graph.Graph, statement string) string {
	seen := map[string]bool{statement: true}
	queue := []string{statement}
	last := ""
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		var next []string
		for _, e := range g.Out(graph.Uses, cur) {
			if n, _ := g.Node(e.To); n.Kind == "definition" {
				next = append(next, e.To)
			}
		}
		for _, e := range g.Out(graph.Defines, cur) {
			next = append(next, e.To)
			last = e.To
		}
		for _, n := range next {
			if !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	return last
}

func round(d time.Duration) time.Duration {
	switch {
	case d > time.Second:
		return d.Round(time.Millisecond)
	case d > time.Millisecond:
		return d.Round(10 * time.Microsecond)
	}
	return d.Round(time.Microsecond)
}
