package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/build"
)

var update = flag.Bool("update", false, "write the files in testdata/contract and m2.proto again")

const tree = "../testdata/nodes/valid"

var (
	once    sync.Once
	out     string
	openErr error
)

// open builds the test tree once and opens a service on it.
func open(t testing.TB) *api.Service {
	t.Helper()
	once.Do(func() {
		out, openErr = os.MkdirTemp("", "m2api")
		if openErr == nil {
			_, openErr = build.Run(context.Background(), build.Options{Root: tree, Out: out})
		}
	})
	if openErr != nil {
		t.Fatal(openErr)
	}
	s, err := api.Open(api.Config{Root: tree, Out: out, Commit: "0123abc"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMain(m *testing.M) {
	code := m.Run()
	if out != "" {
		_ = os.RemoveAll(out)
	}
	os.Exit(code)
}

// contract has one call or more for each method, and the errors. The answer of each one is in testdata/contract/NAME.json.
var contract = []struct {
	name, method, query, body string
}{
	{name: "index"},
	{name: "resolve", method: "resolve", query: "ref=m2://Q529ES"},
	{name: "resolve-id", method: "resolve", query: "ref=A000040"},
	{name: "get", method: "get", query: "ref=Q529ES"},
	{name: "status", method: "status", query: "ref=Q529ES"},
	{name: "get_formal", method: "get_formal", query: "ref=Q529ES"},
	{name: "neighbours", method: "neighbours", query: "ref=Q529ES&depth=1"},
	{name: "closure", method: "closure", query: "ref=Q529ES&strategy=min-size"},
	{name: "rdeps", method: "rdeps", query: "ref=CM1BRG"},
	{name: "path", method: "path", query: "from=DSG7KN&to=Q529ES"},
	{name: "frontier", method: "frontier", query: "open=true"},
	{name: "search", method: "search", query: "q=" + url.QueryEscape("order of a subgroup divides the order of the group") + "&k=3"},
	{name: "search-post", method: "search", body: `{"q": "Lagrange", "k": 2, "kind": "theorem"}`},
	{name: "search_formal", method: "search_formal", query: "q=" + url.QueryEscape("Nat.Prime (Nat.card G) → IsCyclic G") + "&k=3"},
	{name: "search_formula", method: "search_formula", query: "q=" + url.QueryEscape("|Y| = |H|^{q-1}") + "&k=3"},
	{name: "similar", method: "similar", query: "ref=Q529ES&k=3"},
	{name: "premises", method: "premises", query: "goal=" + url.QueryEscape("⊢ Nat.card H ∣ Nat.card G") + "&k=3"},
	{name: "error-method", method: "put"},
	{name: "error-missing", method: "get"},
	{name: "error-param", method: "get", query: "ref=Q529ES&lang=fr"},
	{name: "error-k", method: "search", query: "q=group&k=1000"},
	{name: "error-type", method: "neighbours", query: "ref=Q529ES&depth=two"},
	{name: "error-tag", method: "get", query: "ref=ZZZZZZ"},
	{name: "error-body", method: "search", body: `["q"]`},
}

// TestContract checks the answer of each endpoint against testdata/contract. Give -update to write the files again after a change that is compatible, and read the diff.
func TestContract(t *testing.T) {
	srv := httptest.NewServer(open(t).Handler(api.Options{Rate: -1}))
	defer srv.Close()
	covered := map[string]bool{}
	for _, c := range contract {
		t.Run(c.name, func(t *testing.T) {
			u := srv.URL + "/" + api.Version
			if c.method != "" {
				u += "/" + c.method
			}
			var resp *http.Response
			var err error
			if c.body != "" {
				resp, err = http.Post(u, "application/json", strings.NewReader(c.body))
			} else {
				if c.query != "" {
					u += "?" + c.query
				}
				resp, err = http.Get(u) // #nosec G107 -- the URL is the test server.
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type is %q", ct)
			}
			var v map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
				t.Fatal(err)
			}
			if v["api"] != api.Version {
				t.Errorf("api is %v", v["api"])
			}
			if _, ok := v["error"]; !ok && c.method != "" {
				covered[c.method] = true
				if v["build"] == "" || v["commit"] != "0123abc" {
					t.Errorf("the response does not say what it is about: build %v, commit %v", v["build"], v["commit"])
				}
			}
			// The output hash changes with each change of the test tree or of a step, thus the files do not keep it.
			if _, ok := v["build"]; ok {
				v["build"] = "BUILD"
			}
			got, err := json.MarshalIndent(map[string]any{"status": resp.StatusCode, "body": round(v)}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join("testdata", "contract", c.name+".json")
			if *update {
				if err := os.WriteFile(path, got, 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path) // #nosec G304 -- the path is a test file.
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("the answer is not the answer in %s. Give -update if the change is compatible.\ngot:\n%s", path, got)
			}
		})
	}
	for _, m := range api.Methods() {
		if !covered[m.Name] {
			t.Errorf("no call of the contract covers the method %s", m.Name)
		}
	}
}

// round rounds each number to 6 digits after the point. The last digits of a score can be different on another CPU.
func round(v any) any {
	switch x := v.(type) {
	case float64:
		return math.Round(x*1e6) / 1e6
	case []any:
		for i := range x {
			x[i] = round(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = round(x[k])
		}
	}
	return v
}

func TestRateLimit(t *testing.T) {
	h := open(t).Handler(api.Options{Rate: 1, Burst: 2})
	codes := func(client string) []int {
		var out []int
		for range 3 {
			r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			r.RemoteAddr = client + ":1234"
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			out = append(out, w.Code)
			if w.Code == http.StatusTooManyRequests && w.Header().Get("Retry-After") == "" {
				t.Error("a 429 has no Retry-After")
			}
		}
		return out
	}
	for _, client := range []string{"192.0.2.1", "192.0.2.2"} {
		got := codes(client)
		if got[0] != 200 || got[1] != 200 || got[2] != http.StatusTooManyRequests {
			t.Errorf("%s: the codes are %v, and they must be 200, 200, 429", client, got)
		}
	}
}

func TestOpen(t *testing.T) {
	if _, err := api.Open(api.Config{Root: t.TempDir()}); err == nil {
		t.Error("Open of a tree with no build gives no error")
	}
}

func TestCall(t *testing.T) {
	s := open(t)
	r, err := s.Call("get", map[string]any{"ref": "Q529ES"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.About) != 1 || r.About[0].Tag != "Q529ES" || !strings.HasPrefix(r.About[0].Hash, "blake3:") {
		t.Errorf("about is %+v", r.About)
	}
	_, err = s.Call("neighbours", map[string]any{"ref": "Q529ES", "depth": 4.0})
	var e *api.Error
	if !errors.As(err, &e) || e.Code != api.Invalid {
		t.Errorf("a depth of 4 gives %v", err)
	}
}
