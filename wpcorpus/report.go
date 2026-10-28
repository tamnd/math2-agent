package wpcorpus

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
)

// CensusReportPath is the completeness report of WP4.9.
var CensusReportPath = filepath.Join("reports", "census.md")

// CensusArticle is one article in scope with its records, the input of the
// census report.
type CensusArticle struct {
	Page    int
	Title   string
	Type    string
	State   string
	Meta    *Metadata // nil when the article is not fetched
	Census  *Census   // nil when the article is not parsed
	Objects []Object
}

// ReadCensusArticles reads the scope list, the stops, and the metadata, the
// census and the objects of each article in scope. The state of each article
// is the state of WP1.8 that the files give.
func ReadCensusArticles(root string) ([]*CensusArticle, error) {
	cs, err := ReadScopeFile(root, ScopePath)
	if err != nil {
		return nil, err
	}
	stops, err := ReadStops(root)
	if err != nil {
		return nil, err
	}
	stopped := map[int64]bool{}
	for _, s := range stops {
		stopped[s.PageID] = true
	}
	metas, err := ReadMetadataFiles(root)
	if err != nil {
		return nil, err
	}
	byPage := map[int]*Metadata{}
	for _, m := range metas {
		byPage[m.Page] = m
	}
	var out []*CensusArticle
	for _, c := range cs {
		if !c.InScope {
			continue
		}
		a := &CensusArticle{Page: int(c.PageID), Title: c.Title, State: StateListed, Meta: byPage[int(c.PageID)]}
		switch {
		case stopped[c.PageID]:
			a.State = StateStopped
		case a.Meta != nil:
			a.State = StateFetched
			a.Census, err = ReadCensus(root, a.Page, a.Meta.Revision)
			if errors.Is(err, fs.ErrNotExist) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", CensusPath(a.Page, a.Meta.Revision), err)
			}
			if a.Objects, err = ReadObjects(root, a.Page, a.Meta.Revision); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("%s: %w", ObjectsPath(a.Page, a.Meta.Revision), err)
			}
			a.State = StateParsed
		}
		a.Type = typeOf(a)
		out = append(out, a)
	}
	return out, nil
}

// IsItem tells if an object is an item, and not a part of a section such as
// a citation or an infobox.
func IsItem(o *Object) bool {
	return !partKinds[o.Kind]
}

// tally is one row of a table of the report.
type tally struct {
	inScope, parsed, objects, items, gaps int
	strength                              map[string]int
}

func (t *tally) add(a *CensusArticle) {
	t.inScope++
	if a.Census == nil {
		return
	}
	t.parsed++
	t.objects += len(a.Objects)
	for i := range a.Objects {
		if IsItem(&a.Objects[i]) {
			t.items++
		}
	}
	t.gaps += len(a.Census.Gaps()) + len(a.Census.PointerGaps())
	if t.strength == nil {
		t.strength = map[string]int{}
	}
	t.strength[a.Census.Strength]++
}

// useCount is a count of uses and of the articles that have them.
type useCount struct {
	uses     int
	articles []string
}

func (c *useCount) add(title string, n int) {
	c.uses += n
	if len(c.articles) == 0 || c.articles[len(c.articles)-1] != title {
		c.articles = append(c.articles, title)
	}
}

// examples gives up to three titles of a count.
func (c *useCount) examples() string {
	n := min(3, len(c.articles))
	return strings.Join(c.articles[:n], ", ")
}

// byUses gives the keys of the counts with the most uses first.
func byUses(m map[string]*useCount) []string {
	keys := sortedKeys(m)
	sort.SliceStable(keys, func(i, j int) bool { return m[keys[i]].uses > m[keys[j]].uses })
	return keys
}

func bump(m map[string]*useCount, key, title string, n int) {
	c := m[key]
	if c == nil {
		c = &useCount{}
		m[key] = c
	}
	c.add(title, n)
}

// reportRows is the most rows that a table of unmapped names shows.
const reportRows = 50

