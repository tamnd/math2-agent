package cluster

import (
	"cmp"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/resource"
)

// BucketSize is the number of records of one bucket of a partitioned run (RS10.8). A component with more records is one bucket.
const BucketSize = 50000

// keyHash gives the 64-bit hash of a join key. Two keys with one hash only put two components into one, which does not change a join.
func keyHash(k string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(k))
	return h.Sum64()
}

// mark is one join key of one node. A node is a record, an existing resource, a crosswalk link or a decision of a person.
type mark struct {
	key  uint64
	node int32
	kind uint8
}

const (
	join uint8 = iota // a key that joins all the nodes that have it
	weak              // an id that joins its nodes only when a crosswalk or a translation_of edge names it
	need              // a crosswalk or a translation_of edge that names an id
)

// joinSystem reports whether a shared value of the system can join two records in a pass: a strong id of C1, an ISBN of C3, an Open Library work id of C5 or an id that puts a filler on a group.
func joinSystem(sys string) bool {
	return slices.Contains(Strong, sys) || slices.Contains(FillerJoins, sys) || sys == "isbn" || sys == "openlibrary_work"
}

// Partition is the partition of the records of a run into buckets (RS10.8). No pass can join two records of different buckets.
type Partition struct {
	Bucket     []int           // the bucket of each record, in the order of the records
	Buckets    int             // the number of buckets
	Components int             // the number of components of the records
	Largest    int             // the records of the largest component
	Large      map[uint64]bool // the hashes of the blocks of C4 with more than MaxBlock records
	resources  []int           // the bucket of each existing resource, or -1 for a resource with no record
	links      []int           // the bucket of each crosswalk link, or -1
}

// partitioner collects the join keys of the nodes of a run. A node is a record, an existing resource, a crosswalk link or a decision of a person.
type partitioner struct {
	ms     []mark
	nodes  int32
	blocks *blockSet
	edges  [][2]int32 // the pairs of C4 that can join or go to the queue
}

func (pt *partitioner) next() int32 {
	pt.nodes++
	return pt.nodes - 1
}

func (pt *partitioner) add(k string, node int32, kind uint8) {
	pt.ms = append(pt.ms, mark{key: keyHash(k), node: node, kind: kind})
}

// record adds the keys of a record: its key, its ids, its blocks of C4, its key of C5 and the targets of its translation_of edges.
func (pt *partitioner) record(r *Record) int32 {
	n := pt.next()
	pt.add("rec:"+r.Key(), n, join)
	for sys, vs := range r.IDs {
		kind := weak
		if joinSystem(sys) {
			kind = join
		}
		for _, v := range vs {
			pt.add("id:"+sys+":"+v, n, kind)
		}
	}
	if Fillers[r.Source] {
		return n
	}
	fs := families(r)
	if len(fs) > 0 && r.Year() > 0 {
		pt.blocks.add(n, "a:"+fs[0], "a:"+fs[0]+":"+strconv.Itoa(r.Year()), r)
	}
	if r.Venue != "" && r.Volume != "" && r.Page != "" {
		k := "v:" + r.Venue + ":" + r.Volume + ":" + r.Page
		pt.blocks.add(n, k, k, r)
	}
	if t := WorkTitle(r.Title); len([]rune(t)) >= 12 && len(fs) > 0 {
		pt.add("w:"+t+"|"+fs[0], n, join)
	}
	for _, rel := range r.Relations {
		if rel.Kind == "translation_of" {
			pt.add("id:"+rel.To, n, need)
		}
	}
	return n
}

// resource adds the keys of an existing resource: its id and the keys of its records. A resource that is not live has no keys.
func (pt *partitioner) resource(e *resource.Resource) int32 {
	n := pt.next()
	if !e.Live() {
		return n
	}
	pt.add("res:"+e.ID, n, join)
	for _, c := range e.Catalogue {
		pt.add("rec:"+c.Source+":"+c.NativeID, n, join)
	}
	return n
}

func (pt *partitioner) link(l Link) int32 {
	n := pt.next()
	pt.add("id:"+l.A, n, need)
	pt.add("id:"+l.B, n, need)
	return n
}

func (pt *partitioner) decision(d Decision) int32 {
	n := pt.next()
	for _, x := range []string{d.A, d.B} {
		k := "res:" + x
		if strings.Contains(x, ":") {
			k = "rec:" + x
		}
		pt.add(k, n, join)
	}
	return n
}

