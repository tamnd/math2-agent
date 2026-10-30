// Package sourceworks builds the source records of the sources of math2 that are also works (spec corpus resources RS7.5 and RS7.6, phase R5). A reference work or a wiki of corpus K5.3 is a resource of kind website, a database is a resource of kind dataset, and a formal library of K06 is a resource of kind software.
//
// The list of the sources is the file sites.yaml in this package. It gives the kind and the Wikidata item of each source. The name and the home page come from the entry of manifests/sources.yaml, then from the adapter of the source, and last from sites.yaml. A source of the catalogue that is not in sites.yaml, and that has the kind wiki, reference, database or library, also gets a record.
//
// The record of a source has the id source, with the source id, and the id wikidata when the item is known. It has no structure: the items of the source are mentions and statements, not parts of the work (RS7.6). The package makes no request.
package sourceworks

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/zeebo/blake3"
	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Source is the name of the source in manifests/sources.yaml.
const Source = "source_works"

// Version is the version of the builder.
const Version = "source_works/1"

//go:embed sites.yaml
var sitesYAML []byte

// Site is one entry of sites.yaml.
type Site struct {
	ID   string `yaml:"id"`   // the source id of manifests/sources.yaml
	Kind string `yaml:"kind"` // website, dataset or software
	QID  string `yaml:"qid"`  // the Wikidata item, or "" when it is not known
	Name string `yaml:"name"`
	Home string `yaml:"home"`
}

// Kinds gives the kind of resource for a kind of source, for a source of the catalogue that is not in sites.yaml.
var Kinds = map[string]string{"wiki": "website", "reference": "website", "database": "dataset", ingest.Library: "software"}

// Sites reads sites.yaml. It checks that each entry has a source id, a kind of resource and a valid Wikidata item, and that no source id comes twice.
func Sites() ([]Site, error) {
	var out []Site
	if err := yaml.Unmarshal(sitesYAML, &out); err != nil {
		return nil, fmt.Errorf("sourceworks: sites.yaml: %w", err)
	}
	seen := map[string]bool{}
	for _, s := range out {
		if _, err := ids.Normalize("source", s.ID); err != nil {
			return nil, fmt.Errorf("sourceworks: sites.yaml: %w", err)
		}
		if seen[s.ID] {
			return nil, fmt.Errorf("sourceworks: sites.yaml: the source %s comes twice", s.ID)
		}
		seen[s.ID] = true
		if !slices.Contains(resource.Kinds, s.Kind) {
			return nil, fmt.Errorf("sourceworks: sites.yaml: %s: the kind %q is not a kind of resource", s.ID, s.Kind)
		}
		if s.QID != "" {
			if _, err := ids.Normalize("wikidata", s.QID); err != nil {
				return nil, fmt.Errorf("sourceworks: sites.yaml: %s: %w", s.ID, err)
			}
		}
	}
	return out, nil
}

// Lookup gives the entry of the adapter of a source, as ingest.Lookup does. It gives false for a source with no adapter.
type Lookup func(id string) (ingest.Source, bool)

// Adapters is the Lookup of the registered adapters.
func Adapters(id string) (ingest.Source, bool) {
	a, ok := ingest.Lookup(id)
	if !ok {
		return ingest.Source{}, false
	}
	return a.Source(), true
}

// Build gives one source record for each site, and for each source of the catalogue with a kind in Kinds that is not a site. The catalogue can be nil, and so can the lookup. The records are in the order of the sites, then in the order of the catalogue.
func Build(sites []Site, cat *ingest.Catalogue, lookup Lookup) []cluster.Record {
	var src []ingest.Source
	if cat != nil {
		src = cat.Sources
	}
	entry := func(id string) *ingest.Source {
		for i := range src {
			if src[i].ID == id {
				return &src[i]
			}
		}
		return nil
	}
	var out []cluster.Record
	done := map[string]bool{}
	for _, s := range sites {
		name, home := s.Name, s.Home
		if lookup != nil {
			if a, ok := lookup(s.ID); ok {
				name, home = first(a.Name, name), first(a.Home, home)
			}
		}
		if e := entry(s.ID); e != nil {
			name, home = first(e.Name, name), first(e.Home, home)
		}
		out = append(out, record(s.ID, s.Kind, s.QID, name, home))
		done[s.ID] = true
	}
	for _, e := range src {
		if kind := Kinds[e.Kind]; kind != "" && !done[e.ID] && e.ID != Source {
			out = append(out, record(e.ID, kind, "", e.Name, e.Home))
		}
	}
	return out
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func record(id, kind, qid, name, home string) cluster.Record {
	r := cluster.Record{Source: Source, NativeID: id, Level: "work", Kind: kind, Title: name, IDs: resource.IDs{"source": {id}}}
	if qid != "" {
		r.IDs["wikidata"] = resource.Values{qid}
	}
	if home != "" {
		r.Links = []string{home}
	}
	return r
}

// Entry gives the entry of the source in manifests/sources.yaml. The records are facts about the sources, and the Wikidata items are CC0.
func Entry() ingest.Source {
	return ingest.Source{
		ID:          Source,
		Name:        "The sources of math2 that are also works",
		Kind:        "catalogue",
		Home:        "https://github.com/tamnd/math2-agent",
		Licence:     "CC0-1.0",
		Class:       ingest.Open,
		Channel:     "catalogue",
		Adapter:     Version,
		Terms:       &ingest.Terms{Robots: "n/a, the builder reads sites.yaml and manifests/sources.yaml and makes no request", Rate: "none", AI: "none stated"},
		Attribution: "math2, from manifests/sources.yaml, with the items of Wikidata, https://www.wikidata.org, CC0 1.0",
	}
}

// Snapshot gives the snapshot of a set of records: the hash of the records, on the day.
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
