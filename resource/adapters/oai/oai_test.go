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

func TestDC(t *testing.T) {
	var m DC
	err := Unmarshal([]byte(`<oai_dc:dc xmlns:oai_dc="http://www.openarchives.org/OAI/2.0/oai_dc/" xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:title xml:lang="ru">Об  уравнениях</dc:title><dc:title xml:lang="en">On equations</dc:title><dc:creator>Ivanov, I.</dc:creator><dc:creator> </dc:creator>
<dc:identifier>https://doi.org/10.1/x</dc:identifier></oai_dc:dc>`), &m)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Title) != 2 || m.Title[1].Lang != "en" || Texts(m.Title)[0] != "Об уравнениях" || len(Texts(m.Creator)) != 1 || len(m.Identifier) != 1 {
		t.Errorf("dc %+v", m)
	}
}
