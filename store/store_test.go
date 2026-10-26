package store_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/store"
)

const tree = "testdata/tree"

// open builds the tree and opens its store.
func open(t *testing.T) *store.Store {
	t.Helper()
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: tree, Out: out}); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(filepath.Join(out, build.StoreFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpen(t *testing.T) {
	s := open(t)
	if n, ok := s.Node("G5S1M9"); !ok || n.Kind != "concept" || n.Name != "Group" || n.Hash == "" {
		t.Errorf("the node G5S1M9 is %+v, %v", n, ok)
	}
	if _, ok := s.Node("7K3QX2"); ok {
		t.Error("the store has the node 7K3QX2, and the tree has not")
	}
	st, ok := s.Status("3C53TD")
	if !ok || st.Evidence != status.Claimed || st.Fidelity != status.F0 {
		t.Errorf("the status of 3C53TD is %+v, %v", st, ok)
	}
	if _, ok := s.Status("G5S1M9"); !ok {
		t.Error("the store has no status for the concept G5S1M9")
	}
	if _, err := store.Open(filepath.Join(t.TempDir(), "none.db")); err == nil {
		t.Error("Open accepts a file that does not exist")
	}
}

// TestGraphHash checks that the graph that Open reads has the hash of the graph that the build made.
func TestGraphHash(t *testing.T) {
	nodes, err := node.Load(tree)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := open(t).Hash(), graph.Build(nodes).Hash(); got != want {
		t.Errorf("the graph of the store has the hash %s, and the graph of the tree has %s", got, want)
	}
}

func TestResolve(t *testing.T) {
	s := open(t)
	tests := []struct {
		ref  string
		want string // the tags, or the start of the error
	}{
		{"3C53TD", "3C53TD"},
		{"group", "G5S1M9"},
		{"m2://G5S1M9", "G5S1M9"},
		{"m2://concept/group", "G5S1M9"},
		{"m2://definition/subgroup", "D2SB01"},
		{"m2://statement/3C53TD", "3C53TD"},
		{"m2://lean/Subgroup.card_subgroup_dvd_card", "3C53TD"},
		{"m2://ext/wikidata/Q83478", "G5S1M9"},
		{"m2://concept/subgroup", "error: m2://concept/subgroup names D2SB01, and D2SB01 is a definition, not a concept"},
		{"m2://lean/Nat.succ", "error: no node has lean/Nat.succ"},
		{"m2://ext/wikidata/Q1", "error: no node has ext/wikidata/Q1"},
		{"m2://msc/20D60", "error: m2://msc/20D60 names a msc view, not a node"},
		{"m2://3C53TD@2027.03", "error: m2://3C53TD@2027.03 names a release or a commit"},
		{"7K3QX2", "error: no node has the tag 7K3QX2"},
		{"ring", "error: no node has the alias ring"},
		{"Not An Alias", "error: \"Not An Alias\" is not a tag"},
		{"3c53td", "3C53TD"},
		{"3C53-TD", "3C53TD"},
		{"g5sim9", "G5S1M9"},
		{"d2sbo1", "D2SB01"},
		{"7k3q-x2ab-ce", "error: no node has the tag 7K3QX2ABCE"},
		{"7K3QX2ABCF", "error: the check character of 7K3QX2ABCF is not correct"},
		{"m2://inv/7K3QX2M9R2", "error: m2://inv/7K3QX2M9R2 names an inventory row, not a node"},
	}
	for _, tt := range tests {
		tags, err := s.Resolve(tt.ref)
		got := strings.Join(tags, " ")
		if err != nil {
			got = "error: " + err.Error()
		}
		if !strings.HasPrefix(got, tt.want) {
			t.Errorf("Resolve(%q) gives %q, and it must give %q", tt.ref, got, tt.want)
		}
	}
}

func TestNeighbourhood(t *testing.T) {
	s := open(t)
	nb, err := s.Neighbourhood("D2SB01", 1)
	if err != nil {
		t.Fatal(err)
	}
	var edges []string
	for _, e := range nb.Edges {
		edges = append(edges, e.Type+" "+e.From+" "+e.To)
	}
	wantNodes := []string{"3C53TD", "C05ET1", "D1F0A1", "D2SB01", "G5SB02"}
	wantEdges := []string{"defines D2SB01 G5SB02", "uses 3C53TD D2SB01", "uses C05ET1 D2SB01", "uses D2SB01 D1F0A1"}
	if !reflect.DeepEqual(nb.Nodes, wantNodes) || !reflect.DeepEqual(edges, wantEdges) {
		t.Errorf("the neighbourhood of D2SB01 has the nodes %v and the edges %q, and it must have %v and %q", nb.Nodes, edges, wantNodes, wantEdges)
	}
	two, err := s.Neighbourhood("D2SB01", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(two.Nodes) <= len(nb.Nodes) || len(two.Edges) <= len(nb.Edges) {
		t.Errorf("2 hops give %d nodes and %d edges, and 1 hop gives %d and %d", len(two.Nodes), len(two.Edges), len(nb.Nodes), len(nb.Edges))
	}
	if zero, err := s.Neighbourhood("D2SB01", 0); err != nil || len(zero.Nodes) != 1 || len(zero.Edges) != 0 {
		t.Errorf("0 hops give %+v, %v", zero, err)
	}
	if _, err := s.Neighbourhood("7K3QX2", 2); err == nil {
		t.Error("Neighbourhood accepts a tag that is not in the store")
	}
}

func TestClosure(t *testing.T) {
	s := open(t)
	c, err := s.Closure("3C53TD", graph.MinDepth)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"3C53TD", "C05ET1", "D1F0A1", "D2SB01", "P1QG01"}
	if !reflect.DeepEqual(c.Nodes, want) || !reflect.DeepEqual(c.Proofs, []string{"P1QG01"}) {
		t.Errorf("the closure of 3C53TD has the nodes %v and the proofs %v", c.Nodes, c.Proofs)
	}
	if c.Evidence != status.Claimed || c.Weakest != status.Claimed || len(c.Below) != 0 || len(c.Hypotheses) != 0 {
		t.Errorf("the trust profile of 3C53TD is %+v", c)
	}
	c, err = s.Closure("S1RH01", graph.MinSize)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Hypotheses, []string{"H1RH01"}) || !reflect.DeepEqual(c.Below, []string{"H1RH01"}) || c.Weakest != status.Open {
		t.Errorf("the closure of S1RH01 has the hypotheses %v, the nodes below %v and the weakest level %v", c.Hypotheses, c.Below, c.Weakest)
	}
	if _, err := s.Closure("7K3QX2", graph.MinDepth); err == nil {
		t.Error("Closure accepts a tag that is not in the store")
	}
}

func TestDependents(t *testing.T) {
	s := open(t)
	tests := []struct {
		tag         string
		needs, uses []string
	}{
		{"C05ET1", []string{"3C53TD", "P1QG01", "P2QG02"}, []string{"3C53TD", "P1QG01", "P2QG02"}},
		{"P1QG01", []string{}, []string{"3C53TD"}},
		{"D1F0A1", []string{"3C53TD", "C05ET1", "D2SB01", "P1QG01", "P2QG02"}, []string{"3C53TD", "C05ET1", "D2SB01", "P1QG01", "P2QG02"}},
		{"T0NT01", []string{}, []string{}},
	}
	for _, tt := range tests {
		d, err := s.Dependents(tt.tag)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d.Needs, tt.needs) || !reflect.DeepEqual(d.Uses, tt.uses) {
			t.Errorf("Dependents(%s) gives the needs %v and the uses %v, and it must give %v and %v", tt.tag, d.Needs, d.Uses, tt.needs, tt.uses)
		}
	}
}

