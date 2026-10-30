package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	rescluster "github.com/tamnd/math2-agent/resource/cluster"
)

func TestResGold(t *testing.T) {
	root := t.TempDir()
	recs := []rescluster.Record{
		{Source: "arxiv", Snapshot: "s1", NativeID: "2301.00001", Level: "expression", Kind: "preprint", Title: "On the zeros of zeta", Creators: []resource.Creator{{Name: "Ada Lovelace", Role: "author"}}, Date: "2023", IDs: resource.IDs{"arxiv": {"2301.00001"}}},
		{Source: "zbmath", Snapshot: "s1", NativeID: "77", Level: "expression", Kind: "article", Title: "On the zeros of zeta", Creators: []resource.Creator{{Name: "Lovelace, Ada", Role: "author"}}, Date: "2024", IDs: resource.IDs{"arxiv": {"2301.00001"}}},
	}
	if err := rescluster.WriteRecords(root, "arxiv", "records", recs[:1]); err != nil {
		t.Fatal(err)
	}
	if err := rescluster.WriteRecords(root, "zbmath", "records", recs[1:]); err != nil {
		t.Fatal(err)
	}
	run := func(want int, args ...string) string {
		t.Helper()
		var out, errb bytes.Buffer
		if code := runRes(append([]string{"gold", "-root", root}, args...), &out, &errb); code != want {
			t.Fatalf("m2 res gold %v = %d, want %d: %s", args, code, want, errb.String())
		}
		return out.String()
	}
	if out := run(ExitOK, "sample", "-set", "preprint", "-n", "10"); !strings.Contains(out, "added 1 pairs") {
		t.Errorf("sample printed %q", out)
	}
	run(ExitOK, "label", "-set", "preprint", "-a", "arxiv:2301.00001", "-b", "zbmath:77", "-value", "same", "-by", "ann")
	run(ExitFailure, "label", "-set", "preprint", "-a", "arxiv:2301.00001", "-b", "zbmath:77", "-value", "same", "-by", "ann")
	if out := run(ExitOK, "label", "-set", "preprint", "-a", "zbmath:77", "-b", "arxiv:2301.00001", "-value", "same", "-by", "judge"); !strings.Contains(out, "labelled 1") {
		t.Errorf("label printed %q", out)
	}
	out := run(ExitOK, "eval")
	if !strings.Contains(out, "preprint: 1 pairs, precision 1.0000, recall 1.0000, the targets: meet") || !strings.Contains(out, "work: no labelled pair") {
		t.Errorf("eval printed %q", out)
	}
	data, err := os.ReadFile(filepath.Join(root, ResGoldReport))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "| C1 | 1 | 0 | 0 | 1.0000 | 1.0000 |") {
		t.Errorf("the report is %s", data)
	}
	run(ExitUsage, "sample", "-set", "edition")
}

func TestResGoldCite(t *testing.T) {
	root := t.TempDir()
	rs := []resource.Resource{
		{ID: "8F2K1QW7ZJ", Kind: "textbook", Title: "Principles of Mathematical Analysis", Expressions: []resource.Expression{{Key: "e3", Kind: "edition", Language: "en"}}},
		{ID: "8F2AAAAAA3", Kind: "article", Title: "On primes", Expressions: []resource.Expression{{Key: "e1", Kind: "version", Language: "de"}}},
	}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	if err := resource.WriteCites(root, []resource.Cite{{From: "8F2AAAAAA3/e1", N: 1, To: "8F2K1QW7ZJ/e3"}, {From: "8F2AAAAAA3/e1", N: 2}}); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := run("res", "gold", "-root", root, "sample", "-set", "cite"); code != ExitOK || !strings.Contains(stdout, "added 2 references") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, by := range []string{"a", "b"} {
		if code, _, stderr := run("res", "gold", "-root", root, "label", "-set", "cite", "-ref", "8F2AAAAAA3/e1#1", "-value", "8F2K1QW7ZJ", "-by", by); code != ExitOK {
			t.Fatalf("exit code %d, stderr %q", code, stderr)
		}
	}
	code, stdout, stderr := run("res", "gold", "-root", root, "eval")
	if code != ExitOK || !strings.Contains(stdout, "cite: 1 references, precision 1.0000, recall 1.0000") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}
