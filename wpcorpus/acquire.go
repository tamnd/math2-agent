package wpcorpus

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/wikipedia-cli/wiki/parse"
	"go.yaml.in/yaml/v3"
)

// MetadataDir is the directory of the metadata plane (WP12).
const MetadataDir = "metadata"

// MetadataPath gives the path of the metadata file of a page.
func MetadataPath(page int) string {
	return filepath.Join(MetadataDir, Shard(page), strconv.Itoa(page)+".md")
}

// Metadata is the metadata file of an article in scope: the facts about the
// article and its revision of record.
type Metadata struct {
	Page       int      `yaml:"page"`
	Title      string   `yaml:"title"`
	Revision   int      `yaml:"revision"`
	Timestamp  string   `yaml:"timestamp"`
	Length     int      `yaml:"length"`
	SHA1       string   `yaml:"sha1,omitempty"`
	Dump       string   `yaml:"dump,omitempty"`
	Type       string   `yaml:"type"`
	Signals    string   `yaml:"signals,omitempty"`
	Rule       string   `yaml:"rule,omitempty"`
	Projects   []string `yaml:"projects,omitempty"`
	Class      string   `yaml:"class,omitempty"`
	Importance string   `yaml:"importance,omitempty"`
	Wikidata   string   `yaml:"wikidata,omitempty"`
	// Short is the short description of the article. It is CC BY-SA text,
	// so it stays in the corpus repository (WQ06).
	Short      string   `yaml:"short_description,omitempty"`
	Categories []string `yaml:"categories,omitempty"`
	Redirects  []string `yaml:"redirects,omitempty"`
	MSC        []string `yaml:"msc,omitempty"`
	// Unstable is true when no revision passed the rule of WP2.6 and the
	// revision of the dump is the revision of record.
	Unstable bool `yaml:"unstable,omitempty"`
}

// MarshalMetadata gives the metadata file of an article. The header holds
// the facts, and the body is a summary for the reader.
func MarshalMetadata(m *Metadata) ([]byte, error) {
	var b bytes.Buffer
	if err := writeFrontMatter(&b, m); err != nil {
		return nil, err
	}
	fmt.Fprintf(&b, "\n# %s\n\n", m.Title)
	fmt.Fprintf(&b, "The revision of record is [%d](https://en.wikipedia.org/w/index.php?oldid=%d) of %s. It has %d bytes of wikitext.", m.Revision, m.Revision, m.Timestamp, m.Length)
	if m.Unstable {
		b.WriteString(" No revision of the last 90 days passed the rule of WP2.6, so it is the revision of the dump.")
	}
	b.WriteString("\n")
	return b.Bytes(), nil
}

