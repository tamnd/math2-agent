package cluster

import (
	"cmp"
	"hash/fnv"
	"slices"
	"strconv"

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
	next uint64 // of a block of an author: the block of the next year
	node int32
	kind uint8
}

const (
	join  uint8 = iota // a key that joins all the nodes that have it
	weak               // an id that joins its nodes only when a crosswalk or a translation_of edge names it
	need               // a crosswalk or a translation_of edge that names an id
	block              // a block of C4
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

// Partitioned gives the partition of the records of in into buckets of about size records. The records must have one key each, as Run makes them.
func Partitioned(in Input, size int) *Partition {
	rs := in.Records
	nr := len(rs)
	nodes := nr + len(in.Existing) + len(in.Crosswalks) + len(in.Decisions)
	var ms []mark
	add := func(k string, node int, kind uint8) {
		ms = append(ms, mark{key: keyHash(k), node: int32(node), kind: kind}) // #nosec G115 -- a run has fewer than 2^31 nodes.
	}
	for i := range rs {
		r := &rs[i]
		add("rec:"+r.Key(), i, join)
		for sys, vs := range r.IDs {
			kind := weak
			if joinSystem(sys) {
				kind = join
			}
			for _, v := range vs {
				add("id:"+sys+":"+v, i, kind)
			}
		}
		if Fillers[r.Source] {
			continue
		}
		fs := families(r)
		if len(fs) > 0 && r.Year() > 0 {
			a := "a:" + fs[0] + ":"
			ms = append(ms, mark{key: keyHash(a + strconv.Itoa(r.Year())), next: keyHash(a + strconv.Itoa(r.Year()+1)), node: int32(i), kind: block}) // #nosec G115 -- as above.
		}
		if r.Venue != "" && r.Volume != "" && r.Page != "" {
			add("v:"+r.Venue+":"+r.Volume+":"+r.Page, i, block)
		}
		if t := WorkTitle(r.Title); len([]rune(t)) >= 12 && len(fs) > 0 {
			add("w:"+t+"|"+fs[0], i, join)
		}
		for _, rel := range r.Relations {
			if rel.Kind == "translation_of" {
				add("id:"+rel.To, i, need)
			}
		}
	}
	for i := range in.Existing {
		e := &in.Existing[i]
		if !e.Live() {
			continue
		}
		add("res:"+e.ID, nr+i, join)
		for _, c := range e.Catalogue {
			add("rec:"+c.Source+":"+c.NativeID, nr+i, join)
		}
	}
	base := nr + len(in.Existing)
	for i, l := range in.Crosswalks {
		add("id:"+l.A, base+i, need)
		add("id:"+l.B, base+i, need)
	}
	base += len(in.Crosswalks)
	for i, d := range in.Decisions {
		for _, x := range []string{d.A, d.B} {
			k := "res:" + x
			if slices.Contains([]byte(x), ':') {
				k = "rec:" + x
			}
			add(k, base+i, join)
		}
	}
	slices.SortFunc(ms, func(a, b mark) int {
		return cmp.Or(cmp.Compare(a.key, b.key), cmp.Compare(a.kind, b.kind), cmp.Compare(a.node, b.node))
	})

	parent := make([]int32, nodes)
	for i := range parent {
		parent[i] = int32(i) // #nosec G115 -- as above.
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
	p := &Partition{Large: map[uint64]bool{}}
	blocks := map[uint64][]int32{}
	for i := 0; i < len(ms); {
		j := i
		for j < len(ms) && ms[j].key == ms[i].key {
			j++
		}
		run := ms[i:j]
		if run[0].kind == block {
			// The blocks of C4 count records, and a record is in a block once.
			var recs []int32
			for _, m := range run {
				if m.kind == block && (len(recs) == 0 || recs[len(recs)-1] != m.node) {
					recs = append(recs, m.node)
				}
			}
			if len(recs) > MaxBlock {
				p.Large[run[0].key] = true
			} else {
				blocks[run[0].key] = recs
			}
		} else {
			named := false
			for _, m := range run {
				named = named || m.kind != weak
			}
			for _, m := range run[1:] {
				if named {
					union(run[0].node, m.node)
				}
			}
		}
		i = j
	}
	for _, recs := range blocks {
		for _, n := range recs[1:] {
			union(recs[0], n)
		}
	}
	for _, m := range ms {
		if m.kind == block && m.next != 0 {
			if next, ok := blocks[m.next]; ok && blocks[m.key] != nil {
				union(m.node, next[0])
			}
		}
	}

	// The buckets: the components in the order of their first record, each bucket filled to the size.
	count := map[int32]int{}
	for i := range nr {
		count[find(int32(i))]++ // #nosec G115 -- as above.
	}
	p.Components = len(count)
	for _, n := range count {
		p.Largest = max(p.Largest, n)
	}
	of := map[int32]int{}
	p.Bucket = make([]int, nr)
	fill := 0
	for i := range nr {
		root := find(int32(i)) // #nosec G115 -- as above.
		b, ok := of[root]
		if !ok {
			if p.Buckets == 0 || (fill > 0 && fill+count[root] > size) {
				p.Buckets++
				fill = 0
			}
			b = p.Buckets - 1
			of[root] = b
			fill += count[root]
		}
		p.Bucket[i] = b
	}
	at := func(node int) int {
		if b, ok := of[find(int32(node))]; ok { // #nosec G115 -- as above.
			return b
		}
		return -1
	}
	p.resources = make([]int, len(in.Existing))
	for i := range in.Existing {
		p.resources[i] = at(nr + i)
	}
	p.links = make([]int, len(in.Crosswalks))
	for i := range in.Crosswalks {
		p.links[i] = at(nr + len(in.Existing) + i)
	}
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
