package wpcorpus

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // the database/sql driver "sqlite"
)

// The signals of WP1.2. A candidate holds a set of them.
const (
	S1 Signals = 1 << iota // the WikiProject Mathematics tag
	S2                     // the tag of Statistics, Philosophy/Logic or Cryptography
	S3                     // the tag of Computer science, Physics or Systems
	S4                     // the title is cited by math2
	S5                     // a Wikidata class or identifier of mathematics
	S6                     // a category in the tree of categories.yaml
	S7                     // an item template or 3 or more <math> elements
	S8                     // a link from a list, glossary, outline or timeline in S1
	S9                     // a link from 3 or more articles in S1, and a math category
)

// Signals is a set of the signals of WP1.2.
type Signals uint16

// String gives the signals as "S1,S6", or "" for the empty set.
func (s Signals) String() string {
	var parts []string
	for i := range 9 {
		if s&(1<<i) != 0 {
			parts = append(parts, "S"+strconv.Itoa(i+1))
		}
	}
	return strings.Join(parts, ",")
}

// ParseSignals reads the form that String gives.
func ParseSignals(v string) (Signals, error) {
	var s Signals
	for _, p := range strings.Split(v, ",") {
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(p, "S"))
		if err != nil || !strings.HasPrefix(p, "S") || n < 1 || n > 9 {
			return 0, fmt.Errorf("%q is not a signal", p)
		}
		s |= 1 << (n - 1)
	}
	return s, nil
}

// The projects of the signals S1 to S3, as the PageAssessments API names them.
var (
	projectsS1 = []string{"Mathematics"}
	projectsS2 = []string{"Statistics", "Philosophy/Logic", "Cryptography"}
	projectsS3 = []string{"Computer science", "Physics", "Systems"}
)

// ProjectSignal gives the signal of a WikiProject, or 0 when the project is
// not one of S1 to S3.
func ProjectSignal(project string) Signals {
	switch {
	case slices.Contains(projectsS1, project):
		return S1
	case slices.Contains(projectsS2, project):
		return S2
	case slices.Contains(projectsS3, project):
		return S3
	}
	return 0
}

// CategoryRoot is one root of the category tree of signal S6.
type CategoryRoot struct {
	Title string `yaml:"title"`
	Depth int    `yaml:"depth"`
}

// CategoryTree is the content of categories.yaml.
type CategoryTree struct {
	Version  int            `yaml:"version"`
	Roots    []CategoryRoot `yaml:"roots"`
	Exclude  []string       `yaml:"exclude"`
	Keywords []string       `yaml:"keywords"`
}

// LoadCategoryTree reads manifests/wikipedia/categories.yaml of the corpus.
func LoadCategoryTree(root string) (*CategoryTree, error) {
	var t CategoryTree
	if err := readYAML(filepath.Join(root, ManifestDir, "categories.yaml"), &t); err != nil {
		return nil, err
	}
	if len(t.Roots) == 0 {
		return nil, errors.New("categories.yaml: the tree has no roots")
	}
	for i, r := range t.Roots {
		if r.Title == "" || r.Depth < 0 {
			return nil, fmt.Errorf("categories.yaml: root %d needs a title and a depth of 0 or more", i+1)
		}
		t.Roots[i].Title = catTitle(r.Title)
	}
	for i, e := range t.Exclude {
		t.Exclude[i] = catTitle(e)
	}
	for i, k := range t.Keywords {
		t.Keywords[i] = strings.ToLower(catTitle(k))
	}
	return &t, nil
}

// MathKeyword tells if a category title has one of the keywords.
func (t *CategoryTree) MathKeyword(title string) bool {
	l := strings.ToLower(catTitle(title))
	for _, k := range t.Keywords {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

// catTitle gives the form of a title in the tables: no namespace prefix,
// and underscores for spaces.
func catTitle(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "Category:"))
	return strings.ReplaceAll(s, " ", "_")
}

// The tables of the scope store. `wiki dump load` writes them (WP2.2 step 2).
// The scope build names the load command of each table that is missing.
const (
	TablePage       = "page"         // page: namespaces 0 and 14
	TableRedirect   = "redirect"     // redirect: namespace 0
	TableProps      = "props"        // page_props: wikibase_item and disambiguation
	TableCatTargets = "cat_targets"  // linktarget: namespace 14
	TableSubcats    = "subcats"      // categorylinks: the subcategory links
	TableCatPages   = "cat_pages"    // categorylinks: the pages in the categories of the tree
	TableS1Links    = "s1_links"     // pagelinks: the links from the articles of S1
	TableLinkTarget = "link_targets" // linktarget: the targets of s1_links in namespace 0
	TablePageCats   = "page_cats"    // categorylinks: the categories of the link candidates
)

