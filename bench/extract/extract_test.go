// Package extract_test measures the extraction of results from papers (spec 17.3.2).
//
// The test reads the content plane of tamnd/arxiv with the arxiv_results adapter, as m2 ingest does, and compares the records with labels that a person wrote. Each line of a labels file is one result:
//
//	id  kind  role  deps  path
//
// The id is the id of the record, for example 2601.00001#L101. A result that the content plane does not mark has an id that no record has, so it counts as a miss. The kind and the role are of spec 04.2, with - for no role. The deps are the ids of the results that the result uses, with a space between them, or -. The path is the source type of the paper: source (LaTeX), render (HTML), native (PDF text) or vision (a model). The file has ids only, and no text of a paper.
//
// The report gives, for each source type and for all of them: the result recall and precision, the kind accuracy, and the precision, the recall and the F1 of the dependency edges. The kind is correct when the kind and the role are both correct. Unfolding fidelity is not measured, because the adapter does not unfold results. Unfolding needs a model.
//
// Without flags, the test runs on the papers in testdata, which are in the words of the test. They are small, and they have the hard cases on purpose: a proof in an appendix, a proof with no link, an equation inside a result, a custom environment, a reference in plain words, prose after a statement and a result that the extractor did not mark. Run it on a checkout of tamnd/arxiv with:
//
//	go test ./bench/extract -root DIR -labels FILE -v -report extract.md
package extract_test

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/arxivresults"
)

var (
	root   = flag.String("root", "", "a checkout of tamnd/arxiv. Empty means the papers in testdata.")
	labels = flag.String("labels", "", "the labels file. Empty means testdata/labels.tsv.")
	report = flag.String("report", "", "write the results as Markdown to this file")
)

// A label is one line of a labels file.
type label struct {
	id, kind, role, path string
	deps                 []string
}

// readLabels reads a labels file.
func readLabels(path string) (map[string]label, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the path is a labels file of the test.
	if err != nil {
		return nil, err
	}
	out := map[string]label{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			return nil, fmt.Errorf("%s:%d: the line has %d fields, and a label has 5", path, n, len(f))
		}
		l := label{id: f[0], kind: f[1], role: f[2], path: f[4]}
		if f[3] != "-" {
			l.deps = strings.Fields(f[3])
		}
		if _, ok := out[l.id]; ok {
			return nil, fmt.Errorf("%s:%d: %s has two labels", path, n, l.id)
		}
		out[l.id] = l
	}
	return out, sc.Err()
}

