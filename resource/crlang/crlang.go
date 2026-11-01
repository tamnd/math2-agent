// Package crlang builds the source records of the language that Crossref gives for a DOI (spec corpus resources RS2.3 and RS8.3). The works route of Crossref has no select for the language, so the snapshots of the series lists do not have it, and many books of the series have no expression with a language.
//
// A run asks the works route of Crossref for each DOI of a resource in scope with no expression with a language. A work with a language tag gives one record with the DOI, so the clustering puts the record on that resource. The source is a filler of package cluster: it gives the language only when no catalogue gives one. The REST API of Crossref needs no key.
package crlang

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/crossref"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// Source is the id of the source in manifests/sources.yaml.
const Source = "crossref_language"

// Version is the version of the adapter.
const Version = "crossref_language/1"

// WorksURL is the works route of the REST API of Crossref.
const WorksURL = crossref.Base + "/works/"

// Work is the message of one work of Crossref, with the fields that the source uses.
type Work struct {
	DOI      string `json:"DOI"`
	Language string `json:"language"`
}

// Lookup gives the work of one DOI. A DOI that Crossref does not know gives nil and no error.
type Lookup func(ctx context.Context, doi string) (*Work, error)

// URL gives the URL of the work of the DOI at base.
func URL(base, doi string) string {
	return base + url.PathEscape(doi)
}

// Counts are the counts of one run.
type Counts struct {
	Asked int // the DOIs that the run asked for
	Found int // the DOIs with a language
}

var tag = regexp.MustCompile(`^[a-z]{2,3}$`)

// needs reports whether the resource is live, in scope and has no expression with a language. A resource that a record of the source is on also needs it, because the records of a run replace all the records of the last run.
func needs(r *resource.Resource) bool {
	if !r.Live() || r.Scope == nil || r.Scope.Value != resource.In {
		return false
	}
	return r.Holds(Source) || !slices.ContainsFunc(r.Expressions, func(e resource.Expression) bool { return e.Language != "" })
}

// doi gives the first DOI of the resource or of one of its expressions, or "".
func doi(r *resource.Resource) string {
	if v := r.IDs["doi"]; len(v) > 0 {
		return v[0]
	}
	for _, e := range r.Expressions {
		if v := e.IDs["doi"]; len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// Build asks the lookup for the DOI of each resource that needs a language, and gives one record for each DOI with a valid language tag.
func Build(ctx context.Context, rs []resource.Resource, lookup Lookup) ([]cluster.Record, Counts, error) {
	var c Counts
	var out []cluster.Record
	seen := map[string]bool{}
	for i := range rs {
		r := &rs[i]
		d := doi(r)
		if d == "" || !needs(r) || seen[d] {
			continue
		}
		seen[d] = true
		c.Asked++
		w, err := lookup(ctx, d)
		if err != nil {
			return nil, c, err
		}
		if w == nil {
			continue
		}
		l := strings.ToLower(strings.TrimSpace(w.Language))
		if !tag.MatchString(l) {
			continue
		}
		c.Found++
		out = append(out, cluster.Record{Source: Source, NativeID: d, Level: "work", Kind: r.Kind, Title: r.Title, Language: l, IDs: resource.IDs{"doi": {d}}})
	}
	return out, c, nil
}

// Entry gives the entry of the source in manifests/sources.yaml.
func Entry() ingest.Source {
	return ingest.Source{
		ID:          Source,
		Name:        "Crossref, the language of the works of the DOIs",
		Kind:        "catalogue",
		Home:        "https://www.crossref.org",
		Licence:     "LicenseRef-facts",
		Class:       ingest.RecordOnly,
		Channel:     "catalogue",
		Adapter:     Version,
		Attribution: "Crossref, https://www.crossref.org",
	}
}

// Snapshot gives the snapshot of the records of one run: the hash of the records and the day.
func Snapshot(recs []cluster.Record, day string) (ingest.Snapshot, error) {
	h := blake3.New()
	enc := json.NewEncoder(h)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			return ingest.Snapshot{}, err
		}
	}
	return ingest.Snapshot{URL: "m2 res import " + Source, Hash: "blake3:" + hex.EncodeToString(h.Sum(nil)), Date: day}, nil
}
