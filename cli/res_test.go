package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/tag"
)

func TestResMint(t *testing.T) {
	root := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "resource", "testdata", "rudin.json"))
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Replace(string(data), `"id":"8F2K1QW7ZJ"`, `"id":"~0000000a"`, 1)
	writeFile(t, root, "records/resources/~00.jsonl", line)

	code, stdout, stderr := run("res", "mint", "-root", root, "-commit", "c0ffee1")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	f := strings.Fields(stdout)
	if len(f) != 3 || f[0] != "~0000000a" || !tag.IsTag(f[1]) || f[2] != "textbook" {
		t.Fatalf("stdout is %q", stdout)
	}
	have, err := resource.Registered(root)
	if err != nil || !have[f[1]] {
		t.Fatalf("the register does not have %s: %v", f[1], err)
	}
	if _, err := os.Stat(filepath.Join(root, "records/resources/~00.jsonl")); !os.IsNotExist(err) {
		t.Fatal("the shard of the provisional id is still there")
	}
}

func TestResNotWritten(t *testing.T) {
	if code, _, stderr := run("res", "gaps"); code != ExitNotWritten || !strings.Contains(stderr, "RS-M4") {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if code, _, _ := run("res", "nothing"); code != ExitUsage {
		t.Fatalf("exit code %d for an unknown subcommand", code)
	}
	if code, _, _ := run("res"); code != ExitUsage {
		t.Fatalf("exit code %d with no subcommand", code)
	}
}

func TestResCensus(t *testing.T) {
	in := t.TempDir()
	root := t.TempDir()
	var a, b strings.Builder
	for i := range 40 {
		line := fmt.Sprintf(`{"native_id":"%d","kind":"article","date":"1995","ids":{"doi":"10.1000/%d"},"signals":{"msc":["11A41"]}}`+"\n", i, i)
		a.WriteString(line)
		if i >= 20 {
			b.WriteString(line)
		}
	}
	writeFile(t, in, "zbmath.jsonl", a.String())
	writeFile(t, in, "crossref.jsonl", b.String())

	code, stdout, stderr := run("res", "census", "-in", in, "-root", root, "-pairs", "zbmath:crossref")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "zbmath 40 records, 40 in") {
		t.Errorf("stdout is %q", stdout)
	}
	data, err := os.ReadFile(filepath.Join(root, "reports", "resources-census.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "| zbmath | crossref | article | 1990s | 40 | 20 | 20 |") {
		t.Errorf("the report has no estimate:\n%s", data)
	}

	if code, _, _ := run("res", "census", "-in", in, "-root", root, "-pairs", "zbmath:nothing"); code != ExitUsage {
		t.Errorf("exit code %d for a pair with no file", code)
	}
	if code, _, _ := run("res", "census", "-in", in, "-pairs", "zbmath"); code != ExitUsage {
		t.Errorf("exit code %d for a bad pair", code)
	}
	if code, _, _ := run("res", "census"); code != ExitUsage {
		t.Errorf("exit code %d with no -in", code)
	}
	writeFile(t, in, "bad.md", "---\nrecords:\n    - native_id: \"1\"\n      colour: red\n---\n")
	if code, _, stderr := run("res", "census", "-in", in, "-root", root); code != ExitFailure || !strings.Contains(stderr, "bad.md: record 1") {
		t.Errorf("exit code %d, stderr %q for an unknown field", code, stderr)
	}
}

func TestResSampleArxiv(t *testing.T) {
	ax := t.TempDir()
	rec := "---\nrecords:\n  - id: \"2310.00001\"\n    title: A\n    categories: [math.AC]\n  - id: \"2310.00002\"\n    title: B\n    categories: [hep-th]\n---\n"
	if err := os.WriteFile(filepath.Join(ax, "2310.md"), []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var out, errb bytes.Buffer
	if code := runRes([]string{"sample", "-catalogue", "arxiv", "-arxiv", ax, "-dir", dir}, &out, &errb); code != ExitOK {
		t.Fatalf("code = %d, stderr = %s", code, errb.String())
	}
	recs, err := mdrec.ReadFile(filepath.Join(dir, "arxiv.md"))
	if err != nil || len(recs) != 1 || !strings.Contains(string(recs[0]), "2310.00001") {
		t.Errorf("arxiv.md = %q, %v", recs, err)
	}
	out.Reset()
	if code := runRes([]string{"census", "-in", dir, "-root", t.TempDir()}, &out, &errb); code != ExitOK {
		t.Errorf("census code = %d, stderr = %s", code, errb.String())
	}
}

func TestResSampleUsage(t *testing.T) {
	for _, args := range [][]string{{"sample"}, {"sample", "-catalogue", "nope", "-dir", "x"}, {"lookup"}, {"lookup", "-dir", "x", "-against", "nope"}} {
		var out, errb bytes.Buffer
		if code := runRes(args, &out, &errb); code != ExitUsage {
			t.Errorf("%v: code = %d", args, code)
		}
	}
}

func TestResCluster(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "build/resources/source-records/arxiv/a.jsonl", `{"source":"arxiv","snapshot":"s1","native_id":"2301.00001","level":"expression","kind":"preprint","title":"On the zeros of zeta","creators":[{"name":"Ada Lovelace","role":"author"}],"date":"2023-01-02","language":"en","ids":{"arxiv":"2301.00001"}}`+"\n")
	writeFile(t, root, "build/resources/source-records/zbmath/z.jsonl", `{"source":"zbmath","snapshot":"s1","native_id":"7700001","level":"expression","kind":"article","title":"On the zeros of zeta","creators":[{"name":"Lovelace, Ada","role":"author"}],"date":"2024","language":"en","ids":{"arxiv":"2301.00001","zbl":"1500.11001"}}`+"\n")
	code, stdout, stderr := run("res", "cluster", "-root", root, "-commit", "c0ffee1", "-day", "2026-10-30")
	if code != ExitOK || !strings.Contains(stdout, "2 source records, 1 resources, 1 minted") || !strings.Contains(stdout, "C1: 1 joins") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	rs, err := resource.ReadResources(root)
	if err != nil || len(rs) != 1 || !tag.IsTag(rs[0].ID) || len(rs[0].Expressions) != 2 {
		t.Fatalf("got %+v, %v", rs, err)
	}
	if code, stdout, _ := run("res", "cluster", "-root", root, "-commit", "c0ffee2", "-day", "2026-10-31"); code != ExitOK || !strings.Contains(stdout, "0 minted") {
		t.Fatalf("a second run minted again: %q", stdout)
	}
	again, err := resource.ReadResources(root)
	if err != nil || len(again) != 1 || again[0].ID != rs[0].ID {
		t.Fatalf("the second run changed the id: %+v", again)
	}
}

func TestResResolve(t *testing.T) {
	root := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "resource", "testdata", "rudin.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "records/resources/8F2.jsonl", string(data))
	rs, err := resource.ReadResources(root)
	if err != nil || len(rs) != 1 {
		t.Fatalf("got %d resources: %v", len(rs), err)
	}
	var id string
	for _, e := range rs[0].Expressions {
		for _, m := range e.Manifestations {
			if v := m.IDs.Get("isbn"); v != "" && id == "" {
				id = "isbn:" + v
			}
		}
	}
	if id == "" {
		t.Fatal("the test record has no ISBN")
	}
	code, stdout, stderr := run("res", "resolve", "-root", root, "-index", id, "doi:10.9999/none")
	if code != ExitOK || !strings.Contains(stdout, `"status":"found"`) || !strings.Contains(stdout, `"status":"not-found"`) || !strings.Contains(stderr, "1 found, 0 with candidates, 1 not found") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "build/resources/ids.parquet")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "refs.txt", "W. Rudin, Principles of Mathematical Analysis, 3rd ed., McGraw-Hill, 1976.\n")
	if code, stdout, stderr := run("res", "resolve", "-root", root, "-in", filepath.Join(root, "refs.txt")); code != ExitOK || !strings.Contains(stdout, `"status":"found"`) {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, _, _ := run("res", "resolve", "-root", root); code != ExitUsage {
		t.Fatalf("no input gave exit code %d", code)
	}
}

func TestResScope(t *testing.T) {
	root := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "resource", "testdata", "rudin.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "records/resources/8F2.jsonl", string(data))
	writeFile(t, root, "records/resources/decisions.jsonl", `{"id":"8F2K1QW7ZJ","kind":"scope","value":"border","reason":"a test of the decision of a person","by":"tamnd","at":"2026-10-30"}`+"\n")
	code, stdout, stderr := run("res", "scope", "-root", root, "-day", "2026-10-30")
	if code != ExitOK || !strings.Contains(stdout, "in 0, border 1, out 0, deferred 0") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	rs, err := resource.ReadResources(root)
	if err != nil || rs[0].Scope.Rule != "person" {
		t.Fatalf("got %+v, %v", rs[0].Scope, err)
	}
	if code, _, stderr := run("res", "scope", "-root", root, "-fit"); code != ExitFailure || !strings.Contains(stderr, "no examples") {
		t.Fatalf("a fit with no examples gave exit code %d, stderr %q", code, stderr)
	}
}
