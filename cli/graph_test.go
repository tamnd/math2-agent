package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/store"
)

const graphTree = "../store/testdata/tree"

func TestGraph(t *testing.T) {
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: graphTree, Out: out}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		args []string
		code int
		want []string // each string is in the output
	}{
		{[]string{"neighbourhood", "-hops", "1", "D2SB01"}, ExitOK, []string{"nodes at most 1 edges from D2SB01 (5)", "  G5SB02  concept", "edges (4)", "  D2SB01  defines  G5SB02"}},
		{[]string{"neighbourhood", "-hops", "-1", "D2SB01"}, ExitUsage, []string{"is negative"}},
		{[]string{"closure", "lagrange"}, ExitFailure, []string{"no node has the alias lagrange"}},
		{[]string{"closure", "3C53TD"}, ExitOK, []string{"proofs      P1QG01", "  D2SB01  definition", "hypotheses  none"}},
		{[]string{"closure", "-measure", "size", "m2://lean/Subgroup.card_subgroup_dvd_card"}, ExitOK, []string{"closure     3C53TD"}},
		{[]string{"closure", "-measure", "width", "3C53TD"}, ExitUsage, []string{"is not depth or size"}},
		{[]string{"rdeps", "C05ET1"}, ExitOK, []string{"needs C05ET1 (3)", "uses C05ET1 (3)", "  3C53TD  statement  E1"}},
		{[]string{"path", "group", "3C53TD"}, ExitOK, []string{"  G5S1M9  concept", "  D1F0A1  definition", "  3C53TD  statement"}},
		{[]string{"path", "3C53TD", "group"}, ExitFailure, []string{"no learning path goes from 3C53TD to G5S1M9"}},
		{[]string{"path", "group"}, ExitUsage, []string{"give two tags"}},
		{[]string{"frontier"}, ExitOK, []string{"0 nodes"}},
		{[]string{"frontier", "-open", "-msc", "11P"}, ExitOK, []string{"K7GD01  conjecture  E0  -  11P32  Goldbach", "        lower bound 4e18 by B1GD01 at E1, via P3GD01", "1 nodes"}},
		{[]string{"frontier", "-open", "-topic", "T0NT01", "-evidence", "E0"}, ExitOK, []string{"K7GD01", "1 nodes"}},
		{[]string{"frontier", "-evidence", "E9"}, ExitUsage, []string{"is not one of E0 to E5"}},
		{[]string{"frontier", "3C53TD"}, ExitUsage, []string{"give no argument"}},
		{[]string{"cycle", "3C53TD"}, ExitUsage, []string{"unknown query"}},
	}
	for _, tt := range tests {
		args := append([]string{"graph", tt.args[0], "-out", out}, tt.args[1:]...)
		if tt.args[0] == "cycle" {
			args = append([]string{"graph"}, tt.args...)
		}
		code, stdout, stderr := run(args...)
		if code != tt.code {
			t.Errorf("m2 graph %s exits with code %d, and it must exit with %d: %s", strings.Join(tt.args, " "), code, tt.code, stderr)
		}
		for _, w := range tt.want {
			if !strings.Contains(stdout+stderr, w) {
				t.Errorf("m2 graph %s does not print %q:\n%s%s", strings.Join(tt.args, " "), w, stdout, stderr)
			}
		}
	}
}

func TestGraphJSON(t *testing.T) {
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: graphTree, Out: out}); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("graph", "frontier", "-out", out, "-json", "-open")
	if code != ExitOK {
		t.Fatalf("m2 graph frontier exits with code %d: %s", code, stderr)
	}
	var es []store.Entry
	if err := json.Unmarshal([]byte(stdout), &es); err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || es[1].Tag != "K7GD01" || len(es[1].Bounds) != 1 {
		t.Errorf("m2 graph frontier -json -open gives %+v", es)
	}
	code, stdout, _ = run("graph", "path", "-out", out, "-json", "G5S1M9", "3C53TD")
	if code != ExitOK || strings.Join(strings.Fields(stdout), "") != `["G5S1M9","D1F0A1","D2SB01","3C53TD"]` {
		t.Errorf("m2 graph path -json exits with code %d and prints %s", code, stdout)
	}
}

func TestGraphWithoutBuild(t *testing.T) {
	code, _, stderr := run("graph", "closure", "-root", t.TempDir(), "3C53TD")
	if code != ExitFailure || !strings.Contains(stderr, "Run m2 build first") {
		t.Errorf("m2 graph without a build exits with code %d: %s", code, stderr)
	}
	if code, _, _ := run("graph"); code != ExitUsage {
		t.Errorf("m2 graph with no query exits with code %d", code)
	}
}
