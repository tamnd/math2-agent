// Package resolve finds the resource of one id or one citation (RS3.6, RS3.7). It works in passes, the cheapest first: the local index of all ids, the crosswalks of the snapshots, the live APIs, and the citation matcher. It never uses the search engine of a closed service.
package resolve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/parquet-go/parquet-go"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Version names the passes and the weights of the matcher. The matcher writes it into each cites edge, so that a later matcher can redo the weak matches (RS3.7).
const Version = "resolve/1"

// IndexFile is the index of all ids in the tree (RS12.1).
const IndexFile = "build/resources/ids.parquet"

// The thresholds of the matcher.
const (
	High   = 0.9  // a score at or above it is a match
	Middle = 0.75 // a score at or above it and below High goes to the queue res-resolve
)

// The outcomes of a resolution.
const (
	Found      = "found"
	Candidates = "candidates" // the best scores are in the middle band
	NotFound   = "not-found"
)

// The passes of RS3.6.
const (
	Local     = "local"
	Crosswalk = "crosswalk"
	Live      = "live"
	Match     = "match"
)

// Row is one id of the index. The id is the key of the system and the value in normal form, as doi:10.1000/x.
type Row struct {
	ID            string `parquet:"id"`
	Resource      string `parquet:"resource"`
	Expression    string `parquet:"expression,optional"`
	Manifestation string `parquet:"manifestation,optional"`
}

// Ref is the place of an id in the catalogue: a resource, and an expression and a manifestation of it when the id names one.
type Ref struct {
	Resource      string `json:"resource"`
	Expression    string `json:"expression,omitempty"`
	Manifestation string `json:"manifestation,omitempty"`
}

// At gives the reference as <id>, <id>/e<n> or <id>/e<n>/m<n>.
func (r Ref) At() string {
	s := r.Resource
	if r.Expression != "" {
		s += "/" + r.Expression
		if r.Manifestation != "" {
			s += "/" + r.Manifestation
		}
	}
	return s
}

// Index holds the ids of the catalogue and the records for the matcher.
type Index struct {
	refs  map[string][]Ref
	res   map[string]*resource.Resource
	block map[string][]string // the blocks of the matcher: family name and year, or venue, volume and page
}

// Build makes the index of the live resources.
func Build(rs []resource.Resource) *Index {
	x := &Index{refs: map[string][]Ref{}, res: map[string]*resource.Resource{}, block: map[string][]string{}}
	for i := range rs {
		r := &rs[i]
		if !r.Live() {
			continue
		}
		x.res[r.ID] = r
		for _, row := range Rows(r) {
			x.add(row)
		}
		if f, y := first(r); f != "" && y > 0 {
			k := blockKey(f, y)
			x.block[k] = append(x.block[k], r.ID)
		}
	}
	return x
}

func (x *Index) add(row Row) {
	ref := Ref{row.Resource, row.Expression, row.Manifestation}
	if !slices.Contains(x.refs[row.ID], ref) {
		x.refs[row.ID] = append(x.refs[row.ID], ref)
	}
}

// Rows gives the rows of the index for one resource, in a fixed order.
func Rows(r *resource.Resource) []Row {
	var out []Row
	add := func(m resource.IDs, e, mf string) {
		for _, sys := range slices.Sorted(mapsKeys(m)) {
			for _, v := range m[sys] {
				out = append(out, Row{ID: sys + ":" + v, Resource: r.ID, Expression: e, Manifestation: mf})
			}
		}
	}
	add(r.IDs, "", "")
	for _, e := range r.Expressions {
		add(e.IDs, e.Key, "")
		for _, m := range e.Manifestations {
			add(m.IDs, e.Key, m.Key)
		}
	}
	return out
}

func mapsKeys(m resource.IDs) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// WriteIndex writes the rows of the index to IndexFile in the tree at root, in the order of the ids.
func WriteIndex(root string, rs []resource.Resource) (int, error) {
	var rows []Row
	for i := range rs {
		if rs[i].Live() {
			rows = append(rows, Rows(&rs[i])...)
		}
	}
	slices.SortFunc(rows, func(a, b Row) int { return strings.Compare(a.ID+" "+a.Resource, b.ID+" "+b.Resource) })
	p := filepath.Join(root, IndexFile)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return 0, err
	}
	if err := parquet.WriteFile(p, rows, parquet.CreatedBy("m2", Version, "")); err != nil {
		return 0, fmt.Errorf("resolve: write %s: %w", IndexFile, err)
	}
	return len(rows), nil
}

// ReadIndex reads the rows of IndexFile in the tree at root.
func ReadIndex(root string) ([]Row, error) {
	return parquet.ReadFile[Row](filepath.Join(root, IndexFile))
}

