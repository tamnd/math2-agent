package site

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudflare(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/7K3/7K3QX2.md", statement)
	writeFile(t, content, "nodes/1H8/1H8D2C.md", concept)
	out := filepath.Join(t.TempDir(), "site")
	opt := Options{Content: content, Out: out, URL: "https://example.org/math2", Repo: "o/math2", AgentRepo: "o/math2-agent", Version: "test", Cloudflare: true}
	if err := Build(opt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "node", "7K3QX2", "index.html")); !os.IsNotExist(err) {
		t.Errorf("a Cloudflare build has the page of the short link node/7K3QX2/ (%v)", err)
	}
	// The files of the nodes are in the shards, not in files of their own.
	for _, p := range []string{"wiki/lagranges-theorem/index.html", "node/7K3QX2.md", "api/node/7K3QX2.json"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(p))); !os.IsNotExist(err) {
			t.Errorf("a Cloudflare build has the file %s (%v)", p, err)
		}
	}
	n := 16
	for _, p := range []string{"wiki/lagranges-theorem/index.html", "node/7K3QX2.md", "api/node/7K3QX2.json"} {
		var shard map[string]string
		if err := json.Unmarshal([]byte(readFile(t, out, fmt.Sprintf("%s/%d.json", ShardDir, shardOf(p, n)))), &shard); err != nil {
			t.Fatal(err)
		}
		if shard[p] == "" {
			t.Errorf("the shard of %s does not have the file", p)
		}
	}
	var links map[string]string
	if err := json.Unmarshal([]byte(readFile(t, out, LinksFile)), &links); err != nil {
		t.Fatal(err)
	}
	if got := links["7K3QX2"]; got != "wiki/lagranges-theorem/" {
		t.Errorf("the link of 7K3QX2 is %q", got)
	}
	if len(links) != 2 {
		t.Errorf("%s has %d links, want 2", LinksFile, len(links))
	}
	if w := readFile(t, out, "_worker.js"); !strings.Contains(w, `const base = "/math2";`) || !strings.Contains(w, "/"+LinksFile) || !strings.Contains(w, "const shards = 16;") {
		t.Errorf("_worker.js does not have the base path and the links file:\n%s", w)
	}
	var routes struct {
		Version int
		Include []string
	}
	if err := json.Unmarshal([]byte(readFile(t, out, "_routes.json")), &routes); err != nil {
		t.Fatal(err)
	}
	if routes.Version != 1 || len(routes.Include) != 5 || routes.Include[0] != "/math2/node/*" || routes.Include[1] != "/math2/wiki/*" {
		t.Errorf("_routes.json is %+v", routes)
	}
}

func TestNoCloudflare(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/7K3/7K3QX2.md", statement)
	out := buildSite(t, content, "")
	for _, p := range []string{"_worker.js", "_routes.json", LinksFile} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(p))); !os.IsNotExist(err) {
			t.Errorf("a build for GitHub Pages has %s (%v)", p, err)
		}
	}
	readNode(t, out, "7K3QX2")
}
