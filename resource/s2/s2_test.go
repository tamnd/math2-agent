package s2

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

func TestBuild(t *testing.T) {
	in := &resource.Scope{Value: resource.In}
	rs := []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "article", Title: "A", Scope: in, IDs: resource.IDs{"doi": {"10.1/a"}}},
		{ID: "8F2BBBBBB6", Kind: "preprint", Title: "B", Scope: in, Expressions: []resource.Expression{{Key: "e1", IDs: resource.IDs{"arxiv": {"2101.00001"}}}}},
		{ID: "8F2CCCCCC9", Kind: "article", Title: "C", Scope: in, IDs: resource.IDs{"doi": {"10.1/c"}, "s2": {"5"}}},
		{ID: "8F2DDDDDD2", Kind: "article", Title: "D", Scope: &resource.Scope{Value: resource.Out}, IDs: resource.IDs{"doi": {"10.1/d"}}},
		{ID: "8F2EEEEEE5", Kind: "article", Title: "E", Scope: in, IDs: resource.IDs{"doi": {"10.1/e"}}},
	}
	var asked [][]string
	lookup := func(_ context.Context, q []string) ([]*Paper, error) {
		asked = append(asked, q)
		out := make([]*Paper, len(q))
		for i, id := range q {
			switch id {
			case "DOI:10.1/a":
				out[i] = &Paper{CorpusID: 11, Title: " Paper  A ", Year: 2001, ExternalIDs: map[string]any{"DOI": "10.1/A", "ArXiv": "math/0101001", "MAG": "7"}}
			case "ARXIV:2101.00001":
				out[i] = &Paper{CorpusID: 22, Title: "B"}
			}
		}
		return out, nil
	}
	recs, n, err := Build(context.Background(), rs, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || strings.Join(asked[0], " ") != "DOI:10.1/a ARXIV:2101.00001 DOI:10.1/e" {
		t.Errorf("asked %q", asked)
	}
	if n.Asked != 3 || n.Found != 2 || n.Requests != 1 || len(recs) != 2 {
		t.Fatalf("counts %+v, records %+v", n, recs)
	}
	a := recs[0]
	if a.NativeID != "11" || a.Level != "expression" || a.Kind != "article" || a.Title != "Paper A" || a.Date != "2001" || a.IDs["s2"][0] != "11" || a.IDs["doi"][0] != "10.1/a" || a.IDs["arxiv"][0] != "math/0101001" {
		t.Errorf("record %+v", a)
	}
	if b := recs[1]; b.Kind != "preprint" || b.IDs["arxiv"][0] != "2101.00001" {
		t.Errorf("record %+v", b)
	}
}

func TestBatchSize(t *testing.T) {
	in := &resource.Scope{Value: resource.In}
	rs := make([]resource.Resource, BatchSize+1)
	for i := range rs {
		rs[i] = resource.Resource{ID: "x", Kind: "article", Title: "x", Scope: in, IDs: resource.IDs{"doi": {"10.1/" + strings.Repeat("a", i+1)}}}
	}
	calls := 0
	_, n, err := Build(context.Background(), rs, func(_ context.Context, q []string) ([]*Paper, error) {
		calls++
		if len(q) > BatchSize {
			t.Errorf("a batch of %d ids", len(q))
		}
		return make([]*Paper, len(q)), nil
	})
	if err != nil || calls != 2 || n.Requests != 2 || n.Found != 0 {
		t.Errorf("calls %d, counts %+v, %v", calls, n, err)
	}
}

func TestBatch(t *testing.T) {
	post := func(_ context.Context, url string, body []byte) ([]byte, error) {
		var in struct{ IDs []string }
		if err := json.Unmarshal(body, &in); err != nil || len(in.IDs) == 0 || url != "u" {
			t.Errorf("body %s, url %s", body, url)
		}
		return []byte(`[{"corpusId":1,"title":"T"},null]`), nil
	}
	ps, err := Batch("u", post)(context.Background(), []string{"DOI:10.1/a", "DOI:10.1/b"})
	if err != nil || len(ps) != 2 || ps[0].CorpusID != 1 || ps[1] != nil {
		t.Errorf("papers %+v, %v", ps, err)
	}
	if _, err := Batch("u", post)(context.Background(), []string{"DOI:10.1/a"}); err == nil {
		t.Error("an answer with more papers than ids must be an error")
	}
}