// Lookup gives the places of one id in normal form, as doi:10.1000/x.
func (x *Index) Lookup(id string) []Ref { return x.refs[id] }

// Resource gives the live resource with the id.
func (x *Index) Resource(id string) *resource.Resource { return x.res[id] }

// Key gives the normal form sys:value of an id that a source gives, as doi:https://doi.org/10.1000/X or arXiv:2301.00001v2.
func Key(s string) (string, error) {
	sys, v, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return "", fmt.Errorf("resolve: %q has no system, as doi:10.1000/x", s)
	}
	sys = strings.ToLower(sys)
	n, err := ids.Normalize(sys, v)
	if err != nil {
		return "", err
	}
	return sys + ":" + n, nil
}

// A Fetcher asks a live API for the other ids of one id. It gives the ids in the form sys:value. A fetcher gives no ids and no error for an id that the API does not know.
type Fetcher interface {
	Name() string
	Systems() []string // the systems of the ids that the fetcher can ask for
	Fetch(ctx context.Context, id string) ([]string, error)
}

// Resolver runs the passes of RS3.6 for ids and the matcher of RS3.7 for citations.
type Resolver struct {
	Index      *Index
	Crosswalks map[string][]string // the ids that a crosswalk of a snapshot links to each id
	Fetchers   []Fetcher           // the live APIs. None means that the resolver does not use the network
	Cache      *Cache              // the answers of the live APIs. Nil means no cache
}

// Result is the outcome of one resolution.
type Result struct {
	Query      string      `json:"query"`
	Status     string      `json:"status"`
	Pass       string      `json:"pass,omitempty"`
	Ref        *Ref        `json:"ref,omitempty"`
	Score      float64     `json:"score,omitempty"`
	Candidates []Candidate `json:"candidates,omitempty"`
	Via        string      `json:"via,omitempty"` // the id that the crosswalk or the live API gave
	Version    string      `json:"version"`
}

// Candidate is one resource that the matcher scored.
type Candidate struct {
	Resource string  `json:"resource"`
	Score    float64 `json:"score"`
}

// Links makes the crosswalk table from the links of the snapshots. Each link goes both ways. A link with an id that does not normalize is left out.
func Links(ls []cluster.Link) map[string][]string {
	out := map[string][]string{}
	for _, l := range ls {
		a, errA := Key(l.A)
		b, errB := Key(l.B)
		if errA != nil || errB != nil || a == b {
			continue
		}
		if !slices.Contains(out[a], b) {
			out[a] = append(out[a], b)
		}
		if !slices.Contains(out[b], a) {
			out[b] = append(out[b], a)
		}
	}
	return out
}

// ID resolves one id.
func (r *Resolver) ID(ctx context.Context, s string) (Result, error) {
	key, err := Key(s)
	if err != nil {
		return Result{}, err
	}
	res := Result{Query: key, Status: NotFound, Version: Version}
	if refs := r.Index.Lookup(key); len(refs) > 0 {
		return r.found(res, Local, refs, ""), nil
	}
	for _, other := range r.Crosswalks[key] {
		if refs := r.Index.Lookup(other); len(refs) > 0 {
			return r.found(res, Crosswalk, refs, other), nil
		}
	}
	sys, _, _ := strings.Cut(key, ":")
	for _, f := range r.Fetchers {
		if !slices.Contains(f.Systems(), sys) {
			continue
		}
		others, ok := r.Cache.Get(f.Name(), key)
		if !ok {
			others, err = f.Fetch(ctx, key)
			if err != nil {
				return Result{}, fmt.Errorf("resolve: %s: %w", f.Name(), err)
			}
			r.Cache.Put(f.Name(), key, others)
		}
		for _, other := range others {
			if refs := r.Index.Lookup(other); len(refs) > 0 {
				return r.found(res, Live, refs, other), nil
			}
		}
	}
	return res, nil
}

// found takes the deepest place. Two resources for one id is a defect of the catalogue, and the result gives them as candidates.
func (r *Resolver) found(res Result, pass string, refs []Ref, via string) Result {
	res.Pass, res.Via = pass, via
	resources := map[string]bool{}
	for _, f := range refs {
		resources[f.Resource] = true
	}
	if len(resources) > 1 {
		res.Status = Candidates
		for _, id := range slices.Sorted(func(yield func(string) bool) {
			for k := range resources {
				if !yield(k) {
					return
				}
			}
		}) {
			res.Candidates = append(res.Candidates, Candidate{Resource: id, Score: 1})
		}
		return res
	}
	best := refs[0]
	for _, f := range refs[1:] {
		if len(f.At()) > len(best.At()) {
			best = f
		}
	}
	res.Status, res.Ref, res.Score = Found, &best, 1
	return res
}
