package graph

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/parquet-go/parquet-go"
)

// WriteParquet writes the build format of spec 05.7 to the directory dir: nodes/<kind>.parquet for each kind that has nodes, and edges/<family>.parquet for each of the three families. The rows are in the order of Nodes and Edges. The same graph gives the same bytes. WriteParquet returns the paths of the files from dir, in byte order.
func (g *Graph) WriteParquet(dir string) ([]string, error) {
	files := map[string][]byte{}
	kinds := map[string][]Node{}
	for _, n := range g.Nodes {
		kinds[n.Kind] = append(kinds[n.Kind], n)
	}
	for kind, rows := range kinds {
		b, err := encode(rows)
		if err != nil {
			return nil, err
		}
		files["nodes/"+kind+".parquet"] = b
	}
	for _, f := range []string{Logical, Conceptual, Record} {
		var rows []Edge
		for _, e := range g.Edges {
			if families[e.Type] == f {
				rows = append(rows, e)
			}
		}
		b, err := encode(rows)
		if err != nil {
			return nil, err
		}
		files["edges/"+f+".parquet"] = b
	}
	var paths []string
	for rel, b := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { // #nosec G301 -- the build format is public.
			return nil, err
		}
		if err := os.WriteFile(p, b, 0o644); err != nil { // #nosec G306 G703 -- the build format is public, and WriteParquet makes each path.
			return nil, err
		}
		paths = append(paths, rel)
	}
	slices.Sort(paths)
	return paths, nil
}

func encode[T any](rows []T) ([]byte, error) {
	var b bytes.Buffer
	if err := parquet.Write(&b, rows, parquet.CreatedBy("m2", "graph/1", "")); err != nil {
		return nil, fmt.Errorf("graph: write parquet: %w", err)
	}
	return b.Bytes(), nil
}
