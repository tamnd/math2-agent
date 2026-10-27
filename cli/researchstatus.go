package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"text/tabwriter"
	"time"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/inventory"
)

// CensusFile is the summary of the census in a checkout of tamnd/arxiv, as ax census report writes it.
const CensusFile = "reports/census.json"

// censusSummary is the part of the census summary that the status reads.
type censusSummary struct {
	Generated    string `json:"generated"`
	ScopeCounted bool   `json:"scope_counted"`
	Scope        int    `json:"scope"`
	Extracted    int    `json:"extracted"`
	Stopped      int    `json:"stopped"`
	NoState      int    `json:"no_state"`
	Gaps         int    `json:"gaps"`
}

// ResearchStatus is the status of the research layer (corpus arxiv AX1.5 and AX12.6).
type ResearchStatus struct {
	Census   string         `json:"census"`             // the date of the census summary, or "" when the checkout has none
	Counted  bool           `json:"scope_counted"`      // false when the census did not count the scope, so NoState is not known
	Scope    int            `json:"scope"`              // the papers in the scope
	Done     int            `json:"extracted"`          // the papers with a census record
	Stopped  int            `json:"stopped"`            // the papers with a stop record
	NoState  int            `json:"no_state"`           // the papers in the scope with no record
	Gaps     int            `json:"gaps"`               // the census gaps
	Rows     int            `json:"rows"`               // the rows of the layer
	Open     int            `json:"open"`               // the open rows of the layer
	Outcomes map[string]int `json:"outcomes"`           // the rows of the layer by outcome
	Nodes    int            `json:"nodes"`              // the nodes that a row of the layer accounts for
	Levels   map[string]int `json:"levels,omitempty"`   // these nodes by writing level, from the last build
	Unbuilt  int            `json:"unbuilt,omitempty"`  // these nodes that the last build has no level for
	Complete bool           `json:"complete"`           // the three numbers are zero and the scope is counted
	Problems []string       `json:"problems,omitempty"` // the inputs that were missing
}

// researchStatus prints the three numbers of AX1.5, the outcomes of the research rows and the writing levels of their nodes.
func researchStatus(root, corpus string, rows []inventory.Row, asJSON bool, stdout, stderr io.Writer) int {
	if corpus == "" {
		fmt.Fprintln(stderr, "m2 corpus status: -layer research needs -corpus, the checkout of tamnd/arxiv")
		return ExitUsage
	}
	st := ResearchStatus{Outcomes: map[string]int{}}
	data, err := os.ReadFile(filepath.Join(corpus, CensusFile)) // #nosec G304 -- the caller names the checkout.
	switch {
	case errors.Is(err, fs.ErrNotExist):
		st.Problems = append(st.Problems, "the checkout has no "+CensusFile)
	case err != nil:
		fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
		return ExitFailure
	default:
		var c censusSummary
		if err := json.Unmarshal(data, &c); err != nil {
			fmt.Fprintf(stderr, "m2 corpus status: %s: %v\n", CensusFile, err)
			return ExitFailure
		}
		st.Census, st.Counted, st.Scope, st.Done, st.Stopped, st.NoState, st.Gaps = c.Generated, c.ScopeCounted, c.Scope, c.Extracted, c.Stopped, c.NoState, c.Gaps
		if !c.ScopeCounted {
			st.Problems = append(st.Problems, "the census did not count the scope, so the papers with no state are not known")
		}
	}
	day := time.Now()
	tags := map[string]bool{}
	for i := range rows {
		r := &rows[i]
		if r.Layer != inventory.Research {
			continue
		}
		st.Rows++
		if r.IsOpen(day) {
			st.Open++
		}
		if r.Outcome == nil {
			st.Outcomes["open"]++
			continue
		}
		st.Outcomes[r.Outcome.Value]++
		if r.Outcome.Value == inventory.OutcomeNode {
			for _, t := range r.Outcome.Target {
				tags[t] = true
			}
		}
	}
	st.Nodes = len(tags)
	if st.Nodes > 0 {
		levels, err := readLevels(root)
		if err != nil {
			fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
			return ExitFailure
		}
		if levels == nil {
			st.Problems = append(st.Problems, "the tree has no build/"+build.WritingFile+", so the writing levels are not known")
		} else {
			st.Levels = map[string]int{}
			for t := range tags {
				if l, ok := levels[t]; ok {
					st.Levels[l]++
				} else {
					st.Unbuilt++
				}
			}
		}
	}
	st.Complete = st.Census != "" && st.Counted && st.NoState == 0 && st.Gaps == 0 && st.Open == 0
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(st); err != nil {
			fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
			return ExitFailure
		}
		return ExitOK
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "layer\t%s\n", inventory.Research)
	if st.Census != "" {
		fmt.Fprintf(tw, "census\t%s\n", st.Census)
	}
	if st.Counted {
		fmt.Fprintf(tw, "scope\t%d\n", st.Scope)
		fmt.Fprintf(tw, "papers with no state\t%d\n", st.NoState)
	} else {
		fmt.Fprintf(tw, "papers with no state\tnot known\n")
	}
	if st.Census != "" {
		fmt.Fprintf(tw, "extracted\t%d\n", st.Done)
		fmt.Fprintf(tw, "stopped\t%d\n", st.Stopped)
		fmt.Fprintf(tw, "census gaps\t%d\n", st.Gaps)
	} else {
		fmt.Fprintf(tw, "census gaps\tnot known\n")
	}
	fmt.Fprintf(tw, "rows\t%d\n", st.Rows)
	fmt.Fprintf(tw, "open rows\t%d\n", st.Open)
	for _, k := range slices.Sorted(maps.Keys(st.Outcomes)) {
		fmt.Fprintf(tw, "outcome %s\t%d\n", k, st.Outcomes[k])
	}
	fmt.Fprintf(tw, "nodes\t%d\n", st.Nodes)
	for _, k := range slices.Sorted(maps.Keys(st.Levels)) {
		fmt.Fprintf(tw, "level %s\t%d\n", k, st.Levels[k])
	}
	if st.Unbuilt > 0 {
		fmt.Fprintf(tw, "level not built\t%d\n", st.Unbuilt)
	}
	fmt.Fprintf(tw, "complete\t%t\n", st.Complete)
	if err := tw.Flush(); err != nil {
		return ExitFailure
	}
	for _, p := range st.Problems {
		fmt.Fprintf(stderr, "m2 corpus status: %s\n", p)
	}
	return ExitOK
}

// readLevels returns the writing level of each node from build/writing.json of the tree, or nil when the tree has no such file.
func readLevels(root string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "build", build.WritingFile)) // #nosec G304 -- the path is in the tree.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var w build.Levels
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("%s: %w", build.WritingFile, err)
	}
	out := map[string]string{}
	for _, r := range w.Nodes {
		out[r.Tag] = r.Level.String()
	}
	return out, nil
}
