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
	"strconv"
	"strings"
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
	// Areas is the counts of each MSC code at two digits. ax census report writes them from v0.5.22 on.
	Areas map[string]censusArea `json:"areas"`
}

// censusArea is the part of the census counts of one area that the status reads.
type censusArea struct {
	Scope     int `json:"scope"`
	Extracted int `json:"extracted"`
	Stopped   int `json:"stopped"`
	NoState   int `json:"no_state"`
	Gaps      int `json:"gaps"`
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
	Formal   map[string]int `json:"formal,omitempty"`   // these nodes by formal outcome of K2.5, from the last build
	Complete bool           `json:"complete"`           // the three numbers are zero and the scope is counted
	Areas    []AreaStatus   `json:"areas"`              // the same numbers for each MSC code at two digits
	Problems []string       `json:"problems,omitempty"` // the inputs that were missing
}

// AreaStatus is the status of the research layer in one MSC code at two digits (AX-M8). The code of a row is its first MSC code. The census gives the code of a paper from its MSC class, or its arXiv category when it has no MSC class, so an area can have papers and no rows, or rows and no papers.
type AreaStatus struct {
	Code     string         `json:"code"`
	Scope    int            `json:"scope"`
	NoState  int            `json:"no_state"`
	Gaps     int            `json:"gaps"`
	Rows     int            `json:"rows"`
	Open     int            `json:"open"`
	Nodes    int            `json:"nodes"`
	Levels   map[string]int `json:"levels,omitempty"`
	Formal   map[string]int `json:"formal,omitempty"`
	Complete bool           `json:"complete"`
}

// ResearchReport is the report of the research layer in tamnd/math2 (corpus arxiv AX11.4).
const ResearchReport = "reports/research-layer.md"

// researchStatus prints the three numbers of AX1.5, the outcomes of the research rows, and the writing levels and the formal outcomes of their nodes, in total and for each MSC code at two digits. With write, it writes them to reports/research-layer.md in the tree.
func researchStatus(root, corpus string, rows []inventory.Row, asJSON, write bool, stdout, stderr io.Writer) int {
	if corpus == "" {
		fmt.Fprintln(stderr, "m2 corpus status: -layer research needs -corpus, the checkout of tamnd/arxiv")
		return ExitUsage
	}
	st, err := computeResearch(root, corpus, rows, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
		return ExitFailure
	}
	if write {
		path := filepath.Join(root, filepath.FromSlash(ResearchReport))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- the reports are public.
			fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
			return ExitFailure
		}
		if err := os.WriteFile(path, []byte(researchMarkdown(st)), 0o644); err != nil { // #nosec G306 -- the reports are public.
			fmt.Fprintf(stderr, "m2 corpus status: %v\n", err)
			return ExitFailure
		}
	}
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
	for _, k := range slices.Sorted(maps.Keys(st.Formal)) {
		fmt.Fprintf(tw, "formal %s\t%d\n", k, st.Formal[k])
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

// computeResearch reads the census summary of the checkout corpus, the rows, and the writing levels and the formal outcomes of the last build of the tree at root.
func computeResearch(root, corpus string, rows []inventory.Row, day time.Time) (*ResearchStatus, error) {
	st := &ResearchStatus{Outcomes: map[string]int{}, Areas: []AreaStatus{}}
	areas := map[string]*AreaStatus{}
	area := func(code string) *AreaStatus {
		a := areas[code]
		if a == nil {
			a = &AreaStatus{Code: code}
			areas[code] = a
		}
		return a
	}
	data, err := os.ReadFile(filepath.Join(corpus, CensusFile)) // #nosec G304 -- the caller names the checkout.
	switch {
	case errors.Is(err, fs.ErrNotExist):
		st.Problems = append(st.Problems, "the checkout has no "+CensusFile)
	case err != nil:
		return nil, err
	default:
		var c censusSummary
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", CensusFile, err)
		}
		st.Census, st.Counted, st.Scope, st.Done, st.Stopped, st.NoState, st.Gaps = c.Generated, c.ScopeCounted, c.Scope, c.Extracted, c.Stopped, c.NoState, c.Gaps
		if !c.ScopeCounted {
			st.Problems = append(st.Problems, "the census did not count the scope, so the papers with no state are not known")
		}
		if c.Areas == nil {
			st.Problems = append(st.Problems, "the census has no counts for each area, which ax census report writes from v0.5.22 on")
		}
		for code, x := range c.Areas {
			a := area(code)
			a.Scope, a.NoState, a.Gaps = x.Scope, x.NoState, x.Gaps
		}
	}
	tags := map[string]string{} // the area of each node
	for i := range rows {
		r := &rows[i]
		if r.Layer != inventory.Research {
			continue
		}
		code := "unknown"
		if len(r.MSC) > 0 && len(r.MSC[0]) >= 2 {
			code = r.MSC[0][:2]
		}
		a := area(code)
		st.Rows++
		a.Rows++
		if r.IsOpen(day) {
			st.Open++
			a.Open++
		}
		if r.Outcome == nil {
			st.Outcomes["open"]++
			continue
		}
		st.Outcomes[r.Outcome.Value]++
		if r.Outcome.Value == inventory.OutcomeNode {
			for _, t := range r.Outcome.Target {
				if _, ok := tags[t]; !ok {
					tags[t] = code
					a.Nodes++
				}
			}
		}
	}
	st.Nodes = len(tags)
	if st.Nodes > 0 {
		levels, err := readLevels(root)
		if err != nil {
			return nil, err
		}
		if levels == nil {
			st.Problems = append(st.Problems, "the tree has no build/"+build.WritingFile+", so the writing levels are not known")
		} else {
			st.Levels = map[string]int{}
			for t, code := range tags {
				if l, ok := levels[t]; ok {
					st.Levels[l]++
					a := area(code)
					if a.Levels == nil {
						a.Levels = map[string]int{}
					}
					a.Levels[l]++
				} else {
					st.Unbuilt++
				}
			}
		}
		formal, err := build.ReadFormal(filepath.Join(root, "build"))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			st.Problems = append(st.Problems, "the tree has no build/"+build.FormalFile+", so the formal outcomes are not known")
		case err != nil:
			return nil, err
		default:
			outcome := map[string]string{}
			for _, n := range formal.Nodes {
				outcome[n.Tag] = n.Outcome
			}
			st.Formal = map[string]int{}
			for t, code := range tags {
				o, ok := outcome[t]
				if !ok {
					// A node of a kind that gets no formal outcome, or a node that the last build did not have.
					continue
				}
				st.Formal[o]++
				a := area(code)
				if a.Formal == nil {
					a.Formal = map[string]int{}
				}
				a.Formal[o]++
			}
		}
	}
	st.Complete = st.Census != "" && st.Counted && st.NoState == 0 && st.Gaps == 0 && st.Open == 0
	for _, code := range slices.Sorted(maps.Keys(areas)) {
		a := areas[code]
		a.Complete = st.Census != "" && st.Counted && a.NoState == 0 && a.Gaps == 0 && a.Open == 0
		st.Areas = append(st.Areas, *a)
	}
	return st, nil
}

