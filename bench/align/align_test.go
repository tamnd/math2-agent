// Package align_test measures the precision and the recall of the aligner on labelled pairs (spec 17.3.3).
//
// Each line of pairs.tsv is an item of one source and a record of a second source, with the label same or different:
//
//	split  source  id  target  target_id  label  origin
//
// The origin tells who says that the label is right. The first pairs come from the exact matches that the sources state: the OEIS, Mathlib and Erdős problem ids of the Wikidata items, and the Erdős problem of each main theorem of formal-conjectures. The decisions of people in registers/crosswalk.reg of math2 add pairs with the origin crosswalk.reg. The file has the ids only, and no text of a source.
//
// The evaluation turns off the id channel, because the ids are the labels. For each item it runs Match on the records of the target source and asks Options.Pick for a same_as proposal, as m2 align does. A proposal is correct when its record has the label same. The precision is the share of the proposals that are correct, and the recall is the share of the items with a same pair that get a correct proposal.
//
// The split dev or test of a pair comes from the hash of its item. The aligner has no parameters that it learns. Only the threshold is fitted, on the dev split: it is the lowest threshold at which the precision on dev is 0.98 or more. The report gives the precision and the recall on the test split, at the default threshold and at the fitted threshold.
//
// Without flags, the test runs on the small records in testdata, which are in the words of the test. Run it on the records of the sources with:
//
//	go test ./bench/align -root DIR -v -report align.md
//
// DIR is a tree with the records of the sources in build/records, from m2 ingest. Give -write to make pairs.tsv again from the records in DIR, and -tree to add the decisions of people from a math2 working tree.
package align_test

import (
	"bufio"
	"cmp"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/node"
)

var (
	root   = flag.String("root", "", "a tree with the records of the sources in build/records. Empty means the records in testdata.")
	tree   = flag.String("tree", "", "a math2 working tree. With -write, the decisions in its registers/crosswalk.reg are added to the pairs, and its nodes are the items of these pairs.")
	write  = flag.Bool("write", false, "write pairs.tsv again from the records in -root")
	report = flag.String("report", "", "write the results as Markdown to this file")
	errs   = flag.Bool("errors", false, "log each wrong proposal on the dev split. Look at the dev split only, thus the test split stays unseen.")
)

// pairsFile is the file of the labelled pairs. The records in testdata have their own pairs.
const pairsFile = "pairs.tsv"

// header is the first line of a pairs file.
const header = "# split\tsource\tid\ttarget\ttarget_id\tlabel\torigin"

// The labels and the splits.
const (
	same      = "same"
	different = "different"
	dev       = "dev"
	test      = "test"
)

// math2 is the source of the items that are nodes of the tree.
const math2 = "math2"

// A pair is one line of a pairs file.
type pair struct {
	split, source, id, target, targetID, label, origin string
}

func (p pair) String() string {
	return strings.Join([]string{p.split, p.source, p.id, p.target, p.targetID, p.label, p.origin}, "\t")
}

// splitOf returns the split of an item: dev for one item in five, by the hash of the source and the id, and test for the others.
func splitOf(source, id string) string {
	h := blake3.Sum256([]byte(source + "\x00" + id))
	if h[0] < 256/5 {
		return dev
	}
	return test
}

// readPairs reads a pairs file.
func readPairs(path string) ([]pair, error) {
	f, err := os.Open(path) // #nosec G304 -- the path is a pairs file of the test.
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []pair
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		n++
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 7 {
			return nil, fmt.Errorf("%s:%d: the line has %d fields, and a pair has 7", path, n, len(f))
		}
		out = append(out, pair{f[0], f[1], f[2], f[3], f[4], f[5], f[6]})
	}
	return out, sc.Err()
}

// records reads the records of a source in the tree at dir, by id.
func records(dir, source string) (map[string]ingest.Record, []ingest.Record, error) {
	recs, _, err := ingest.ReadRecords(dir, source)
	if err != nil {
		return nil, nil, err
	}
	by := make(map[string]ingest.Record, len(recs))
	for _, r := range recs {
		by[r.ID] = r
	}
	return by, recs, nil
}