// CensusReport writes reports/census.md, the completeness report of WP4.9.
func CensusReport(w io.Writer, as []*CensusArticle) error {
	bw := bufio.NewWriter(w)
	all := &tally{}
	byType := map[string]*tally{}
	byMSC := map[string]*tally{}
	states := map[string]int{}
	witnesses := map[string]int{}
	gaps := map[string]*useCount{}
	diffs := map[string]*useCount{}
	templates := map[string]*useCount{}
	headings := map[string]*useCount{}
	signals := map[string]*useCount{}
	var html, differ int
	ptr := map[string]int{}
	for _, a := range as {
		states[a.State]++
		all.add(a)
		if byType[a.Type] == nil {
			byType[a.Type] = &tally{}
		}
		byType[a.Type].add(a)
		if a.Meta != nil {
			seen := map[string]bool{}
			for _, code := range a.Meta.MSC {
				if len(code) >= 2 && !seen[code[:2]] {
					seen[code[:2]] = true
					if byMSC[code[:2]] == nil {
						byMSC[code[:2]] = &tally{}
					}
					byMSC[code[:2]].add(a)
				}
			}
		}
		c := a.Census
		if c == nil {
			continue
		}
		for _, wn := range c.Witnesses {
			witnesses[wn]++
		}
		for _, q := range c.Gaps() {
			bump(gaps, q.Cue+" ("+q.Kind+")", a.Title, 1)
		}
		for _, p := range c.Pointers {
			ptr["all"]++
			switch {
			case p.Finding != "":
				ptr[p.Finding]++
			case p.Object == "":
				bump(gaps, "pointer ("+p.Via+")", a.Title, 1)
			}
		}
		if c.HTML != nil {
			html++
			d := c.Differences()
			if len(d) > 0 {
				differ++
			}
			for _, x := range d {
				bump(diffs, x.Cause, a.Title, 1)
			}
		}
		for k, n := range c.Unmapped {
			bump(templates, k, a.Title, n)
		}
		for _, h := range c.UnmappedHeadings {
			bump(headings, h, a.Title, 1)
		}
		gapSignals(signals, a)
	}

	fmt.Fprintf(bw, "# The census of the Wikipedia corpus\n\n")
	fmt.Fprintf(bw, "This report is made by `m2 wp census report` from the records of the corpus (WP4.9). The scope list has %d articles. %d of them are parsed, %d are fetched and not parsed, %d are only listed, and %d are stopped.\n\n", len(as), states[StateParsed], states[StateFetched], states[StateListed], states[StateStopped])

	fmt.Fprintf(bw, "## The four numbers\n\n")
	fmt.Fprintf(bw, "The Wikipedia part of the textbook layer is complete only when each number is 0 (WP1.7).\n\n")
	fmt.Fprintf(bw, "| Number | Count | Note |\n|---|---:|---|\n")
	fmt.Fprintf(bw, "| articles in scope that are not parsed or stopped | %d | rule Y01 and rule Y02 |\n", states[StateListed]+states[StateFetched])
	fmt.Fprintf(bw, "| census gaps: cues and pointers with no object | %d | rule Y03 |\n", all.gaps)
	fmt.Fprintf(bw, "| items with no outcome | %d | rule Y18 comes with the assembly, so each item counts |\n", all.items)
	fmt.Fprintf(bw, "| open rows of the inventory | not measured | the inventory of K04 does not read the corpus yet |\n")

	fmt.Fprintf(bw, "\n## By article type\n\n")
	writeTallies(bw, "Type", byType)
	fmt.Fprintf(bw, "\n## By MSC code\n\n")
	if len(byMSC) == 0 {
		fmt.Fprintf(bw, "No metadata file has an MSC code yet, so this table is empty.\n")
	} else {
		fmt.Fprintf(bw, "An article with codes in two areas counts in each area.\n\n")
		writeTallies(bw, "MSC", byMSC)
	}

	fmt.Fprintf(bw, "\n## Census strength\n\n")
	fmt.Fprintf(bw, "A census is strong when W-text, W-html and W-in were read and agree, normal when W-text and one more witness agree, and weak in the other cases (WP4.5). Of the %d parsed articles, W-html was read for %d and W-in for %d.\n\n", all.parsed, witnesses[WitnessHTML], witnesses[WitnessIn])
	fmt.Fprintf(bw, "| Strength | Articles |\n|---|---:|\n")
	for _, s := range []string{StrengthStrong, StrengthNormal, StrengthWeak} {
		fmt.Fprintf(bw, "| %s | %d |\n", s, all.strength[s])
	}

	fmt.Fprintf(bw, "\n## Gaps by pattern\n\n")
	if len(gaps) == 0 {
		fmt.Fprintf(bw, "No census has a gap.\n")
	} else {
		fmt.Fprintf(bw, "Each gap is a cue or a pointer with no object and no waiver (rule Y03).\n\n")
		writeCounts(bw, "Pattern", "Gaps", gaps, len(gaps))
	}
	if ptr["all"] > 0 {
		fmt.Fprintf(bw, "\nThe censuses have %s. %d of them point to an anchor that the article does not have (`broken-anchor`), and %d to an anchor that only the Parsoid HTML has (`template-anchor`).\n", count(ptr["all"], "incoming pointer"), ptr[FindingBrokenAnchor], ptr[FindingTemplateAnchor])
	}

	fmt.Fprintf(bw, "\n## Differences of the readings\n\n")
	if html == 0 {
		fmt.Fprintf(bw, "No census has the witness W-html yet.\n")
	} else {
		fmt.Fprintf(bw, "%d of the %d articles with the witness W-html have a difference of rule Y07, which is %.1f%%. The target is below 2%%.\n", differ, html, 100*float64(differ)/float64(html))
		if len(diffs) > 0 {
			fmt.Fprintf(bw, "\n")
			writeCounts(bw, "Cause", "Articles", diffs, len(diffs))
		}
	}

	fmt.Fprintf(bw, "\n## Unmapped templates\n\n")
	writeNames(bw, "Template", templates, "No census has an unmapped template (rule Y08).")
	fmt.Fprintf(bw, "\n## Unmapped headings\n\n")
	writeNames(bw, "Heading", headings, "No census has an unmapped heading (rule Y09).")

	fmt.Fprintf(bw, "\n## Gap signals\n\n")
	fmt.Fprintf(bw, "These are places where Wikipedia does not have an item, or has it in a weak form (WP4.7). They are not gaps of the extraction. The red links, the items that are missing in English and the items that are missing everywhere come with the links pass, the Wikidata pass and the inventory.\n\n")
	fmt.Fprintf(bw, "| Signal | Items | Articles | Examples |\n|---|---:|---:|---|\n")
	for _, s := range []string{signalStub, signalNoProof, signalInformal, signalCitation} {
		c := signals[s]
		if c == nil {
			c = &useCount{}
		}
		fmt.Fprintf(bw, "| %s | %d | %d | %s |\n", s, c.uses, len(c.articles), c.examples())
	}
	return bw.Flush()
}

