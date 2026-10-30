package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/queue"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cites"
	resresolve "github.com/tamnd/math2-agent/resource/resolve"
	resscope "github.com/tamnd/math2-agent/resource/scope"
)

// citeSources are the sources of references that m2 res resolve -source reads.
var citeSources = map[string]bool{"crossref": true, "arxiv": true}

// citeRun matches the references of the citing expressions of one run and keeps the edges and the weak matches.
type citeRun struct {
	m      *cites.Matcher
	done   map[string]bool
	edges  []resource.Cite
	weak   []resresolve.Result
	works  int
	found  int
	source string
}

func newCiteRun(root, source string, r *resresolve.Resolver) (*citeRun, error) {
	ds, err := resscope.ReadDecisions(root)
	if err != nil {
		return nil, err
	}
	m := &cites.Matcher{Resolver: r, Decisions: map[string]string{}}
	for _, d := range ds {
		if d.Kind == "resolve" && strings.Contains(d.ID, "#") {
			m.Decisions[d.ID] = d.Target
		}
	}
	return &citeRun{m: m, done: map[string]bool{}, source: source}, nil
}

// add matches the references of one citing expression.
func (c *citeRun) add(ctx context.Context, from string, cs []resresolve.Citation, keepText bool) error {
	es, w, err := c.m.Edges(ctx, from, cs, keepText)
	if err != nil {
		return err
	}
	c.works++
	c.done[from] = true
	for _, e := range es {
		if e.To != "" {
			c.found++
		}
	}
	c.edges, c.weak = append(c.edges, es...), append(c.weak, w...)
	return nil
}

// write writes the edges and the queue items of the run. The edges of the other citing expressions stay.
func (c *citeRun) write(root string, stderr io.Writer) error {
	old, err := resource.ReadCites(root)
	if err != nil {
		return err
	}
	if err := resource.WriteCites(root, cites.Merge(old, c.edges, c.done)); err != nil {
		return err
	}
	if err := mergeResolveQueue(root, c.weak, c.done); err != nil {
		return err
	}
	un, err := cites.ReadUnresolved(root)
	if err != nil {
		return err
	}
	if err := cites.WriteUnresolved(root, cites.CountRuns(un, c.edges, c.done, time.Now().UTC().Format(time.DateOnly))); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "%s: %d works, %d references, %d matched, %d with candidates\n", c.source, c.works, len(c.edges), c.found, len(c.weak))
	return nil
}

// inScope reports whether the references of a resource are read: a live resource with the scope in.
func inScope(r *resource.Resource) bool {
	return r != nil && r.Live() && r.Scope != nil && r.Scope.Value == resource.In
}

