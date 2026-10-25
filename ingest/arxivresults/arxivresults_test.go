package arxivresults_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/arxivresults"
)

const commit = "8a1f0c2d3e4b5a69788796a5b4c3d2e1f0a1b2c3"

// archive makes a git archive like the one GitHub gives, with the files in one directory, in the order of the arguments.
func archive(t *testing.T, files ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": commit}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(files); i += 2 {
		name, body := files[i], files[i+1]
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "arxiv-main/" + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(body))
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()
	return buf.Bytes()
}

// front makes the front matter of a content file.
func front(paper, cats, licence, access, local string) string {
	return "---\npaper: \"" + paper + "\"\nversion: v3\ncategories: [" + cats + "]\nmsc_class: \"20D60; 20-01\"\naccess: " + access + "\nlicence_of_source: " + licence + "\nlocal_id: " + local + "\npath: source\n---\n"
}

const groups = `
**Definition 1.1** {#def-1 .definition tag=AB12 env=definition}

The order of a finite group is the number of its elements.

**Theorem 1.2** (Lagrange) {#thm-1 .statement tag=CD34 env=theorem}

The order of a subgroup $H$ of $G$ divides the order of $G$:
$$
|G| = [G : H] \, |H|.
$$
{#eq-1 .equation tag=EQ01}
Here $[G : H]$ is the number of cosets.

**Figure 1** {#fig-1 .figure tag=FG01}

A picture of the cosets.

**Remark 1.3** {#rem-1 .remark tag=RM01 env=remark}

See [Theorem 1.2](#thm-1).

**Corollary 1.4** {#cor-1 .statement env=corollary}

The order of an element divides the order of the group, by [(1)](#eq-1).
`

const appendix = `
**Proof of [Theorem 1.2](#thm-1)** {#proof-1 .proof tag=PR01}

The cosets of $H$ have $|H|$ elements each, by [Definition 1.1](#def-1), and they cover $G$. See also [Figure 1](#fig-1) and [the remark](#rem-1).
`

