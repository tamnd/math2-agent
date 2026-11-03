package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLLMsParts builds a site with a small part size. The full text goes into several files, each at most the part size past its head, and llms.txt lists them all.
func TestLLMsParts(t *testing.T) {
	defer func(n int) { llmsPart = n }(llmsPart)
	llmsPart = 1 << 10
	content := newContent(t)
	writeFile(t, content, "nodes/7K3/7K3QX2.md", statement)
	writeFile(t, content, "nodes/1H8/1H8D2C.md", concept)
	out := filepath.Join(t.TempDir(), "site")
	if err := Build(Options{Content: content, Out: out, URL: "https://example.org/math2", Repo: "o/math2"}); err != nil {
		t.Fatal(err)
	}
	index := readFile(t, out, "llms.txt")
	full := readFile(t, out, "llms-full.txt")
	text := full
	n := 1
	for ; ; n++ {
		name := llmsName(n)
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			break
		}
		if !strings.Contains(index, name) || !strings.Contains(full, name) {
			t.Errorf("llms.txt or llms-full.txt does not list %s", name)
		}
		text += string(data)
	}
	if n < 2 {
		t.Fatal("the full text is in one file")
	}
	for _, tag := range []string{"7K3QX2", "1H8D2C"} {
		if !strings.Contains(text, `tag="`+tag+`"`) {
			t.Errorf("no part has the node %s", tag)
		}
	}
}