// ParseMetadata reads a file that MarshalMetadata wrote.
func ParseMetadata(data []byte) (*Metadata, error) {
	head, _, err := splitFrontMatter(data)
	if err != nil {
		return nil, err
	}
	var m Metadata
	if err := yaml.Unmarshal(head, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// WriteMetadata writes the metadata file of an article.
func WriteMetadata(root string, m *Metadata) error {
	data, err := MarshalMetadata(m)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(root, MetadataPath(m.Page)), data)
}

// PageFacts gives the facts of the wikitext of a page: its categories and
// its short description.
func PageFacts(wikitext string) (categories []string, short string) {
	doc, err := parse.Parse(wikitext)
	if err != nil {
		return nil, ""
	}
	seen := map[string]bool{}
	var walk func(ns []*parse.Node)
	walk = func(ns []*parse.Node) {
		for _, n := range ns {
			switch n.Kind {
			case parse.KindLink:
				name := strings.TrimSpace(n.Name)
				if i := strings.IndexByte(name, ':'); i > 0 && strings.EqualFold(strings.TrimSpace(name[:i]), "category") {
					c := strings.TrimSpace(name[i+1:])
					if j := strings.IndexByte(c, '|'); j >= 0 {
						c = strings.TrimSpace(c[:j])
					}
					c = strings.ReplaceAll(c, "_", " ")
					if c != "" && !seen[c] {
						seen[c] = true
						categories = append(categories, c)
					}
				}
			case parse.KindTemplate:
				if short == "" && parse.TemplateKey(n.Name) == "short description" {
					if p := n.Param("1"); p != nil {
						short = strings.TrimSpace(doc.Plain(p.Value))
					}
				}
				for _, p := range n.Params {
					walk(p.Value)
				}
			}
			walk(n.Children)
		}
	}
	walk(doc.Root.Children)
	return categories, short
}

// StoreFacts are the facts of the articles from the SQL tables of the dump:
// the Wikidata item, the short description of page_props, and the titles
// of the redirects to each article.
type StoreFacts struct {
	Wikidata  map[int64]string
	Short     map[int64]string
	Redirects map[string][]string // by the title of the target, with underscores
	// Target gives the title of the target of each redirect to a page of
	// titles, by the title of the redirect, with underscores.
	Target map[string]string
}

// Facts reads the facts of the pages from the tables page, props and
// redirect of the store. titles are the titles of the pages, with
// underscores.
func (s *Store) Facts(ids []int64, titles map[string]bool) (*StoreFacts, error) {
	f := &StoreFacts{Wikidata: map[int64]string{}, Short: map[int64]string{}, Redirects: map[string][]string{}, Target: map[string]string{}}
	want := make(map[int64]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	if ok, err := s.Has("props"); err != nil {
		return nil, err
	} else if ok {
		rows, err := s.DB.Query(`SELECT pp_page, pp_propname, pp_value FROM props WHERE pp_propname IN ('wikibase_item', 'wikibase-shortdesc')`)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var name, value string
			if err := rows.Scan(&id, &name, &value); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if !want[id] {
				continue
			}
			if name == "wikibase_item" {
				f.Wikidata[id] = value
			} else {
				f.Short[id] = value
			}
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	if ok, err := s.Has("redirect"); err != nil {
		return nil, err
	} else if ok {
		rows, err := s.DB.Query(`SELECT p.page_title, r.rd_title FROM redirect r JOIN page p ON p.page_id = r.rd_from WHERE r.rd_namespace = 0 AND p.page_namespace = 0`)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var from, to string
			if err := rows.Scan(&from, &to); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if titles[to] {
				f.Target[from] = to
				f.Redirects[to] = append(f.Redirects[to], strings.ReplaceAll(from, "_", " "))
			}
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		for _, rs := range f.Redirects {
			slices.Sort(rs)
		}
	}
	return f, nil
}

// importanceRank gives the order of WP2.9 for the importance of an article.
var importanceRank = map[string]int{"Top": 0, "High": 1, "Mid": 2, "Low": 3}

// FetchOrder sorts the articles in the order of WP2.9: the articles of S4,
// then the articles of the pilot, then by importance, then by page id.
func FetchOrder(cs []*Candidate, pilot map[string]bool) {
	rank := func(c *Candidate) (int, int) {
		group := 2
		switch {
		case c.Signals&S4 != 0:
			group = 0
		case pilot[normTitle(c.Title)]:
			group = 1
		}
		imp, ok := importanceRank[c.Importance]
		if !ok {
			imp = 4
		}
		return group, imp
	}
	slices.SortStableFunc(cs, func(a, b *Candidate) int {
		ga, ia := rank(a)
		gb, ib := rank(b)
		if ga != gb {
			return ga - gb
		}
		if ia != ib {
			return ia - ib
		}
		return cmpID(a.PageID, b.PageID)
	})
}

// LinkResolver gives a Resolver for the articles in scope that follows the
// redirects of the store. target gives the target of a redirect title.
func LinkResolver(cs []*Candidate, target map[string]string) Resolver {
	ids := make(map[string]int, len(cs))
	for _, c := range cs {
		if c.InScope {
			ids[normTitle(c.Title)] = int(c.PageID)
		}
	}
	return func(title string) int {
		t := normTitle(title)
		if id := ids[t]; id != 0 {
			return id
		}
		if to, ok := target[t]; ok {
			return ids[normTitle(to)]
		}
		return 0
	}
}