// resolveCrossref reads the open references of Crossref for each DOI of each live resource in scope, matches them, and writes the cites edges (RS3.7, RS6.4). The references with candidates replace the items of the same citing expressions in the queue res-resolve.
func resolveCrossref(ctx context.Context, root string, rs []resource.Resource, r *resresolve.Resolver, client *fetch.Client, base string, limit int, stderr io.Writer) error {
	run, err := newCiteRun(root, "crossref", r)
	if err != nil {
		return err
	}
	for i := range rs {
		res := &rs[i]
		if !inScope(res) {
			continue
		}
		for _, doi := range resDOIs(res) {
			from := cites.Citing(res, "doi", doi)
			if from == "" || run.done[from] || (limit > 0 && run.works >= limit) {
				continue
			}
			var body struct {
				Message struct {
					Reference []cites.CrossrefRef `json:"reference"`
				} `json:"message"`
			}
			err := client.JSON(ctx, base+"/works/"+url.PathEscape(doi), &body)
			if errors.Is(err, fetch.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			var cs []resresolve.Citation
			for _, ref := range body.Message.Reference {
				cs = append(cs, cites.FromCrossref(ref))
			}
			if err := run.add(ctx, from, cs, cites.Open(res, from)); err != nil {
				return err
			}
		}
	}
	return run.write(root, stderr)
}

// resolveArxiv reads the bibliography entries of the analysis plane of a checkout of tamnd/arxiv, the latest version of each paper, and writes the cites edges of each paper that is a live resource in scope (RS6.4). The entries hold no text, so the edges hold the year and the ids only.
func resolveArxiv(ctx context.Context, root, checkout string, r *resresolve.Resolver, limit int, stderr io.Writer) error {
	run, err := newCiteRun(root, "arxiv", r)
	if err != nil {
		return err
	}
	files, err := mdrec.Glob(filepath.Join(checkout, "analysis", "bib", "*", "*"+mdrec.Ext))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("%s has no bibliography entries in analysis/bib", checkout)
	}
	for _, path := range files {
		if limit > 0 && run.works >= limit {
			break
		}
		recs, err := mdrec.Read(path)
		if err != nil {
			return err
		}
		bs, err := mdrec.Unmarshal[axbib](recs)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if len(bs) == 0 {
			continue
		}
		latest := 0
		for _, b := range bs {
			latest = max(latest, b.Version)
		}
		paper := bs[0].Paper
		res, err := r.ID(ctx, "arxiv:"+paper)
		if err != nil {
			return err
		}
		if res.Status != resresolve.Found {
			continue
		}
		rec := r.Index.Resource(res.Ref.Resource)
		if !inScope(rec) {
			continue
		}
		id := fmt.Sprintf("%sv%d", paper, latest)
		if exprOf(rec, "arxiv", id) == "" {
			id = paper // the catalogue has no expression of the version
		}
		from := cites.Citing(rec, "arxiv", id)
		if from == "" || run.done[from] {
			continue
		}
		var cs []resresolve.Citation
		for _, b := range bs {
			if b.Version == latest {
				cs = append(cs, cites.FromArxiv(b.Year, b.DOI, b.ArXiv))
			}
		}
		if err := run.add(ctx, from, cs, false); err != nil {
			return err
		}
	}
	return run.write(root, stderr)
}

// axbib is one bibliography entry of the analysis plane of tamnd/arxiv.
type axbib struct {
	Paper   string `json:"paper"`
	Version int    `json:"version"`
	ArXiv   string `json:"arxiv"`
	DOI     string `json:"doi"`
	Year    int    `json:"year"`
}

// exprOf gives the key of the expression that has the id, itself or in a manifestation, or an empty string.
func exprOf(r *resource.Resource, key, value string) string {
	for _, e := range r.Expressions {
		if slices.Contains(e.IDs[key], value) || slices.ContainsFunc(e.Manifestations, func(m resource.Manifestation) bool { return slices.Contains(m.IDs[key], value) }) {
			return e.Key
		}
	}
	return ""
}

// resDOIs gives the DOIs of a resource at each level, each once.
func resDOIs(r *resource.Resource) []string {
	var out []string
	add := func(vs []string) {
		for _, v := range vs {
			if !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	for _, e := range r.Expressions {
		add(e.IDs["doi"])
		for _, m := range e.Manifestations {
			add(m.IDs["doi"])
		}
	}
	add(r.IDs["doi"])
	return out
}

// mergeResolveQueue writes the references with candidates to the queue res-resolve. The items of the citing expressions in done go away first, and the other items stay.
func mergeResolveQueue(root string, weak []resresolve.Result, done map[string]bool) error {
	path := filepath.Join(root, ResResolveQueue)
	items, err := readResQueue(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var buf bytes.Buffer
	write := func(it queue.Item) error {
		line, err := json.Marshal(it)
		if err != nil {
			return err
		}
		buf.Write(append(line, '\n'))
		return nil
	}
	for _, it := range items {
		if from, _, ok := strings.Cut(it.Target, "#"); ok && done[from] {
			continue
		}
		if err := write(it); err != nil {
			return err
		}
	}
	for _, res := range weak {
		if err := write(queue.Item{Target: res.Query, Kind: res.Pass, Goal: "the reference has one resource or is not found", Priority: candidateScore(res)}); err != nil {
			return err
		}
	}
	if buf.Len() == 0 {
		return nil
	}
	return writeOut(path, buf.Bytes())
}
