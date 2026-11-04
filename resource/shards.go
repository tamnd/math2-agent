package resource

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource/spill"
)

// shardGroups is the number of spill files of a ShardSpill.
const shardGroups = 256

// ShardSpill holds resources in spill files, one file for each group of shards, until Write puts them into the shards of records/resources/. It keeps the resources of one group in memory at one time, so that a run can write more resources than the memory holds.
type ShardSpill struct {
	dir string
	ws  []*spill.Writer
}

// NewShardSpill makes the spill files in dir, which must exist and belong to the run.
func NewShardSpill(dir string) (*ShardSpill, error) {
	s := &ShardSpill{dir: dir, ws: make([]*spill.Writer, shardGroups)}
	for i := range s.ws {
		w, err := spill.Create(s.file(i))
		if err != nil {
			return nil, err
		}
		s.ws[i] = w
	}
	return s, nil
}

func (s *ShardSpill) file(i int) string {
	return filepath.Join(s.dir, fmt.Sprintf("shards-%04d.gz", i))
}

// Add adds one resource.
func (s *ShardSpill) Add(r *Resource) error {
	if len(r.ID) < 3 {
		return fmt.Errorf("resource: the id %q is too short", r.ID)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(Shard(r.ID)))
	return s.ws[h.Sum32()%shardGroups].Write(r)
}

// Write writes the resources to the shards of records/resources/ in the tree at root, removes the other shards and removes the spill files. It gives the number of resources.
func (s *ShardSpill) Write(root string) (int, error) {
	for _, w := range s.ws {
		if err := w.Close(); err != nil {
			return 0, err
		}
	}
	keep := map[string]bool{}
	n := 0
	for i := range s.ws {
		shards := map[string][]Resource{}
		if err := spill.Read(s.file(i), func(line []byte) error {
			var r Resource
			if err := json.Unmarshal(line, &r); err != nil {
				return err
			}
			shards[Shard(r.ID)] = append(shards[Shard(r.ID)], r)
			n++
			return nil
		}); err != nil {
			return 0, err
		}
		for k, rs := range shards {
			if err := WriteShard(root, k, rs); err != nil {
				return 0, err
			}
			keep[k] = true
		}
		if err := os.Remove(s.file(i)); err != nil {
			return 0, err
		}
	}
	return n, RemoveShards(root, keep)
}

// EachShard reads each shard of records/resources/ in the tree at root and calls fn with its resources. When fn reports a change, it writes the shard again, as a Markdown file. It keeps one shard in memory at one time. fn must not change an id.
func EachShard(root string, fn func(rs []Resource) (bool, error)) error {
	files, err := mdrec.Glob(filepath.Join(root, Dir, "*"+mdrec.Ext))
	if err != nil {
		return err
	}
	for _, path := range files {
		name := filepath.Base(path)
		if !IsShard(name) {
			continue
		}
		recs, err := mdrec.Read(path)
		if err != nil {
			return err
		}
		rs := make([]Resource, len(recs))
		for n, rec := range recs {
			if rs[n], err = Parse(rec); err != nil {
				return fmt.Errorf("%s: record %d: %w", filepath.ToSlash(filepath.Join(Dir, name)), n+1, err)
			}
		}
		changed, err := fn(rs)
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		data, err := lines(rs)
		if err != nil {
			return err
		}
		// Glob gives the Markdown name of a shard that is still JSON Lines, so the write puts the shard into Markdown.
		if err := writeFile(path, data); err != nil {
			return err
		}
		if err := os.Remove(strings.TrimSuffix(path, mdrec.Ext) + mdrec.Old); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
