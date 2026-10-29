package site

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
)

func TestLevels(t *testing.T) {
	for _, withBuild := range []bool{false, true} {
		out := buildFixtures(t, withBuild)
		var index struct {
			Nodes []nodeEntry `json:"nodes"`
		}
		data, err := os.ReadFile(filepath.Join(out, "api", "nodes.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &index); err != nil {
			t.Fatal(err)
		}
		count := map[string]int{}
		for _, e := range index.Nodes {
			if e.Level == "" {
				t.Fatalf("build %v: the node %s has no writing level in api/nodes.json", withBuild, e.Tag)
			}
			count[e.Level]++
		}
		page, err := os.ReadFile(filepath.Join(out, "levels", "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range []string{"W0", "W1", "W2", "W3"} {
			want := `<a href="/math2/levels/` + l + `/">`
			if !strings.Contains(string(page), want) {
				t.Errorf("build %v: the levels page has no link %s", withBuild, want)
			}
			list, err := os.ReadFile(filepath.Join(out, "levels", l, "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Count(string(list), "<tr><td><a href="); got != count[l] {
				t.Errorf("build %v: the list of %s has %d nodes, and api/nodes.json has %d", withBuild, l, got, count[l])
			}
		}
		e := index.Nodes[0]
		var node struct {
			Writing struct {
				Tag   string `json:"tag"`
				Level string `json:"level"`
			} `json:"writing"`
		}
		data, err = os.ReadFile(filepath.Join(out, "api", "node", e.Tag+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &node); err != nil {
			t.Fatal(err)
		}
		if node.Writing.Tag != e.Tag || node.Writing.Level != e.Level {
			t.Errorf("build %v: api/node/%s.json has the writing %+v", withBuild, e.Tag, node.Writing)
		}
		html := readNode(t, out, e.Tag)
		if !strings.Contains(html, `<tr><th>Writing</th><td><a href="/math2/levels/`+e.Level+`/"`) {
			t.Errorf("build %v: the page of %s does not show its level", withBuild, e.Tag)
		}
		if e.Level != "W3" && !strings.Contains(html, "What this page needs for W") {
			t.Errorf("build %v: the page of %s has no work list", withBuild, e.Tag)
		}
	}
}

func TestLevelsFromBuild(t *testing.T) {
	// A site with a build shows the levels of writing.json, also when the tree changed since.
	dir := t.TempDir()
	if _, err := build.Run(t.Context(), build.Options{Root: fixtures, Out: dir}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, build.WritingFile)
	var w build.Levels
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &w); err != nil {
		t.Fatal(err)
	}
	w.Nodes[0].Level = 3
	w.Nodes[0].Next = nil
	data, _ = json.Marshal(w)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	opt := Options{Content: fixtures, Build: dir, Out: filepath.Join(t.TempDir(), "site"), URL: "https://example.org/math2", Repo: "o/math2", AgentRepo: "o/math2-agent", Version: "test"}
	if err := Build(opt); err != nil {
		t.Fatal(err)
	}
	if html := readNode(t, opt.Out, w.Nodes[0].Tag); !strings.Contains(html, `levels/W3/`) || strings.Contains(html, "What this node needs") {
		t.Errorf("the page of %s does not show the level W3 of writing.json", w.Nodes[0].Tag)
	}
}
