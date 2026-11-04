package resource

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"

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
