package thesesfr

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

const facets = `[{"name":"Statut","checkboxes":[{"name":"soutenue"}]},
{"name":"Disciplines","checkboxes":[{"name":"Mathématiques appliquées"},{"name":"Physique"},{"name":"MATHEMATIQUES"},{"name":"Math \"pures\""}]}]`

// thesisA has the shape that the API gave on 2026-10-31.
const thesisA = `{"id":"2020UPASS008","titrePrincipal":"Limites d'ensembles  quasiminimaux","titreEN":"Limits of quasiminimal sets","etabSoutenanceN":"université Paris-Saclay","etabSoutenancePpn":"241345251",
"dateSoutenance":"06/01/2020","auteurs":[{"ppn":"248088939","nom":"Labourie","prenom":"Camille"}],"directeurs":[{"ppn":"072229802","nom":"David","prenom":"Guy"}],
"rapporteurs":[{"ppn":"086953192","nom":"De Pauw","prenom":"Thierry"}],"nnt":"2020UPASS008","doi":"10.70675/1372869ezdef2z496czbac7z204c5ae67aae","discipline":"Mathématiques fondamentales","status":"soutenue"}`

const thesisB = `{"id":"1965REN0S021","titrePrincipal":"Sur les groupes","titreEN":"","etabSoutenanceN":"Université de Rennes (1896-1969)","dateSoutenance":"01/01/1965",
"auteurs":[{"ppn":null,"nom":"Tchen","prenom":"Henri"}],"directeurs":[{"ppn":null,"nom":"Directeur de thèse inconnu","prenom":""}],"nnt":"1965REN0S021","doi":null,"status":"soutenue"}`

const thesisC = `{"id":"2019PESC0034","titrePrincipal":"Équations dispersives","titreEN":"Équations dispersives","etabSoutenanceN":"Paris Est","dateSoutenance":"27/03/2019",
"auteurs":[{"ppn":"12345678X","nom":"Doe","prenom":"Jane"}],"directeurs":[{"ppn":"072229802","nom":"David","prenom":"Guy"},{"ppn":"","nom":"Perelman","prenom":"Galina"}],"nnt":"2019PESC0034","status":"soutenue"}`

var years = regexp.MustCompile(`datedebut="([0-9]+)"~datefin="([0-9]+)"`)

func TestFetchAndRecords(t *testing.T) {
	Gap = 0
	var queries int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		queries++
		if q.Get("mailto") != "" || q.Get("api_key") != "" {
			t.Errorf("query %v", q)
		}
		if strings.HasSuffix(r.URL.Path, "/facets/") {
			_, _ = io.WriteString(w, facets)
			return
		}
		f := q.Get("filtres")
		if q.Get("nombre") != "1000" || q.Get("tri") != "dateAsc" || !strings.Contains(f, `Statut="soutenue"`) {
			t.Errorf("query %v", q)
		}
		page := func(total int, theses ...string) {
			_, _ = fmt.Fprintf(w, `{"totalHits":%d,"theses":[%s]}`, total, strings.Join(theses, ","))
		}
		switch {
		case strings.Contains(f, "Disciplines"):
			if f != `[Disciplines="Mathématiques appliquées"~Disciplines="MATHEMATIQUES"~Statut="soutenue"]` {
				t.Errorf("filter %s", f)
			}
			page(2, thesisA, thesisC)
		case f == `[Domaines thématiques="Mathématiques"~Statut="soutenue"]`:
			page(1500, thesisA)
		default:
			m := years.FindStringSubmatch(f)
			if m == nil {
				t.Errorf("filter %s", f)
				page(0)
				return
			}
			from, _ := strconv.Atoi(m[1])
			to, _ := strconv.Atoi(m[2])
			switch {
			case from == 2020 && to == 2020 && q.Get("debut") == "0":
				page(1200, thesisA)
			case from == 2020 && to == 2020 && q.Get("debut") == "1000":
				page(1200, thesisB)
			case from <= 2020 && 2020 <= to:
				page(1500)
			default:
				page(0)
			}
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
	var buf bytes.Buffer
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL + "/api/v1/theses", Get: get}, &buf); err != nil {
		t.Fatal(err)
	}
	if queries < 10 {
		t.Errorf("only %d queries, so the ranges of years were not split", queries)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var rs []cluster.Record
	if err := (Adapter{}).Records(snap, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 || rs[0].NativeID != "2020UPASS008" || rs[1].NativeID != "1965REN0S021" || rs[2].NativeID != "2019PESC0034" {
		t.Fatalf("records %+v", rs)
	}
	a := rs[0]
	if a.Kind != "thesis" || a.Thesis != "doctoral" || a.Level != "work" || a.Title != "Limites d'ensembles quasiminimaux" || a.TitleEn != "Limits of quasiminimal sets" ||
		a.Date != "2020-01-06" || a.Publisher != "université Paris-Saclay" || a.Links[0] != "https://theses.fr/2020UPASS008" {
		t.Errorf("record %+v", a)
	}
	if a.IDs["nnt"][0] != "2020UPASS008" || a.IDs["doi"][0] != "10.70675/1372869ezdef2z496czbac7z204c5ae67aae" {
		t.Errorf("ids %v", a.IDs)
	}
	if len(a.Creators) != 2 || a.Creators[0].Name != "Camille Labourie" || a.Creators[0].Role != "author" || a.Creators[0].IDs["idref"][0] != "248088939" ||
		a.Creators[1].Name != "Guy David" || a.Creators[1].Role != "advisor" || a.Creators[1].IDs["idref"][0] != "072229802" {
		t.Errorf("creators %+v", a.Creators)
	}
	if b := rs[1]; len(b.Creators) != 1 || b.Creators[0].IDs != nil || b.Date != "1965-01-01" || b.IDs["doi"] != nil || b.TitleEn != "" {
		t.Errorf("record %+v", b)
	}
	if c := rs[2]; c.TitleEn != "" || len(c.Creators) != 3 || c.Creators[0].IDs != nil || c.Creators[2].Role != "advisor" || c.Creators[2].IDs != nil {
		t.Errorf("record %+v", c)
	}
	for _, r := range rs {
		if why := cluster.Check(&r); why != "" {
			t.Errorf("%s: %s", r.NativeID, why)
		}
	}
}

func TestDate(t *testing.T) {
	for in, want := range map[string]string{"06/01/2020": "2020-01-06", "2020": "", "": "", "6/1/2020": ""} {
		if got := Date(in); got != want {
			t.Errorf("Date(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRead(t *testing.T) {
	var recs []ingest.Record
	if err := (Adapter{}).Read(strings.NewReader(strings.ReplaceAll(thesisA, "\n", "")+"\n"), func(r ingest.Record) error { recs = append(recs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].ID != "2020UPASS008" || recs[0].Kind != "thesis" || recs[0].URL != "https://theses.fr/2020UPASS008" {
		t.Errorf("records %+v", recs)
	}
	if err := (Adapter{}).Records(strings.NewReader(`{"titrePrincipal":"x"}`), func(cluster.Record) error { return nil }); err == nil {
		t.Error("a thesis with no id gives no error")
	}
}
