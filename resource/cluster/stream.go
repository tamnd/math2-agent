package cluster

import (
	"bufio"
	"cmp"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/tag"
)

// Stream is the summary of RunFiles.
type Stream struct {
	Records   int            // the source records of the files
	Resources int            // the live resources
	Pairs     []Pair         // the items of the queue res-cluster
	Joins     map[string]int // the joins of each pass
	Skipped   int            // the blocks of C4 that had more than MaxBlock records
	Partition *Partition     // the partition, with no Bucket
}

// groupsOfShards is the number of files that hold the resources of the shards before they are written. The resources of one file are in memory at one time.
const groupsOfShards = 256

// RunFiles is a partitioned run (RS10.8) that reads the source records from the files and writes the resources to the shards of records/resources/ in the tree at root. The records of in are not used. It writes the records of each bucket and the resources to files in a new directory in spill, and removes the directory at the end. It keeps in memory the keys of the records, the records of one bucket and the resources of one file of shards. The resources and the pairs are those of Run on the records of the files.
func RunFiles(in Input, files []string, spill string, size int, root string) (*Stream, error) {
	dir, err := os.MkdirTemp(spill, "cluster-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	name := func(kind string, n int) string { return filepath.Join(dir, fmt.Sprintf("%s-%04d.gz", kind, n)) }

	// The keys of the nodes, and the partition.
	var pt partitioner
	exist := make([]int32, len(in.Existing))
	for i := range in.Existing {
		exist[i] = pt.resource(&in.Existing[i])
	}
	links := make([]int32, len(in.Crosswalks))
	for i, l := range in.Crosswalks {
		links[i] = pt.link(l)
	}
	for _, d := range in.Decisions {
		pt.decision(d)
	}
	var recs []int32
	for _, f := range files {
		rs, err := readFile(f)
		if err != nil {
			return nil, err
		}
		for i := range rs {
			recs = append(recs, pt.record(&rs[i]))
		}
	}
	of, p := pt.buckets(recs, size)
	st := &Stream{Records: len(recs), Joins: map[string]int{}, Skipped: len(p.Large), Partition: p}

	// The records of each bucket.
	ws := make([]*spillWriter, p.Buckets)
	for b := range ws {
		if ws[b], err = createSpill(name("records", b)); err != nil {
			return nil, err
		}
	}
	n := 0
	for _, f := range files {
		rs, err := readFile(f)
		if err != nil {
			return nil, err
		}
		for i := range rs {
			if n >= len(recs) {
				return nil, fmt.Errorf("cluster: %s changed in the run", f)
			}
			if err := ws[of[recs[n]]].write(&rs[i]); err != nil {
				return nil, err
			}
			n++
		}
	}
	for _, w := range ws {
		if err := w.close(); err != nil {
			return nil, err
		}
	}

	// The passes on each bucket. The last file holds the existing resources with no record.
	existBy := make([][]resource.Resource, p.Buckets+1)
	for i, n := range exist {
		b := int(of[n])
		if b < 0 {
			b = p.Buckets
		}
		existBy[b] = append(existBy[b], in.Existing[i])
	}
	linkBy := make([][]Link, p.Buckets)
	for i, n := range links {
		if b := of[n]; b >= 0 {
			linkBy[b] = append(linkBy[b], in.Crosswalks[i])
		}
	}
	type fresh struct {
		key       string
		bucket, n int
		id        string
	}
	var news []fresh
	for b := range p.Buckets + 1 {
		out := map[string]resource.Resource{}
		var fr map[string]string
		if b < p.Buckets {
			var rs []Record
			if err := readSpill(name("records", b), func(line []byte) error {
				var r Record
				rs = append(rs, r)
				return json.Unmarshal(line, &rs[len(rs)-1])
			}); err != nil {
				return nil, err
			}
			bin := in
			bin.Records, bin.Existing, bin.Crosswalks, bin.Large = rs, existBy[b], linkBy[b], p.Large
			g, _ := passes(bin)
			out, fr = groupResources(bin, g, false)
			for k, n := range g.joins {
				st.Joins[k] += n
			}
			st.Pairs = append(st.Pairs, g.pairs...)
			if err := os.Remove(name("records", b)); err != nil {
				return nil, err
			}
		}
		for _, e := range existBy[b] {
			if _, ok := out[e.ID]; !ok {
				out[e.ID] = e
			}
		}
		w, err := createSpill(name("resources", b))
		if err != nil {
			return nil, err
		}
		for k, id := range sortedKeys(out) {
			r := out[id]
			if err := w.write(&r); err != nil {
				return nil, err
			}
			if key, ok := fr[id]; ok {
				news = append(news, fresh{key: key, bucket: b, n: k})
			}
		}
		if err := w.close(); err != nil {
			return nil, err
		}
	}

	// The provisional ids, in the order of the smallest key of each new group, as one run gives them.
	slices.SortFunc(news, func(a, b fresh) int { return cmp.Compare(a.key, b.key) })
	used := map[string]bool{}
	for i := range news {
		news[i].id = provisional(news[i].key, used)
		news[i].key = ""
	}
	used = nil
	slices.SortFunc(news, func(a, b fresh) int { return cmp.Or(cmp.Compare(a.bucket, b.bucket), cmp.Compare(a.n, b.n)) })
	each := func(fn func(r *resource.Resource) error) error {
		c := 0
		for b := range p.Buckets + 1 {
			k := 0
			if err := readSpill(name("resources", b), func(line []byte) error {
				var r resource.Resource
				if err := json.Unmarshal(line, &r); err != nil {
					return err
				}
				if c < len(news) && news[c].bucket == b && news[c].n == k {
					r.ID = news[c].id
					c++
				}
				k++
				return fn(&r)
			}); err != nil {
				return err
			}
		}
		return nil
	}

	// The steps that read all the resources, as finish runs them.
	exprs := map[string]string{}
	if err := each(func(r *resource.Resource) error {
		offeringIndex(exprs, r)
		return nil
	}); err != nil {
		return nil, err
	}
	need, needLive := map[uint64]bool{}, map[uint64]bool{}
	if err := each(func(r *resource.Resource) error {
		offering(r, exprs)
		for _, rel := range r.Relations {
			if t, ok := target(rel); ok {
				need[keyHash(rel.To)] = true
				needLive[keyHash(t)] = true
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	live := map[uint64]bool{}
	byID := map[uint64]string{}
	const many = "\x00"
	if err := each(func(r *resource.Resource) error {
		if !r.Live() {
			return nil
		}
		if h := keyHash(r.ID); needLive[h] {
			live[h] = true
		}
		externalIDs(r, func(k string) {
			h := keyHash(k)
			if !need[h] {
				return
			}
			if cur, ok := byID[h]; ok && cur != r.ID {
				byID[h] = many
			} else {
				byID[h] = r.ID
			}
		})
		return nil
	}); err != nil {
		return nil, err
	}
	need, needLive = nil, nil
	gs := make([]*spillWriter, groupsOfShards)
	for i := range gs {
		if gs[i], err = createSpill(name("shards", i)); err != nil {
			return nil, err
		}
	}
	if err := each(func(r *resource.Resource) error {
		offering(r, exprs)
		access(r)
		resolve(r, func(id string) bool { return live[keyHash(id)] }, func(k string) []string {
			if id, ok := byID[keyHash(k)]; ok && id != many {
				return []string{id}
			}
			return nil
		})
		if ps := resource.Check(r); len(ps) > 0 && tag.IsProvisional(r.ID) {
			return fmt.Errorf("cluster: the new resource of %s has a problem: %s", r.Catalogue[0].NativeID, ps[0])
		}
		if r.Live() {
			st.Resources++
		}
		if len(r.ID) < 3 {
			return fmt.Errorf("resource: the id %q is too short", r.ID)
		}
		return gs[keyHash(resource.Shard(r.ID))%groupsOfShards].write(r)
	}); err != nil {
		return nil, err
	}
	for _, g := range gs {
		if err := g.close(); err != nil {
			return nil, err
		}
	}

	// The shards.
	keep := map[string]bool{}
	for i := range groupsOfShards {
		shards := map[string][]resource.Resource{}
		if err := readSpill(name("shards", i), func(line []byte) error {
			var r resource.Resource
			if err := json.Unmarshal(line, &r); err != nil {
				return err
			}
			shards[resource.Shard(r.ID)] = append(shards[resource.Shard(r.ID)], r)
			return nil
		}); err != nil {
			return nil, err
		}
		for _, k := range sortedKeys(shards) {
			if err := resource.WriteShard(root, k, shards[k]); err != nil {
				return nil, err
			}
			keep[k] = true
		}
	}
	if err := resource.RemoveShards(root, keep); err != nil {
		return nil, err
	}
	slices.SortFunc(st.Pairs, func(a, b Pair) int {
		return cmp.Or(cmp.Compare(a.Pass, b.Pass), cmp.Compare(a.A, b.A), cmp.Compare(a.B, b.B))
	})
	return st, nil
}

// spillWriter writes one JSON value on each line of a file compressed with gzip.
type spillWriter struct {
	f *os.File
	z *gzip.Writer
	w *bufio.Writer
}

func createSpill(path string) (*spillWriter, error) {
	f, err := os.Create(path) // #nosec G304 -- the path is in the spill directory of the run.
	if err != nil {
		return nil, err
	}
	z, err := gzip.NewWriterLevel(f, gzip.BestSpeed)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &spillWriter{f: f, z: z, w: bufio.NewWriter(z)}, nil
}

func (s *spillWriter) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := s.w.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *spillWriter) close() error {
	return errors.Join(s.w.Flush(), s.z.Close(), s.f.Close())
}

// readSpill calls fn with each line of a file of spillWriter.
func readSpill(path string, fn func([]byte) error) error {
	f, err := os.Open(path) // #nosec G304 -- the path is in the spill directory of the run.
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	z, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	r := bufio.NewReaderSize(z, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 1 {
			if err := fn(line[:len(line)-1]); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
