package pack

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/status"
)

// fixtures is a small math2 tree with group theory, number theory and analysis.
const fixtures = "../testdata/nodes/valid"

const header = "id: v\ntitle: Groups\neditors: [github:ada]\n"

func volume(t *testing.T, body string) *Volume {
	t.Helper()
	v, err := Parse("volumes/v.yaml", []byte(header+body))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func packed(t *testing.T, root, body string) *Document {
	t.Helper()
	d, err := Pack(root, volume(t, body), Options{})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// copyTree copies the fixtures to a new directory, so that a test can write a lock file.
func copyTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(fixtures)); err != nil {
		t.Fatal(err)
	}
	return dir
}

func state(r *check.Report, id string) check.State {
	for _, res := range r.Results {
		if res.Rule == id {
			return res.State
		}
	}
	return ""
}

func findings(r *check.Report, id string) []string {
	var out []string
	for _, f := range r.Findings {
		if f.Rule == id {
			out = append(out, f.Msg)
		}
	}
	return out
}

func tags(d *Document) []string {
	var out []string
	for _, it := range d.Items() {
		out = append(out, it.Tag)
	}
	return out
}

func TestPackClosureAndOrder(t *testing.T) {
	d := packed(t, fixtures, "kind: book\ntargets: [Q529ES, CM1BRG]\nchoose: {DSG7KN: GY1XDB}\n")
	r := d.Check()
	for _, id := range []string{"PC2", "PC3", "PC6"} {
		if s := state(r, id); s != check.Pass {
			t.Errorf("%s is %s: %v", id, s, findings(r, id))
		}
	}
	got := tags(d)
	for _, want := range []string{"Q529ES", "CM1BRG", "BFBKVM", "ENS73X", "GY1XDB", "N98ZA2", "E18G47"} {
		if !slices.Contains(got, want) {
			t.Errorf("the volume has no %s: %v", want, got)
		}
	}
	// The retired proof is not in the volume, and a concept is not an item when its definition is.
	for _, no := range []string{"DTT8HP", "DSG7KN", "X7J0XA"} {
		if slices.Contains(got, no) {
			t.Errorf("the volume has %s: %v", no, got)
		}
	}
	if it := d.Home("X7J0XA"); it == nil || it.Tag != "N98ZA2" {
		t.Errorf("the home of the concept X7J0XA is %v, want its definition N98ZA2", it)
	}
	// Each item comes after the items that it and its proofs use.
	for _, it := range d.Items() {
		for _, u := range d.tree.nodes[it.Tag].List("uses") {
			if h := d.Home(u); h != nil && h.position > it.position {
				t.Errorf("%s comes before %s, which it uses", it.Tag, u)
			}
		}
	}
	lagrange := d.Home("Q529ES")
	if lagrange.Label != "Claim" || lagrange.Claim != "claimed" || len(lagrange.Proofs) != 1 || lagrange.Proofs[0].Tag != "ENS73X" || lagrange.Proofs[0].Number != "" {
		t.Errorf("Lagrange's theorem is %+v", lagrange)
	}
	if lagrange.Main == "" {
		t.Errorf("Lagrange's theorem has no statement")
	}
}

