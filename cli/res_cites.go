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

	"github.com/tamnd/math2-agent/queue"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cites"
	resresolve "github.com/tamnd/math2-agent/resource/resolve"
	resscope "github.com/tamnd/math2-agent/resource/scope"
)

// citeSources are the sources of references that m2 res resolve -source reads.
var citeSources = map[string]bool{"crossref": true}

// resolveCrossref reads the open references of Crossref for each DOI of each live resource in scope, matches them, and writes the cites edges (RS3.7, RS6.4). The edges of the other citing expressions stay. The references with candidates replace the items of the same citing expressions in the queue res-resolve.
func resolveCrossref(ctx context.Context, root string, rs []resource.Resource, r *resresolve.Resolver, client *fetch.Client, base string, limit int, stderr io.Writer) error {
	ds, err := resscope.ReadDecisions(root)
	if err != nil {
		return err
	}
	m := &cites.Matcher{Resolver: r, Decisions: map[string]string{}}
	for _, d := range ds {
		if d.Kind == "resolve" && strings.Contains(d.ID, "#") {
			m.Decisions[d.ID] = d.Target
		}
	}
	old, err := resource.ReadCites(root)
	if err != nil {
		return err
	}
	done := map[string]bool{}
	var edges []resource.Cite
	var weak []resresolve.Result
	works, found := 0, 0
	for i := range rs {
		res := &rs[i]
		if !res.Live() || res.Scope == nil || res.Scope.Value != resource.In {
			continue
		}
		for _, doi := range resDOIs(res) {
			from := cites.Citing(res, "doi", doi)
			if from == "" || done[from] {
				continue
			}
			if limit > 0 && works >= limit {
				break
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
			works++
			done[from] = true
			var cs []resresolve.Citation
			for _, ref := range body.Message.Reference {
				cs = append(cs, cites.FromCrossref(ref))
			}
			es, w, err := m.Edges(ctx, from, cs, cites.Open(res, from))
			if err != nil {
				return err
			}
			for _, e := range es {
				if e.To != "" {
					found++
				}
			}
			edges, weak = append(edges, es...), append(weak, w...)
		}
	}
	if err := resource.WriteCites(root, cites.Merge(old, edges, done)); err != nil {
		return err
	}
	if err := mergeResolveQueue(root, weak, done); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "crossref: %d works, %d references, %d matched, %d with candidates\n", works, len(edges), found, len(weak))
	return nil
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