// archive makes a git archive of the content plane in the tree at dir, like the one that GitHub gives.
func archive(dir string) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": strings.Repeat("0", 40)}})
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(filepath.Join(dir, "content"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path) // #nosec G304 -- the path is in the tree of the test.
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "arxiv-main/" + filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(data))}); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// row is the counts of one source type.
type row struct {
	path                       string
	labelled, extracted, found int
	kinds                      int
	edgeTP, edgeFP, edgeFN     int
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// num formats the share a/b with two decimals, or a dash when b is 0.
func num(a, b int) string {
	if b == 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", ratio(a, b))
}

// f1 formats the F1 of the dependency edges of r, or a dash when r has no edges.
func f1(r row) string {
	if r.edgeTP+r.edgeFP+r.edgeFN == 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", ratio(2*r.edgeTP, 2*r.edgeTP+r.edgeFP+r.edgeFN))
}

func (r *row) add(s row) {
	r.labelled += s.labelled
	r.extracted += s.extracted
	r.found += s.found
	r.kinds += s.kinds
	r.edgeTP += s.edgeTP
	r.edgeFP += s.edgeFP
	r.edgeFN += s.edgeFN
}

// measure compares the records with the labels and returns one row for each source type, in the order of the names, and the row of all.
func measure(t *testing.T, recs []ingest.Record, ls map[string]label) ([]row, row) {
	by := map[string]*row{}
	get := func(path string) *row {
		if by[path] == nil {
			by[path] = &row{path: path}
		}
		return by[path]
	}
	seen := map[string]bool{}
	for _, r := range recs {
		got := strings.Fields(r.Meta["deps"])
		l, ok := ls[r.ID]
		if !ok {
			s := get(r.Meta["path"])
			s.extracted++
			s.edgeFP += len(got)
			t.Logf("not a labelled result: %s %s", r.ID, r.Title)
			continue
		}
		seen[r.ID] = true
		s := get(l.path)
		s.extracted++
		s.found++
		if r.Kind == l.kind && cmpRole(r.Meta["role"]) == l.role {
			s.kinds++
		} else {
			t.Logf("wrong kind: %s is %s %s, and the label says %s %s", r.ID, r.Kind, cmpRole(r.Meta["role"]), l.kind, l.role)
		}
		for _, d := range got {
			if slices.Contains(l.deps, d) {
				s.edgeTP++
			} else {
				s.edgeFP++
				t.Logf("wrong edge: %s uses %s", r.ID, d)
			}
		}
		for _, d := range l.deps {
			if !slices.Contains(got, d) {
				s.edgeFN++
				t.Logf("missed edge: %s uses %s", r.ID, d)
			}
		}
	}
	for id, l := range ls {
		s := get(l.path)
		s.labelled++
		if !seen[id] {
			s.edgeFN += len(l.deps)
			t.Logf("missed result: %s", id)
		}
	}
	var rows []row
	all := row{path: "all"}
	for _, p := range slices.Sorted(maps.Keys(by)) {
		rows = append(rows, *by[p])
		all.add(*by[p])
	}
	return rows, all
}

func cmpRole(role string) string {
	if role == "" {
		return "-"
	}
	return role
}

func TestExtraction(t *testing.T) {
	dir, lpath := "testdata", filepath.Join("testdata", "labels.tsv")
	if *root != "" {
		dir = *root
	}
	if *labels != "" {
		lpath = *labels
	}
	ls, err := readLabels(lpath)
	if err != nil {
		t.Fatal(err)
	}
	tgz, err := archive(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := arxivresults.Adapter{}
	src := a.Source()
	recs, err := ingest.Import(a, &src, bytes.NewReader(tgz))
	if err != nil {
		t.Fatal(err)
	}
	rows, all := measure(t, recs, ls)

	var b strings.Builder
	fmt.Fprintf(&b, "Extraction (spec 17.3.2), adapter %s, %d labelled results.\n\n", a.Version(), all.labelled)
	b.WriteString("| Source type | Labelled | Extracted | Recall | Precision | Kind accuracy | Edge precision | Edge recall | Edge F1 |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range append(rows, all) {
		fmt.Fprintf(&b, "| %s | %d | %d | %s | %s | %s | %s | %s | %s |\n", r.path, r.labelled, r.extracted, num(r.found, r.labelled), num(r.found, r.extracted), num(r.kinds, r.found), num(r.edgeTP, r.edgeTP+r.edgeFP), num(r.edgeTP, r.edgeTP+r.edgeFN), f1(r))
	}
	b.WriteString("\nA dash means that the share has no items to count. Unfolding fidelity is not measured, because the adapter does not unfold results.\n")
	t.Log("\n" + b.String())
	if *report != "" {
		if err := os.WriteFile(*report, []byte(b.String()), 0o644); err != nil { // #nosec G306 G703 -- the report is for the release notes.
			t.Fatal(err)
		}
	}
	if *root == "" {
		checkTestdata(t, all)
	}
}

// checkTestdata checks the numbers on the papers in testdata, thus a change of the adapter that changes them also changes this test.
func checkTestdata(t *testing.T, all row) {
	t.Helper()
	want := row{path: "all", labelled: 19, extracted: 17, found: 17, kinds: 16, edgeTP: 14, edgeFP: 2, edgeFN: 2}
	if all != want {
		t.Errorf("the counts on testdata are %+v, and the test wants %+v", all, want)
	}
}
