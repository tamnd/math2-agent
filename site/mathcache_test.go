package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMathCache builds a site twice with a cache file. The second build takes its math from the file, and the pages are the same.
func TestMathCache(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "nodes/7K3/7K3QX2.md", statement)
	writeFile(t, content, "nodes/1H8/1H8D2C.md", concept)
	cache := filepath.Join(t.TempDir(), "cache", "math.gob.gz")
	build := func() string {
		out := filepath.Join(t.TempDir(), "site")
		if err := Build(Options{Content: content, Out: out, URL: "https://example.org/math2", Repo: "o/math2", Cache: cache}); err != nil {
			t.Fatal(err)
		}
		return readNode(t, out, "7K3QX2")
	}
	first := build()
	c := loadMathCache(cache, map[string]string{`\Z`: `\mathbb{Z}`})
	if len(c.old) == 0 {
		t.Fatal("the build wrote no formulas to the cache")
	}
	for k := range c.old {
		c.old[k] = "CACHED"
	}
	if err := (&mathCache{key: c.key, used: c.old}).save(cache); err != nil {
		t.Fatal(err)
	}
	if second := build(); !strings.Contains(second, "CACHED") {
		t.Fatal("the second build did not take its math from the cache")
	}
	if strings.Contains(first, "CACHED") {
		t.Fatal("the first build had math from nowhere")
	}

	// Other macros give an empty cache.
	if c := loadMathCache(cache, map[string]string{`\Z`: `\mathbf{Z}`}); len(c.old) != 0 {
		t.Fatal("the cache of other macros was used")
	}
	// A bad file gives an empty cache.
	if err := os.WriteFile(cache, []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := loadMathCache(cache, nil); len(c.old) != 0 {
		t.Fatal("a bad cache file gave formulas")
	}
}