// researchMarkdown is reports/research-layer.md.
func researchMarkdown(st *ResearchStatus) string {
	var b strings.Builder
	known := func(ok bool, n int) string {
		if !ok {
			return "not known"
		}
		return strconv.Itoa(n)
	}
	b.WriteString("# The research layer\n\n")
	b.WriteString("`m2 corpus status -layer research -write` writes this report. Do not edit it.\n\n")
	b.WriteString("The research layer is complete only when three numbers are zero: the papers in scope with no state, the census gaps, and the open rows (corpus arxiv AX1.5). The open count alone can be zero while many papers are not read.\n\n")
	census := st.Census
	if census == "" {
		census = "none"
	}
	fmt.Fprintf(&b, "- Census: %s\n", census)
	fmt.Fprintf(&b, "- Papers in scope: %s\n", known(st.Counted, st.Scope))
	fmt.Fprintf(&b, "- Papers with no state: %s\n", known(st.Counted, st.NoState))
	fmt.Fprintf(&b, "- Census gaps: %s\n", known(st.Census != "", st.Gaps))
	fmt.Fprintf(&b, "- Rows: %d, of which %d are open\n", st.Rows, st.Open)
	fmt.Fprintf(&b, "- Nodes: %d\n", st.Nodes)
	fmt.Fprintf(&b, "- Complete: %s\n\n", map[bool]string{true: "yes", false: "no"}[st.Complete])

	b.WriteString("## Outcomes of the rows\n\n| Outcome | Rows |\n|---|---|\n")
	for _, k := range slices.Sorted(maps.Keys(st.Outcomes)) {
		fmt.Fprintf(&b, "| %s | %d |\n", k, st.Outcomes[k])
	}
	b.WriteString("\n## Nodes\n\nThe writing levels are W0 to W3 (corpus K8.8). The formal outcomes are those of corpus K2.5. A node gets a formal outcome when it is a statement, a definition, a conjecture or a problem.\n\n")
	b.WriteString("| | Nodes |\n|---|---|\n")
	for _, k := range slices.Sorted(maps.Keys(st.Levels)) {
		fmt.Fprintf(&b, "| level %s | %d |\n", k, st.Levels[k])
	}
	if st.Unbuilt > 0 {
		fmt.Fprintf(&b, "| level not built | %d |\n", st.Unbuilt)
	}
	for _, k := range slices.Sorted(maps.Keys(st.Formal)) {
		fmt.Fprintf(&b, "| formal %s | %d |\n", k, st.Formal[k])
	}
	b.WriteString("\n## By area\n\nThe area of a row is its first MSC code at two digits. The census gives the area of a paper from its MSC class, or from its arXiv category when the paper has no MSC class.\n\n")
	b.WriteString("| Area | Scope | No state | Census gaps | Rows | Open rows | Nodes | Levels | Formal outcomes | Complete |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, a := range st.Areas {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %d | %d | %s | %s | %s |\n", a.Code, known(st.Counted, a.Scope), known(st.Counted, a.NoState), known(st.Census != "", a.Gaps), a.Rows, a.Open, a.Nodes, counts(a.Levels), counts(a.Formal), map[bool]string{true: "yes", false: "no"}[a.Complete])
	}
	if len(st.Problems) > 0 {
		b.WriteString("\n## Inputs that are missing\n\n")
		for _, p := range st.Problems {
			fmt.Fprintf(&b, "- %s\n", strings.ToUpper(p[:1])+p[1:]+".")
		}
	}
	return b.String()
}

// counts writes a map of counts as "a 1, b 2", in the order of the keys, or "-" when it is empty.
func counts(m map[string]int) string {
	if len(m) == 0 {
		return "-"
	}
	var parts []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(parts, ", ")
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
