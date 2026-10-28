package wpcorpus

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The directories of the planes in the corpus (WP12).
const (
	WikitextDir = "content/wikitext"
	ObjectsDir  = "analysis/objects"
	CensusDir   = "analysis/census"
	MarkdownDir = "content/markdown"
)

// Shard gives the shard directory of a page: the page id divided by 10000,
// with four digits.
func Shard(page int) string { return fmt.Sprintf("%04d", page/10000) }

// WikitextPath gives the path of the content file of a page, relative to the
// corpus root.
func WikitextPath(page int) string {
	return filepath.Join(WikitextDir, Shard(page), strconv.Itoa(page)+".wiki")
}

// MarkdownPath gives the path of the Markdown rendering of a page (WP3.12).
func MarkdownPath(page int) string {
	return filepath.Join(MarkdownDir, Shard(page), strconv.Itoa(page)+".md")
}

// ObjectsPath gives the path of the objects file of a page revision.
func ObjectsPath(page, rev int) string {
	return filepath.Join(ObjectsDir, Shard(page), fmt.Sprintf("%d-r%d.md", page, rev))
}

// CensusPath gives the path of the census file of a page revision.
func CensusPath(page, rev int) string {
	return filepath.Join(CensusDir, Shard(page), fmt.Sprintf("%d-r%d.md", page, rev))
}

// Page is one page of the content plane.
type Page struct {
	ID        int
	Title     string
	Revision  int
	Timestamp string
	Wikitext  string
}

// Header gives the header comment of a content file (WP12.1). It names the
// page, the revision, the history and the licence, so that the file alone
// gives the attribution that CC BY-SA asks for.
func Header(p *Page) string {
	return fmt.Sprintf("<!-- wikipedia: en\n     page: %d\n     title: %s\n     revision: %d\n     timestamp: %s\n     history: https://en.wikipedia.org/w/index.php?curid=%d&action=history\n     licence: CC BY-SA 4.0 (https://creativecommons.org/licenses/by-sa/4.0/)\n     source: https://en.wikipedia.org/w/index.php?oldid=%d -->\n",
		p.ID, p.Title, p.Revision, p.Timestamp, p.ID, p.Revision)
}

const headerEnd = " -->\n"

// ParseContent reads a content file: the header and the wikitext after it.
func ParseContent(data []byte) (*Page, error) {
	s := string(data)
	if !strings.HasPrefix(s, "<!-- wikipedia:") {
		return nil, errors.New("the file has no wikipedia header")
	}
	i := strings.Index(s, headerEnd)
	if i < 0 {
		return nil, errors.New("the header does not close")
	}
	p := &Page{Wikitext: s[i+len(headerEnd):]}
	for _, l := range strings.Split(s[:i], "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), ": ")
		if !ok {
			continue
		}
		var err error
		switch k {
		case "page":
			p.ID, err = strconv.Atoi(v)
		case "title":
			p.Title = v
		case "revision":
			p.Revision, err = strconv.Atoi(v)
		case "timestamp":
			p.Timestamp = v
		}
		if err != nil {
			return nil, fmt.Errorf("header %s: %w", k, err)
		}
	}
	if p.ID == 0 || p.Revision == 0 || p.Title == "" {
		return nil, errors.New("the header has no page, title or revision")
	}
	return p, nil
}

// ReadContent reads the content file of a page.
func ReadContent(root string, page int) (*Page, error) {
	path := filepath.Join(root, WikitextPath(page))
	data, err := os.ReadFile(path) // #nosec G304 -- the path is in the corpus that the user names
	if err != nil {
		return nil, err
	}
	p, err := ParseContent(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", WikitextPath(page), err)
	}
	return p, nil
}

// WriteContent writes the content file of a page.
func WriteContent(root string, p *Page) error {
	return writeFile(filepath.Join(root, WikitextPath(p.ID)), []byte(Header(p)+p.Wikitext))
}

