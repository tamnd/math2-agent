package cites

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
)

// UnresolvedFile holds each reference that resolves to no resource, with the count of resolver runs that read it. Rule H14 reads it.
const UnresolvedFile = "build/resources/unresolved.md"

// Unresolved is one row of UnresolvedFile. Ref is the reference as <id>/e<n>#<n>.
type Unresolved struct {
	Ref   string `json:"ref"`
	Runs  int    `json:"runs"`
	Since string `json:"since"` // the day of the first run that did not resolve it
}

// From gives the citing expression of the reference.
func (u Unresolved) From() string {
	from, _, _ := strings.Cut(u.Ref, "#")
	return from
}

// ReadUnresolved reads UnresolvedFile in the tree at root. A tree with no file gives no row.
func ReadUnresolved(root string) (map[string]Unresolved, error) {
	recs, err := mdrec.Read(filepath.Join(root, UnresolvedFile))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Unresolved{}, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := mdrec.Unmarshal[Unresolved](recs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", UnresolvedFile, err)
	}
	out := make(map[string]Unresolved, len(rows))
	for _, u := range rows {
		out[u.Ref] = u
	}
	return out, nil
}

// WriteUnresolved writes the rows to UnresolvedFile in the tree at root, sorted by Ref.
func WriteUnresolved(root string, rows map[string]Unresolved) error {
	list := make([]Unresolved, 0, len(rows))
	for _, k := range slices.Sorted(maps.Keys(rows)) {
		list = append(list, rows[k])
	}
	recs, err := mdrec.Marshal(list)
	if err != nil {
		return err
	}
	path := filepath.Join(root, UnresolvedFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return mdrec.Write(path, recs)
}

// CountRuns adds one run to the rows. The rows of the citing expressions of done are replaced: an edge with no target adds one to the count of its reference, and a resolved reference leaves the file. The other rows stay.
func CountRuns(old map[string]Unresolved, edges []resource.Cite, done map[string]bool, day string) map[string]Unresolved {
	out := map[string]Unresolved{}
	for k, u := range old {
		if !done[u.From()] {
			out[k] = u
		}
	}
	for _, c := range edges {
		if c.To != "" {
			continue
		}
		ref := Target(c.From, c.N)
		u, ok := old[ref]
		if !ok {
			u = Unresolved{Ref: ref, Since: day}
		}
		u.Runs++
		out[ref] = u
	}
	return out
}
