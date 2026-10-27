package wpcorpus

import (
	"bufio"
	"bytes"
	"encoding/json"
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
)

// Shard gives the shard directory of a page: the page id divided by 10000,
// with four digits.
func Shard(page int) string { return fmt.Sprintf("%04d", page/10000) }

// WikitextPath gives the path of the content file of a page, relative to the
// corpus root.
func WikitextPath(page int) string {
	return filepath.Join(WikitextDir, Shard(page), strconv.Itoa(page)+".wiki")
}

// ObjectsPath gives the path of the objects file of a page revision.
func ObjectsPath(page, rev int) string {
	return filepath.Join(ObjectsDir, Shard(page), fmt.Sprintf("%d-r%d.jsonl", page, rev))
}

// CensusPath gives the path of the census file of a page revision.
func CensusPath(page, rev int) string {
	return filepath.Join(CensusDir, Shard(page), fmt.Sprintf("%d-r%d.json", page, rev))
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
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for i := range r.Objects {
		if err := enc.Encode(&r.Objects[i]); err != nil {
			return err
		}
	}
	if err := writeFile(filepath.Join(root, ObjectsPath(c.Page, c.Revision)), b.Bytes()); err != nil {
		return err
	}
	b.Reset()
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return err
	}
	return writeFile(filepath.Join(root, CensusPath(c.Page, c.Revision)), b.Bytes())
}

// ReadObjects reads the objects file of a page revision.
func ReadObjects(root string, page, rev int) ([]Object, error) {
	f, err := os.Open(filepath.Join(root, ObjectsPath(page, rev))) // #nosec G304 -- the path is in the corpus that the user names
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []Object
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for n := 1; sc.Scan(); n++ {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var o Object
		if err := json.Unmarshal(sc.Bytes(), &o); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", ObjectsPath(page, rev), n, err)
		}
		out = append(out, o)
	}
	return out, sc.Err()
}

// ReadCensus reads the census file of a page revision.
func ReadCensus(root string, page, rev int) (*Census, error) {
	data, err := os.ReadFile(filepath.Join(root, CensusPath(page, rev))) // #nosec G304 -- the path is in the corpus that the user names
	if err != nil {
		return nil, err
	}
	var c Census
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", CensusPath(page, rev), err)
	}
	return &c, nil
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
