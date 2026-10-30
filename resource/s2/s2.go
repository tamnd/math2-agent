// Package s2 builds the source records of Semantic Scholar (spec corpus resources RS3.2 and RS8.3, phase R3). The source gives the Semantic Scholar corpus id, the key s2, to the resources that have a DOI or an arXiv id. Each record has the corpus id and the DOI or the arXiv id of the paper, so the clustering puts the corpus id on the resource that has the same DOI or arXiv id.
//
// The spec names the Datasets API, which needs a key. math2 does not hold a key, so the source uses the batch endpoint of the public Graph API, which needs none and shares one rate limit among all users. A run asks only for the resources that have no corpus id yet.
package s2

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Source is the name of the source in manifests/sources.yaml.
const Source = "s2"

// Version is the version of the builder.
const Version = "s2/1"

// BatchURL is the batch endpoint of the Graph API, with the fields that the source reads.
const BatchURL = "https://api.semanticscholar.org/graph/v1/paper/batch?fields=corpusId,externalIds,title,year"

// BatchSize is the most ids of one request of the batch endpoint.
const BatchSize = 500

// Paper is one paper of an answer of the batch endpoint.
type Paper struct {
	CorpusID    int            `json:"corpusId"`
	Title       string         `json:"title"`
	Year        int            `json:"year"`
	ExternalIDs map[string]any `json:"externalIds"`
}

// Lookup asks the batch endpoint for the papers of the ids, as DOI:10.1/x or ARXIV:2101.00001. The answer has one entry for each id in the same order, and nil for an id that Semantic Scholar does not have.
type Lookup func(ctx context.Context, ids []string) ([]*Paper, error)

// Batch makes the Lookup of the batch endpoint at url with post, which posts a JSON body and gives the body of the answer.
func Batch(url string, post func(ctx context.Context, url string, body []byte) ([]byte, error)) Lookup {
	return func(ctx context.Context, ids []string) ([]*Paper, error) {
		body, err := json.Marshal(map[string][]string{"ids": ids})
		if err != nil {
			return nil, err
		}
		out, err := post(ctx, url, body)
		if err != nil {
			return nil, err
		}
		var ps []*Paper
		if err := json.Unmarshal(out, &ps); err != nil {
			return nil, fmt.Errorf("s2: %w", err)
		}
		if len(ps) != len(ids) {
			return nil, fmt.Errorf("s2: %d papers for %d ids", len(ps), len(ids))
		}
		return ps, nil
	}
}

// Counts are the counts of one run.
type Counts struct {
	Asked    int // the resources that the run asked for
	Found    int // the resources that Semantic Scholar has
	Requests int
}

// query gives the id that the run asks for: the DOI of the resource or of its first expression with one, else the arXiv id.
func query(r *resource.Resource) (key, value string) {
	for _, k := range []string{"doi", "arxiv"} {
		if v := r.IDs[k]; len(v) > 0 {
			return k, v[0]
		}
		for _, e := range r.Expressions {
			if v := e.IDs[k]; len(v) > 0 {
				return k, v[0]
			}
		}
	}
	return "", ""
}

// has reports whether the resource or one of its expressions has a corpus id.
func has(r *resource.Resource) bool {
	if len(r.IDs[Source]) > 0 {
		return true
	}
	for _, e := range r.Expressions {
		if len(e.IDs[Source]) > 0 {
			return true
		}
	}
	return false
}

// Build asks for each live in-scope resource that has a DOI or an arXiv id and no corpus id, in batches of BatchSize, and gives one source record for each paper that Semantic Scholar has. The record keeps the kind of the resource, because the Graph API does not give a kind of RS2.2.
func Build(ctx context.Context, rs []resource.Resource, lookup Lookup) ([]cluster.Record, Counts, error) {
	var c Counts
	type ask struct {
		kind, key, value string
	}
	var asks []ask
	for i := range rs {
		r := &rs[i]
		if !r.Live() || r.Scope == nil || r.Scope.Value != resource.In || has(r) {
			continue
		}
		if k, v := query(r); k != "" {
			asks = append(asks, ask{r.Kind, k, v})
		}
	}
	c.Asked = len(asks)
	var out []cluster.Record
	seen := map[string]bool{}
	for start := 0; start < len(asks); start += BatchSize {
		part := asks[start:min(start+BatchSize, len(asks))]
		q := make([]string, len(part))
		for i, a := range part {
			q[i] = strings.ToUpper(a.key) + ":" + a.value
		}
		c.Requests++
		ps, err := lookup(ctx, q)
		if err != nil {
			return nil, c, err
		}
		for i, p := range ps {
			if p == nil || p.CorpusID <= 0 {
				continue
			}
			c.Found++
			rec := record(p, part[i].kind)
			rec.IDs[part[i].key] = resource.Values{part[i].value}
			if seen[rec.NativeID] {
				continue
			}
			seen[rec.NativeID] = true
			out = append(out, rec)
		}
	}
	return out, c, nil
}

// record makes the source record of one paper. It keeps the DOI and the arXiv id of the paper in the normal form of RS3.3.
func record(p *Paper, kind string) cluster.Record {
	id := strconv.Itoa(p.CorpusID)
	rec := cluster.Record{Source: Source, NativeID: id, Level: "expression", Kind: kind, Title: strings.Join(strings.Fields(p.Title), " "), IDs: resource.IDs{Source: {id}}}
	if p.Year > 0 {
		rec.Date = strconv.Itoa(p.Year)
	}
	for key, ext := range map[string]string{"doi": "DOI", "arxiv": "ArXiv"} {
		if s, ok := p.ExternalIDs[ext].(string); ok {
			if v, err := ids.Normalize(key, s); err == nil {
				rec.IDs[key] = resource.Values{v}
			}
		}
	}
	return rec
}

// Entry gives the entry of the source in manifests/sources.yaml. The database of Semantic Scholar is under ODC-By, and the source keeps the ids only.
func Entry() ingest.Source {
	return ingest.Source{
		ID:          Source,
		Name:        "Semantic Scholar, the corpus ids of the papers",
		Kind:        "catalogue",
		Home:        "https://www.semanticscholar.org",
		Licence:     "ODC-By-1.0",
		Class:       ingest.RecordOnly,
		Channel:     "catalogue",
		Adapter:     Version,
		Attribution: "Semantic Scholar, Allen Institute for AI, https://www.semanticscholar.org, ODC-By 1.0",
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
	return ingest.Snapshot{URL: "m2 res import s2", Hash: "blake3:" + hex.EncodeToString(h.Sum(nil)), Date: day}, nil
}