// WriteResult writes the objects file and the census file of a result.
func WriteResult(root string, r *Result) error {
	c := &r.Census
	data, err := MarshalObjects(c.Page, c.Revision, c.Title, r.Objects)
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(root, ObjectsPath(c.Page, c.Revision)), data); err != nil {
		return err
	}
	if data, err = MarshalCensus(c); err != nil {
		return err
	}
	return writeFile(filepath.Join(root, CensusPath(c.Page, c.Revision)), data)
}

// ReadObjects reads the objects file of a page revision.
func ReadObjects(root string, page, rev int) ([]Object, error) {
	data, err := os.ReadFile(filepath.Join(root, ObjectsPath(page, rev))) // #nosec G304 -- the path is in the corpus that the user names
	if err != nil {
		return nil, err
	}
	objs, err := ParseObjects(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ObjectsPath(page, rev), err)
	}
	return objs, nil
}

// ReadCensus reads the census file of a page revision.
func ReadCensus(root string, page, rev int) (*Census, error) {
	data, err := os.ReadFile(filepath.Join(root, CensusPath(page, rev))) // #nosec G304 -- the path is in the corpus that the user names
	if err != nil {
		return nil, err
	}
	c, err := ParseCensus(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", CensusPath(page, rev), err)
	}
	return c, nil
}

// Pages returns the ids of the pages in the content plane, in order.
func Pages(root string) ([]int, error) {
	var out []int
	err := filepath.WalkDir(filepath.Join(root, WikitextDir), func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return filepath.SkipAll
			}
			return err
		}
		name := d.Name()
		if d.IsDir() || !strings.HasSuffix(name, ".wiki") {
			return nil
		}
		id, err := strconv.Atoi(strings.TrimSuffix(name, ".wiki"))
		if err != nil {
			return nil
		}
		out = append(out, id)
		return nil
	})
	slices.Sort(out)
	return out, err
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the corpus is a public repository
		return err
	}
	return os.WriteFile(path, data, 0o644) // #nosec G306 -- the corpus is a public repository
}

// RemoveStale removes the objects files and the census files of the other
// revisions of a page, so that the analysis plane holds only the revision of
// the content plane.
func RemoveStale(root string, page, rev int) error {
	keep := map[string]bool{ObjectsPath(page, rev): true, CensusPath(page, rev): true}
	for _, dir := range []string{ObjectsDir, CensusDir} {
		d := filepath.Join(dir, Shard(page))
		ents, err := os.ReadDir(filepath.Join(root, d))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		prefix := strconv.Itoa(page) + "-r"
		for _, e := range ents {
			p := filepath.Join(d, e.Name())
			if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || keep[p] {
				continue
			}
			if err := os.Remove(filepath.Join(root, p)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ScopeResolver gives a Resolver for the articles of scope.tsv. A link to an
// article that is not in scope keeps its URL on Wikipedia. When the corpus
// has no scope.tsv, the resolver knows no title.
func ScopeResolver(root string) (Resolver, error) {
	cs, err := ReadScopeFile(root, ScopePath)
	if errors.Is(err, os.ErrNotExist) {
		return func(string) int { return 0 }, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make(map[string]int, len(cs))
	for _, c := range cs {
		if c.InScope {
			ids[normTitle(c.Title)] = int(c.PageID)
		}
	}
	return func(title string) int { return ids[normTitle(title)] }, nil
}

// normTitle gives the form of a title that the resolver compares: spaces
// as underscores and the first letter in upper case.
func normTitle(t string) string {
	t = strings.ReplaceAll(strings.TrimSpace(t), " ", "_")
	if t == "" {
		return t
	}
	r := []rune(t)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// WriteMarkdown writes the Markdown rendering of a page to the content plane.
func WriteMarkdown(root string, page int, data []byte) error {
	return writeFile(filepath.Join(root, MarkdownPath(page)), data)
}

// NormTitle gives the form of a title that the resolvers compare: spaces for
// underscores, and the first letter in upper case.
func NormTitle(t string) string { return normTitle(t) }
