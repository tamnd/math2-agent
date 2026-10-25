// Package coverage measures how much of mathematics math2 covers, for each MSC2020 code (spec 18).
//
// Coverage has five levels on two planes (spec 18.1). C0 (recorded) and C1 (indexed) are on the record plane. C2 (mapped), C3 (standard) and C4 (formal) are on the statement plane. Each level is a share from 0 to 1.
//
// C0 is the share of the documents of an area that have a record in the record plane. The number of documents comes from a pinned count of zbMATH Open in manifests/coverage/expected.yaml. The count is for the top-level codes, thus C0 has a value for these codes only. C1 is the share of the same documents that have extracted results. M2 extracts no results (spec 19), thus C1 is 0 where C0 has a value. C2 to C4 are shares of the core of an area (spec 18.3). The cores are not written yet, thus these levels have no value.
//
// Compute gives one row for each code of the topic map. A record or a node counts once for each of its codes and once for each code above them. WriteParquet writes the rows, and Markdown writes a summary of the rows.
package coverage

import (
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/msc"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

// Dir is the directory of the coverage reports in the math2 tree.
const Dir = "reports/coverage"

// Input is what Compute measures.
type Input struct {
	Map      *msc.Map                   // the topic map, which gives the codes
	Nodes    []*node.Node               // the nodes, with their MSC codes in the msc field
	Status   []status.Status            // the status of each node. A node with no status has no level.
	Records  map[string][]ingest.Record // the records of each source, by the id of the source. The codes of a record are in Meta["msc"].
	Expected *Expected                  // the count of the documents of each class. Nil means that no count is known.
}

// Row is the coverage of one MSC2020 code.
type Row struct {
	Code  string `parquet:"code"`
	Label string `parquet:"label"`
	Level int32  `parquet:"level"` // 1, 2 or 3, as msc.Level

	// The record plane.
	Records  int64 `parquet:"records"`  // the records of all sources with a code in the area. A document can have a record in more than one source.
	Recorded int64 `parquet:"recorded"` // the records of the counted source with a code in the area
	Expected int64 `parquet:"expected"` // the documents of the counted source in the area, or 0 when the count is not known
	Results  int64 `parquet:"results"`  // the results extracted from the records

	// The statement plane. The counts are of live nodes.
	Nodes       int64 `parquet:"nodes"`
	Statements  int64 `parquet:"statements"`
	Conjectures int64 `parquet:"conjectures"` // the conjectures and the problems
	E2          int64 `parquet:"e2"`          // the nodes at E2 or above
	E5          int64 `parquet:"e5"`          // the nodes at E5 with F2 or above

	// The levels. Nil means that the level has no value for the code.
	C0 *float64 `parquet:"c0,optional"`
	C1 *float64 `parquet:"c1,optional"`
	C2 *float64 `parquet:"c2,optional"`
	C3 *float64 `parquet:"c3,optional"`
	C4 *float64 `parquet:"c4,optional"`
}

// Compute returns the coverage of each code of the map, in the order of the map.
func Compute(in Input) []Row {
	codes := in.Map.Codes()
	rows := make([]Row, len(codes))
	at := make(map[string]*Row, len(codes))
	for i, c := range codes {
		e, _ := in.Map.Lookup(c)
		rows[i] = Row{Code: c, Label: e.Label, Level: int32(msc.Level(c))} // #nosec G115 -- the level is 1, 2 or 3.
		at[c] = &rows[i]
	}

	counted := ""
	if in.Expected != nil {
		counted = in.Expected.Source
	}
	for src, recs := range in.Records {
		for _, r := range recs {
			for _, c := range areas(in.Map, strings.Fields(r.Meta["msc"])) {
				at[c].Records++
				if src == counted {
					at[c].Recorded++
				}
			}
		}
	}

	levels := make(map[string]status.Status, len(in.Status))
	for _, s := range in.Status {
		levels[s.Tag] = s
	}
	for _, n := range in.Nodes {
		s, ok := levels[n.ID]
		if s.Retired || n.Meta["retired"] != nil {
			continue
		}
		for _, c := range areas(in.Map, n.List("msc")) {
			r := at[c]
			r.Nodes++
			switch n.Kind() {
			case "statement":
				r.Statements++
			case "conjecture", "problem":
				r.Conjectures++
			}
			if ok && s.Evidence >= status.Standard {
				r.E2++
			}
			if ok && s.Evidence == status.Kernel && s.Fidelity >= status.F2 {
				r.E5++
			}
		}
	}

	for i := range rows {
		r := &rows[i]
		if in.Expected != nil && r.Level == 1 {
			r.Expected = int64(in.Expected.Counts[r.Code])
		}
		if r.Expected > 0 {
			r.C0 = share(r.Recorded, r.Expected)
			r.C1 = share(r.Results, r.Expected)
		}
	}
	return rows
}

// areas returns the codes of the map that the codes name, with the codes above them, each once. A code that is not in the map, such as a code of MSC2010, counts for the nearest code above it that is in the map.
func areas(m *msc.Map, codes []string) []string {
	var out []string
	for _, c := range codes {
		for ; c != ""; c = msc.Parent(c) {
			if _, ok := m.Lookup(c); ok && !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
	}
	return out
}

// share returns n/d, at most 1.
func share(n, d int64) *float64 {
	f := min(float64(n)/float64(d), 1)
	return &f
}
