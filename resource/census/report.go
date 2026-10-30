package census

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// ReportFile is the path of the report of the census, relative to the root of the tree.
const ReportFile = "reports/resources-census.md"

// WriteReport writes the report of the census as Markdown: the counts of each catalogue by outcome and by dimension, the overlap of each pair, the estimates of the independent pairs with their intervals, and the strata that no pair covers (RS15.2).
func (c *Census) WriteReport(w io.Writer, pairs [][2]string) error {
	var b strings.Builder
	cats := c.Catalogues()
	fmt.Fprintf(&b, "# Census of the catalogues of resources\n\n")
	fmt.Fprintf(&b, "The census applies the scope tests S1 to S7 (RS9.2) to each record. A record that no test decides and that has no class, venue or subject is deferred. The sample of in records of each catalogue has at most %d records, with the seed %d.\n\n", c.SampleSize, c.Seed)

	fmt.Fprintf(&b, "## Counts\n\n")
	fmt.Fprintf(&b, "| Catalogue | Records | Weight | In | Border | Out | Deferred | Total |\n|---|--:|--:|--:|--:|--:|--:|--:|\n")
	for _, cat := range cats {
		fmt.Fprintf(&b, "| %s | %d | %s |%s | %s |\n", cat.Name, cat.Records, fmt.Sprintf("%.2f", cat.Weight), tallyCells(cat.All), num(cat.All.Total()))
	}
	b.WriteString("\n")

	for _, dim := range Dims {
		fmt.Fprintf(&b, "## Counts by %s\n\n", dim)
		fmt.Fprintf(&b, "| Catalogue | %s | In | Border | Out | Deferred | Total |\n|---|---|--:|--:|--:|--:|--:|\n", dim)
		for _, cat := range cats {
			vals := make([]string, 0, len(cat.By[dim]))
			for v := range cat.By[dim] {
				vals = append(vals, v)
			}
			slices.Sort(vals)
			for _, v := range vals {
				t := cat.By[dim][v]
				fmt.Fprintf(&b, "| %s | %s |%s | %s |\n", cat.Name, v, tallyCells(t), num(t.Total()))
			}
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "## Overlap\n\n")
	fmt.Fprintf(&b, "Each row gives the share of the sample of in records of A that has a match in B. The match is a lookup of the DOI in the API of B, or else a strong id or the title and the first author in the records of B. A match in the records of B is corrected by the weight of B. A lookup tells that B has the work, not that B has it in scope.\n\n")
	fmt.Fprintf(&b, "| A | B | Match | Sampled | Matched | Share |\n|---|---|---|--:|--:|--:|\n")
	for _, o := range c.Overlaps() {
		how := "records"
		if o.Lookup {
			how = "lookup"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %.3f |\n", o.A, o.B, how, o.Sampled, o.Matched, o.Fraction(c.cats[o.B].Weight))
	}
	b.WriteString("\n")

	est := c.Estimates(pairs)
	fmt.Fprintf(&b, "## Estimates\n\n")
	if len(pairs) == 0 {
		b.WriteString("No pair of catalogues is named as independent, so the census gives no estimate.\n\n")
	} else {
		names := make([]string, len(pairs))
		for i, p := range pairs {
			names[i] = p[0] + " and " + p[1]
		}
		fmt.Fprintf(&b, "The pairs that are taken as independent are: %s. Each row gives the Chapman estimate of the in works of the stratum, its 95%% interval by a bootstrap of %d rounds over the samples of A, of B and of the match, and the recall of each catalogue. The count m is the in works of A that are in works of B. A lookup counts for m only when B is a catalogue of mathematics alone (%s), and else the sample of A is matched against the in records of B. A stratum needs %d records of the sample of A and %d matches for an estimate.\n\n", strings.Join(names, "; "), Bootstraps, c.wholeNames(), MinSampled, MinMatched)
		fmt.Fprintf(&b, "| A | B | Kind group | Decade | n A | n B | m | Estimate | Low | High | Recall A | Recall B |\n|---|---|---|---|--:|--:|--:|--:|--:|--:|--:|--:|\n")
		slices.SortStableFunc(est, func(x, y StratumEstimate) int {
			return cmp.Or(cmp.Compare(x.Stratum.Kind, y.Stratum.Kind), cmp.Compare(x.Stratum.Decade, y.Stratum.Decade))
		})
		for _, e := range est {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %d | %d | %s | %s | %s | %.3f | %.3f |\n", e.A, e.B, e.Stratum.Kind, e.Stratum.Decade, e.NA, e.NB, e.M, num(e.N), num(e.CI.Low), num(e.CI.High), e.RecallA, e.RecallB)
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "## Strata with no estimate\n\n")
	un := c.Uncovered(est)
	if len(un) == 0 {
		b.WriteString("Each stratum with in records has an estimate.\n")
	} else {
		b.WriteString("These strata have in records, but no independent pair has enough of them for an estimate.\n\n| Kind group | Decade |\n|---|---|\n")
		for _, s := range un {
			fmt.Fprintf(&b, "| %s | %s |\n", s.Kind, s.Decade)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func tallyCells(t Tally) string {
	var b strings.Builder
	for i, o := range Outcomes {
		if i > 0 {
			b.WriteString(" |")
		}
		b.WriteString(" " + num(t[o]))
	}
	return b.String()
}

// num writes a count with no decimals.
func num(f float64) string { return fmt.Sprintf("%.0f", f) }

// wholeNames gives the names of Whole, or none.
func (c *Census) wholeNames() string {
	var names []string
	for n, ok := range c.Whole {
		if ok {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// ReportRow is one row of the estimates of ReportFile.
type ReportRow struct {
	A, B         string
	Kind, Decade string
	N, Low, High float64
	Line         int // the line of the row in ReportFile
}

// ReadEstimates reads the rows of the estimates of ReportFile in the tree at root. A tree with no file has no rows.
func ReadEstimates(root string) ([]ReportRow, error) {
	data, err := os.ReadFile(filepath.Join(root, ReportFile)) // #nosec G304 -- the caller owns the tree.
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []ReportRow
	in := false
	for n, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## ") {
			in = line == "## Estimates"
			continue
		}
		if !in || !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| A |") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		if len(cells) != 12 {
			return nil, fmt.Errorf("%s:%d: an estimate has %d cells, and it must have 12", ReportFile, n+1, len(cells))
		}
		r := ReportRow{A: cells[0], B: cells[1], Kind: cells[2], Decade: cells[3], Line: n + 1}
		for i, f := range []*float64{&r.N, &r.Low, &r.High} {
			if *f, err = strconv.ParseFloat(cells[7+i], 64); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", ReportFile, n+1, err)
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// Target gives the target of the estimated recall of a stratum (RS1.7), or 0 when the stratum has no target.
func Target(kind, decade string) float64 {
	switch kind {
	case "article":
		if decade != "unknown" && decade >= "1990s" {
			return 0.99
		}
	case "book":
		if decade == "unknown" {
			return 0
		}
		if decade >= "1950s" {
			return 0.97
		}
		return 0.90
	case "thesis", "report-notes", "course":
		return 0.90
	}
	return 0
}