func TestPath(t *testing.T) {
	s := open(t)
	p, err := s.Path("G5S1M9", "3C53TD")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"G5S1M9", "D1F0A1", "D2SB01", "3C53TD"}; !reflect.DeepEqual(p, want) {
		t.Errorf("the path from G5S1M9 to 3C53TD is %v, and it must be %v", p, want)
	}
	if _, err := s.Path("3C53TD", "G5S1M9"); err == nil {
		t.Error("Path finds a path from 3C53TD to G5S1M9")
	}
	if _, err := s.Path("G5S1M9", "7K3QX2"); err == nil {
		t.Error("Path accepts a tag that is not in the store")
	}
}

func TestFrontier(t *testing.T) {
	s := open(t)
	tags := func(f store.Filter) string {
		t.Helper()
		es, err := s.Frontier(f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range es {
			out = append(out, e.Tag)
		}
		return strings.Join(out, " ")
	}
	tests := []struct {
		f    store.Filter
		want string
	}{
		{store.Filter{}, ""},
		{store.Filter{Open: true}, "H1RH01 K7GD01"},
		{store.Filter{Open: true, MSC: "11P"}, "K7GD01"},
		{store.Filter{Open: true, MSC: "11M26"}, "H1RH01"},
		{store.Filter{Open: true, Topic: "T0NT01"}, "K7GD01"},
		{store.Filter{Open: true, Evidence: status.Claimed}, ""},
	}
	for _, tt := range tests {
		if got := tags(tt.f); got != tt.want {
			t.Errorf("Frontier(%+v) gives %q, and it must give %q", tt.f, got, tt.want)
		}
	}
	es, err := s.Frontier(store.Filter{Open: true, Topic: "T0NT01"})
	if err != nil {
		t.Fatal(err)
	}
	want := []store.Bound{{Tag: "B1GD01", Side: "lower", Value: "4e18", Via: "P3GD01", Evidence: status.Claimed}}
	if len(es) != 1 || !reflect.DeepEqual(es[0].Bounds, want) || !reflect.DeepEqual(es[0].MSC, []string{"11P32"}) {
		t.Errorf("the open frontier of T0NT01 is %+v", es)
	}
}

// TestFormalFrontier writes a store where one statement has a formal statement and no kernel proof (query Q6).
func TestFormalFrontier(t *testing.T) {
	nodes, err := node.Load(tree)
	if err != nil {
		t.Fatal(err)
	}
	sts := []status.Status{
		{Tag: "3C53TD", Kind: "statement", Evidence: status.Claimed, Fidelity: status.F1},
		{Tag: "C05ET1", Kind: "statement", Evidence: status.Kernel, Fidelity: status.F2},
		{Tag: "S1RH01", Kind: "statement", Evidence: status.Claimed, Fidelity: status.F0},
	}
	path := filepath.Join(t.TempDir(), "store.db")
	for range 2 {
		if err := store.Write(path, store.Input{Nodes: nodes, Status: sts}); err != nil {
			t.Fatal(err)
		}
	}
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	es, err := s.Frontier(store.Filter{MSC: "20"})
	if err != nil {
		t.Fatal(err)
	}
	want := []store.Entry{{Tag: "3C53TD", Kind: "statement", Name: name(nodes, "3C53TD"), Evidence: status.Claimed, Fidelity: status.F1, MSC: []string{"20D60"}}}
	if !reflect.DeepEqual(es, want) {
		t.Errorf("the formal frontier is %+v, and it must be %+v", es, want)
	}
	if _, ok := s.Status("G5S1M9"); ok {
		t.Error("the store has a status for G5S1M9, and the input has none")
	}
}

func name(nodes []*node.Node, tag string) string {
	for _, n := range nodes {
		if n.ID == tag {
			return n.Name()
		}
	}
	return ""
}

func TestOpenOtherVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	if err := os.WriteFile(path, []byte("not a store"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(path); err == nil {
		t.Error("Open accepts a file that is not a store")
	}
}
