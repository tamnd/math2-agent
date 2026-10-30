package cli

import (
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/depth"
)

func TestResRecords(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	rs := []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "textbook", Title: "A", Scope: &resource.Scope{Value: resource.In}},
		{ID: "8F3BBBBBBT", Kind: "article", Title: "B", Scope: &resource.Scope{Value: resource.In}},
	}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := run("res", "records", "export", "-root", root); code != ExitUsage {
		t.Errorf("export with no -out gave exit %d, want %d", code, ExitUsage)
	}
	if code, _, _ := run("res", "records", "keep", "-root", root); code != ExitFailure {
		t.Errorf("keep with no depth rows gave exit %d, want %d", code, ExitFailure)
	}
	if err := depth.Write(root, []depth.Row{{ID: "8F2AAAAAA3", Tier: depth.T2}, {ID: "8F3BBBBBBT", Tier: depth.T4}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"export", "-root", root, "-out", out}, "wrote 2 shards"},
		{[]string{"keep", "-root", root}, "kept 1 resources of the tiers T1, T2 and T5, and removed 1 resources"},
		{[]string{"pin", "-root", root, "-out", out, "-commit", "0123abcd"}, "tamnd/math2-records at 0123abcd, 2 shards"},
	} {
		code, stdout, stderr := run(append([]string{"res", "records"}, c.args...)...)
		if code != ExitOK || !strings.Contains(stdout, c.want) {
			t.Errorf("m2 res records %v = %d\n%s\n%s", c.args, code, stdout, stderr)
		}
	}
}
