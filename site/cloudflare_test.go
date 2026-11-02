package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestCloudflare(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/7K3/7K3QX2.md", statement)
	writeFile(t, content, "nodes/1H8/1H8D2C.md", concept)
	out := filepath.Join(t.TempDir(), "site")
	packDir := filepath.Join(t.TempDir(), "packs")
	opt := Options{Content: content, Out: out, URL: "https://example.org/math2", Repo: "o/math2", AgentRepo: "o/math2-agent", Version: "test", Cloudflare: true, SocialPacks: packDir}
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
	if w := readFile(t, out, "_worker.js"); !strings.Contains(w, "caches.default.match(key)") || !strings.Contains(w, "caches.default.put(key") || !strings.Contains(w, "cacheVersion") {
		t.Error("the worker does not cache sharded responses by build")
	}
	if h := readFile(t, out, "_headers"); !strings.Contains(h, "/social/*") || !strings.Contains(h, "immutable") {
		t.Error("social cards have no immutable cache header")
	}
	var nodeShard map[string]string
	path := "wiki/lagranges-theorem/index.html"
	if err := json.Unmarshal([]byte(readFile(t, out, fmt.Sprintf("%s/%d.json", ShardDir, shardOf(path, n)))), &nodeShard); err != nil {
		t.Fatal(err)
	}
	page := nodeShard[path]
	if !strings.Contains(page, `property="og:type" content="article"`) || !strings.Contains(page, `name="twitter:card" content="summary_large_image"`) || !strings.Contains(page, `property="article:tag" content="statement"`) {
		t.Error("the node has incomplete sharing metadata")
	}
	imageURL := regexp.MustCompile(`property="og:image" content="https://example.org/math2/social/([0-9a-f]{24})/([0-9]+)/([0-9]+)/7K3QX2.jpg"`).FindStringSubmatch(page)
	if imageURL == nil {
		t.Fatal("the node has no packed social image URL")
	}
	pack := filepath.Join(out, "social", "packs", imageURL[1]+".pack")
	data, err := os.ReadFile(pack)
	if err != nil {
		t.Fatal(err)
	}
	offset, _ := strconv.Atoi(imageURL[2])
	length, _ := strconv.Atoi(imageURL[3])
	if offset < 0 || length < 1 || offset+length > len(data) {
		t.Fatal("the social image points outside its pack")
	}
	img, err := jpeg.DecodeConfig(bytes.NewReader(data[offset : offset+length]))
	if err != nil || img.Width != socialWidth || img.Height != socialHeight {
		t.Fatalf("social pack image: %v, %v", img, err)
	}
	if _, err := os.Stat(filepath.Join(packDir, imageURL[1]+".pack")); err != nil {
		t.Errorf("the R2 staging directory has no pack: %v", err)
	}
	if individual, _ := filepath.Glob(filepath.Join(out, "social", "7K3QX2-*.jpg")); len(individual) != 0 {
		t.Error("individual social images remain in Pages")
	}
	var routes struct {
		Version int
		Include []string
	}
	if err := json.Unmarshal([]byte(readFile(t, out, "_routes.json")), &routes); err != nil {
		t.Fatal(err)
	}
	if routes.Version != 1 || len(routes.Include) != 6 || routes.Include[0] != "/math2/node/*" || routes.Include[1] != "/math2/wiki/*" || routes.Include[5] != "/math2/social/*" {
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

func TestCloudflarePacksWithoutR2Staging(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/7K3/7K3QX2.md", statement)
	out := filepath.Join(t.TempDir(), "site")
	if err := Build(Options{Content: content, Out: out, URL: "https://example.org/math2", Cloudflare: true}); err != nil {
		t.Fatal(err)
	}
	packs, err := filepath.Glob(filepath.Join(out, "social", "packs", "*.pack"))
	if err != nil || len(packs) == 0 {
		t.Fatalf("Cloudflare build without R2 staging has no Pages pack: %v, %v", packs, err)
	}
	if w := readFile(t, out, "_worker.js"); !strings.Contains(w, `const socialPacks = true;`) || !strings.Contains(w, `source = "pages"`) {
		t.Error("the worker cannot serve its Pages pack fallback")
	}
}
