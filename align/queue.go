package align

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zeebo/blake3"
)

// The edges of a proposal.
const (
	SameAs    = "same_as"   // the node is the record (spec 05.3)
	CloseTo   = "close_to"  // the node and the record are near, and not the same. The person who accepts it writes how they differ.
	Aligned   = "aligned"   // the record, a paper, states the node
	Duplicate = "duplicate" // two nodes can be the same node (spec 05.4). A person merges them or keeps them apart.
)

// QueueDir is the directory of the alignment queue in a math2 working tree. It has one file for each system, <system>.jsonl, with one proposal on each line.
const QueueDir = "queues/align"

// DedupQueue is the file of the dedup queue in a math2 working tree. The system of its proposals is math2, and the target of each is a second node.
const DedupQueue = "queues/dedup.jsonl"

// A Proposal is one edge that the aligner proposes and that a person must accept or reject. A queue file has the proposals in the order of the work: the most central nodes first, and then the highest scores.
//
// A proposal has no title or text of the record, because the licence of a source can forbid math2 to store them. The m2 align queue command reads them from the records.
type Proposal struct {
	ID         string             `json:"id"`                 // the hash of the node, the edge, the system and the target, in 12 hex digits
	Node       string             `json:"node"`               // the tag of the node
	Edge       string             `json:"edge"`               // SameAs, CloseTo, Aligned or Duplicate
	System     string             `json:"system"`             // the system of the target, for example oeis, or math2 for a duplicate
	Target     string             `json:"target"`             // the id in the system, the URI of the record of an aligned edge, or the tag of the second node
	Source     string             `json:"source"`             // the ingest source of the record
	Score      float64            `json:"score"`              // from 0 to 1
	Channels   map[string]float64 `json:"channels,omitempty"` // the score of each channel
	Centrality int                `json:"centrality"`         // the number of nodes that depend on the node, or on the two nodes of a duplicate
	Proposed   string             `json:"proposed"`           // the date of the first run that proposed the edge, YYYY-MM-DD
	By         string             `json:"by"`                 // the program that proposed the edge, for example m2/0.2.0 align
	Embedder   string             `json:"embedder,omitempty"` // the Embedder of the scores
	Records    string             `json:"records"`            // the hash of the records file of the source
}

// proposalID returns the ID of a proposal.
func proposalID(node, edge, system, target string) string {
	sum := blake3.Sum256([]byte(node + "\x00" + edge + "\x00" + system + "\x00" + target))
	return hex.EncodeToString(sum[:6])
}

// QueueFile returns the path of the queue file of a system, from the root of the working tree.
func QueueFile(system string) string {
	if system == "math2" {
		return DedupQueue
	}
	return QueueDir + "/" + system + ".jsonl"
}

// ReadQueue returns the proposals in all the queue files of the working tree at root, in the order of the files. A tree with no queue has no proposals.
func ReadQueue(root string) ([]Proposal, error) {
	files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(QueueDir), "*.jsonl"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	files = append(files, filepath.Join(root, filepath.FromSlash(DedupQueue)))
	var out []Proposal
	for _, f := range files {
		ps, err := readFile(f)
		if err != nil {
			return nil, err
		}
		out = append(out, ps...)
	}
	return out, nil
}

func readFile(path string) ([]Proposal, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the path is a queue file of the working tree.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Proposal
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 1<<20)
	for n := 1; sc.Scan(); n++ {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var p Proposal
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		out = append(out, p)
	}
	return out, sc.Err()
}

// writeFile writes the proposals to a queue file in the order of the work. It removes the file when there are no proposals.
func writeFile(path string, ps []Proposal) error {
	if len(ps) == 0 {
		err := os.Remove(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	slices.SortFunc(ps, func(a, b Proposal) int {
		return cmp.Or(cmp.Compare(b.Centrality, a.Centrality), cmp.Compare(b.Score, a.Score),
			cmp.Compare(a.Node, b.Node), cmp.Compare(a.Edge, b.Edge), cmp.Compare(a.Target, b.Target))
	})
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, p := range ps {
		if err := enc.Encode(p); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the queue is part of the working tree, which others read.
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644) // #nosec G306 -- the queue is part of the working tree, which others read.
}

// remove takes the proposal with the id out of its queue file.
func remove(root string, p Proposal) error {
	path := filepath.Join(root, filepath.FromSlash(QueueFile(p.System)))
	ps, err := readFile(path)
	if err != nil {
		return err
	}
	ps = slices.DeleteFunc(ps, func(q Proposal) bool { return q.ID == p.ID })
	return writeFile(path, ps)
}