// buckets joins the nodes into components and packs the components of the records into buckets of about size records, in the order of the records. It gives the bucket of each node, -1 for a node whose component has no record, and the partition with no Bucket.
func (pt *partitioner) buckets(records []int32, size int) ([]int32, *Partition, error) {
	large, err := pt.blocks.pairs(func(x, y int32) { pt.edges = append(pt.edges, [2]int32{x, y}) })
	if err != nil {
		return nil, nil, err
	}
	ms := pt.ms
	slices.SortFunc(ms, func(a, b mark) int {
		return cmp.Or(cmp.Compare(a.key, b.key), cmp.Compare(a.kind, b.kind), cmp.Compare(a.node, b.node))
	})
	parent := make([]int32, pt.nodes)
	for i := range parent {
		parent[i] = int32(i) // #nosec G115 -- a run has fewer than 2^31 nodes.
	}
	find := func(x int32) int32 {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int32) {
		if x, y := find(a), find(b); x != y {
			parent[max(x, y)] = min(x, y)
		}
	}
	p := &Partition{Large: large}
	for i := 0; i < len(ms); {
		j := i
		for j < len(ms) && ms[j].key == ms[i].key {
			j++
		}
		if run := ms[i:j]; slices.ContainsFunc(run, func(m mark) bool { return m.kind != weak }) {
			for _, m := range run[1:] {
				union(run[0].node, m.node)
			}
		}
		i = j
	}
	for _, e := range pt.edges {
		union(e[0], e[1])
	}
	pt.ms, pt.edges = nil, nil

	count := map[int32]int{}
	for _, n := range records {
		count[find(n)]++
	}
	p.Components = len(count)
	for _, n := range count {
		p.Largest = max(p.Largest, n)
	}
	of := map[int32]int32{}
	fill := 0
	for _, n := range records {
		root := find(n)
		if _, ok := of[root]; ok {
			continue
		}
		if p.Buckets == 0 || (fill > 0 && fill+count[root] > size) {
			p.Buckets++
			fill = 0
		}
		of[root] = int32(p.Buckets - 1) // #nosec G115 -- as above.
		fill += count[root]
	}
	out := make([]int32, pt.nodes)
	for i := range out {
		b, ok := of[find(int32(i))] // #nosec G115 -- as above.
		if !ok {
			b = -1
		}
		out[i] = b
	}
	return out, p, nil
}

// Partitioned gives the partition of the records of in into buckets of about size records.
func Partitioned(in Input, size int) *Partition {
	pt := partitioner{blocks: newBlockSet("")}
	recs := make([]int32, len(in.Records))
	for i := range in.Records {
		recs[i] = pt.record(&in.Records[i])
	}
	res := make([]int32, len(in.Existing))
	for i := range in.Existing {
		res[i] = pt.resource(&in.Existing[i])
	}
	links := make([]int32, len(in.Crosswalks))
	for i, l := range in.Crosswalks {
		links[i] = pt.link(l)
	}
	for _, d := range in.Decisions {
		pt.decision(d)
	}
	of, p, _ := pt.buckets(recs, size) // the blocks are in memory, so there is no error
	at := func(ns []int32) []int {
		out := make([]int, len(ns))
		for i, n := range ns {
			out[i] = int(of[n])
		}
		return out
	}
	p.Bucket, p.resources, p.links = at(recs), at(res), at(links)
	return p
}

// RunPartitioned runs the passes as Run does, one bucket of the partition at a time (RS10.8). The resources, the pairs and the joins are those of Run. The log has the joins of each pass in the order of the buckets.
func RunPartitioned(in Input, size int) (*Result, *Partition, error) {
	in.Records = unique(in.Records)
	p := Partitioned(in, size)
	recs := make([][]Record, p.Buckets)
	for i, b := range p.Bucket {
		recs[b] = append(recs[b], in.Records[i])
	}
	exist := make([][]resource.Resource, p.Buckets)
	out := map[string]resource.Resource{}
	for i, b := range p.resources {
		if b >= 0 {
			exist[b] = append(exist[b], in.Existing[i])
		} else {
			out[in.Existing[i].ID] = in.Existing[i]
		}
	}
	links := make([][]Link, p.Buckets)
	for i, b := range p.links {
		if b >= 0 {
			links[b] = append(links[b], in.Crosswalks[i])
		}
	}
	joins := map[string]int{}
	var pairs []Pair
	var log []Join
	type fresh struct {
		key string
		r   resource.Resource
	}
	var news []fresh
	for b := range p.Buckets {
		bin := in
		bin.Records, bin.Existing, bin.Crosswalks, bin.Large = recs[b], exist[b], links[b], p.Large
		g, _ := passes(bin)
		rs, fr := groupResources(bin, g, false)
		for id, r := range rs {
			if key, ok := fr[id]; ok {
				news = append(news, fresh{key, r})
				continue
			}
			out[id] = r
		}
		for k, n := range g.joins {
			joins[k] += n
		}
		pairs = append(pairs, g.pairs...)
		log = append(log, g.log...)
	}
	// The provisional ids again, in the order of the smallest key of each group, as one run gives them.
	slices.SortFunc(news, func(a, b fresh) int { return cmp.Compare(a.key, b.key) })
	used := map[string]bool{}
	for _, n := range news {
		r := n.r
		r.ID = provisional(n.key, used)
		out[r.ID] = r
	}
	// Each existing resource that no group has keeps its record, as in one run.
	for i, b := range p.resources {
		if _, ok := out[in.Existing[i].ID]; !ok && b >= 0 {
			out[in.Existing[i].ID] = in.Existing[i]
		}
	}
	order := map[string]int{Person: 0, C1: 1, C2: 2, C3: 3, C4: 4, C5: 5}
	slices.SortStableFunc(log, func(a, b Join) int { return cmp.Compare(order[a.Pass], order[b.Pass]) })
	res, err := finish(out, pairs, joins)
	if err != nil {
		return nil, nil, err
	}
	res.Skipped = len(p.Large)
	res.Log = log
	return res, p, nil
}
