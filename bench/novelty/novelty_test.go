// Package novelty_test measures the recall of the novelty check (spec 17.5).
//
// Each item of items.yaml is a known result in the words of an agent output: a name, a statement and the first terms of a sequence when it has one. The item names the matches that make it known, for example graph:Q529ES or zbmath:bench-01. An item with no matches is a result that the searches of the suite do not have, and the check must not call it known. The items and the records are in our own words.
//
// The searches are the graph, from the search index of a build of testdata/nodes/valid, and the record plane with the papers of records.yaml. The source of the papers is zbmath, but their ids are ids of the suite and not ids of zbMATH Open. The searches of the web are not in the suite, because a run sends each query to the service.
//
// With no flags, the judge is an oracle. It gives same to a match of the item and different to each other match. Thus the recall of known is the share of the known items whose match is in the [novelty.Judged] best matches of a search. baseline.tsv has the recall, and the test fails when it is more than 0.02 below. Give -update to write baseline.tsv again.
//
// With -live, the judge is the judge of the explore role with the Claude model of the rung -rung of its ladder. Then the recall also measures the judge, and the report gives the items with no matches that the judge made known. It needs ANTHROPIC_API_KEY, and it spends money. Give -report FILE to write the report. A rediscovery that the check missed and a person found later goes into items.yaml.
package novelty_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/explore"
	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/llm/claude"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/novelty"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/search"
)

var (
	itemsFile = flag.String("items", "items.yaml", "the file of the items")
	live      = flag.Bool("live", false, "judge with the Claude model of the explore role. This needs ANTHROPIC_API_KEY and spends money.")
	rung      = flag.Int("rung", 0, "the rung of the ladder of the explore role, with -live")
	report    = flag.String("report", "", "write the report of the run to this file")
	update    = flag.Bool("update", false, "write baseline.tsv again from this run")
)

const (
	tree         = "../../testdata/nodes/valid"
	recordsFile  = "records.yaml"
	baselineFile = "baseline.tsv"
	source       = "zbmath"
)

// item is one item of items.yaml.
type item struct {
	ID        string   `yaml:"id"`
	Name      string   `yaml:"name"`
	Statement string   `yaml:"statement"`
	Terms     []string `yaml:"terms"`
	Known     []string `yaml:"known"` // the matches that make the item known, as SOURCE:ID
}

func (it item) claim() novelty.Claim {
	return novelty.Claim{Name: it.Name, Statement: it.Statement, Terms: it.Terms}
}

// known reports whether the match is a match of the item.
func (it item) known(m novelty.Match) bool {
	return slices.Contains(it.Known, strings.TrimPrefix(m.Source, "records:")+":"+m.ID)
}

