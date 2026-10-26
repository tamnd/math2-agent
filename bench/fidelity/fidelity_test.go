// Package fidelity_test measures the F1 battery as a classifier (spec 17.3.1).
//
// The battery gives each Lean binding a verdict: faithful, or misformalized with a class of spec 10.5. Each item of the suite is an informal statement and a formal statement, with a label: faithful, or the class of the fault of the formal statement. Each line of items.tsv is an item:
//
//	id  label  origin  checked_by  statement
//
// The formal statement of the item is the theorem Suite.ID in lean/Suite.lean. The suite uses only the Lean core library, and the statements and the labels are in our own words. The origin synthetic means that the item was written for the suite. Spec 17.3.1 asks a person to check each synthetic item, and checked_by has the reviewer id of that person, or - before the check.
//
// The test builds M2Extract in lean/ of the repository and the suite in lean/ of this directory, extracts the records and runs battery.Run on one node for each item, as m2 build does. A check that fails catches the item, with the class of the first check that fails. A check that flags, or that stays open, with a class flags the item: a reviewer sees the class, but a flag does not block F1. An example is the constant audit, which stays open when no node binds a constant and names the junk values of the statement. The checks 2 and 9 need models, and the test does not run them. The items have no edges to other nodes, thus the checks 7 and 8 skip.
//
// The report is a confusion matrix: a row for each label and a column for each class that the battery gives. It also gives the recall of each class, which is the share of its items that the battery catches, the recall with the flags, and the false-alarm rate on the faithful items. The tactics of the checks have limits in heartbeats, thus a run on the same toolchain gives the same matrix. The noise band of each class is zero items. baseline.tsv has the number of items that the battery catches in each class, and the test fails when a class catches fewer, or when the battery fails more faithful items. Give -update to write baseline.tsv again.
//
// The live set of spec 17.3.1 comes from the reviews of a math2 tree. Each review of a binding is an item, with the label faithful, or the class of a not-faithful review. The battery result of the item is the report of the binding in build/battery.json. Run it with:
//
//	go test ./bench/fidelity -tree DIR -v -report fidelity.md
//
// DIR is a math2 working tree after m2 build. Give -out when the build is not in build/ of the tree. The test skips when lake is not on the PATH, unless -tree is given.
package fidelity_test

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/battery"
	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

var (
	tree   = flag.String("tree", "", "a math2 working tree after m2 build. Empty means the suite in items.tsv.")
	out    = flag.String("out", "", "the output directory of m2 build for -tree. Empty means build/ in the tree.")
	report = flag.String("report", "", "write the results as Markdown to this file")
	update = flag.Bool("update", false, "write baseline.tsv again from this run")
)

const (
	itemsFile    = "items.tsv"
	baselineFile = "baseline.tsv"
	faithful     = "faithful"
	failed       = "fail" // the column of a check that fails with no class, for example check 1
)

// minItems is the number of items of a class from which the recall of the class must be 0.9 or more (the acceptance of tamnd/math2-agent#53).
const minItems = 30

type item struct {
	id, label, origin, checked, statement string
}

// outcome is the result of the battery on an item.
type outcome struct {
	item
	caught  string // the class of the first check that fails, failed for a check with no class, or empty
	flagged string // the class of the first check that flags or stays open with a class, when no check fails
}

