package resolve

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource/census/fetch"
)

// CacheFile holds the answers of the live APIs with their dates. The answers become part of the next snapshot (RS3.6 pass 3).
const CacheFile = "build/resources/resolve-cache.md"

// Answer is one answer of a live API.
type Answer struct {
	API string   `json:"api"`
	ID  string   `json:"id"`
	IDs []string `json:"ids"`
	At  string   `json:"at"` // the date of the answer, YYYY-MM-DD
}

// Cache holds the answers of the live APIs. A nil cache keeps nothing.
type Cache struct {
	Day     string
	answers map[string]Answer
	dirty   bool
}

// ReadCache reads the cache file at path. A missing file gives an empty cache.
func ReadCache(path, day string) (*Cache, error) {
	c := &Cache{Day: day, answers: map[string]Answer{}}
	recs, err := mdrec.Read(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	as, err := mdrec.Unmarshal[Answer](recs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, a := range as {
		c.answers[a.API+" "+a.ID] = a
	}
	return c, nil
}

// Get gives the cached answer of the API for the id.
func (c *Cache) Get(api, id string) ([]string, bool) {
	if c == nil {
		return nil, false
	}
	a, ok := c.answers[api+" "+id]
	return a.IDs, ok
}

// Put keeps an answer.
func (c *Cache) Put(api, id string, others []string) {
	if c == nil {
		return
	}
	c.answers[api+" "+id] = Answer{API: api, ID: id, IDs: others, At: c.Day}
	c.dirty = true
}

// Write writes the cache to path, in the order of the API and the id, when it has a new answer.
func (c *Cache) Write(path string) error {
	if c == nil || !c.dirty {
		return nil
	}
	keys := make([]string, 0, len(c.answers))
	for k := range c.answers {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	as := make([]Answer, 0, len(keys))
	for _, k := range keys {
		as = append(as, c.answers[k])
	}
	recs, err := mdrec.Marshal(as)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return mdrec.Write(path, recs)
}

// CrossrefBase is the REST API of Crossref.
const CrossrefBase = "https://api.crossref.org"

// Crossref asks the REST API of Crossref for a DOI. It gives the ISBNs of the work, the preprint that the relations name, and the DOIs of the other versions.
type Crossref struct {
	Base   string
	Client *fetch.Client
}

// Name gives the name of the API.
func (Crossref) Name() string { return "crossref" }

// Systems gives the systems that the API takes.
func (Crossref) Systems() []string { return []string{"doi"} }

// Fetch asks for one DOI in the form doi:10.1000/x.
func (f Crossref) Fetch(ctx context.Context, id string) ([]string, error) {
	doi := strings.TrimPrefix(id, "doi:")
	var body struct {
		Message struct {
			ISBN     []string `json:"ISBN"`
			Relation map[string][]struct {
				IDType string `json:"id-type"`
				ID     string `json:"id"`
			} `json:"relation"`
		} `json:"message"`
	}
	err := f.Client.JSON(ctx, f.Base+"/works/"+url.PathEscape(doi), &body)
	if errors.Is(err, fetch.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	add := func(s string) {
		if k, err := Key(s); err == nil && k != id && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	for _, v := range body.Message.ISBN {
		add("isbn:" + v)
	}
	for _, kind := range []string{"has-preprint", "is-preprint-of", "has-version", "is-version-of", "is-identical-to"} {
		for _, r := range body.Message.Relation[kind] {
			switch r.IDType {
			case "doi":
				if a, ok := strings.CutPrefix(strings.ToLower(r.ID), "10.48550/arxiv."); ok {
					add("arxiv:" + a)
				} else {
					add("doi:" + r.ID)
				}
			case "arxiv":
				add("arxiv:" + r.ID)
			}
		}
	}
	return out, nil
}
