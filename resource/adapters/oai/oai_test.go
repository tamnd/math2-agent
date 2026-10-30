package oai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHarvest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("metadataPrefix") == "dc" && q.Get("set") == "math":
			_, _ = io.WriteString(w, `<OAI-PMH><ListRecords><record><header><identifier>a</identifier><setSpec>math</setSpec></header><metadata><dc><title>G&G</title></dc></metadata></record>
<record><header status="deleted"><identifier>b</identifier></header></record><resumptionToken cursor="0">T1</resumptionToken></ListRecords></OAI-PMH>`)
		case q.Get("resumptionToken") == "T1" && q.Get("set") == "":
			_, _ = io.WriteString(w, `<OAI-PMH><ListRecords><record><header><identifier>c</identifier></header><metadata><dc><title>X</title></dc></metadata></record><resumptionToken/></ListRecords></OAI-PMH>`)
		case q.Get("set") == "none":
			_, _ = io.WriteString(w, `<OAI-PMH><error code="noRecordsMatch">no records</error></OAI-PMH>`)
		default:
			_, _ = io.WriteString(w, `<OAI-PMH><error code="badArgument">bad argument</error></OAI-PMH>`)
		}
	}))
	defer srv.Close()
	get := func(ctx context.Context, u string) (io.ReadCloser, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := srv.Client().Do(req)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	var got []string
	err := Harvest(context.Background(), get, srv.URL, "dc", "math", func(r Record) error {
		var dc struct {
			Title string `xml:"title"`
		}
		if !r.Deleted {
			if err := Unmarshal(r.Metadata, &dc); err != nil {
				return err
			}
		}
		got = append(got, r.ID+":"+dc.Title+":"+strings.Join(r.Sets, ","))
		return nil
	})
	if err != nil || strings.Join(got, " ") != "a:G&G:math b:: c:X:" {
		t.Errorf("Harvest = %v, %v", got, err)
	}
	if err := Harvest(context.Background(), get, srv.URL, "dc", "none", func(Record) error { return nil }); err != nil {
		t.Errorf("noRecordsMatch gave %v", err)
	}
	if err := Harvest(context.Background(), get, srv.URL, "x", "", func(Record) error { return nil }); err == nil || !strings.Contains(err.Error(), "bad argument") {
		t.Errorf("badArgument gave %v", err)
	}
}
