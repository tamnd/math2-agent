package cluster

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/tag"
)

// Version names the passes and the precedence tables. A change of a pass, a threshold or a table needs a new version, because the passes are deterministic for a fixed set of snapshots and a fixed version (RS10.2).
const Version = "cluster/1"

// The thresholds of the passes.
const (
	ISBNTitle = 0.8  // C3: the title similarity of two records with one ISBN
	High      = 0.9  // C4: a score at or above it joins
	Middle    = 0.75 // C4: a score at or above it and below High goes to the queue
	MaxBlock  = 200  // C4: a block with more records is not compared, and the result counts it
	Window    = 6    // RS10.4: the most years from a preprint to its publication
)

// The passes of RS10.2 that this package runs.
const (
	C1 = "C1"
	C2 = "C2"
	C3 = "C3"
	C4 = "C4"
	C5 = "C5"
	// Person is a join that a person decided in the queue res-cluster (RS10.6). It runs before C1, and no guard refuses it.
	Person = "person"
)

// Strong are the strong ids of RS3.5 that join records in pass C1. ISBN-13 is strong only with a title match, so it is in pass C3.
var Strong = []string{"doi", "arxiv", "zbl", "zbl_id", "jfm", "mr", "openlibrary_edition", "wikidata", "openalex", "hal", "course", "offering"}

// levels gives the level of a resource where each system of ids goes. A system that is not in the map goes to the expression.
var levels = map[string]string{
	"wikidata": "work", "openlibrary_work": "work", "course": "work",
	"isbn": "manifestation", "oclc": "manifestation", "lccn": "manifestation", "openlibrary_edition": "manifestation",
	"hathi": "manifestation", "hathi_record": "manifestation", "ia": "manifestation", "gbooks": "manifestation", "gutenberg": "manifestation",
}

// Precedence gives the order of the sources for each field of a resource (RS10.1). A source that is not in the list comes after the sources of the list, in the order of the names. For the title, a record in the language of the work comes first.
var Precedence = map[string][]string{
	"title":    {"bnf", "dnb", "ndl", "loc", "zbmath", "arxiv", "crossref", "openalex", "openlibrary", "hal", "wikidata"},
	"creators": {"zbmath", "arxiv", "crossref", "openalex", "loc", "bnf", "dnb", "ndl", "openlibrary", "hal", "wikidata"},
	"msc":      {"zbmath", "arxiv", "classifier"},
	"kind":     {"zbmath", "crossref", "openalex", "loc", "openlibrary", "hal", "wikidata", "arxiv"},
}

// wholes are the kinds of a volume, a journal or a series. Clustering never joins a chapter with one of them (RS10.5).
var wholes = map[string]bool{"edited-volume": true, "proceedings": true, "journal": true, "series": true, "monograph": true, "textbook": true, "handbook": true, "encyclopedia": true}

// papers are the kinds whose preprint and publication are expressions of one work (RS10.4).
var papers = map[string]bool{"article": true, "preprint": true, "conference-paper": true, "survey": true, "chapter": true}

// Link is one pair of ids that a crosswalk of a snapshot links, as doi:10.1000/x and arxiv:2301.00001 (pass C2).
type Link struct {
	A string `json:"a"`
	B string `json:"b"`
}

// Decision is a decision of a person on a pair of the queue res-cluster (RS10.6). Kind is merge or split. A and B are the keys of two records (source:native_id) or two resource ids. A merge joins the records before the passes. A split keeps the records of A and the records of B in two resources, and when they were one resource, A keeps its id and B gets a new id.
type Decision struct {
	Kind string
	A, B string
}

// Input is what the passes read.
type Input struct {
	Records    []Record
	Crosswalks []Link
	Existing   []resource.Resource // the resource records of records/resources/
	Order      map[string]int      // the line of each resource id in the register. A smaller line is older
	Decisions  []Decision          // the merge and split decisions of people
	By         string              // the build and its version, for the history
	Day        string              // the date of the run, YYYY-MM-DD
}

// Pair is a pair of records for the queue res-cluster: a pair of the middle band of C4, or a join that a guard refused.
type Pair struct {
	A      string  `json:"a"` // the keys of the records, source:native_id
	B      string  `json:"b"`
	Pass   string  `json:"pass"`
	Score  float64 `json:"score,omitempty"`
	Reason string  `json:"reason"`
}

