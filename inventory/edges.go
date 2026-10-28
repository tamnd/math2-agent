package inventory

import (
	"cmp"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
)

// EdgesDir is the directory of the edges between mentions, relative to the root of the tree.
const EdgesDir = "records/edges"

// Edge is one edge from a mention to a target in the record plane (corpus wikipedia WP6.4). The record holds no article text, so it has no link label.
type Edge struct {
	From       string `json:"from"`              // the mention id of the source
	To         string `json:"to"`                // the mention id of the target, the page id form of a page out of the scope, or the title of a category or a template
	Predicate  string `json:"predicate"`         // WP6.3
	Method     string `json:"method"`            // WP6.3
	Confidence string `json:"confidence"`        // WP6.3
	Span       [2]int `json:"span"`              // the byte span of the link in the revision of the source
	Revision   int    `json:"revision"`          // the revision of record of the source
	Finding    string `json:"finding,omitempty"` // a finding of the resolution, as broken-anchor
	By         string `json:"by"`                // the importer and its version
}

// WriteEdges writes the edges of the source to records/edges/<source>/ in the tree at root, in shards by the source mention. It replaces the edges of the last import of the source.
func WriteEdges(root, source string, es []Edge) error {
	shards := map[string][]Edge{}
	for _, e := range es {
		k := MentionShard(e.From)
		shards[k] = append(shards[k], e)
	}
	files := map[string][]byte{}
	for k, part := range shards {
		slices.SortFunc(part, func(a, b Edge) int {
			return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.Span[0], b.Span[0]), cmp.Compare(a.To, b.To), cmp.Compare(a.Predicate, b.Predicate))
		})
		data, err := encodeLines(part)
		if err != nil {
			return err
		}
		files[k+".jsonl"] = data
	}
	return replaceDir(filepath.Join(root, EdgesDir, source), files)
}

// ReadEdges reads the edges of the source from the tree at root. A tree with no edges gives no error.
func ReadEdges(root, source string) ([]Edge, error) {
	var out []Edge
	err := readLines(filepath.Join(root, EdgesDir, source), func(_ string, _ int, line []byte) error {
		var e Edge
		if err := json.Unmarshal(line, &e); err != nil {
			return fmt.Errorf("an edge: %w", err)
		}
		out = append(out, e)
		return nil
	})
	return out, err
}
