package dblp

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// dump is a small dump. The byte \xf6 is ö in ISO-8859-1.
const dump = "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>\n<!DOCTYPE dblp SYSTEM \"dblp.dtd\">\n<dblp>\n" +
	`<article mdate="2020-01-01" key="journals/tcs/Erdos90"><author>Paul Erd&ouml;s</author><author orcid="0000-0002-1825-0097">John Smith 0001</author><title>On <i>sums</i> of sets.</title>` +
	`<pages>101-110</pages><year>1990</year><volume>75</volume><journal>Theor. Comput. Sci.</journal><ee>https://doi.org/10.1016/0304-3975(90)90001-A</ee></article>` + "\n" +
	`<www key="homepages/e/PaulErdos"><author>Paul Erd&ouml;s</author><title>Home Page</title></www>` + "\n" +
	`<article key="journals/corr/abs-2001-00001" publtype="informal"><author>Ann B</author><title>A note.</title><year>2020</year><journal>CoRR</journal><ee>https://arxiv.org/abs/2001.00001</ee></article>` + "\n" +
	`<article key="journals/cacm/X20"><author>C D</author><title>Not kept.</title><year>2020</year></article>` + "\n" +
	`<inproceedings key="conf/stoc/G` + "\xf6" + `del31"><author>Kurt G` + "\xf6" + `del</author><title>Formally undecidable.</title><booktitle>STOC</booktitle><year>1931</year><crossref>conf/stoc/1931</crossref></inproceedings>` + "\n" +
	`<proceedings key="conf/stoc/1931"><editor>E F</editor><title>Proceedings of STOC 1931</title><publisher>ACM</publisher><isbn>978-3-540-20000-0</isbn><year>1931</year></proceedings>` + "\n" +
	`<book key="books/sp/Knuth97"><editor>Don K</editor><title>A book in the tree</title><isbn>3-540-20000-2</isbn><publisher>Springer</publisher><year>1997</year></book>` + "\n" +
	`<phdthesis key="phd/de/Nobody"><author>N O</author><title>A thesis not kept</title><school>Bonn</school><year>2000</year></phdthesis>` + "\n" +
	"</dblp>\n"

func TestFetchAndRecords(t *testing.T) {
	root := t.TempDir()
	rs := []resource.Resource{{ID: "r-book", Kind: "edited-volume", Title: "A book in the tree", IDs: resource.IDs{"isbn": {"9783540200000"}}}}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "dblp.xml.gz")
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = io.WriteString(zw, dump)
	_ = zw.Close()
	if err := os.WriteFile(path, gz.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: path, Root: root}, &buf); err != nil {
		t.Fatal(err)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(snap)
	var out []cluster.Record
	if err := (Adapter{}).Records(bytes.NewReader(data), func(r cluster.Record) error { out = append(out, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(out) != 4 {
		t.Fatalf("records %+v", out)
	}
	r := out[0]
	if r.NativeID != "journals/tcs/Erdos90" || r.Kind != "article" || r.Level != "work" || r.Title != "On sums of sets" || r.Date != "1990" || r.Volume != "75" || r.Page != "101" ||
		r.Links[0] != "https://dblp.org/rec/journals/tcs/Erdos90" {
		t.Errorf("record %+v", r)
	}
	if r.IDs["doi"][0] != "10.1016/0304-3975(90)90001-a" || len(r.Creators) != 2 || r.Creators[0].Name != "Paul Erdös" || r.Creators[1].Name != "John Smith" {
		t.Errorf("ids %+v creators %+v", r.IDs, r.Creators)
	}
	if r := out[1]; r.NativeID != "conf/stoc/Gödel31" || r.Kind != "conference-paper" || r.Creators[0].Name != "Kurt Gödel" {
		t.Errorf("record %+v", r)
	}
	if r := out[2]; r.Kind != "proceedings" || r.Level != "manifestation" || r.Publisher != "ACM" || r.Creators[0].Role != "editor" || r.IDs["isbn"][0] != "9783540200000" {
		t.Errorf("record %+v", r)
	}
	if r := out[3]; r.NativeID != "books/sp/Knuth97" || r.Kind != "edited-volume" || r.Level != "manifestation" || r.IDs["isbn"][0] != "9783540200000" {
		t.Errorf("record %+v", r)
	}
	var ir []ingest.Record
	if err := (Adapter{}).Read(bytes.NewReader(data), func(r ingest.Record) error { ir = append(ir, r); return nil }); err != nil || len(ir) != 4 || ir[0].URL != "https://dblp.org/rec/journals/tcs/Erdos90" {
		t.Errorf("ingest records %+v, %v", ir, err)
	}
}

func TestEachAndKind(t *testing.T) {
	var ps []Publication
	err := Each(context.Background(), bytes.NewReader([]byte(dump)), func(p Publication) error { ps = append(ps, p); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 7 {
		t.Fatalf("publications %+v", ps)
	}
	if k := Kind(ps[1]); k != "preprint" || ps[1].ArXiv[0] != "2001.00001" {
		t.Errorf("kind %s of %+v", k, ps[1])
	}
	if k := Kind(ps[6]); k != "thesis" || Keep(ps[6], nil) {
		t.Errorf("kind %s of %+v", k, ps[6])
	}
	if !Keep(ps[1], map[string]bool{"arxiv:2001.00001": true}) || Keep(ps[2], nil) {
		t.Error("keep")
	}
}