// buildDir writes a build output with the status of two statements at E2.
func buildDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	data, err := json.Marshal([]status.Status{{Tag: "BFBKVM", Evidence: status.Standard}, {Tag: "Q529ES", Evidence: status.Standard}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, build.StatusFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPackBuildLevels(t *testing.T) {
	d, err := Pack(fixtures, volume(t, "kind: book\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\nlevels: {evidence: E2}\n"), Options{Build: buildDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	if it := d.Home("BFBKVM"); it.Label != "Lemma" || it.Claim != "" {
		t.Errorf("BFBKVM is a %s, want a Lemma", it.Label)
	}
	if it := d.Home("Q529ES"); it.Label != "Theorem" {
		t.Errorf("Q529ES is a %s, want a Theorem", it.Label)
	}
	if r := d.Check(); state(r, "PC1") != check.Pass {
		t.Errorf("PC1: %v", findings(r, "PC1"))
	}
	files, err := d.HTML()
	if err != nil {
		t.Fatal(err)
	}
	if want := `<a href="#BFBKVM" title="Lemma 1.4" class="ref">`; !strings.Contains(string(files["index.html"]), want) {
		t.Errorf("the page has no %s", want)
	}
}

func TestPackNumbers(t *testing.T) {
	d := packed(t, fixtures, "kind: book\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\n")
	for _, c := range d.Chapters {
		k := 0
		for j, s := range c.Sections {
			if want := fmt.Sprintf("%d.%d", c.Number, j+1); s.Number != want {
				t.Errorf("section %s, want %s", s.Number, want)
			}
			for _, it := range s.Items {
				k++
				if want := fmt.Sprintf("%d.%d", c.Number, k); it.Number != want {
					t.Errorf("%s has %s, want %s", it.Tag, it.Number, want)
				}
			}
		}
	}
	if len(d.Chapters) < 2 {
		t.Fatalf("%d chapters, want the chapters of number theory and group theory", len(d.Chapters))
	}
	var titles []string
	for _, c := range d.Chapters {
		titles = append(titles, c.Title)
	}
	if !slices.Contains(titles, "Group theory") {
		t.Errorf("the chapters are %v, want the topic Group theory", titles)
	}
	ids := map[string]bool{}
	for _, c := range d.Chapters {
		if ids[c.ID] {
			t.Errorf("the chapter %s starts two times: %v", c.ID, titles)
		}
		ids[c.ID] = true
	}
}

func TestPackCharacterization(t *testing.T) {
	d := packed(t, fixtures, "kind: book\ntargets: [DSG7KN]\nchoose: {DSG7KN: GY1XDB}\n")
	def, alt := d.Home("GY1XDB"), d.Home("GJX121")
	if def == nil || alt == nil {
		t.Fatalf("the volume is %v", tags(d))
	}
	if def.Label != "Definition" || def.Concept != "DSG7KN" || d.Home("DSG7KN") != def {
		t.Errorf("the chosen definition is %+v", def)
	}
	if alt.Label != "Characterization" || alt.Characterizes != "GY1XDB" || alt.position < def.position {
		t.Errorf("the other definition is %+v", alt)
	}
	if len(alt.Proofs) != 1 || alt.Proofs[0].Tag != "RBG1TM" {
		t.Errorf("the characterization has the proofs %v, want RBG1TM", alt.Proofs)
	}
	if got := d.Lock().Choose; got["DSG7KN"] != "GY1XDB" {
		t.Errorf("the lock chooses %v", got)
	}
}

func TestPackNoChoice(t *testing.T) {
	r := packed(t, fixtures, "kind: book\ntargets: [DSG7KN]\n").Check()
	if got := findings(r, "PC3"); len(got) != 1 || !strings.Contains(got[0], "chooses none") {
		t.Errorf("PC3 is %v", got)
	}
}

func TestPackProofModes(t *testing.T) {
	for kind, want := range map[string]int{"book": 1, "reference": 0} {
		d := packed(t, fixtures, "kind: "+kind+"\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\n")
		if got := len(d.Home("Q529ES").Proofs); got != want {
			t.Errorf("a %s has %d proofs of Lagrange's theorem, want %d", kind, got, want)
		}
	}
}

func TestPackAssumes(t *testing.T) {
	d := packed(t, fixtures, "kind: book\ntargets: [Q529ES]\nassumes: [GY1XDB, N98ZA2]\n")
	for _, no := range []string{"GY1XDB", "N98ZA2"} {
		if d.Home(no) != nil {
			t.Errorf("the volume has the assumed node %s", no)
		}
	}
	if len(d.Assumed) != 2 || d.Assumed[0].URI != "m2://GY1XDB" {
		t.Errorf("assumed is %v", d.Assumed)
	}
	if r := d.Check(); state(r, "PC3") != check.Pass {
		t.Errorf("PC3: %v", findings(r, "PC3"))
	}
}

func TestPackOutline(t *testing.T) {
	d := packed(t, fixtures, `kind: book
targets: [Q529ES, CM1BRG]
choose: {DSG7KN: GY1XDB}
outline:
  - title: Groups
    sections:
      - title: Lagrange
        nodes: [Q529ES, GY1XDB, N98ZA2, E18G47, 7SW3MR, BFBKVM, NOSUCH]
`)
	r := d.Check()
	if got := findings(r, "PC2"); len(got) == 0 || !strings.Contains(strings.Join(got, "\n"), "Q529ES uses") {
		t.Errorf("PC2 is %v, want the forward references of Q529ES", got)
	}
	if got := strings.Join(findings(r, "PC3"), "\n"); !strings.Contains(got, "CM1BRG is not in the outline") {
		t.Errorf("PC3 is %s", got)
	}
	if got := strings.Join(findings(r, "PC6"), "\n"); !strings.Contains(got, "NOSUCH") {
		t.Errorf("PC6 is %s", got)
	}
	if d.Home("Q529ES").Number != "1.1" {
		t.Errorf("the outline order is not kept: %v", tags(d))
	}
}

func TestPackLevels(t *testing.T) {
	r := packed(t, fixtures, "kind: book\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\nlevels: {writing: W3, evidence: E3}\n").Check()
	if state(r, "PC1") != check.Fail || len(findings(r, "PC1")) == 0 {
		t.Errorf("PC1 is %s", state(r, "PC1"))
	}
}

func TestPackGlue(t *testing.T) {
	d := packed(t, fixtures, `kind: book
targets: [Q529ES]
choose: {DSG7KN: GY1XDB}
glue:
  - at: preface
    text: This volume follows one count to the theorem of Lagrange.
    writer: github:ada
  - at: preface
    text: We define a group as a set with an operation.
    writer: github:ada
  - at: group-theory
    text: The order of a subgroup is the next idea.
    writer: m2-agent/writer@0.9.1
    run: r-2026-10-27-0001
  - at: nowhere
    text: A note.
    writer: github:ada
`)
	if len(d.Preface) != 2 {
		t.Errorf("the preface has %d parts", len(d.Preface))
	}
	got := strings.Join(findings(d.Check(), "PC7"), "\n")
	for _, want := range []string{`mathematics ("We define")`, "no person confirmed", "nowhere has no place"} {
		if !strings.Contains(got, want) {
			t.Errorf("PC7 has no %q: %s", want, got)
		}
	}
}

func TestPackHTML(t *testing.T) {
	d := packed(t, fixtures, "kind: book\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\n")
	files, err := d.HTML()
	if err != nil {
		t.Fatal(err)
	}
	page := string(files["index.html"])
	for _, want := range []string{
		`id="Q529ES"`,
		`<a href="#BFBKVM" title="Claim `,
		`<script type="application/ld+json">`,
		`"identifier": "m2://Q529ES"`,
		`class="katex"`,
		`href="katex/katex.min.css"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page has no %s", want)
		}
	}
	if files[Style] == nil || files["katex/katex.min.css"] == nil {
		t.Errorf("the files are %d, with no style sheet", len(files))
	}
	again, _ := d.HTML()
	if !bytes.Equal(files["index.html"], again["index.html"]) {
		t.Error("two builds of the HTML are not the same")
	}
}

func TestPackLaTeX(t *testing.T) {
	d := packed(t, fixtures, "kind: book\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\n")
	files, err := d.LaTeX()
	if err != nil {
		t.Fatal(err)
	}
	if files[MacroSty] == nil || files[MainBib] == nil || files[ArxivReadme] != nil {
		t.Errorf("a book has the files %v", slices.Sorted(maps.Keys(files)))
	}
	tex := string(files[MainTeX])
	for _, want := range []string{
		`\documentclass[11pt]{book}`,
		`\usepackage{m2}`,
		`\phantomsection\label{Q529ES}`,
		`\begin{proof}\phantomsection\label{ENS73X}`,
		`\cite[Section 3.2, Theorem 8]{src-book-isbn-9780471433347}`,
		`\bibitem{src-book-isbn-9780471433347}`,
		`\href{https://math2.tamnd.com/node/2J2QXT/}`,
		`\end{document}`,
	} {
		if !strings.Contains(tex, want) {
			t.Errorf("main.tex has no %s", want)
		}
	}
	again, _ := d.LaTeX()
	if !sameFiles(files, again) {
		t.Error("two builds of the LaTeX are not the same")
	}
}

func TestPackArxiv(t *testing.T) {
	d := packed(t, fixtures, "kind: article\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\n")
	files, err := d.LaTeX()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files[MainTeX]), `\documentclass[11pt]{article}`) {
		t.Error("an article does not use the class article")
	}
	var readme struct {
		Process struct {
			Compiler string `json:"compiler"`
		} `json:"process"`
		Sources []struct {
			Filename string `json:"filename"`
			Usage    string `json:"usage"`
		} `json:"sources"`
		SpecVersion    int `json:"spec_version"`
		TeXLiveVersion int `json:"texlive_version"`
	}
	if err := json.Unmarshal(files[ArxivReadme], &readme); err != nil {
		t.Fatal(err)
	}
	if readme.Process.Compiler != "pdflatex" || readme.SpecVersion != 1 || readme.TeXLiveVersion != TeXLive {
		t.Errorf("00README.json is %s", files[ArxivReadme])
	}
	for _, s := range readme.Sources {
		if files[s.Filename] == nil {
			t.Errorf("00README.json names %s, which is not in the tree", s.Filename)
		}
		if s.Filename == MainTeX && s.Usage != "toplevel" {
			t.Errorf("main.tex has the usage %s", s.Usage)
		}
	}
}

func TestPackPreTeXt(t *testing.T) {
	d := packed(t, fixtures, "kind: book\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\n")
	files, err := d.PreTeXt()
	if err != nil {
		t.Fatal(err)
	}
	ptx := files[MainPTX]
	dec := xml.NewDecoder(bytes.NewReader(ptx))
	ids := map[string]bool{}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("main.ptx is not well-formed XML: %v", err)
		}
		if e, ok := tok.(xml.StartElement); ok {
			for _, a := range e.Attr {
				if a.Name.Local == "id" {
					if ids[a.Value] {
						t.Errorf("main.ptx has the id %s two times", a.Value)
					}
					ids[a.Value] = true
				}
			}
		}
	}
	for _, id := range []string{"Q529ES", "GY1XDB", "m2-7SW3MR", "src-book-isbn-9780471433347"} {
		if !ids[id] {
			t.Errorf("main.ptx has no element with the id %s", id)
		}
	}
	for _, want := range []string{`<pretext xml:lang="en-US">`, `<book `, `<definition xml:id="GY1XDB"`, `<xref ref="src-book-isbn-9780471433347" detail="Section 3.2, Theorem 8"/>`} {
		if !bytes.Contains(ptx, []byte(want)) {
			t.Errorf("main.ptx has no %s", want)
		}
	}
	again, _ := d.PreTeXt()
	if !sameFiles(files, again) {
		t.Error("two builds of the PreTeXt are not the same")
	}
}

func TestPackOutput(t *testing.T) {
	d := packed(t, fixtures, "kind: book\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\n")
	for _, f := range Backends {
		if files, err := d.Output(f); err != nil || len(files) == 0 {
			t.Errorf("Output(%s) gives %d files, %v", f, len(files), err)
		}
	}
	if _, err := d.Output("jats"); err == nil {
		t.Error("Output(jats) gives no error")
	}
}

func TestPackSources(t *testing.T) {
	d := packed(t, fixtures, "kind: book\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\n")
	if len(d.Sources) != 1 {
		t.Fatalf("the sources are %+v", d.Sources)
	}
	s := d.Sources[0]
	if s.Number != 1 || s.Key != "src-book-isbn-9780471433347" || s.Title != "ISBN 9780471433347" {
		t.Errorf("the source is %+v", s)
	}
	for _, it := range d.Items() {
		if it.Tag == "Q529ES" && (len(it.Sources) != 1 || it.Sources[0].Locator != "Section 3.2, Theorem 8") {
			t.Errorf("the citations of Q529ES are %+v", it.Sources)
		}
	}
}

func TestCreditString(t *testing.T) {
	for _, c := range []struct {
		c    Credit
		want string
	}{
		{Credit{Person: "github:ada", Edited: true, Nodes: 3, Glue: 1}, "github:ada, editor, 3 nodes, 1 part of the editorial text"},
		{Credit{Person: "github:bob", Nodes: 1, Reviews: 2}, "github:bob, 1 node, 2 reviews"},
		{Credit{Person: "github:eve"}, "github:eve"},
	} {
		if got := c.c.String(); got != c.want {
			t.Errorf("the credit is %q, not %q", got, c.want)
		}
	}
}

func TestPackCites(t *testing.T) {
	// The statement of Lagrange's theorem links to 2J2QXT, which is not in the volume. ENS73X links to DTT8HP in its section Idea, which the volume does not show.
	d := packed(t, fixtures, "kind: book\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\n")
	files, err := d.HTML()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(d.Cited, func(r Ref) bool { return r.Tag == "2J2QXT" }) || slices.ContainsFunc(d.Cited, func(r Ref) bool { return r.Tag == "DTT8HP" }) {
		t.Fatalf("the cited nodes are %v", d.Cited)
	}
	if want := `href="` + DefaultSite + `/node/2J2QXT/" title="A4 has no subgroup of order 6, m2://2J2QXT" class="cite"`; !strings.Contains(string(files["index.html"]), want) {
		t.Errorf("the page has no citation %s", want)
	}
}

func TestPackLock(t *testing.T) {
	root := copyTree(t)
	d := packed(t, root, "kind: book\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\nrelease: 2026.10\n")
	if got := strings.Join(findings(d.Check(), "PC9"), "\n"); !strings.Contains(got, "does not exist") {
		t.Fatalf("PC9 is %s", got)
	}
	data, err := d.Lock().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("node: Q529ES@2026.10")) || !bytes.HasPrefix(data, []byte(lockHeader)) {
		t.Errorf("the lock is\n%s", data)
	}
	if err := os.MkdirAll(filepath.Join(root, Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(LockFile("v"))), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := d.Check(); state(r, "PC9") != check.Pass {
		t.Errorf("PC9 is %v", findings(r, "PC9"))
	}
}

func TestPackUnknownFormat(t *testing.T) {
	r := packed(t, fixtures, "kind: book\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\nformats: [html, latex, jats]\n").Check()
	if got := findings(r, "PC9"); len(got) != 1 || !strings.Contains(got[0], "jats has no back end") {
		t.Errorf("PC9 is %v", got)
	}
}

func TestPackExercises(t *testing.T) {
	r := packed(t, fixtures, "kind: course\ntargets: [Q529ES]\nchoose: {DSG7KN: GY1XDB}\nexercises: {per_section: 1}\n").Check()
	if state(r, "PC10") != check.Fail {
		t.Errorf("PC10 is %s, want fail: the tree has no exercises", state(r, "PC10"))
	}
}

func TestPackMissingTarget(t *testing.T) {
	r := packed(t, fixtures, "kind: book\ntargets: [ZZZZZZ, msc:20-XX]\nchoose: {DSG7KN: GY1XDB}\n").Check()
	if got := strings.Join(findings(r, "PC6"), "\n"); !strings.Contains(got, "ZZZZZZ is not a node") {
		t.Errorf("PC6 is %s", got)
	}
}