func dataRoot() string {
	if *root != "" {
		return *root
	}
	return "testdata"
}

func pairsPath() string {
	if *root != "" {
		return pairsFile
	}
	return filepath.Join("testdata", pairsFile)
}

// TestPairs checks the pairs file: each line has a split from the hash of its item, a label and an origin, and no line is there two times.
func TestPairs(t *testing.T) {
	for _, path := range []string{pairsFile, filepath.Join("testdata", pairsFile)} {
		ps, err := readPairs(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(ps) == 0 {
			t.Errorf("%s has no pairs", path)
		}
		seen := map[[4]string]bool{}
		for _, p := range ps {
			if p.split != splitOf(p.source, p.id) {
				t.Errorf("%s: %s %s has the split %s, and the hash gives %s", path, p.source, p.id, p.split, splitOf(p.source, p.id))
			}
			if p.label != same && p.label != different || p.origin == "" {
				t.Errorf("%s: the pair %s has a bad label or no origin", path, p)
			}
			k := [4]string{p.source, p.id, p.target, p.targetID}
			if seen[k] {
				t.Errorf("%s: the pair %s is there two times", path, p)
			}
			seen[k] = true
		}
	}
}

// mainErdos matches the id of the main theorem of an Erdős problem in formal-conjectures, for example Erdos42.erdos_42. The other theorems of the module are variants, parts or lemmas, and they are not the problem.
var mainErdos = regexp.MustCompile(`^Erdos(\d+)\.erdos_(\d+)$`)

// TestWritePairs makes pairs.tsv from the records in -root, when -write is given.
func TestWritePairs(t *testing.T) {
	if !*write {
		t.Skip("give -write and -root to make pairs.tsv again")
	}
	if *root == "" {
		t.Fatal("-write needs -root")
	}
	targets := map[string]map[string]ingest.Record{}
	for _, s := range []string{"oeis", "erdosproblems", "wikidata", "mathlib", "formal_conjectures"} {
		by, _, err := records(*root, s)
		if err != nil {
			t.Fatalf("the pairs need the records of %s: %v", s, err)
		}
		targets[s] = by
	}
	var ps []pair
	add := func(source, id, target, targetID, label, origin string) {
		if _, ok := targets[target][targetID]; !ok && target != math2 {
			t.Logf("%s %s: %s %s is not in the records", source, id, target, targetID)
			return
		}
		if _, ok := targets[source][id]; !ok && source != math2 {
			t.Logf("%s %s is not in the records", source, id)
			return
		}
		ps = append(ps, pair{splitOf(source, id), source, id, target, targetID, label, origin})
	}
	// The Wikidata properties are exact matches. An item with two or more ids in a system is not one thing of that system, and it gives no pair.
	for _, r := range slices.Sorted(maps.Keys(targets["wikidata"])) {
		m := targets["wikidata"][r].Meta
		for _, c := range []struct{ key, source, prop string }{{"oeis", "oeis", "P829"}, {"erdos", "erdosproblems", "P14106"}} {
			if v := m[c.key]; v != "" && !strings.Contains(v, "|") {
				add("wikidata", r, c.source, v, same, "wikidata/"+c.prop)
			}
		}
		if v := m["mathlib"]; v != "" && !strings.Contains(v, "|") {
			add("mathlib", v, "wikidata", r, same, "wikidata/P14534")
		}
	}
	// A main theorem of formal-conjectures states its Erdős problem when its name and its link agree.
	for _, id := range slices.Sorted(maps.Keys(targets["formal_conjectures"])) {
		m := mainErdos.FindStringSubmatch(id)
		if m == nil || m[1] != m[2] || targets["formal_conjectures"][id].Meta["erdos"] != m[1] {
			continue
		}
		add("formal_conjectures", id, "erdosproblems", m[1], same, "formal_conjectures/name")
	}
	if *tree != "" {
		ds, err := align.ReadDecisions(*tree)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range ds {
			if d.Edge != align.SameAs || d.Decision == align.Confirmed {
				continue
			}
			label := same
			if d.Decision == align.Rejected {
				label = different
			}
			ps = append(ps, pair{splitOf(math2, d.Tag), math2, d.Tag, align.SourceOf(d.System), d.ID, label, "crosswalk.reg"})
		}
	}
	// A later decision on the same pair replaces the earlier one.
	last := map[[4]string]int{}
	for i, p := range ps {
		last[[4]string{p.source, p.id, p.target, p.targetID}] = i
	}
	var out []pair
	for i, p := range ps {
		if last[[4]string{p.source, p.id, p.target, p.targetID}] == i {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b pair) int {
		return cmp.Or(cmp.Compare(a.source, b.source), cmp.Compare(a.target, b.target), cmp.Compare(a.id, b.id), cmp.Compare(a.targetID, b.targetID))
	})
	var b strings.Builder
	b.WriteString(header + "\n")
	for _, p := range out {
		b.WriteString(p.String() + "\n")
	}
	if err := os.WriteFile(pairsFile, []byte(b.String()), 0o644); err != nil { // #nosec G306 -- the pairs file is part of the repository.
		t.Fatal(err)
	}
	t.Logf("wrote %d pairs", len(out))
}

// camel matches the place between a lowercase letter or a digit and an uppercase letter, as in IsPrimitiveRoot.
var camel = regexp.MustCompile(`([\p{Ll}\d])(\p{Lu})`)

// itemOf returns the item of a record of a source. The item has no ids, because the ids are the labels. The words of a Mathlib name, such as Subgroup.card_subgroup_dvd_card, are a name of the item.
func itemOf(source string, r ingest.Record) align.Item {
	it := align.Item{Ref: r.ID, Names: []string{r.Title, r.Meta["name"], r.Meta["title"]}, Text: r.Text}
	if source == "mathlib" {
		it.Names = append(it.Names, declName(r.ID))
	}
	return it
}

// declName returns the name of a Mathlib declaration in words. The name is the last part of the declaration. A predicate takes the part before it as its noun, thus Ideal.IsPrime is prime ideal. A lowercase last part takes the part before it as context, thus Ideal.Quotient.ring is quotient ring.
func declName(decl string) string {
	parts := strings.Split(decl, ".")
	name, ns := parts[len(parts)-1], ""
	if len(parts) > 1 {
		ns = parts[len(parts)-2]
	}
	switch {
	case ns != "" && len(name) > 2 && strings.HasPrefix(name, "Is") && unicode.IsUpper(rune(name[2])):
		name = name[2:] + " " + ns
	case ns != "" && name != "" && unicode.IsLower(rune(name[0])):
		name = ns + " " + name
	}
	return camel.ReplaceAllString(strings.ReplaceAll(name, "_", " "), "$1 $2")
}

// nodeItems returns the items of the nodes of the tree, without their ids.
func nodeItems(dir string) (map[string]align.Item, error) {
	nodes, err := node.Load(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]align.Item{}
	for _, n := range nodes {
		it := align.ItemOf(n)
		it.SameAs, it.Decls = nil, nil
		out[n.ID] = it
	}
	return out, nil
}

// An outcome is the result of the aligner on one item.
type outcome struct {
	split, source, target string
	labelled              bool    // the item has a pair with the label same
	score                 float64 // the score of the first candidate
	margin                bool    // the first candidate is ahead of the second by the margin
	correct, wrong        bool    // the first candidate has the label same, or the label different or no label when the item has a same pair
	top                   bool    // a record with the label same is among the candidates
	item                  string  // the item and its first candidate, for -errors
}

// proposed reports whether the item gets a same_as proposal at the threshold.
func (o outcome) proposed(threshold float64) bool { return o.margin && o.score >= threshold }

// evaluate runs the aligner on each item of the pairs and returns the outcomes.
func evaluate(t *testing.T, dir string, ps []pair, opt align.Options) []outcome {
	t.Helper()
	type key struct{ source, id, target string }
	labels := map[key]map[string]string{}
	var order []key
	for _, p := range ps {
		k := key{p.source, p.id, p.target}
		if labels[k] == nil {
			labels[k] = map[string]string{}
			order = append(order, k)
		}
		labels[k][p.targetID] = p.label
	}
	slices.SortFunc(order, func(a, b key) int {
		return cmp.Or(cmp.Compare(a.target, b.target), cmp.Compare(a.source, b.source), cmp.Compare(a.id, b.id))
	})
	indexes := map[string]*align.Index{}
	items := map[string]map[string]ingest.Record{}
	var nodes map[string]align.Item
	var out []outcome
	for _, k := range order {
		x, ok := indexes[k.target]
		if !ok {
			_, recs, err := records(dir, k.target)
			if err != nil {
				t.Fatal(err)
			}
			if x, err = align.NewIndex(k.target, recs, opt.Embedder); err != nil {
				t.Fatal(err)
			}
			indexes[k.target] = x
		}
		var it align.Item
		if k.source == math2 {
			if nodes == nil {
				if *tree == "" {
					t.Fatal("the pairs of crosswalk.reg need -tree")
				}
				var err error
				if nodes, err = nodeItems(*tree); err != nil {
					t.Fatal(err)
				}
			}
			if it, ok = nodes[k.id]; !ok {
				continue // the node is gone
			}
		} else {
			if items[k.source] == nil {
				by, _, err := records(dir, k.source)
				if err != nil {
					t.Fatal(err)
				}
				items[k.source] = by
			}
			r, ok := items[k.source][k.id]
			if !ok {
				t.Fatalf("%s %s is not in the records", k.source, k.id)
			}
			it = itemOf(k.source, r)
		}
		cands := x.Match(it, opt.K)
		o := outcome{split: splitOf(k.source, k.id), source: k.source, target: k.target}
		for _, l := range labels[k] {
			o.labelled = o.labelled || l == same
		}
		for _, c := range cands {
			o.top = o.top || labels[k][c.Record.ID] == same
		}
		if len(cands) > 0 {
			o.score = cands[0].Score
			_, o.margin = align.Options{Threshold: 1e-9, Margin: opt.Margin}.Pick(cands)
			o.item = fmt.Sprintf("%s %s %q → %s %q", k.source, k.id, it.Names, cands[0].Record.ID, cands[0].Record.Title)
			switch labels[k][cands[0].Record.ID] {
			case same:
				o.correct = true
			case different:
				o.wrong = true
			default:
				o.wrong = o.labelled
			}
		}
		out = append(out, o)
	}
	return out
}

// A row is the result on one pair of sources and one split.
type row struct {
	pair                                string // source → target
	items, labelled, proposals, correct int
	wrong, top                          int
}

func (r row) precision() float64 { return ratio(r.correct, r.correct+r.wrong) }
func (r row) recall() float64    { return ratio(r.correct, r.labelled) }

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// rows returns the results at the threshold, for each pair of sources in the split and for all of them.
func rows(outs []outcome, split string, threshold float64) []row {
	by := map[string]*row{}
	all := &row{pair: "all"}
	for _, o := range outs {
		if o.split != split {
			continue
		}
		name := o.source + " → " + o.target
		if by[name] == nil {
			by[name] = &row{pair: name}
		}
		for _, r := range []*row{by[name], all} {
			r.items++
			if o.labelled {
				r.labelled++
			}
			if o.top {
				r.top++
			}
			if o.proposed(threshold) {
				r.proposals++
				if o.correct {
					r.correct++
				}
				if o.wrong {
					r.wrong++
				}
			}
		}
	}
	var out []row
	for _, k := range slices.Sorted(maps.Keys(by)) {
		out = append(out, *by[k])
	}
	return append(out, *all)
}

// fit returns the lowest threshold from 0.50 to 1.00 at which the precision on the dev split is 0.98 or more, and false when no threshold gives a proposal with that precision.
func fit(outs []outcome) (float64, bool) {
	for i := 50; i <= 100; i++ {
		th := float64(i) / 100
		all := rows(outs, dev, th)
		if r := all[len(all)-1]; r.proposals > 0 && r.precision() >= 0.98 {
			return th, true
		}
	}
	return 1, false
}

// table writes the rows as Markdown.
func table(b *strings.Builder, title string, rs []row) {
	fmt.Fprintf(b, "%s\n\n| Sources | Items | With a match | Proposals | Correct | Wrong | Precision | Recall | Match in the top 5 |\n|---|---|---|---|---|---|---|---|---|\n", title)
	for _, r := range rs {
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d | %.3f | %.3f | %d |\n", r.pair, r.items, r.labelled, r.proposals, r.correct, r.wrong, r.precision(), r.recall(), r.top)
	}
	b.WriteString("\n")
}

func TestPrecision(t *testing.T) {
	ps, err := readPairs(pairsPath())
	if err != nil {
		t.Fatal(err)
	}
	if *root != "" {
		if _, err := os.Stat(filepath.Join(*root, ingest.RecordsDir)); errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%s has no records. Run m2 ingest in it first", *root)
		}
	}
	var opt align.Options
	opt.K, opt.Margin = 5, 0.05
	outs := evaluate(t, dataRoot(), ps, opt)
	const def = 0.9
	th, ok := fit(outs)
	var b strings.Builder
	fmt.Fprintf(&b, "The pairs file has %d pairs (hash %s). The embedder is %s, the margin is %.2f, and the id channel is off.\n\n", len(ps), fileHash(pairsPath()), align.Hash{}.Name(), opt.Margin)
	table(&b, fmt.Sprintf("Test split, at the default threshold %.2f:", def), rows(outs, test, def))
	if ok {
		table(&b, fmt.Sprintf("Test split, at the threshold %.2f, the lowest at which the precision on the dev split is 0.98 or more:", th), rows(outs, test, th))
	} else {
		b.WriteString("No threshold gives a precision of 0.98 or more on the dev split.\n\n")
	}
	table(&b, fmt.Sprintf("Dev split, at the default threshold %.2f:", def), rows(outs, dev, def))
	t.Log("\n" + b.String())
	if *errs {
		for _, o := range outs {
			if o.split == dev && o.proposed(def) && o.wrong {
				t.Logf("%.3f %s", o.score, o.item)
			}
		}
	}
	if *report != "" {
		if err := os.WriteFile(*report, []byte(b.String()), 0o644); err != nil { // #nosec G306 G703 -- the report is for the release notes.
			t.Fatal(err)
		}
	}
	if *root == "" {
		checkTestdata(t, outs, th, ok)
	}
}

// checkTestdata checks the results on the records in testdata, whose answers the test knows.
func checkTestdata(t *testing.T, outs []outcome, th float64, ok bool) {
	t.Helper()
	all := rows(outs, test, 0.9)
	r := all[len(all)-1]
	if r.items != 15 || r.labelled != 15 || r.proposals != 3 || r.correct != 2 || r.wrong != 1 || r.top != 15 {
		t.Errorf("the test split at 0.9 gives %+v", r)
	}
	if !ok || th != 0.5 {
		t.Errorf("the fitted threshold is %v, %v", th, ok)
	}
}

// fileHash returns a short hash of the file, thus a report names the pairs that it measured.
func fileHash(path string) string {
	data, err := os.ReadFile(path) // #nosec G304 -- the path is a pairs file of the test.
	if err != nil {
		return "-"
	}
	h := blake3.Sum256(data)
	return "blake3:" + hex.EncodeToString(h[:8])
}