// Result is the outcome of the passes.
type Result struct {
	Resources []resource.Resource // each resource, in the order of the id. A new resource has a provisional id
	Pairs     []Pair              // the items of the queue res-cluster
	Joins     map[string]int      // the joins of each pass
	Skipped   int                 // the blocks of C4 that had more than MaxBlock records
	Log       []Join              // each join, in the order of the passes
}

// Join is one join of two groups: the keys of the two records that the pass compared, and the pass.
type Join struct {
	A, B string
	Pass string
}

// JoinedBy gives, for each pair of record keys, the pass of the join that put the two records in one resource, or "" when the run kept them apart. The log is in the order of the passes, so it replays the log once for each pass.
func (r *Result) JoinedBy(pairs [][2]string) []string {
	out := make([]string, len(pairs))
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		p, ok := parent[x]
		if !ok || p == x {
			return x
		}
		root := find(p)
		parent[x] = root
		return root
	}
	for k := 0; k < len(r.Log); {
		pass := r.Log[k].Pass
		for ; k < len(r.Log) && r.Log[k].Pass == pass; k++ {
			if x, y := find(r.Log[k].A), find(r.Log[k].B); x != y {
				parent[y] = x
			}
		}
		for i, p := range pairs {
			if out[i] == "" && find(p[0]) == find(p[1]) {
				out[i] = pass
			}
		}
	}
	return out
}

type groups struct {
	parent []int
	kinds  []map[string]bool
	rs     []Record
	joins  map[string]int
	pairs  []Pair
	seen   map[[2]int]bool
	log    []Join
	apart  [][2]int     // the pairs of records that a person split
	lose   map[int]bool // the records of the side B of a split, which do not keep the id of the side A
}

// split reports whether a person split a record of the group a from a record of the group b.
func (g *groups) split(a, b int) bool {
	for _, p := range g.apart {
		x, y := g.find(p[0]), g.find(p[1])
		if (x == a && y == b) || (x == b && y == a) {
			return true
		}
	}
	return false
}

// merge joins the groups of the records i and j for a person. No guard refuses it.
func (g *groups) merge(i, j int) {
	a, b := g.find(i), g.find(j)
	if a == b {
		return
	}
	if b < a {
		a, b = b, a
	}
	g.parent[b] = a
	for k := range g.kinds[b] {
		g.kinds[a][k] = true
	}
	g.joins[Person]++
	g.log = append(g.log, Join{g.rs[i].Key(), g.rs[j].Key(), Person})
}

func (g *groups) find(i int) int {
	for g.parent[i] != i {
		g.parent[i] = g.parent[g.parent[i]]
		i = g.parent[i]
	}
	return i
}

// join joins the groups of the records i and j, unless a guard refuses. A refused join becomes a pair once.
func (g *groups) join(i, j int, pass string) bool {
	a, b := g.find(i), g.find(j)
	if a == b || g.split(a, b) {
		return false
	}
	if reason := refuse(g.kinds[a], g.kinds[b]); reason != "" {
		k := [2]int{min(a, b), max(a, b)}
		if !g.seen[k] {
			g.seen[k] = true
			g.pairs = append(g.pairs, Pair{A: g.rs[i].Key(), B: g.rs[j].Key(), Pass: pass, Reason: reason})
		}
		return false
	}
	if b < a {
		a, b = b, a
	}
	g.parent[b] = a
	for k := range g.kinds[b] {
		g.kinds[a][k] = true
	}
	g.joins[pass]++
	g.log = append(g.log, Join{g.rs[i].Key(), g.rs[j].Key(), pass})
	return true
}

func refuse(a, b map[string]bool) string {
	chapter := func(m map[string]bool) bool { return m["chapter"] }
	whole := func(m map[string]bool) bool {
		for k := range m {
			if wholes[k] {
				return true
			}
		}
		return false
	}
	if (chapter(a) && whole(b)) || (chapter(b) && whole(a)) {
		return "a chapter and its volume are two resources (RS10.5)"
	}
	return ""
}