func TestBattery(t *testing.T) {
	var out []outcome
	if *tree != "" {
		out = live(t, *tree)
	} else {
		out = suite(t)
	}
	m := tabulate(out)
	text := m.markdown()
	t.Log("\n" + text)
	if *report != "" {
		if err := os.WriteFile(*report, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, label := range m.labels {
		if label != faithful && m.total[label] >= minItems && m.recall(label) < 0.9 {
			t.Logf("%s has %d items and a recall of %.2f, under the target of 0.9", label, m.total[label], m.recall(label))
		}
	}
	if *tree != "" {
		return
	}
	if *update {
		writeBaseline(t, m)
		return
	}
	for label, want := range readBaseline(t) {
		got := m.caught[label]
		if label == faithful && got > want {
			t.Errorf("the battery fails %d faithful items, and baseline.tsv has %d. A new false alarm blocks the release", got, want)
		}
		if label != faithful && got < want {
			t.Errorf("the battery catches %d items of %s, and baseline.tsv has %d. A drop in the recall of a class blocks the release", got, label, want)
		}
	}
}

// suite runs the battery on the items of items.tsv.
func suite(t *testing.T) []outcome {
	t.Helper()
	if _, err := exec.LookPath("lake"); err != nil {
		t.Skip("lake is not on the PATH")
	}
	items := readItems(t)
	repo, err := filepath.Abs(filepath.Join("..", "..", "lean"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := lean.Toolchain(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := lean.Toolchain("lean"); err != nil || got != want {
		t.Fatalf("lean/lean-toolchain has %q (%v), and it must have %q as in the M2Extract package", got, err, want)
	}
	for _, dir := range []string{repo, "lean"} {
		cmd := exec.Command("lake", "build")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("lake build in %s: %v\n%s", dir, err, out)
		}
	}

	ctx := context.Background()
	sess, err := lean.Start(ctx, filepath.Join(repo, ".lake", "build", "bin", "m2extract"), "lean")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sess.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := sess.Import("Suite"); err != nil {
		t.Fatal(err)
	}
	decls := make([]string, len(items))
	for i, it := range items {
		decls[i] = "Suite." + it.id
	}
	recs, err := sess.Extract(decls, decls)
	if err != nil {
		t.Fatal(err)
	}
	checks, err := sess.Checks(lean.CheckHeartbeats, decls...)
	if err != nil {
		t.Fatal(err)
	}
	records := map[string]lean.Record{}
	for _, r := range recs {
		if r.Error != "" {
			t.Fatalf("%s: %s", r.Decl, r.Error)
		}
		if c, ok := checks[r.Decl]; ok {
			r.Checks = &c
		}
		records[r.Decl] = r
	}

	nodes := make([]*node.Node, len(items))
	byTag := map[string]item{}
	for i, it := range items {
		tag := fmt.Sprintf("B%05d", i+1)
		src := "---\ntag: " + tag + "\nkind: theorem\nname: " + it.id + "\nformal:\n  - {system: lean4, decl: Suite." + it.id + ", match: exact}\n---\n\n## Statement\n\n" + it.statement + "\n"
		if nodes[i], err = node.Parse("nodes/"+tag[:2]+"/"+tag+".md", []byte(src)); err != nil {
			t.Fatal(err)
		}
		byTag[tag] = it
	}
	reports, err := battery.Run(ctx, battery.Input{Nodes: nodes, Records: records}, battery.Options{Prover: sess})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != len(items) {
		t.Fatalf("the battery gives %d reports for %d items", len(reports), len(items))
	}
	out := make([]outcome, len(reports))
	for i, r := range reports {
		out[i] = judge(byTag[r.Tag], r)
	}
	return out
}

// live makes an item of each review of the tree in root, with the report of its binding in build/battery.json.
func live(t *testing.T, root string) []outcome {
	t.Helper()
	reviews, err := status.LoadReviews(root)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := build.ReadBattery(cmp.Or(*out, filepath.Join(root, "build")))
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[status.Key]battery.Report{}
	for _, r := range reports {
		byKey[status.Key{Tag: r.Tag, Decl: r.Decl}] = r
	}
	var out []outcome
	for _, id := range slices.Sorted(maps.Keys(reviews)) {
		rv := reviews[id]
		tag, _, ok := strings.Cut(strings.TrimPrefix(id, "r-"), "-")
		if !ok {
			continue
		}
		r, ok := byKey[status.Key{Tag: tag, Decl: rv.Decl}]
		if !ok {
			t.Logf("%s: build/battery.json has no report of %s %s", id, tag, rv.Decl)
			continue
		}
		label := faithful
		if rv.Verdict == status.NotFaithful {
			label = rv.Class
		}
		out = append(out, judge(item{id: id, label: label, origin: "live", checked: rv.Reviewer}, r))
	}
	if len(out) == 0 {
		t.Skip("the tree has no review with a battery report")
	}
	return out
}

// judge reads the verdict of the battery from a report.
func judge(it item, r battery.Report) outcome {
	o := outcome{item: it}
	for _, c := range r.Checks {
		switch {
		case c.Result == battery.Fail && o.caught == "":
			o.caught = cmp.Or(c.Class, failed)
		case (c.Result == battery.Flag || c.Result == battery.Open) && c.Class != "" && o.flagged == "":
			o.flagged = c.Class
		}
	}
	if o.caught != "" {
		o.flagged = ""
	}
	return o
}

// matrix is the confusion matrix of a run.
type matrix struct {
	labels  []string                  // the labels with at least one item: faithful, then the classes in the order of battery.Classes
	columns []string                  // faithful, then each class that the battery gives
	cells   map[string]map[string]int // the number of items by label and by column
	total   map[string]int            // the number of items by label
	caught  map[string]int            // the items that a check fails, by label
	flagged map[string]int            // the items that a check fails or flags, by label
	checked int                       // the items that a person checked
	n       int
}

func tabulate(out []outcome) matrix {
	m := matrix{cells: map[string]map[string]int{}, total: map[string]int{}, caught: map[string]int{}, flagged: map[string]int{}, n: len(out)}
	cols := map[string]bool{}
	for _, o := range out {
		col := cmp.Or(o.caught, faithful)
		cols[col] = true
		if m.cells[o.label] == nil {
			m.cells[o.label] = map[string]int{}
		}
		m.cells[o.label][col]++
		m.total[o.label]++
		if o.caught != "" {
			m.caught[o.label]++
		}
		if o.caught != "" || o.flagged != "" {
			m.flagged[o.label]++
		}
		if o.checked != "-" && o.checked != "" {
			m.checked++
		}
	}
	order := append([]string{faithful}, battery.Classes()...)
	for _, l := range slices.Sorted(maps.Keys(m.total)) {
		if !slices.Contains(order, l) {
			order = append(order, l)
		}
	}
	order = append(order, failed)
	for _, l := range order {
		if m.total[l] > 0 {
			m.labels = append(m.labels, l)
		}
		if cols[l] {
			m.columns = append(m.columns, l)
		}
	}
	return m
}

func (m matrix) recall(label string) float64 {
	return float64(m.caught[label]) / float64(m.total[label])
}

func (m matrix) markdown() string {
	var b strings.Builder
	items := "items"
	if m.n == 1 {
		items = "item"
	}
	fmt.Fprintf(&b, "# The F1 battery\n\nThe run has %d %s, and a person checked %d of them. The rows are the labels, and the columns are the verdicts of the battery: faithful when no check fails, or the class of the first check that fails. The recall is the share of the items of a class that a check fails. The recall with flags also counts an item that a check flags with any class.\n\n", m.n, items, m.checked)
	b.WriteString("| label | items |")
	for _, c := range m.columns {
		b.WriteString(" " + c + " |")
	}
	b.WriteString(" recall | recall with flags |\n|---|---|")
	b.WriteString(strings.Repeat("---|", len(m.columns)+2) + "\n")
	for _, l := range m.labels {
		fmt.Fprintf(&b, "| %s | %d |", l, m.total[l])
		for _, c := range m.columns {
			fmt.Fprintf(&b, " %d |", m.cells[l][c])
		}
		if l == faithful {
			b.WriteString(" | |\n")
			continue
		}
		fmt.Fprintf(&b, " %.2f | %.2f |\n", m.recall(l), float64(m.flagged[l])/float64(m.total[l]))
	}
	if n := m.total[faithful]; n > 0 {
		fmt.Fprintf(&b, "\nThe false-alarm rate on the faithful items is %.2f, and %d of %d faithful items have a flag.\n", float64(m.caught[faithful])/float64(n), m.flagged[faithful]-m.caught[faithful], n)
	}
	var small []string
	for _, l := range append([]string{}, battery.Classes()...) {
		if m.total[l] < minItems {
			small = append(small, fmt.Sprintf("%s (%d)", l, m.total[l]))
		}
	}
	if len(small) > 0 {
		fmt.Fprintf(&b, "\nThe target is a recall of 0.9 on each class with %d items or more. These classes have fewer items: %s.\n", minItems, strings.Join(small, ", "))
	}
	b.WriteString("\nThe checks 2 and 9 need models, and this run does not have them. The noise band of each class is zero items, because the tactics of the checks have limits in heartbeats.\n")
	return b.String()
}

func readItems(t *testing.T) []item {
	t.Helper()
	f, err := os.Open(itemsFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []item
	seen := map[string]bool{}
	labels := append([]string{faithful}, battery.Classes()...)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.Split(line, "\t")
		if len(p) != 5 {
			t.Fatalf("%s: the line %q has %d fields, and it must have 5", itemsFile, line, len(p))
		}
		it := item{id: p[0], label: p[1], origin: p[2], checked: p[3], statement: p[4]}
		switch {
		case seen[it.id]:
			t.Fatalf("%s: the id %s is not unique", itemsFile, it.id)
		case !slices.Contains(labels, it.label):
			t.Fatalf("%s: %s has the label %q, which is not faithful or a class of spec 10.5", itemsFile, it.id, it.label)
		}
		seen[it.id] = true
		out = append(out, it)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func readBaseline(t *testing.T) map[string]int {
	t.Helper()
	out := map[string]int{}
	data, err := os.ReadFile(baselineFile)
	if errors.Is(err, fs.ErrNotExist) {
		return out
	}
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(data)) {
		p := strings.Fields(line)
		if len(p) != 3 || strings.HasPrefix(line, "#") {
			continue
		}
		v, err := strconv.Atoi(p[2])
		if err != nil {
			t.Fatalf("%s: %v", baselineFile, err)
		}
		out[p[0]] = v
	}
	return out
}

func writeBaseline(t *testing.T, m matrix) {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("# label\titems\tcaught\n# The number of items of each label and the number that the battery catches. go test ./bench/fidelity -update writes this file.\n")
	for _, l := range m.labels {
		fmt.Fprintf(&b, "%s\t%d\t%d\n", l, m.total[l], m.caught[l])
	}
	if err := os.WriteFile(baselineFile, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