func read(t *testing.T, tgz []byte) []ingest.Record {
	t.Helper()
	a := arxivresults.Adapter{}
	src := a.Source()
	recs, err := ingest.Import(a, &src, bytes.NewReader(tgz))
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

func TestRead(t *testing.T) {
	recs := read(t, archive(t,
		"README.md", "# arxiv\n",
		"content/en/2601/2601.01234/02_groups.md", front("2601.01234", "math.GR", "cc-by", "open", "s2")+groups,
		"content/en/2601/2601.01234/09_appendix.md", front("2601.01234", "math.GR", "cc-by", "open", "a1")+appendix,
		"content/vi/2601/2601.01234/02_groups.md", front("2601.01234", "math.GR", "cc-by", "open", "s2")+groups,
		"content/en/2601/2601.05678/01_loss.md", front("2601.05678", "cs.LG", "cc-by", "open", "s1")+groups,
		"content/en/2601/2601.09999/01_closed.md", front("2601.09999", "math.GR", "arxiv-1.0", "record", "s1")+groups,
	))
	var ids []string
	by := map[string]ingest.Record{}
	for _, r := range recs {
		ids = append(ids, r.ID)
		by[r.ID] = r
	}
	if got, want := strings.Join(ids, " "), "2601.01234#AB12 2601.01234#CD34 2601.01234#cor-1"; got != want {
		t.Fatalf("the ids are %s, want %s", got, want)
	}
	thm := by["2601.01234#CD34"]
	if thm.Kind != "statement" || thm.Title != "Theorem 1.2 (Lagrange)" || thm.Locator != "ax://paper/2601.01234#CD34" || thm.URL != "https://arxiv.org/abs/2601.01234v3" || thm.Licence != "CC-BY-4.0" || thm.Class != ingest.Open {
		t.Errorf("the theorem is %+v", thm)
	}
	if !strings.Contains(thm.Text, "|G| = [G : H]") || !strings.HasSuffix(thm.Text, "number of cosets.") || strings.Contains(thm.Text, "picture") {
		t.Errorf("the text of the theorem is %q", thm.Text)
	}
	want := map[string]string{
		"paper": "2601.01234", "version": "v3", "env": "theorem", "role": "theorem", "number": "1.2", "local": "thm-1", "section": "s2",
		"file": "02_groups.md", "path": "source", "proofs": "2601.01234#PR01", "deps": "2601.01234#AB12", "msc": "20D60 20-01", "categories": "math.GR",
	}
	for k, v := range want {
		if thm.Meta[k] != v {
			t.Errorf("meta %s is %q, want %q", k, thm.Meta[k], v)
		}
	}
	if len(thm.Meta) != len(want) {
		t.Errorf("the meta is %v", thm.Meta)
	}
	cor := by["2601.01234#cor-1"]
	if cor.Locator != "ax://paper/2601.01234#cor-1" || cor.Meta["deps"] != "2601.01234#CD34" || cor.Meta["role"] != "corollary" {
		t.Errorf("the corollary is %+v", cor)
	}
	if def := by["2601.01234#AB12"]; def.Kind != "definition" || def.Meta["role"] != "" || def.Meta["deps"] != "" {
		t.Errorf("the definition is %+v", def)
	}
}

func TestReadLicence(t *testing.T) {
	recs := read(t, archive(t, "content/en/2601/2601.01234/02_groups.md", front("2601.01234", "math.GR", "cc-by-nc-nd", "verbatim", "s2")+groups))
	if len(recs) != 3 {
		t.Fatalf("%d records", len(recs))
	}
	for _, r := range recs {
		if r.Class != ingest.Verbatim || r.Text != "" || len(r.Fingerprints) == 0 || r.Meta["derivatives"] != "no" || r.Licence != "CC-BY-NC-ND-4.0" {
			t.Errorf("%s: %+v", r.ID, r)
		}
	}
}

func TestReadProofs(t *testing.T) {
	body := `
**Lemma 1** {#lem-1 .statement tag=LM01 env=lemma}

Each tree with two vertices or more has a leaf.

**Proof.** {#proof-1 .proof tag=PR01}

Take a longest path.

**Remark 2** {#rem-1 .remark tag=RM01 env=remark}

A remark.

**Proof.** {#proof-2 .proof tag=PR02}

This proof follows a remark, so no result has it. It links to [Lemma 1](#lem-1) in the text.

**Lemma 3** {#lem-2 .statement tag=LM02 env=hilfssatz}

Each tree with $n$ vertices has $n - 1$ edges.

**Proof.** {#proof-3 .proof tag=PR03}

By [Lemma 1](#lem-1) and induction, with [Lemma 3](#lem-2) for smaller trees.
`
	recs := read(t, archive(t, "content/en/2601/2601.04321/01_trees.md", front("2601.04321", "math.CO", "cc0", "open", "s1")+body))
	if len(recs) != 2 {
		t.Fatalf("%d records", len(recs))
	}
	if r := recs[0]; r.ID != "2601.04321#LM01" || r.Meta["proofs"] != "2601.04321#PR01" || r.Meta["deps"] != "" {
		t.Errorf("the first lemma is %+v", r)
	}
	if r := recs[1]; r.Meta["proofs"] != "2601.04321#PR03" || r.Meta["deps"] != "2601.04321#LM01" || r.Meta["role"] != "" || r.Meta["env"] != "hilfssatz" || r.Meta["number"] != "3" {
		t.Errorf("the second lemma is %+v", r)
	}
}

func TestReadErrors(t *testing.T) {
	a := arxivresults.Adapter{}
	src := a.Source()
	for name, files := range map[string][]string{
		"no front matter": {"content/en/2601/2601.01234/01.md", "# A title\n"},
		"two versions": {
			"content/en/2601/2601.01234/01.md", front("2601.01234", "math.GR", "cc-by", "open", "s1"),
			"content/en/2601/2601.01234/02.md", strings.Replace(front("2601.01234", "math.GR", "cc-by", "open", "s2"), "v3", "v4", 1),
		},
		"no paper": {"content/en/2601/2601.01234/01.md", front("", "math.GR", "cc-by", "open", "s1")},
	} {
		if _, err := ingest.Import(a, &src, bytes.NewReader(archive(t, files...))); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestPull(t *testing.T) {
	tgz := archive(t, "content/en/2601/2601.01234/02_groups.md", front("2601.01234", "math.GR", "cc-by", "open", "s2")+groups)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) }))
	defer srv.Close()
	res, err := ingest.Pull(context.Background(), arxivresults.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: srv.URL + "/main.tar.gz", Date: "2026-10-26"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Manifest.Records != 3 || res.Snapshot.Commit != commit {
		t.Errorf("the pull gave %d records at %q", res.Manifest.Records, res.Snapshot.Commit)
	}
}
