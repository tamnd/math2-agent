package novelty

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/search"
)

// K is the number of matches of a search when its K is zero.
const K = 10

// Graph searches the statement plane with the search index of the build (spec 14), with each channel of the index.
type Graph struct {
	Index *search.Index
	Nodes []*node.Node // the nodes of the index, which give the text of each match to the judge
	K     int          // the number of matches. Zero means [K].
}

// Name returns graph.
func (g *Graph) Name() string { return "graph" }

// Search returns the nodes that the index finds for the name and the statement of the claim. The node of the claim and the declarations with no node are not matches.
func (g *Graph) Search(_ context.Context, c Claim) (string, []Match, error) {
	if g.Index == nil {
		return "", nil, errors.New("novelty: the graph search has no index")
	}
	q := strings.TrimSpace(c.Name + "\n" + c.Statement)
	rs, err := g.Index.Search(search.Query{Text: q, K: max(g.K, K) + 1})
	if err != nil {
		return q, nil, err
	}
	var out []Match
	for _, r := range rs {
		if r.Tag == "" || r.Tag == c.Tag || len(out) == max(g.K, K) {
			continue
		}
		m := Match{Source: g.Name(), ID: r.Tag, Score: r.Score, Title: r.Name}
		if i := slices.IndexFunc(g.Nodes, func(n *node.Node) bool { return n.ID == r.Tag }); i >= 0 {
			m.Text = align.ItemOf(g.Nodes[i]).Text
		}
		out = append(out, m)
	}
	return q, out, nil
}

// MinScore is the lowest score of a match of the record plane when the MinScore of [Records] is zero.
const MinScore = 0.25

// Records searches the records of one source of the record plane, with the name, formula and embedding channels of the aligner. The records of a problem database give their status: the status of an Erdős problem, or the category of a statement of formal-conjectures.
type Records struct {
	Source   string
	MinScore float64 // the lowest score of a match. Zero means [MinScore].
	K        int     // the number of matches. Zero means [K].

	index *align.Index
}

// NewRecords returns the search of the records of the source. A nil Embedder means [align.Hash].
func NewRecords(source string, recs []ingest.Record, e align.Embedder) (*Records, error) {
	ix, err := align.NewIndex(source, recs, e)
	if err != nil {
		return nil, fmt.Errorf("novelty: the records of %s: %w", source, err)
	}
	return &Records{Source: source, index: ix}, nil
}

// RecordPlane returns the search of each source of the catalogue of the tree at root that has records in build/records. A nil Embedder means [align.Hash].
func RecordPlane(root string, e align.Embedder) ([]Searcher, error) {
	cat, err := ingest.ReadCatalogue(root)
	if err != nil {
		return nil, fmt.Errorf("novelty: %w", err)
	}
	var out []Searcher
	for _, s := range cat.Sources {
		recs, _, err := ingest.ReadRecords(root, s.ID)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("novelty: %s: %w", filepath.ToSlash(filepath.Join(ingest.RecordsDir, s.ID+".jsonl")), err)
		}
		r, err := NewRecords(s.ID, recs, e)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Name returns records: and the source, for example records:oeis.
func (r *Records) Name() string { return "records:" + r.Source }

// Search returns the records whose score with the claim is at least the MinScore.
func (r *Records) Search(_ context.Context, c Claim) (string, []Match, error) {
	if r.index == nil {
		return "", nil, errors.New("novelty: make the search of a source with NewRecords")
	}
	q := strings.TrimSpace(c.Name + "\n" + c.Statement)
	it := align.Item{Ref: c.Tag, Text: c.Statement}
	if c.Name != "" {
		it.Names = []string{c.Name}
	}
	var out []Match
	for _, cand := range r.index.Match(it, max(r.K, K)) {
		if cand.Score < max(r.MinScore, MinScore) {
			continue
		}
		rec := cand.Record
		out = append(out, Match{
			Source: r.Name(),
			ID:     rec.ID,
			URL:    rec.URL,
			Score:  cand.Score,
			Status: cmp.Or(rec.Meta["status"], rec.Meta["category"]),
			Title:  rec.Title,
			Text:   rec.Text,
		})
	}
	return q, out, nil
}
