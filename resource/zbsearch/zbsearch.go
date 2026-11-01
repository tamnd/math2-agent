// Package zbsearch builds the source records of the search of zbMATH Open (spec corpus resources RS1.5 and RS8.3). The source gives the MSC codes and the language to the resources in scope that do not have them, so that they can be at depth D1.
//
// A run asks the REST interface of zbMATH Open for the title, the family name of the first creator and the year of each resource in scope with no MSC code or no language. A document matches when its title of the work, its year and the family name of one of its authors are those of the resource. Each record has the zbMATH ids of the document and the DOI, the ISBN or the arXiv id of the resource, so the clustering puts the record on that resource. The REST interface needs no key.
package zbsearch

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/zbmath"
	"github.com/tamnd/math2-agent/msc"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Source is the id of the source in manifests/sources.yaml.
const Source = "zbmath_search"

// Version is the version of the adapter.
const Version = "zbmath_search/1"

// SearchURL is the search of documents of the REST interface of zbMATH Open.
const SearchURL = zbmath.API + "document/_search"

// MaxWords is the most words of the title in a query. A long title with many words gives no match when one word differs.
const MaxWords = 8

// Doc is one document of a search of zbMATH Open, with the fields that the source uses.
type Doc struct {
	ID         int    `json:"id"`
	Identifier string `json:"identifier"`
	Title      struct {
		Title string `json:"title"`
	} `json:"title"`
	Year     string `json:"year"`
	Language struct {
		Languages []string `json:"languages"`
	} `json:"language"`
	MSC []struct {
		Code   string `json:"code"`
		Scheme string `json:"scheme"`
	} `json:"msc"`
	Contributors struct {
		Authors []struct {
			Name string `json:"name"`
		} `json:"authors"`
		Editors []struct {
			Name string `json:"name"`
		} `json:"editors"`
	} `json:"contributors"`
}

// Search gives the documents of one query. A query with no document gives none and no error.
type Search func(ctx context.Context, query string) ([]Doc, error)

// Query gives the URL of the search at base for the query, with at most five documents.
func Query(base, query string) string {
	return base + "?" + url.Values{"search_string": {query}, "page": {"0"}, "results_per_page": {"5"}}.Encode()
}

// Counts are the counts of one run.
type Counts struct {
	Asked int // the resources that the run asked for
	Found int // the resources with a matching document
}

var word = regexp.MustCompile(`[\pL\pN]+`)

// query gives the query of zbMATH Open for the resource, or "" when the resource has no title or no year.
func query(r *resource.Resource) string {
	words := word.FindAllString(cluster.WorkTitle(r.Title), -1)
	year := year(r)
	if len(words) == 0 || year == 0 {
		return ""
	}
	q := "ti:" + strings.Join(words[:min(len(words), MaxWords)], " ") + " & py:" + strconv.Itoa(year)
	if len(r.Creators) > 0 {
		if f := word.FindAllString(cluster.Family(r.Creators[0].Name), -1); len(f) > 0 {
			q += " & au:" + strings.Join(f, " ")
		}
	}
	return q
}

