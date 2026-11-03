package site

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/inventory"
)

func TestSocialCard(t *testing.T) {
	data, err := socialJPEG("Lagrange's theorem", "Theorem · Group theory", "7K3QX2")
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds(); got.Dx() != socialWidth || got.Dy() != socialHeight {
		t.Errorf("card size %v", got)
	}
	if len(data) > 100_000 {
		t.Errorf("card is %d bytes", len(data))
	}
	if socialPath("7K3QX2", "First", "Theorem") == socialPath("7K3QX2", "Second", "Theorem") {
		t.Error("the card URL did not change with the title")
	}
}

func TestInventoryLetterPagination(t *testing.T) {
	content := newContent(t)
	var rows []inventory.Row
	for i := 1; i <= invPerPage+1; i++ {
		rows = append(rows, inventory.Row{ID: fmt.Sprintf("%010d", i), Name: fmt.Sprintf("Alpha %03d", i), Kind: "statement", Layer: inventory.Textbook, Spread: 1})
	}
	if err := inventory.WriteRows(content, rows); err != nil {
		t.Fatal(err)
	}
	out := buildSite(t, content, "")
	first := readFile(t, out, "inventory/a/index.html")
	second := readFile(t, out, "inventory/a/2/index.html")
	if !strings.Contains(first, "Alpha 100") || strings.Contains(first, "Alpha 101") || !strings.Contains(first, `href="/math2/inventory/a/2/"`) {
		t.Error("first inventory page has the wrong rows or next link")
	}
	if !strings.Contains(second, "Alpha 101") || strings.Contains(second, "Alpha 100") || !strings.Contains(second, `href="/math2/inventory/a/"`) {
		t.Error("second inventory page has the wrong rows or previous link")
	}
}

func TestNodeListPagination(t *testing.T) {
	b := &builder{opt: Options{Out: filepath.Join(t.TempDir(), "site"), URL: "https://example.org/math2"}, base: "/math2"}
	var err error
	b.tmpl, err = parseTemplates(b)
	if err != nil {
		t.Fatal(err)
	}
	var nodes []*nodeView
	for i := 1; i <= listPerPage+1; i++ {
		nodes = append(nodes, &nodeView{Name: fmt.Sprintf("Idea %03d", i), Kind: "concept", URL: fmt.Sprintf("/math2/wiki/idea-%03d/", i)})
	}
	if err := b.writeNodeList("nodes", "Every idea", "Ideas", "", "", nodes, nil, nil); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, b.opt.Out, "nodes/index.html")
	second := readFile(t, b.opt.Out, "nodes/2/index.html")
	if strings.Contains(first, "Idea 151") || !strings.Contains(first, `href="/math2/nodes/2/"`) {
		t.Error("first node page has the wrong rows or next link")
	}
	if !strings.Contains(second, "Idea 151") || !strings.Contains(second, `href="/math2/nodes/"`) {
		t.Error("second node page has the wrong rows or previous link")
	}
	if _, err := os.Stat(filepath.Join(b.opt.Out, "nodes", "3", "index.html")); !os.IsNotExist(err) {
		t.Error("a third node page was written")
	}
}

func TestFrontierPagination(t *testing.T) {
	b := &builder{opt: Options{Out: filepath.Join(t.TempDir(), "site"), URL: "https://example.org/math2"}, base: "/math2"}
	var err error
	b.tmpl, err = parseTemplates(b)
	if err != nil {
		t.Fatal(err)
	}
	var f frontier
	for i := 1; i <= frontierPerPage+1; i++ {
		f.Unproved = append(f.Unproved, &dagItem{link: link{Text: fmt.Sprintf("Statement %03d", i), URL: fmt.Sprintf("/math2/wiki/statement-%03d/", i)}, Kind: "statement"})
	}
	if err := b.writeFrontier(f); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, b.opt.Out, "explore/frontier/index.html")
	second := readFile(t, b.opt.Out, "explore/frontier/2/index.html")
	if strings.Contains(first, "Statement 151") || !strings.Contains(first, `href="/math2/explore/frontier/2/"`) {
		t.Error("first frontier page has the wrong rows or next link")
	}
	if !strings.Contains(second, "Statement 151") || !strings.Contains(second, `href="/math2/explore/frontier/"`) {
		t.Error("second frontier page has the wrong rows or previous link")
	}
}

func TestSocialCardCache(t *testing.T) {
	tmp := t.TempDir()
	build := func(title string) *builder {
		b := &builder{opt: Options{Out: filepath.Join(tmp, "site"), URL: "https://example.org/math2"}, base: "/math2"}
		b.cards = newCardCache(filepath.Join(tmp, "cache", "math.gob.gz"))
		if _, err := b.writeSocial("7K3QX2", title, "Theorem"); err != nil {
			t.Fatal(err)
		}
		b.cards.prune()
		return b
	}
	build("First")
	cached := filepath.Join(tmp, "cache", socialCardVersion, filepath.Base(socialPath("7K3QX2", "First", "Theorem")))
	if err := os.WriteFile(cached, []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}
	build("First")
	got, err := os.ReadFile(filepath.Join(tmp, "site", filepath.FromSlash(socialPath("7K3QX2", "First", "Theorem"))))
	if err != nil || string(got) != "cached" {
		t.Errorf("the build drew the card again instead of using the cache: %q %v", got, err)
	}
	build("Second")
	if _, err := os.Stat(cached); !os.IsNotExist(err) {
		t.Error("prune kept a card that the build did not use")
	}
}
