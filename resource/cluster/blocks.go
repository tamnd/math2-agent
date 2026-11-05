package cluster

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/resource/spill"
)

// blockFiles is the number of groups of the blocks of C4. All the blocks of one author go to one group, so that a group holds a block and the block of the next year.
const blockFiles = 128

// member is one record in one block of C4.
type member struct {
	Node  int32  `json:"n"`
	Block string `json:"b"`
	R     Record `json:"r"`
}

// blockSet collects the records of the blocks of C4 for the partition. Pass C4 joins two records of one block, or of the blocks of one author in two years that follow, only when their score is at or above Middle. So the partition joins only these pairs, and not all the records of a block. With a directory, the blocks go to spill files, and a group is in memory only while its pairs are scored.
type blockSet struct {
	dir string
	ws  []*spill.Writer
	mem [][]member
	err error
}

func newBlockSet(dir string) *blockSet {
	b := &blockSet{dir: dir}
	if dir == "" {
		b.mem = make([][]member, blockFiles)
	}
	return b
}

func (b *blockSet) file(n int) string {
	return filepath.Join(b.dir, fmt.Sprintf("blocks-%04d.gz", n))
}

// add puts the record of the node in the block, in the group of the key.
func (b *blockSet) add(node int32, group, block string, r *Record) {
	if b.err != nil {
		return
	}
	n := int(keyHash(group) % blockFiles)
	if b.dir == "" {
		b.mem[n] = append(b.mem[n], member{Node: node, Block: block, R: *r})
		return
	}
	if b.ws == nil {
		b.ws = make([]*spill.Writer, blockFiles)
		for i := range b.ws {
			if b.ws[i], b.err = spill.Create(b.file(i)); b.err != nil {
				return
			}
		}
	}
	b.err = b.ws[n].Write(member{Node: node, Block: block, R: *r})
}

// pairs calls fn for each pair of nodes that pass C4 can join or queue, one group at a time. It gives the hashes of the blocks with more than MaxBlock records.
func (b *blockSet) pairs(fn func(x, y int32)) (map[uint64]bool, error) {
	if b.err != nil {
		return nil, b.err
	}
	for _, w := range b.ws {
		if err := w.Close(); err != nil {
			return nil, err
		}
	}
	large := map[uint64]bool{}
	for n := range blockFiles {
		var ms []member
		if b.dir == "" {
			ms, b.mem[n] = b.mem[n], nil
		} else if b.ws != nil {
			if err := spill.Read(b.file(n), func(line []byte) error {
				var m member
				ms = append(ms, m)
				return json.Unmarshal(line, &ms[len(ms)-1])
			}); err != nil {
				return nil, err
			}
		}
		blocks := map[string][]int{}
		for i := range ms {
			blocks[ms[i].Block] = append(blocks[ms[i].Block], i)
		}
		compare := func(xs, ys []int, same bool) {
			for a, i := range xs {
				start := 0
				if same {
					start = a + 1
				}
				for _, j := range ys[start:] {
					if ms[i].Node != ms[j].Node && Score(&ms[i].R, &ms[j].R) >= Middle {
						fn(ms[i].Node, ms[j].Node)
					}
				}
			}
		}
		for k, xs := range blocks {
			if len(xs) > MaxBlock {
				large[keyHash(k)] = true
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
	}
	return large, nil
}
