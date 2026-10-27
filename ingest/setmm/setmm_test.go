package setmm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/setmm"
)

// read imports the fixture and returns its records by id, and the ids in order.
func read(t *testing.T, body string) (map[string]ingest.Record, []string) {
	t.Helper()
	a := setmm.Adapter{}
	src := a.Source()
	recs, err := ingest.Import(a, &src, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]ingest.Record{}
	var ids []string
	for _, r := range recs {
		out[r.ID] = r
		ids = append(ids, r.ID)
	}
	return out, ids
}

func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/set.mm")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRead(t *testing.T) {
	recs, ids := read(t, fixture(t))
	if got := strings.Join(ids, " "); got != "ax-1 ax-mp df-self id thm-nocomment" {
		t.Fatalf("the records are %s", got)
	}
	ax := recs["ax-1"]
	if ax.Kind != "statement" || ax.Title != "ax-1" || ax.URL != setmm.Site+"ax-1.html" || ax.Locator != "set.mm:27" || ax.Class != ingest.Open {
		t.Errorf("the record of ax-1 is %+v", ax)
	}
	if ax.Text != "The first axiom: a true formula follows from any formula. (Contributed by A. Tester, 1-Jan-2020.)" {
		t.Errorf("the text of ax-1 is %q", ax.Text)
	}
	want := map[string]map[string]string{
		"ax-1":          {"keyword": "axiom", "statement": "( ph -> ( ps -> ph ) )", "section": "First steps in logic", "contributed": "A. Tester, 1-Jan-2020"},
		"ax-mp":         {"keyword": "axiom", "statement": "ps", "hypotheses": "ph && ( ph -> ps )", "section": "First steps in logic", "contributed": "A. Tester, 2-Jan-2020"},
		"id":            {"keyword": "theorem", "statement": "( ph -> ph )", "section": "First steps in logic", "contributed": "B. Sample, 3-Feb-2021", "discouraged": "yes"},
		"thm-nocomment": {"keyword": "theorem", "statement": "( ps -> ps )", "section": "First steps in logic"},
		"df-self":       {"keyword": "definition", "statement": "( ch -> ch )", "section": "Words for formulas"},
	}
	for id, m := range want {
		if got := recs[id].Meta; len(got) != len(m) {
			t.Errorf("the meta of %s is %v, want %v", id, got, m)
			continue
		}
		for k, v := range m {
			if recs[id].Meta[k] != v {
				t.Errorf("the meta %s of %s is %q, want %q", k, id, recs[id].Meta[k], v)
			}
		}
	}
	if recs["df-self"].Kind != "definition" || recs["id"].Kind != "statement" {
		t.Errorf("the kinds are %s and %s", recs["df-self"].Kind, recs["id"].Kind)
	}
	if recs["thm-nocomment"].Text != "" {
		t.Errorf("a statement with no comment has the text %q", recs["thm-nocomment"].Text)
	}
}

func TestReadErrors(t *testing.T) {
	cases := map[string]string{
		"no statements":   "$c wff $.\n",
		"open comment":    "$( no end\n",
		"open statement":  "$c |- $.\nth $p |- x $= ?\n",
		"duplicate label": "$c |- $.\nth $a |- x $.\nth $p |- x $= ? $.\n",
		"extra close":     "$}\n",
		"bad keyword":     "lbl $q x $.\n",
	}
	for name, body := range cases {
		a := setmm.Adapter{}
		if err := a.Read(strings.NewReader(body), func(ingest.Record) error { return nil }); err == nil {
			t.Errorf("%s: Read gives no error", name)
		}
	}
}

func TestPull(t *testing.T) {
	body := fixture(t)
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	url := srv.URL + "/metamath/set.mm/" + setmm.Commit + "/set.mm"
	res, err := ingest.Pull(context.Background(), setmm.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: url, Date: "2026-10-27"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Snapshot.Commit != setmm.Commit || path != "/metamath/set.mm/"+setmm.Commit+"/set.mm" {
		t.Errorf("the snapshot has the commit %q from the path %s", res.Snapshot.Commit, path)
	}
	if m := res.Manifest; m.Records != 5 || m.Classes[ingest.Open] != 5 {
		t.Errorf("the manifest is %+v", m)
	}
	other := srv.URL + "/metamath/set.mm/develop/set.mm"
	if _, err := ingest.Pull(context.Background(), setmm.Adapter{}, ingest.Options{Root: t.TempDir(), Cache: t.TempDir(), URL: other, Date: "2026-10-27"}); err == nil || !strings.Contains(err.Error(), "pinned commit") {
		t.Errorf("a pull of another commit gives %v", err)
	}
	if got := (setmm.Adapter{}).Dump(); got != setmm.Raw+setmm.Commit+"/set.mm" {
		t.Errorf("Dump gives %s", got)
	}
}