// Store is the SQLite store of one dump.
type Store struct {
	DB *sql.DB
}

// OpenStore opens the store at path. It does not create it.
func OpenStore(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA temp_store=MEMORY"); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

// Close closes the store.
func (s *Store) Close() error { return s.DB.Close() }

// Has tells if the store has the table.
func (s *Store) Has(table string) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n)
	return n > 0, err
}

// TreeCategory is one category of the tree, with its depth below a root.
type TreeCategory struct {
	Title string
	ID    int64 // the lt_id of the title in linktarget
	Depth int
}

// Tree walks the subcategory links from the roots, breadth first, to the
// depth of each root. It does not walk into an excluded category. A
// category that two roots reach keeps the lower depth.
func (s *Store) Tree(t *CategoryTree) ([]TreeCategory, error) {
	excluded := map[string]bool{}
	for _, e := range t.Exclude {
		excluded[e] = true
	}
	depth := map[string]int{}
	for _, r := range t.Roots {
		if d, ok := depth[r.Title]; !ok || d > 0 {
			depth[r.Title] = 0
		}
		frontier := []string{r.Title}
		for d := 1; d <= r.Depth && len(frontier) > 0; d++ {
			subs, err := s.subcategories(frontier)
			if err != nil {
				return nil, err
			}
			frontier = frontier[:0]
			for _, c := range subs {
				if excluded[c] {
					continue
				}
				if old, ok := depth[c]; ok && old <= d {
					continue
				}
				depth[c] = d
				frontier = append(frontier, c)
			}
		}
	}
	titles := make([]string, 0, len(depth))
	for c := range depth {
		titles = append(titles, c)
	}
	slices.Sort(titles)
	ids, err := s.targetIDs(titles)
	if err != nil {
		return nil, err
	}
	out := make([]TreeCategory, 0, len(titles))
	for _, c := range titles {
		if id, ok := ids[c]; ok {
			out = append(out, TreeCategory{Title: c, ID: id, Depth: depth[c]})
		}
	}
	return out, nil
}

// batchSize is the number of values in one IN list.
const batchSize = 500

// jsonList gives the values as a JSON array, for json_each in a query.
func jsonList[T any](v []T) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // strings and integers always encode
	}
	return string(b)
}

func batches[T any](v []T, fn func([]T) error) error {
	for i := 0; i < len(v); i += batchSize {
		if err := fn(v[i:min(i+batchSize, len(v))]); err != nil {
			return err
		}
	}
	return nil
}

// subcategories gives the titles of the subcategories of the categories.
func (s *Store) subcategories(cats []string) ([]string, error) {
	var out []string
	err := batches(cats, func(b []string) error {
		rows, err := s.DB.Query(`SELECT DISTINCT p.page_title FROM cat_targets t
			JOIN subcats l ON l.cl_target_id = t.lt_id
			JOIN page p ON p.page_id = l.cl_from AND p.page_namespace = 14
			WHERE t.lt_title IN (SELECT value FROM json_each(?))`, jsonList(b))
		if err != nil {
			return err
		}
		return scanStrings(rows, &out)
	})
	return out, err
}

func (s *Store) targetIDs(titles []string) (map[string]int64, error) {
	out := map[string]int64{}
	err := batches(titles, func(b []string) error {
		rows, err := s.DB.Query(`SELECT lt_title, lt_id FROM cat_targets WHERE lt_title IN (SELECT value FROM json_each(?))`, jsonList(b))
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var t string
			var id int64
			if err := rows.Scan(&t, &id); err != nil {
				return err
			}
			out[t] = id
		}
		return rows.Err()
	})
	return out, err
}

func scanStrings(rows *sql.Rows, out *[]string) error {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		*out = append(*out, v)
	}
	return rows.Err()
}

// Articles gives the title of each id that is an article: a page in
// namespace 0 that is not a redirect and not a disambiguation page.
func (s *Store) Articles(ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	err := batches(ids, func(b []int64) error {
		rows, err := s.DB.Query(`SELECT page_id, page_title FROM page
			WHERE page_namespace = 0 AND page_is_redirect = 0 AND page_id IN (SELECT value FROM json_each(?1))
			AND page_id NOT IN (SELECT pp_page FROM props WHERE pp_propname = 'disambiguation' AND pp_page IN (SELECT value FROM json_each(?1)))`,
			jsonList(b))
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id int64
			var t string
			if err := rows.Scan(&id, &t); err != nil {
				return err
			}
			out[id] = t
		}
		return rows.Err()
	})
	return out, err
}

