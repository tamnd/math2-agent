package site

import (
	"bufio"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// wpParityDir is the parity data of corpus wikipedia WP9.7 in the math2 tree. The site reads it with its own type, because the importer of Wikipedia uses the package site.
const wpParityDir = "records/parity/wikipedia"

// wpParityItem is the part of an item of the parity data that the article view shows.
type wpParityItem struct {
	Article  string   `json:"article"`
	Title    string   `json:"title"`
	Revision int      `json:"revision"`
	Mention  string   `json:"mention"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Outcome  string   `json:"outcome"`
	Target   []string `json:"target"`
	Reason   string   `json:"reason"`
	Until    string   `json:"until"`
	Node     string   `json:"node"`
	Origin   string   `json:"origin"`
	Assessed string   `json:"assessed"`
}

// wpViewItem is one row of an article view.
type wpViewItem struct {
	Item, Kind, Name, Outcome, Reason, Origin string
	Nodes                                     []wpViewLink
}

// wpViewLink is a link to a node in an article view.
type wpViewLink struct{ Name, URL string }

// wpView is the article view of one Wikipedia article (corpus wikipedia WP9.8).
type wpView struct {
	PageID          int64
	Title, URL, Dir string
	Revision        int
	RevisionURL     string
	Assessed        string
	Signals         string
	Items           []wpViewItem
	Counts          []wpCount // the outcomes, with the number of items in Candidates
	Missing         int
}

// readParity reads the parity data of the tree, by article in the order of the files and the lines. A tree with no parity data gives nil.
func readParity(root string) (map[string][]wpParityItem, []string, error) {
	dir := filepath.Join(root, filepath.FromSlash(wpParityDir))
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	by := map[string][]wpParityItem{}
	var order []string
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name())) // #nosec G304 -- the path comes from a listing of the tree.
		if err != nil {
			return nil, nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for n := 1; sc.Scan(); n++ {
			if strings.TrimSpace(sc.Text()) == "" {
				continue
			}
			var it wpParityItem
			if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
				_ = f.Close()
				return nil, nil, fmt.Errorf("site: %s/%s:%d: %w", wpParityDir, e.Name(), n, err)
			}
			if by[it.Article] == nil {
				order = append(order, it.Article)
			}
			by[it.Article] = append(by[it.Article], it)
		}
		err = sc.Err()
		_ = f.Close()
		if err != nil {
			return nil, nil, err
		}
	}
	return by, order, nil
}

// wpViewDir gives the directory of the view of an article: wikipedia/<slug>, or wikipedia/<slug>-<page id> when the slug is a letter of the lists, a word of the site, or the slug of another article.
func wpViewDir(title string, page int64, taken map[string]bool) string {
	s := Slug(title)
	if len(s) <= 1 || s == "other" || s == "articles" || taken[s] {
		s = strings.Trim(s+"-"+strconv.FormatInt(page, 10), "-")
	}
	taken[s] = true
	return "wikipedia/" + s
}

// wpViews gives the article views of the parity data, in the order of the titles. signals gives the gap signals of each page id, or nil.
func (b *builder) wpViews(signals map[int64]string) ([]wpView, error) {
	by, order, err := readParity(b.opt.Content)
	if err != nil || by == nil {
		return nil, err
	}
	slices.SortFunc(order, func(x, y string) int {
		return cmp.Or(cmp.Compare(strings.ToLower(by[x][0].Title), strings.ToLower(by[y][0].Title)), strings.Compare(x, y))
	})
	taken := map[string]bool{}
	var out []wpView
	for _, art := range order {
		items := by[art]
		first := items[0]
		page, _ := strconv.ParseInt(strings.TrimPrefix(art, "wikipedia://en/"), 10, 64)
		v := wpView{PageID: page, Title: first.Title, Revision: first.Revision, Assessed: first.Assessed, Signals: signals[page],
			URL:         "https://en.wikipedia.org/wiki/" + strings.ReplaceAll(first.Title, " ", "_"),
			RevisionURL: "https://en.wikipedia.org/w/index.php?oldid=" + strconv.Itoa(first.Revision)}
		v.Dir = wpViewDir(first.Title, page, taken)
		count := map[string]*wpCount{}
		for _, it := range items {
			row := wpViewItem{Item: "subject", Kind: it.Kind, Name: it.Name, Outcome: it.Outcome, Reason: it.Reason, Origin: it.Origin}
			if _, tag, ok := strings.Cut(it.Mention, "#"); ok {
				row.Item = "#" + tag
			}
			if it.Outcome == "deferred" && it.Until != "" {
				row.Outcome += " until " + it.Until
			}
			var ids []string
			if it.Outcome == "node" || it.Outcome == "merge" {
				ids = append(ids, it.Target...)
			}
			if it.Node != "" && !slices.Contains(ids, it.Node) {
				ids = append(ids, it.Node)
			}
			for _, id := range ids {
				name := id
				if nv := b.nodes[id]; nv != nil && nv.Name != "" {
					name = nv.Name
				}
				row.Nodes = append(row.Nodes, wpViewLink{Name: name, URL: b.nodeURL(id)})
			}
			v.Items = append(v.Items, row)
			c := count[it.Outcome]
			if c == nil {
				c = &wpCount{Name: it.Outcome}
				count[it.Outcome] = c
			}
			c.Candidates++
			if it.Outcome == "open" || it.Outcome == "no_row" {
				v.Missing++
			}
		}
		v.Counts = countRows(count)
		out = append(out, v)
	}
	return out, nil
}

// writeArticleViews writes the article view of each article of the parity data at wikipedia/<slug>/, and the list of the views at wikipedia/articles/ (corpus wikipedia WP9.8). A view lists the items of the article in the order of the parity data, with the outcome of each item and the links to its nodes. It is a reading of the parity report, and it holds no article text. It gives the directory of the view of each page id.
func (b *builder) writeArticleViews(signals map[int64]string) (map[int64]string, error) {
	views, err := b.wpViews(signals)
	if err != nil || len(views) == 0 {
		return nil, err
	}
	dirs := map[int64]string{}
	items, missing := 0, 0
	for _, v := range views {
		dirs[v.PageID] = b.url(v.Dir + "/")
		items += len(v.Items)
		missing += v.Missing
		desc := fmt.Sprintf("The items of the Wikipedia article %s and the nodes of Math 2.0 that have them.", v.Title)
		if err := b.writePage(v.Dir, "wikipediaarticle", v, page{Title: v.Title + " on Wikipedia", Description: desc, Section: "wikipedia"}); err != nil {
			return nil, err
		}
	}
	d := struct {
		Views          []wpView
		Items, Missing int
		Scope          bool
	}{views, items, missing, b.HasWikipedia()}
	if err := b.writePage("wikipedia/articles", "wikipediaarticles", d, page{Title: "Wikipedia articles and their nodes", Description: "The Wikipedia articles that have an article view, with the outcome of their items in Math 2.0.", Section: "wikipedia"}); err != nil {
		return nil, err
	}
	return dirs, nil
}