// Run runs the passes C1 to C5 and builds the resource records.
func Run(in Input) (*Result, error) {
	rs := unique(in.Records)
	g := &groups{parent: make([]int, len(rs)), kinds: make([]map[string]bool, len(rs)), rs: rs, joins: map[string]int{}, seen: map[[2]int]bool{}}
	for i := range rs {
		g.parent[i] = i
		g.kinds[i] = map[string]bool{rs[i].Kind: true}
	}
	byID := index(rs)
	decide(in, g)
	// C1: a strong id.
	for _, sys := range Strong {
		for _, v := range sortedKeys(byID[sys]) {
			idx := byID[sys][v]
			for _, j := range idx[1:] {
				g.join(idx[0], j, C1)
			}
		}
	}
	// C2: a crosswalk of a snapshot.
	for _, l := range in.Crosswalks {
		a, b := lookup(byID, l.A), lookup(byID, l.B)
		for _, i := range a {
			for _, j := range b {
				g.join(i, j, C2)
			}
		}
	}
	// C3: an ISBN-13 with the title or a creator.
	for _, v := range sortedKeys(byID["isbn"]) {
		idx := byID["isbn"][v]
		for x := 0; x < len(idx); x++ {
			for y := x + 1; y < len(idx); y++ {
				a, b := &rs[idx[x]], &rs[idx[y]]
				if Similarity(a.Title, b.Title) >= ISBNTitle || shareCreator(a, b) {
					g.join(idx[x], idx[y], C3)
				}
			}
		}
	}
	skipped := blocked(g)
	works(g)
	// A pair of the middle band whose records a later join put in one group is no longer a question.
	byKey := map[string]int{}
	for i := range rs {
		byKey[rs[i].Key()] = i
	}
	g.pairs = slices.DeleteFunc(g.pairs, func(p Pair) bool {
		return p.Pass == C4 && g.find(byKey[p.A]) == g.find(byKey[p.B])
	})
	res, err := build(in, g)
	if err != nil {
		return nil, err
	}
	res.Skipped = skipped
	res.Log = g.log
	return res, nil
}

// decide applies the decisions of people: the merges join their records, and the splits keep their records apart in each later pass.
func decide(in Input, g *groups) {
	if len(in.Decisions) == 0 {
		return
	}
	byKey := map[string]int{}
	for i := range g.rs {
		byKey[g.rs[i].Key()] = i
	}
	byRes := map[string][]int{}
	for _, r := range in.Existing {
		for _, c := range r.Catalogue {
			if i, ok := byKey[c.Source+":"+c.NativeID]; ok {
				byRes[r.ID] = append(byRes[r.ID], i)
			}
		}
	}
	records := func(x string) []int {
		if strings.Contains(x, ":") {
			if i, ok := byKey[x]; ok {
				return []int{i}
			}
			return nil
		}
		return byRes[x]
	}
	g.lose = map[int]bool{}
	for _, d := range in.Decisions {
		as, bs := records(d.A), records(d.B)
		if len(as) == 0 || len(bs) == 0 {
			continue
		}
		switch d.Kind {
		case "merge":
			for _, i := range as[1:] {
				g.merge(as[0], i)
			}
			for _, j := range bs {
				g.merge(as[0], j)
			}
		case "split":
			for _, i := range as {
				for _, j := range bs {
					g.apart = append(g.apart, [2]int{i, j})
				}
			}
			for _, j := range bs {
				g.lose[j] = true
			}
		}
	}
}

// unique keeps the last record of each key, in the order of the keys.
func unique(in []Record) []Record {
	m := map[string]Record{}
	for _, r := range in {
		m[r.Key()] = r
	}
	out := make([]Record, 0, len(m))
	for _, k := range sortedKeys(m) {
		out = append(out, m[k])
	}
	return out
}

func sortedKeys[T any](m map[string]T) []string {
	return slices.Sorted(maps.Keys(m))
}

// index gives the records of each value of each system of ids.
func index(rs []Record) map[string]map[string][]int {
	out := map[string]map[string][]int{}
	for i := range rs {
		for sys, vs := range rs[i].IDs {
			if out[sys] == nil {
				out[sys] = map[string][]int{}
			}
			for _, v := range vs {
				if l := out[sys][v]; len(l) == 0 || l[len(l)-1] != i {
					out[sys][v] = append(l, i)
				}
			}
		}
	}
	return out
}

func lookup(byID map[string]map[string][]int, key string) []int {
	sys, v, ok := strings.Cut(key, ":")
	if !ok {
		return nil
	}
	return byID[sys][v]
}

