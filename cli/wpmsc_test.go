package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/wpcorpus"
)

func TestWPMSC(t *testing.T) {
	root := wpCorpus(t)
	cats := "version: 1\nroots:\n  - {title: Mathematics, depth: 1}\nmsc:\n  - {words: topology, code: 54-XX}\n  - {words: algebraic topology, code: 55-XX}\n  - {words: group theory, code: 20-XX}\n"
	if err := os.WriteFile(filepath.Join(root, wpcorpus.ManifestDir, "categories.yaml"), []byte(cats), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, md := range []*wpcorpus.Metadata{
		{Page: 1, Title: "Euler class", Revision: 11, Type: "concept", Categories: []string{"Topology"}},
		{Page: 2, Title: "Simple group", Revision: 21, Type: "concept", Categories: []string{"Group theory", "Topology"}},
		{Page: 3, Title: "Someone", Revision: 31, Type: "biography", Categories: []string{"Living people"}},
		{Page: 4, Title: "Euler lemma", Revision: 41, Type: "statement", Categories: []string{"Lemmas"}},
	} {
		if err := wpcorpus.WriteMetadata(root, md); err != nil {
			t.Fatal(err)
		}
	}
	lead := "# Euler class\n\n::: {#o1 .subject cue=lead name=\"Euler class\"}\n\nIn [mathematics](wikipedia://en/18831), specifically in [algebraic topology](wikipedia://en/2), the **Euler class** is a class.\n\n:::\n"
	path := filepath.Join(root, wpcorpus.MarkdownPath(1))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(lead), 0o600); err != nil {
		t.Fatal(err)
	}
	// The lead of page 4 links to page 1 (55-XX) and to page 2 (a tie), so the link to page 1 gives the code.
	lead4 := "# Euler lemma\n\n::: {#o1 .subject cue=lead name=\"Euler lemma\"}\n\nThe **Euler lemma** is about the [Euler class](wikipedia://en/1) and [simple groups](wikipedia://en/2).\n\n:::\n"
	if err := os.WriteFile(filepath.Join(root, wpcorpus.MarkdownPath(4)), []byte(lead4), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := runWP([]string{"msc", "-corpus", root, "-dry-run"}, &out, &errb); code != ExitOK {
		t.Fatalf("dry run: exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "4 articles, 3 with a code, 1 from the categories, 0 from the lead, 1 from both (0 agree), 1 with the links of the lead, 0 from the model, 1 with a tie, 1 with no signal, 0 written") {
		t.Errorf("dry run output:\n%s", out.String())
	}
	mds, err := wpcorpus.ReadMetadataFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, md := range mds {
		if len(md.MSC) > 0 {
			t.Errorf("the dry run wrote %d: %v", md.Page, md.MSC)
		}
	}

	out.Reset()
	if code := runWP([]string{"msc", "-corpus", root}, &out, &errb); code != ExitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), ", 3 written") || !strings.Contains(out.String(), "55 2\n") {
		t.Errorf("output:\n%s", out.String())
	}
	mds, err = wpcorpus.ReadMetadataFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int][]string{1: {"55-XX"}, 2: {"20-XX", "54-XX"}, 3: nil, 4: {"55-XX"}}
	for _, md := range mds {
		if !slices.Equal(md.MSC, want[md.Page]) {
			t.Errorf("page %d: msc %v, want %v", md.Page, md.MSC, want[md.Page])
		}
	}
	runs, err := filepath.Glob(filepath.Join(root, wpcorpus.RunsDir, "*-wp-msc.md"))
	if err != nil || len(runs) != 1 {
		t.Errorf("run records %v, %v", runs, err)
	}

	out.Reset()
	if code := runWP([]string{"msc", "-corpus", root}, &out, &errb); code != ExitOK || !strings.Contains(out.String(), ", 0 written") {
		t.Errorf("second run: exit %d, output:\n%s", code, out.String())
	}

	// The model decides the tie of page 2 and the article with no signal. Its
	// decision for page 1 does not count, because page 1 has one code.
	decisions := "version: 1\nmodel: test\ndecisions:\n  - {page: 1, code: 01-XX, confidence: 0.9}\n  - {page: 2, code: 20-XX, confidence: 0.8}\n  - {page: 3, code: 01-XX, confidence: 0.95}\n"
	if err := os.WriteFile(filepath.Join(root, wpcorpus.ManifestDir, wpcorpus.MSCModelFile), []byte(decisions), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := runWP([]string{"msc", "-corpus", root}, &out, &errb); code != ExitOK {
		t.Fatalf("model run: exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "4 articles, 4 with a code, 0 from the categories, 0 from the lead, 1 from both (0 agree), 1 with the links of the lead, 2 from the model, 0 with a tie, 0 with no signal, 2 written") {
		t.Errorf("model run output:\n%s", out.String())
	}
	mds, err = wpcorpus.ReadMetadataFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	want = map[int][]string{1: {"55-XX"}, 2: {"20-XX"}, 3: {"01-XX"}, 4: {"55-XX"}}
	for _, md := range mds {
		if !slices.Equal(md.MSC, want[md.Page]) {
			t.Errorf("model run: page %d: msc %v, want %v", md.Page, md.MSC, want[md.Page])
		}
	}
}