func readYAML(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- a file of the suite, or the file of -items.
	if err != nil {
		t.Fatal(err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// searches returns the graph search of the valid tree and the search of the papers of records.yaml.
func searches(t *testing.T) []novelty.Searcher {
	t.Helper()
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: tree, Out: out}); err != nil {
		t.Fatal(err)
	}
	ix, err := search.Open(filepath.Join(out, build.SearchFile), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	files, err := check.ReadTree(tree)
	if err != nil {
		t.Fatal(err)
	}
	g := &novelty.Graph{Index: ix}
	for _, f := range files {
		n, err := node.Parse(f.Path, f.Data)
		if err != nil {
			t.Fatal(err)
		}
		g.Nodes = append(g.Nodes, n)
	}
	var f struct {
		Records []struct{ ID, Title, Text string }
	}
	readYAML(t, recordsFile, &f)
	var recs []ingest.Record
	for _, r := range f.Records {
		recs = append(recs, ingest.Record{ID: r.ID, Kind: "statement", Title: r.Title, Class: ingest.Open, Text: r.Text, Fingerprints: ingest.Fingerprints(r.Text)})
	}
	papers, err := novelty.NewRecords(source, recs, nil)
	if err != nil {
		t.Fatal(err)
	}
	return []novelty.Searcher{g, papers}
}

// oracle gives same to a match of the item and different to the others.
func oracle(it item) novelty.Judge {
	return func(_ context.Context, _ novelty.Claim, m novelty.Match) (string, string, error) {
		if it.known(m) {
			return novelty.Same, "a match of the item", nil
		}
		return novelty.Different, "not a match of the item", nil
	}
}

func TestRecall(t *testing.T) {
	var f struct{ Items []item }
	readYAML(t, *itemsFile, &f)
	ids := map[string]bool{}
	for _, it := range f.Items {
		if it.ID == "" || ids[it.ID] || strings.TrimSpace(it.Statement) == "" {
			t.Fatalf("%s: the item %q has no id, the id of another item, or no statement", *itemsFile, it.ID)
		}
		ids[it.ID] = true
	}
	ss := searches(t)
	judge := oracle
	if *live {
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			t.Fatal("-live needs ANTHROPIC_API_KEY")
		}
		role, err := agent.LoadRole(prompts.FS, "explore")
		if err != nil {
			t.Fatal(err)
		}
		if *rung < 0 || *rung >= len(role.Ladder) {
			t.Fatalf("the ladder of explore has no rung %d", *rung)
		}
		j := explore.Judge(claude.New(), role.Ladder[*rung].Model, role.Checks["judge"])
		judge = func(item) novelty.Judge { return j }
	}

	var rows []string
	found, known, wrong, other := 0, 0, 0, 0
	for _, it := range f.Items {
		res, err := novelty.Check(context.Background(), it.claim(), judge(it), ss...)
		if err != nil {
			t.Fatalf("%s: %v", it.ID, err)
		}
		var ms []string
		for _, m := range res.Matches {
			if m.Verdict != "" {
				ms = append(ms, fmt.Sprintf("%s:%s %s", strings.TrimPrefix(m.Source, "records:"), m.ID, m.Verdict))
			}
		}
		ok := false
		if len(it.Known) > 0 {
			known++
			ok = res.Verdict == novelty.Known && it.known(*res.Match)
			if ok {
				found++
			}
		} else {
			other++
			ok = res.Verdict != novelty.Known
			if !ok {
				wrong++
			}
		}
		if !ok {
			t.Logf("%s: the verdict is %s, and the matches are %s", it.ID, res.Verdict, strings.Join(ms, ", "))
		}
		rows = append(rows, fmt.Sprintf("| %s | %s | %s | %s |", it.ID, strings.Join(it.Known, ", "), res.Verdict, strings.Join(ms, ", ")))
	}
	if known == 0 {
		t.Fatalf("%s has no item with matches", *itemsFile)
	}
	recall := float64(found) / float64(known)
	t.Logf("the recall of known is %.3f (%d of %d), and %d of %d items with no matches are known", recall, found, known, wrong, other)
	judgeName := "the oracle judge"
	if *live {
		judgeName = fmt.Sprintf("the judge of explore at the rung %d", *rung)
	}
	writeReport(t, fmt.Sprintf("# The recall of the novelty check\n\nWith %s, the check found %d of %d known items, a recall of %.3f. It made %d of %d items with no matches known.\n\n| Item | Matches of the item | Verdict | Judged matches |\n|---|---|---|---|\n%s\n", judgeName, found, known, recall, wrong, other, strings.Join(rows, "\n")))

	if *live || *itemsFile != "items.yaml" {
		return
	}
	if wrong > 0 {
		t.Errorf("the oracle judge made %d items with no matches known", wrong)
	}
	if *update {
		text := fmt.Sprintf("# metric\tm2\n# The recall of known of the novelty check with the oracle judge. go test ./bench/novelty -update writes this file.\nrecall\t%.3f\n", recall)
		if err := os.WriteFile(baselineFile, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	base := readBaseline(t)
	if recall < base-0.02 {
		t.Errorf("the recall of known is %.3f, and the baseline is %.3f", recall, base)
	}
}

func readBaseline(t *testing.T) float64 {
	t.Helper()
	data, err := os.ReadFile(baselineFile)
	if err != nil {
		t.Fatalf("%v. Give -update to write it", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Split(line, "\t"); len(f) == 2 && f[0] == "recall" {
			v, err := strconv.ParseFloat(f[1], 64)
			if err != nil {
				t.Fatalf("%s: %v", baselineFile, err)
			}
			return v
		}
	}
	t.Fatalf("%s has no recall", baselineFile)
	return 0
}

func writeReport(t *testing.T, text string) {
	t.Helper()
	if *report == "" {
		return
	}
	if err := os.WriteFile(*report, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