// Resolve gives the page id of each title in namespace 0. It follows one
// redirect. A title that has no page is not in the map.
func (s *Store) Resolve(titles []string) (map[string]int64, error) {
	out := map[string]int64{}
	norm := make([]string, 0, len(titles))
	for _, t := range titles {
		norm = append(norm, strings.ReplaceAll(strings.TrimSpace(t), " ", "_"))
	}
	err := batches(norm, func(b []string) error {
		rows, err := s.DB.Query(`SELECT p.page_title, coalesce(t.page_id, p.page_id) FROM page p
			LEFT JOIN redirect r ON p.page_is_redirect = 1 AND r.rd_from = p.page_id AND r.rd_namespace = 0
			LEFT JOIN page t ON t.page_namespace = 0 AND t.page_title = r.rd_title
			WHERE p.page_namespace = 0 AND p.page_title IN (SELECT value FROM json_each(?))`, jsonList(b))
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var t string
			var id int64
			if err := rows.Scan(&t, &id); err != nil {
				return err
			}
			out[t] = id
		}
		return rows.Err()
	})
	return out, err
}

// int64s runs a query that gives one integer column.
func (s *Store) int64s(q string, args ...any) ([]int64, error) {
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CategoryMembers gives the pages in the categories of the tree, with the
// lowest depth of their categories.
func (s *Store) CategoryMembers(tree []TreeCategory) (map[int64]int, error) {
	depth := make(map[int64]int, len(tree))
	for _, c := range tree {
		depth[c.ID] = c.Depth
	}
	rows, err := s.DB.Query(`SELECT cl_from, cl_target_id FROM cat_pages`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]int{}
	for rows.Next() {
		var page, target int64
		if err := rows.Scan(&page, &target); err != nil {
			return nil, err
		}
		d, ok := depth[target]
		if !ok {
			continue
		}
		if old, seen := out[page]; !seen || d < old {
			out[page] = d
		}
	}
	return out, rows.Err()
}

// LinkTargetIDs gives the distinct target ids of s1_links.
func (s *Store) LinkTargetIDs() ([]int64, error) {
	return s.int64s(`SELECT DISTINCT pl_target_id FROM s1_links ORDER BY pl_target_id`)
}

// LinkCounts gives, for each target title in namespace 0, the number of
// sources of s1_links that link to it, and the number of list sources.
func (s *Store) LinkCounts(lists map[int64]bool) (sources, fromLists map[string]int, err error) {
	rows, err := s.DB.Query(`SELECT l.pl_from, t.lt_title FROM s1_links l
		JOIN link_targets t ON t.lt_id = l.pl_target_id AND t.lt_namespace = 0`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	sources, fromLists = map[string]int{}, map[string]int{}
	type pair struct {
		from  int64
		title string
	}
	seen := map[pair]bool{}
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.from, &p.title); err != nil {
			return nil, nil, err
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		sources[p.title]++
		if lists[p.from] {
			fromLists[p.title]++
		}
	}
	return sources, fromLists, rows.Err()
}

// PageCategories gives the category titles of each page of page_cats.
func (s *Store) PageCategories() (map[int64][]string, error) {
	rows, err := s.DB.Query(`SELECT c.cl_from, t.lt_title FROM page_cats c JOIN cat_targets t ON t.lt_id = c.cl_target_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64][]string{}
	for rows.Next() {
		var id int64
		var t string
		if err := rows.Scan(&id, &t); err != nil {
			return nil, err
		}
		out[id] = append(out[id], t)
	}
	return out, rows.Err()
}

// ListArticle tells if a title is a list, a glossary, an outline or a
// timeline (signal S8).
func ListArticle(title string) bool {
	for _, p := range []string{"List_of_", "Lists_of_", "Glossary_of_", "Outline_of_", "Timeline_of_"} {
		if strings.HasPrefix(strings.ReplaceAll(title, " ", "_"), p) {
			return true
		}
	}
	return false
}

// AssessRow is one row of the output of `wiki assess`.
type AssessRow struct {
	PageID     int64
	Title      string
	Project    string
	Class      string
	Importance string
}

// ReadAssess reads the TSV output of `wiki assess -o tsv`: pageid, ns, title,
// project, class and importance, with a header. It keeps the rows of
// namespace 0.
func ReadAssess(r io.Reader) ([]AssessRow, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	var out []AssessRow
	line := 0
	for sc.Scan() {
		line++
		f := strings.Split(sc.Text(), "\t")
		if line == 1 && len(f) > 0 && f[0] == "pageid" {
			continue
		}
		if len(f) < 6 {
			return nil, fmt.Errorf("line %d: want 6 fields, have %d", line, len(f))
		}
		if f[1] != "0" {
			continue
		}
		id, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		out = append(out, AssessRow{PageID: id, Title: strings.ReplaceAll(f[2], " ", "_"), Project: f[3], Class: f[4], Importance: f[5]})
	}
	return out, sc.Err()
}