// The gap signals of WP4.7 that the census gives.
const (
	signalStub     = "stub with no item"
	signalNoProof  = "statement with no proof in the article"
	signalInformal = "informal statement"
	signalCitation = "citation needed"
)

func gapSignals(m map[string]*useCount, a *CensusArticle) {
	proved := map[string]bool{}
	items := 0
	for i := range a.Objects {
		o := &a.Objects[i]
		if o.Kind == "proof" && o.AttachedTo != "" {
			proved[o.AttachedTo] = true
		}
		if IsItem(o) {
			items++
		}
	}
	if a.Meta != nil && strings.EqualFold(a.Meta.Class, "Stub") && items == 0 {
		bump(m, signalStub, a.Title, 1)
	}
	for i := range a.Objects {
		o := &a.Objects[i]
		if strings.HasPrefix(o.Kind, "statement") && !proved[o.Ref] {
			bump(m, signalNoProof, a.Title, 1)
		}
		if slices.Contains(o.Flags, "informal") {
			bump(m, signalInformal, a.Title, 1)
		}
		if slices.Contains(o.Flags, "citation-needed") {
			bump(m, signalCitation, a.Title, 1)
		}
	}
}

func writeTallies(w io.Writer, name string, m map[string]*tally) {
	fmt.Fprintf(w, "| %s | In scope | Parsed | Objects | Items | Gaps | Strong | Normal | Weak |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|\n", name)
	for _, k := range sortedKeys(m) {
		t := m[k]
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %d | %d | %d | %d |\n", k, t.inScope, t.parsed, t.objects, t.items, t.gaps, t.strength[StrengthStrong], t.strength[StrengthNormal], t.strength[StrengthWeak])
	}
}

