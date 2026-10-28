package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/wpcorpus"
)

// edgeCorpus writes a corpus of two articles in scope and runs m2 wp extract on it.
func edgeCorpus(t *testing.T) string {
	t.Helper()
	root := wpCorpus(t)
	var scope bytes.Buffer
	cs := []*wpcorpus.Candidate{
		{PageID: 100, Title: "Widget", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 200, Title: "Gadget", InScope: true, Verdict: "math", Rule: "strong"},
	}
	if err := wpcorpus.WriteCandidates(&scope, cs, "20261001", true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, wpcorpus.ScopePath), scope.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	pages := []*wpcorpus.Page{
		{ID: 100, Revision: 1, Title: "Widget", Wikitext: "A '''widget''' is a part of a [[gadget]].\n\n== Statement ==\n{{Math theorem|name=Widget lemma|math_statement=Each widget is in a [[Gadget#Gadget theorem|gadget]] and a [[Sprocket]].}}\n"},
		{ID: 200, Revision: 2, Title: "Gadget", Wikitext: "A '''gadget''' is a tool.\n\n== Gadget theorem ==\nEach gadget has a [[widget]].\n"},
	}
	for _, p := range pages {
		if err := wpcorpus.WriteContent(root, p); err != nil {
			t.Fatal(err)
		}
	}
	if code, stdout, stderr := run("wp", "extract", "-corpus", root); code != ExitOK {
		t.Fatalf("extract: exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	return root
}

func TestWPEdges(t *testing.T) {
	root := edgeCorpus(t)
	code, stdout, stderr := run("wp", "edges", "-corpus", root)
	if code != ExitOK {
		t.Fatalf("edges: exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "2 pages, 4 edges, 0 broken anchors, 0 red links, 0 failed") {
		t.Errorf("stdout %q", stdout)
	}
	f, err := wpcorpus.ReadEdges(root, 100)
	if err != nil {
		t.Fatal(err)
	}
	objs, err := wpcorpus.ReadObjects(root, 200, 2)
	if err != nil {
		t.Fatal(err)
	}
	thm := ""
	for _, o := range objs {
		if o.Name == "Gadget theorem" && o.Kind != "section" {
			thm = wpcorpus.MentionID(200, o.Tag)
		}
	}
	var got []string
	for _, e := range f.Edges {
		got = append(got, e.To+e.Target+" "+e.Method+" "+e.Predicate)
	}
	want := []string{"wikipedia://en/200 link-in-lead uses", thm + " link-in-item uses", "Sprocket link-in-item uses"}
	if thm == "" || strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("edges %q, want %q", got, want)
	}
	report, err := os.ReadFile(filepath.Join(root, wpcorpus.EdgesReportPath))
	if err != nil || !strings.Contains(string(report), "| link-in-item | 3 |") {
		t.Errorf("report %s, %v", report, err)
	}

	// A page whose objects file is not the output of the extractor fails.
	path := filepath.Join(root, wpcorpus.WikitextPath(200))
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, bytes.Replace(data, []byte("a tool"), []byte("a new tool"), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run("wp", "edges", "-corpus", root, "-page", "100"); code != ExitFailure || !strings.Contains(stderr, "Run m2 wp extract again") {
		t.Errorf("exit code %d, stderr %q", code, stderr)
	}
}

func TestCorpusImportOfWikipedia(t *testing.T) {
	root := edgeCorpus(t)
	if code, stdout, stderr := run("wp", "edges", "-corpus", root); code != ExitOK {
		t.Fatalf("edges: exit code %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "c"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	tree := t.TempDir()
	code, stdout, stderr := run("corpus", "import", "-root", tree, "-commit", "c", "-source", "wikipedia", "-corpus", root)
	if code != ExitOK || !strings.Contains(stdout, "wikipedia: 5 mentions from 2 articles") || !strings.Contains(stdout, "3 edges") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	ms, err := inventory.ReadMentions(tree)
	if err != nil || len(ms) != 5 {
		t.Fatalf("%d mentions, %v", len(ms), err)
	}
	es, err := inventory.ReadEdges(tree, "wikipedia")
	if err != nil || len(es) != 3 {
		t.Fatalf("%d edges, %v", len(es), err)
	}
	if code, _, stderr := run("corpus", "import", "-root", tree, "-commit", "c", "-source", "wikipedia"); code != ExitUsage || !strings.Contains(stderr, "-corpus") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}
