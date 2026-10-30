// Package refs builds the source refs of the catalogue (spec corpus resources RS8.3 and RS11.5). A reference that resolves to no resource becomes a citation-only record when it is confirmed: two or more resources in scope cite it with consistent fields, or its reference holds an id that a live lookup confirms, or a person confirms it. The native id of a record names the rule that confirmed it, so that rule H05 can count the record.
package refs

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
	"github.com/zeebo/blake3"
)

// Source is the id of the source in manifests/sources.yaml.
const Source = "refs"

// The prefixes of the native ids: one for each rule of confirmation of RS11.5.
const (
	Cited  = "cited:"  // two or more resources in scope cite the work with consistent fields
	ByID   = "id:"     // the reference holds an id that a live lookup confirms
	Person = "person:" // a person confirms the reference
)

// ConfirmKind is the kind of a decision of a person that confirms a reference. The id of the decision is the reference, as <id>/e<n>#<n>.
const ConfirmKind = "confirm"

// Confirmed reports whether a native id of the source refs names a rule of confirmation.
func Confirmed(nativeID string) bool {
	return strings.HasPrefix(nativeID, Cited) || strings.HasPrefix(nativeID, ByID) || strings.HasPrefix(nativeID, Person)
}

// LookupKeys are the id systems that a reference can hold and that a live lookup can confirm. The order is the order of preference.
var LookupKeys = []string{"doi", "arxiv", "isbn", "zbl", "mr"}

// Lookup confirms an id with a live source. It gives false when the source does not have the id.
type Lookup func(ctx context.Context, key, value string) (bool, error)

// Input is what Build reads.
type Input struct {
	Resources []resource.Resource
	Cites     []resource.Cite
	Confirmed map[string]bool // the references that a person confirmed
	Lookup    Lookup          // nil means no live lookup
}

// Counts are the outcomes of one build.
type Counts struct {
	Unresolved int // the parsed references with no target from resources in scope
	Cited      int // the records of rule 1
	ByID       int // the records of rule 2
	Person     int // the records of rule 3
	Lookups    int // the ids that the build looked up
}

type ref struct {
	edge   resource.Cite
	citing string // the citing resource
	year   int
	key    string // the folded title and the family of the first creator
}