func writeCounts(w io.Writer, name, uses string, m map[string]*useCount, rows int) {
	fmt.Fprintf(w, "| %s | %s | Articles | Examples |\n|---|---:|---:|---|\n", name, uses)
	for i, k := range byUses(m) {
		if i == rows {
			break
		}
		c := m[k]
		fmt.Fprintf(w, "| %s | %d | %d | %s |\n", escapeCell(k), c.uses, len(c.articles), escapeCell(c.examples()))
	}
}

func writeNames(w io.Writer, name string, m map[string]*useCount, none string) {
	if len(m) == 0 {
		fmt.Fprintln(w, none)
		return
	}
	if len(m) > reportRows {
		fmt.Fprintf(w, "The table shows the %d names with the most uses of the %d names.\n\n", reportRows, len(m))
	}
	writeCounts(w, name, "Uses", m, reportRows)
}

func escapeCell(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

// WriteReadings prints the readings of a census side by side: W-text and
// W-html, the differences of WP4.3, and the pointers of W-in.
func WriteReadings(w io.Writer, c *Census) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "%s, page %d, revision %d, census %s, witnesses %s\n\n", c.Title, c.Page, c.Revision, c.Strength, strings.Join(c.Witnesses, " "))
	h := c.HTML
	if h == nil {
		fmt.Fprintf(bw, "The census has no witness W-html. Run m2 wp parsoid and then m2 wp extract.\n")
	} else {
		tw := tabwriter.NewWriter(bw, 0, 4, 2, ' ', 0)
		fmt.Fprintf(tw, "reading\tW-text\tW-html\n")
		text := 0
		for _, n := range c.Counts.Math {
			text += n
		}
		fmt.Fprintf(tw, "math count\t%d\t%d\n", text, h.Math)
		fmt.Fprintf(tw, "<math> with an equal alttext\t%d\t%d\n", h.TeX, h.TeX-h.TeXMismatches)
		keys := map[string]bool{}
		for k := range c.Counts.ItemTemplates {
			keys[k] = true
		}
		for k := range h.ItemTemplates {
			keys[k] = true
		}
		for _, k := range slices.Sorted(mapKeys(keys)) {
			fmt.Fprintf(tw, "{{%s}}\t%d\t%d\n", k, c.Counts.ItemTemplates[k], h.ItemTemplates[k])
		}
		fmt.Fprintf(tw, "sections\t%d\t%d\n", c.Counts.Sections, h.Sections)
		if err := tw.Flush(); err != nil {
			return err
		}
		if d := c.HTMLDifferences(); len(d) == 0 {
			fmt.Fprintf(bw, "\nThe readings agree within the threshold of rule Y07.\n")
		} else {
			fmt.Fprintf(bw, "\nThe readings differ (rule Y07):\n")
			for _, s := range d {
				fmt.Fprintf(bw, "- %s\n", s)
			}
		}
	}
	if !slices.Contains(c.Witnesses, WitnessIn) {
		fmt.Fprintf(bw, "\nThe census has no witness W-in. Run m2 wp pointers and then m2 wp extract.\n")
		return bw.Flush()
	}
	fmt.Fprintf(bw, "\nThe article has %d incoming pointers.\n", len(c.Pointers))
	for _, p := range c.Pointers {
		match := p.Object
		if p.Finding != "" {
			match = strings.TrimSpace(p.Finding + " " + p.Object)
		}
		if match == "" {
			match = "no object"
		}
		fmt.Fprintf(bw, "- the %s %q to #%s: %s\n", p.Via, p.From, p.Anchor, match)
	}
	return bw.Flush()
}