func families(r *Record) []string {
	var out []string
	for _, c := range r.Creators {
		if f := Family(c.Name); f != "" && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

func shareCreator(a, b *Record) bool {
	fb := families(b)
	for _, f := range families(a) {
		if slices.Contains(fb, f) {
			return true
		}
	}
	return false
}

// overlap gives the share of the family names of the shorter list that the other list has.
func overlap(a, b *Record) float64 {
	fa, fb := families(a), families(b)
	if len(fa) == 0 || len(fb) == 0 {
		return 0
	}
	if len(fb) < len(fa) {
		fa, fb = fb, fa
	}
	n := 0
	for _, f := range fa {
		if slices.Contains(fb, f) {
			n++
		}
	}
	return float64(n) / float64(len(fa))
}

// Score gives the score of the matcher for two records of one block of C4, from 0 to 1. It gives 0 for a pair that the years of RS10.4 rule out.
func Score(a, b *Record) float64 {
	ya, yb := a.Year(), b.Year()
	if a.Kind == "preprint" && b.Kind != "preprint" && papers[b.Kind] && ya > 0 && yb > 0 && (yb < ya || yb > ya+Window) {
		return 0
	}
	if b.Kind == "preprint" && a.Kind != "preprint" && papers[a.Kind] && ya > 0 && yb > 0 && (ya < yb || ya > yb+Window) {
		return 0
	}
	year := 0.0
	switch d := abs(ya - yb); {
	case ya == 0 || yb == 0:
		year = 0.5
	case d == 0:
		year = 1
	case d == 1 || a.Kind == "preprint" || b.Kind == "preprint":
		year = 0.7
	}
	s := 0.6*Similarity(a.Title, b.Title) + 0.25*overlap(a, b) + 0.15*year
	if a.Venue != "" && a.Venue == b.Venue && a.Volume != "" && a.Volume == b.Volume && a.Page != "" && a.Page == b.Page {
		s = max(s, 0.6*Similarity(a.Title, b.Title)+0.4)
	}
	return s
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// blocked runs pass C4. A block is the family name of the first creator with the year, or the venue with the volume and the first page. A record is compared with its block and with the block of the next year.
func blocked(g *groups) int {
	blocks := map[string][]int{}
	for i := range g.rs {
		r := &g.rs[i]
		if fs := families(r); len(fs) > 0 && r.Year() > 0 {
			blocks["a:"+fs[0]+":"+strconv.Itoa(r.Year())] = append(blocks["a:"+fs[0]+":"+strconv.Itoa(r.Year())], i)
		}
		if r.Venue != "" && r.Volume != "" && r.Page != "" {
			k := "v:" + r.Venue + ":" + r.Volume + ":" + r.Page
			blocks[k] = append(blocks[k], i)
		}
	}
	skipped := 0
	queued := map[[2]int]bool{}
	compare := func(xs, ys []int, same bool) {
		for a, i := range xs {
			start := 0
			if same {
				start = a + 1
			}
			for _, j := range ys[start:] {
				if g.find(i) == g.find(j) {
					continue
				}
				s := Score(&g.rs[i], &g.rs[j])
				switch {
				case s >= High:
					g.join(i, j, C4)
				case s >= Middle && !queued[[2]int{min(i, j), max(i, j)}] && !g.split(g.find(i), g.find(j)):
					queued[[2]int{min(i, j), max(i, j)}] = true
					g.pairs = append(g.pairs, Pair{A: g.rs[i].Key(), B: g.rs[j].Key(), Pass: C4, Score: round(s), Reason: "the score is in the middle band"})
				}
			}
		}
	}
	for _, k := range sortedKeys(blocks) {
		xs := blocks[k]
		if len(xs) > MaxBlock {
			skipped++
			continue
		}
		compare(xs, xs, true)
		if after, ok := strings.CutPrefix(k, "a:"); ok {
			i := strings.LastIndexByte(after, ':')
			y, _ := strconv.Atoi(after[i+1:])
			if ys := blocks["a:"+after[:i]+":"+strconv.Itoa(y+1)]; len(ys) > 0 && len(ys) <= MaxBlock {
				compare(xs, ys, false)
			}
		}
	}
	return skipped
}

func round(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }

// works runs pass C5. It joins the records with one normal title and one first creator, and the records that an Open Library work id or a relation translation_of links. A title of fewer than 12 characters does not join, and two papers join only within the window of RS10.4.
func works(g *groups) {
	byKey := map[string][]int{}
	for i := range g.rs {
		r := &g.rs[i]
		t := WorkTitle(r.Title)
		fs := families(r)
		if len([]rune(t)) < 12 || len(fs) == 0 {
			continue
		}
		byKey[t+"|"+fs[0]] = append(byKey[t+"|"+fs[0]], i)
	}
	for _, k := range sortedKeys(byKey) {
		idx := byKey[k]
		for _, j := range idx[1:] {
			a, b := &g.rs[idx[0]], &g.rs[j]
			if papers[a.Kind] && papers[b.Kind] && a.Year() > 0 && b.Year() > 0 && abs(a.Year()-b.Year()) > Window {
				continue
			}
			g.join(idx[0], j, C5)
		}
	}
	byID := index(g.rs)
	for _, v := range sortedKeys(byID["openlibrary_work"]) {
		idx := byID["openlibrary_work"][v]
		for _, j := range idx[1:] {
			g.join(idx[0], j, C5)
		}
	}
	for i := range g.rs {
		for _, rel := range g.rs[i].Relations {
			if rel.Kind != "translation_of" {
				continue
			}
			for _, j := range lookup(byID, rel.To) {
				g.join(i, j, C5)
			}
		}
	}
}

var dateRE = regexp.MustCompile(`^[0-9]{4}(-[0-9]{2}(-[0-9]{2})?)?`)

// date gives the longest prefix of s of the form YYYY, YYYY-MM or YYYY-MM-DD.
func date(s string) string { return dateRE.FindString(s) }

// rank gives the place of the source in the precedence of the field.
func rank(field, source string) int {
	if i := slices.Index(Precedence[field], source); i >= 0 {
		return i
	}
	return len(Precedence[field])
}

// best gives the record of highest precedence for the field among the records that have it. The ties go to the smaller key.
func best(field string, rs []*Record, has func(*Record) bool, first func(*Record) bool) *Record {
	var out *Record
	better := func(a, b *Record) bool {
		if first != nil && first(a) != first(b) {
			return first(a)
		}
		if ra, rb := rank(field, a.Source), rank(field, b.Source); ra != rb {
			return ra < rb
		}
		return a.Key() < b.Key()
	}
	for _, r := range rs {
		if has(r) && (out == nil || better(r, out)) {
			out = r
		}
	}
	return out
}

// provisional gives a provisional id from the hash of the smallest key of the group, so that a run gives the same ids for the same records.
func provisional(key string, used map[string]bool) string {
	for n := 0; ; n++ {
		h := sha256.Sum256([]byte(key + "#" + strconv.Itoa(n)))
		id := "~" + hex.EncodeToString(h[:4])
		if !used[id] && tag.IsProvisional(id) {
			used[id] = true
			return id
		}
	}
}

func lang(s string) string {
	s, _, _ = strings.Cut(strings.ToLower(s), "-")
	return s
}

// build makes the resource records from the groups. A group keeps the id of the existing resource that has one of its records. A group with the records of two existing resources keeps the older id, and the other resource becomes a duplicate of it (RS10.6).
func build(in Input, g *groups) (*Result, error) {
	members := map[int][]int{}
	for i := range g.rs {
		members[g.find(i)] = append(members[g.find(i)], i)
	}
	existing := map[string]*resource.Resource{}
	owner := map[string]string{}
	for i := range in.Existing {
		r := &in.Existing[i]
		existing[r.ID] = r
		if !r.Live() {
			continue
		}
		for _, c := range r.Catalogue {
			owner[c.Source+":"+c.NativeID] = r.ID
		}
	}
	older := func(a, b string) bool {
		oa, okA := in.Order[a]
		ob, okB := in.Order[b]
		if okA && okB && oa != ob {
			return oa < ob
		}
		if okA != okB {
			return okA
		}
		return a < b
	}
	used := map[string]bool{}
	claimed := map[string]bool{}
	out := map[string]resource.Resource{}
	res := &Result{Joins: g.joins}
	roots := slices.Sorted(maps.Keys(members))
	for _, root := range roots {
		idx := members[root]
		recs := make([]*Record, len(idx))
		var ids []string
		for k, i := range idx {
			recs[k] = &g.rs[i]
			if id := owner[g.rs[i].Key()]; id != "" && !g.lose[i] && !claimed[id] && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		slices.SortFunc(ids, func(a, b string) int {
			if older(a, b) {
				return -1
			}
			return 1
		})
		var prev *resource.Resource
		var dups []string
		id := ""
		if len(ids) > 0 {
			id, dups = ids[0], ids[1:]
			prev = existing[id]
			for _, x := range ids {
				claimed[x] = true
			}
		} else {
			id = provisional(recs[0].Key(), used)
		}
		r := assemble(id, recs, prev)
		if prev == nil {
			r.History = []resource.Event{{Op: "add", At: in.Day, By: in.By}}
			for _, i := range idx {
				if from := owner[g.rs[i].Key()]; from != "" && from != id {
					r.History = append(r.History, resource.Event{Op: "split", At: in.Day, By: in.By, Note: "split from " + from})
					break
				}
			}
		}
		for _, dup := range dups {
			r.History = append(r.History, resource.Event{Op: "merge", At: in.Day, By: in.By, Note: "merged " + dup})
			d := *existing[dup]
			d.Scope = &resource.Scope{Value: resource.Duplicate, Target: id, By: in.By}
			d.History = append(slices.Clone(d.History), resource.Event{Op: "merge", At: in.Day, By: in.By, Note: "duplicate of " + id})
			out[dup] = d
		}
		out[id] = r
	}
	// An existing resource that no group has keeps its record.
	for id, r := range existing {
		if _, ok := out[id]; !ok {
			out[id] = *r
		}
	}
	for _, id := range sortedKeys(out) {
		res.Resources = append(res.Resources, out[id])
	}
	res.Pairs = g.pairs
	slices.SortFunc(res.Pairs, func(a, b Pair) int {
		return cmp.Or(cmp.Compare(a.Pass, b.Pass), cmp.Compare(a.A, b.A), cmp.Compare(a.B, b.B))
	})
	for i := range res.Resources {
		if ps := resource.Check(&res.Resources[i]); len(ps) > 0 && tag.IsProvisional(res.Resources[i].ID) {
			return nil, fmt.Errorf("cluster: the new resource of %s has a problem: %s", res.Resources[i].Catalogue[0].NativeID, ps[0])
		}
	}
	return res, nil
}

// assemble makes one resource from the records of one group. The fields that a person can set (the scope, the relations that no catalogue gives, the history) come from the existing record.
func assemble(id string, recs []*Record, prev *resource.Resource) resource.Resource {
	slices.SortFunc(recs, func(a, b *Record) int { return strings.Compare(a.Key(), b.Key()) })
	r := resource.Resource{ID: id, IDs: resource.IDs{}}
	if prev != nil {
		r.Scope = prev.Scope
		r.History = slices.Clone(prev.History)
		r.Genre = prev.Genre
		r.Level = prev.Level
		for _, rel := range prev.Relations {
			if rel.Source != Version {
				r.Relations = append(r.Relations, rel)
			}
		}
	}
	// The kind: a published paper wins over its preprint.
	if k := best("kind", recs, func(x *Record) bool { return x.Kind != "" && x.Kind != "preprint" }, nil); k != nil {
		r.Kind = k.Kind
	} else {
		r.Kind = recs[0].Kind
	}
	exprs := expressions(recs)
	workLang := ""
	if len(exprs) > 0 {
		workLang = exprs[0].lang
	}
	if t := best("title", recs, func(x *Record) bool { return strings.TrimSpace(x.Title) != "" }, func(x *Record) bool { return workLang != "" && lang(x.Language) == workLang }); t != nil {
		r.Title = t.Title
	}
	if t := best("title", recs, func(x *Record) bool { return x.TitleEn != "" }, nil); t != nil {
		r.TitleEn = t.TitleEn
	}
	if c := best("creators", recs, func(x *Record) bool { return len(x.Creators) > 0 }, nil); c != nil {
		r.Creators = c.Creators
	}
	if m := best("msc", recs, func(x *Record) bool { return len(x.MSC) > 0 }, nil); m != nil {
		for _, code := range m.MSC {
			r.MSC = append(r.MSC, resource.Code{Code: code, From: m.Source})
		}
	}
	for _, x := range recs {
		for _, s := range x.Subjects {
			if !slices.Contains(r.Subjects, s) {
				r.Subjects = append(r.Subjects, s)
			}
		}
		r.Catalogue = append(r.Catalogue, resource.CatalogueRecord{Source: x.Source, Snapshot: x.Snapshot, NativeID: x.NativeID, Level: x.Level})
		for sys, vs := range x.IDs {
			if levels[sys] == "work" {
				r.IDs[sys] = addValues(r.IDs[sys], vs...)
			}
		}
		for _, rel := range x.Relations {
			if rel.Kind != "translation_of" && !hasRelation(r.Relations, rel) {
				rel.Source = Version
				if !strings.Contains(rel.To, ":") {
					r.Relations = append(r.Relations, rel)
				}
			}
		}
	}
	if len(r.IDs) == 0 {
		r.IDs = nil
	}
	var prevExprs []resource.Expression
	if prev != nil {
		prevExprs = prev.Expressions
	}
	r.Expressions = keyed(exprs, prevExprs, workLang)
	for _, e := range r.Expressions {
		if e.Date != "" && (r.Date == "" || e.Date < r.Date) {
			r.Date = e.Date
		}
	}
	return r
}

func hasRelation(rs []resource.Relation, rel resource.Relation) bool {
	for _, x := range rs {
		if x.Kind == rel.Kind && x.To == rel.To && x.From == rel.From {
			return true
		}
	}
	return false
}

func addValues(list resource.Values, vs ...string) resource.Values {
	for _, v := range vs {
		if v != "" && !slices.Contains(list, v) {
			list = append(list, v)
		}
	}
	return list
}

type expr struct {
	version string // preprint, or empty for the published text
	edition int
	lang    string
	recs    []*Record
	date    string
}

// expressions puts the records of one work into expressions (RS10.3, RS10.4). A record is in the expression with its version, its edition number and its language. A record with no edition number or no language goes to the one expression that agrees on the rest, when there is one.
func expressions(recs []*Record) []*expr {
	var out []*expr
	key := func(x *Record) (string, int, string) {
		v := ""
		if x.Kind == "preprint" {
			v = "preprint"
		}
		ed := Edition(x.Edition)
		if ed == 0 {
			ed = Edition(x.Title)
		}
		return v, ed, lang(x.Language)
	}
	var loose []*Record
	for _, x := range recs {
		v, ed, l := key(x)
		if ed == 0 || l == "" {
			loose = append(loose, x)
			continue
		}
		found := false
		for _, e := range out {
			if e.version == v && e.edition == ed && e.lang == l {
				e.recs = append(e.recs, x)
				found = true
				break
			}
		}
		if !found {
			out = append(out, &expr{version: v, edition: ed, lang: l, recs: []*Record{x}})
		}
	}
	for _, x := range loose {
		v, ed, l := key(x)
		var match []*expr
		for _, e := range out {
			if e.version == v && (ed == 0 || e.edition == ed || e.edition == 0) && (l == "" || e.lang == l || e.lang == "") {
				match = append(match, e)
			}
		}
		if len(match) == 1 {
			match[0].recs = append(match[0].recs, x)
			if match[0].lang == "" {
				match[0].lang = l
			}
			if match[0].edition == 0 {
				match[0].edition = ed
			}
			continue
		}
		out = append(out, &expr{version: v, edition: ed, lang: l, recs: []*Record{x}})
	}
	for _, e := range out {
		for _, x := range e.recs {
			if d := date(x.Date); d != "" && (e.date == "" || d < e.date) {
				e.date = d
			}
		}
	}
	// The first expression is the oldest. The language of the work is its language.
	slices.SortStableFunc(out, func(a, b *expr) int {
		da, db := a.date, b.date
		if da == "" {
			da = "9999"
		}
		if db == "" {
			db = "9999"
		}
		return cmp.Or(cmp.Compare(da, db), cmp.Compare(a.version, b.version), cmp.Compare(a.edition, b.edition), cmp.Compare(a.lang, b.lang))
	})
	return out
}

// keyed makes the expressions of a resource. An expression keeps the key of the existing expression with which it shares an id or its label and language. A new expression gets the next key, and a key is never used again.
func keyed(exprs []*expr, prev []resource.Expression, workLang string) []resource.Expression {
	next := 1
	for _, e := range prev {
		if n, err := strconv.Atoi(strings.TrimPrefix(e.Key, "e")); err == nil && n >= next {
			next = n + 1
		}
	}
	taken := map[string]bool{}
	var out []resource.Expression
	for _, e := range exprs {
		x := resource.Expression{Language: e.lang, Date: e.date, IDs: resource.IDs{}}
		switch {
		case e.version == "preprint":
			x.Kind, x.Label = "version", "preprint"
		case e.lang != "" && workLang != "" && e.lang != workLang:
			x.Kind = "translation"
		case papers[e.recs[0].Kind]:
			x.Kind, x.Label = "version", "published"
		default:
			x.Kind = "edition"
		}
		if e.edition > 0 && x.Kind != "version" {
			x.Label = ordinal(e.edition) + " edition"
		}
		manifs := map[string]*resource.Manifestation{}
		var order []string
		for _, r := range e.recs {
			mids := resource.IDs{}
			for sys, vs := range r.IDs {
				switch levels[sys] {
				case "work":
				case "manifestation":
					mids[sys] = addValues(mids[sys], vs...)
				default:
					x.IDs[sys] = addValues(x.IDs[sys], vs...)
				}
			}
			if len(mids) == 0 && r.Level != "manifestation" {
				continue
			}
			k := r.Key()
			if isbn := mids["isbn"]; len(isbn) > 0 {
				k = "isbn:" + isbn[0]
			}
			m := manifs[k]
			if m == nil {
				m = &resource.Manifestation{Kind: "print", IDs: resource.IDs{}}
				manifs[k] = m
				order = append(order, k)
			}
			for sys, vs := range mids {
				m.IDs[sys] = addValues(m.IDs[sys], vs...)
			}
			if m.Publisher == "" {
				m.Publisher = r.Publisher
			}
			if d := date(r.Date); d != "" && (m.Date == "" || d < m.Date) {
				m.Date = d
			}
		}
		for i, k := range order {
			m := manifs[k]
			m.Key = "m" + strconv.Itoa(i+1)
			if len(m.IDs) == 0 {
				m.IDs = nil
			}
			x.Manifestations = append(x.Manifestations, *m)
		}
		if len(x.IDs) == 0 {
			x.IDs = nil
		}
		x.Key = ""
		for _, p := range prev {
			if !taken[p.Key] && sameExpression(&x, &p) {
				x.Key = p.Key
				x.Structure, x.Access, x.Contributors = p.Structure, p.Access, p.Contributors
				x.Manifestations = keepManifestations(x.Manifestations, p.Manifestations)
				break
			}
		}
		if x.Key == "" {
			x.Key = "e" + strconv.Itoa(next)
			next++
		}
		taken[x.Key] = true
		out = append(out, x)
	}
	slices.SortStableFunc(out, func(a, b resource.Expression) int {
		na, _ := strconv.Atoi(a.Key[1:])
		nb, _ := strconv.Atoi(b.Key[1:])
		return cmp.Compare(na, nb)
	})
	return out
}

func sameExpression(a, b *resource.Expression) bool {
	for sys, vs := range a.IDs {
		for _, v := range vs {
			if slices.Contains(b.IDs[sys], v) {
				return true
			}
		}
	}
	return a.Kind == b.Kind && a.Label == b.Label && a.Language == b.Language
}

// keepManifestations gives a manifestation the key of the existing manifestation with which it shares an id. A new manifestation gets the next key.
func keepManifestations(ms, prev []resource.Manifestation) []resource.Manifestation {
	next := 1
	for _, p := range prev {
		if n, err := strconv.Atoi(strings.TrimPrefix(p.Key, "m")); err == nil && n >= next {
			next = n + 1
		}
	}
	taken := map[string]bool{}
	for i := range ms {
		ms[i].Key = ""
		for _, p := range prev {
			if taken[p.Key] {
				continue
			}
			for sys, vs := range ms[i].IDs {
				for _, v := range vs {
					if slices.Contains(p.IDs[sys], v) && ms[i].Key == "" {
						ms[i].Key = p.Key
						ms[i].Links, ms[i].Access = p.Links, p.Access
					}
				}
			}
		}
		if ms[i].Key == "" {
			ms[i].Key = "m" + strconv.Itoa(next)
			next++
		}
		taken[ms[i].Key] = true
	}
	return ms
}

func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return strconv.Itoa(n) + suffix
}