// Build gives the confirmed citation-only records, one for each rule that confirms a work. The cluster passes merge the records of one work. A reference counts only when its citing resource is live and in scope, it has no target, and it has parsed fields.
func Build(ctx context.Context, in Input) ([]cluster.Record, Counts, error) {
	inScope := map[string]bool{}
	for i := range in.Resources {
		r := &in.Resources[i]
		inScope[r.ID] = r.Live() && r.Scope != nil && r.Scope.Value == resource.In
	}
	var c Counts
	var refs []ref
	for _, e := range in.Cites {
		id, _, _ := strings.Cut(e.From, "/")
		if e.To != "" || e.Parsed == nil || !inScope[id] {
			continue
		}
		c.Unresolved++
		y, _ := strconv.Atoi(e.Parsed.Year)
		k := ""
		if t := cluster.WorkTitle(e.Parsed.Title); t != "" && len(e.Parsed.Authors) > 0 {
			if f := cluster.Family(e.Parsed.Authors[0]); f != "" {
				k = t + "|" + f
			}
		}
		refs = append(refs, ref{edge: e, citing: id, year: y, key: k})
	}
	var out []cluster.Record
	// Rule 3: the decisions of people.
	for _, r := range refs {
		if t := target(r.edge); in.Confirmed[t] {
			out = append(out, record(Person+t, r.edge))
			c.Person++
		}
	}
	// Rule 1: two or more citing resources, with one title and one first creator, and years within one.
	groups := map[string][]ref{}
	for _, r := range refs {
		if r.key != "" && r.year > 0 {
			groups[r.key] = append(groups[r.key], r)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(groups)) {
		g := groups[k]
		slices.SortStableFunc(g, func(a, b ref) int { return cmp.Compare(a.year, b.year) })
		for i := 0; i < len(g); {
			j := i
			citing := map[string]bool{}
			for j < len(g) && g[j].year-g[i].year <= 1 {
				citing[g[j].citing] = true
				j++
			}
			if len(citing) >= 2 {
				h := sha256.Sum256([]byte(k + "|" + strconv.Itoa(g[i].year)))
				rec := record(Cited+hex.EncodeToString(h[:8]), g[i].edge)
				rec.Date = strconv.Itoa(mode(g[i:j]))
				out = append(out, rec)
				c.Cited++
			}
			i = j
		}
	}
	// Rule 2: an id that passes its check and that a live lookup confirms.
	if in.Lookup != nil {
		seen := map[string]bool{}
		for _, r := range refs {
			key, v := firstID(r.edge.Parsed)
			if key == "" || seen[key+":"+v] {
				continue
			}
			seen[key+":"+v] = true
			c.Lookups++
			ok, err := in.Lookup(ctx, key, v)
			if err != nil {
				return nil, c, err
			}
			if ok {
				out = append(out, record(ByID+key+":"+v, r.edge))
				c.ByID++
			}
		}
	}
	return out, c, nil
}

func target(e resource.Cite) string { return e.From + "#" + strconv.Itoa(e.N) }

// firstID gives the first id of LookupKeys that the reference holds and that passes its check, in its normal form.
func firstID(p *resource.Parsed) (key, value string) {
	for _, k := range LookupKeys {
		for _, v := range p.IDs[k] {
			n, err := ids.Normalize(k, v)
			if err == nil && ids.Check(k, n) == nil {
				return k, n
			}
		}
	}
	return "", ""
}

// mode gives the year that most references of the group give, and the earliest one on a tie.
func mode(g []ref) int {
	n := map[int]int{}
	best := g[0].year
	for _, r := range g {
		n[r.year]++
		if n[r.year] > n[best] || (n[r.year] == n[best] && r.year < best) {
			best = r.year
		}
	}
	return best
}

// record gives the source record of a reference. A reference with a venue or a volume is an article, and another reference is a monograph.
func record(nativeID string, e resource.Cite) cluster.Record {
	p := e.Parsed
	rec := cluster.Record{Source: Source, NativeID: nativeID, Level: "work", Kind: "monograph", Title: p.Title, Date: p.Year, Volume: p.Volume}
	if p.Venue != "" || p.Volume != "" {
		rec.Kind = "article"
	}
	for _, a := range p.Authors {
		rec.Creators = append(rec.Creators, resource.Creator{Name: a, Role: "author"})
	}
	for k, vs := range p.IDs {
		for _, v := range vs {
			if n, err := ids.Normalize(k, v); err == nil && ids.Check(k, n) == nil {
				if rec.IDs == nil {
					rec.IDs = resource.IDs{}
				}
				rec.IDs[k] = append(rec.IDs[k], n)
			}
		}
	}
	if pages, _, _ := strings.Cut(p.Pages, "-"); pages != "" {
		rec.Page = pages
	}
	return rec
}

// Entry is the entry of the source in manifests/sources.yaml. A person approves it there, as for each catalogue.
func Entry() ingest.Source {
	return ingest.Source{
		ID:      Source,
		Name:    "The references of the resources of the catalogue",
		Kind:    "catalogue",
		Licence: "LicenseRef-facts",
		Class:   ingest.RecordOnly,
		Channel: "catalogue",
		Adapter: Version,
	}
}

// Version names the build of the records and its version.
const Version = "refs/1"

// Snapshot gives the snapshot of a build: the blake3 hash of the records. The same records give the same snapshot.
func Snapshot(recs []cluster.Record, day string) (ingest.Snapshot, error) {
	h := blake3.New()
	enc := json.NewEncoder(h)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			return ingest.Snapshot{}, err
		}
	}
	return ingest.Snapshot{URL: "m2 res import refs", Hash: "blake3:" + hex.EncodeToString(h.Sum(nil)), Date: day}, nil
}