// year gives the year of the date of the resource, or 0.
func year(r *resource.Resource) int {
	if len(r.Date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(r.Date[:4])
	if err != nil {
		return 0
	}
	return y
}

// join gives the key and the value of the id that puts the record on the resource: the DOI, else the ISBN, else the arXiv id.
func join(r *resource.Resource) (key, value string) {
	for _, k := range []string{"doi", "isbn", "arxiv"} {
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

// needs reports whether the resource is live, in scope and has no MSC code or no expression with a language. A resource that a record of the source is on also needs it, because the records of a run replace all the records of the last run.
func needs(r *resource.Resource) bool {
	if !r.Live() || r.Scope == nil || r.Scope.Value != resource.In {
		return false
	}
	return r.Holds(Source) || len(r.MSC) == 0 || !slices.ContainsFunc(r.Expressions, func(e resource.Expression) bool { return e.Language != "" })
}

// matches reports whether the document is the work of the resource: the same title of the work, the same year and, when the resource has creators, the family name of one creator among the authors and the editors.
func matches(d *Doc, r *resource.Resource) bool {
	if cluster.WorkTitle(d.Title.Title) != cluster.WorkTitle(r.Title) || d.Year != strconv.Itoa(year(r)) {
		return false
	}
	if len(r.Creators) == 0 {
		return true
	}
	var names []string
	for _, a := range d.Contributors.Authors {
		names = append(names, a.Name)
	}
	for _, e := range d.Contributors.Editors {
		names = append(names, e.Name)
	}
	for _, c := range r.Creators {
		f := cluster.Family(c.Name)
		if f != "" && slices.ContainsFunc(names, func(n string) bool { return cluster.Family(n) == f }) {
			return true
		}
	}
	return false
}

// Build asks the search for each resource that needs it and gives one record for each resource with a matching document that has an MSC code or a language. The first matching document wins.
func Build(ctx context.Context, rs []resource.Resource, search Search) ([]cluster.Record, Counts, error) {
	var c Counts
	var out []cluster.Record
	seen := map[string]bool{}
	for i := range rs {
		r := &rs[i]
		if !needs(r) {
			continue
		}
		key, value := join(r)
		q := query(r)
		if key == "" || q == "" {
			continue
		}
		c.Asked++
		docs, err := search(ctx, q)
		if err != nil {
			return nil, c, err
		}
		for j := range docs {
			d := &docs[j]
			if !matches(d, r) {
				continue
			}
			rec, ok := record(d, r)
			if !ok {
				break
			}
			c.Found++
			rec.IDs[key] = resource.Values{value}
			if !seen[rec.NativeID] {
				seen[rec.NativeID] = true
				out = append(out, rec)
			}
			break
		}
	}
	return out, c, nil
}

// record makes the record of the document for the resource. It has the creators only when the resource has none. It gives false when the document has no valid MSC code and no known language.
func record(d *Doc, r *resource.Resource) (cluster.Record, bool) {
	id := strconv.Itoa(d.ID)
	rec := cluster.Record{Source: Source, NativeID: id, Level: "work", Kind: r.Kind, Title: strings.Join(strings.Fields(d.Title.Title), " "), Date: d.Year, IDs: resource.IDs{"zbl_id": {id}}, Links: []string{"https://zbmath.org/?q=an:" + d.Identifier}}
	if v, err := ids.Normalize("zbl", d.Identifier); err == nil {
		rec.IDs["zbl"] = resource.Values{v}
	}
	for _, m := range d.MSC {
		if (m.Scheme == "" || m.Scheme == "msc2020") && msc.Valid(m.Code) && !slices.Contains(rec.MSC, m.Code) {
			rec.MSC = append(rec.MSC, m.Code)
		}
	}
	for _, l := range d.Language.Languages {
		if t := zbmath.Language(l); t != "" {
			rec.Language = t
			break
		}
	}
	if len(rec.MSC) == 0 && rec.Language == "" {
		return cluster.Record{}, false
	}
	if len(r.Creators) == 0 {
		for _, a := range d.Contributors.Authors {
			rec.Creators = append(rec.Creators, resource.Creator{Name: a.Name, Role: "author"})
		}
		for _, e := range d.Contributors.Editors {
			rec.Creators = append(rec.Creators, resource.Creator{Name: e.Name, Role: "editor"})
		}
	}
	return rec, true
}

// Entry gives the entry of the source in manifests/sources.yaml.
func Entry() ingest.Source {
	return ingest.Source{
		ID:          Source,
		Name:        "zbMATH Open, the search of documents by title, author and year",
		Kind:        "catalogue",
		Home:        "https://zbmath.org",
		Licence:     "CC-BY-SA-4.0",
		Class:       ingest.RecordOnly,
		Channel:     "catalogue",
		Adapter:     Version,
		Attribution: "zbMATH Open, FIZ Karlsruhe, https://zbmath.org, CC BY-SA 4.0",
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
