package cites

import (
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

func TestCountRuns(t *testing.T) {
	old := map[string]Unresolved{
		"8F2AAAAAA3/e1#1": {Ref: "8F2AAAAAA3/e1#1", Runs: 1, Since: "2026-10-01"},
		"8F2AAAAAA3/e1#2": {Ref: "8F2AAAAAA3/e1#2", Runs: 1, Since: "2026-10-01"},
		"8F2BBBBBB6/e1#1": {Ref: "8F2BBBBBB6/e1#1", Runs: 3, Since: "2026-09-01"},
	}
	edges := []resource.Cite{
		{From: "8F2AAAAAA3/e1", N: 1},
		{From: "8F2AAAAAA3/e1", N: 2, To: "8F2K1QW7ZJ"},
		{From: "8F2AAAAAA3/e1", N: 3},
	}
	got := CountRuns(old, edges, map[string]bool{"8F2AAAAAA3/e1": true}, "2026-10-31")
	want := map[string]Unresolved{
		"8F2AAAAAA3/e1#1": {Ref: "8F2AAAAAA3/e1#1", Runs: 2, Since: "2026-10-01"},
		"8F2AAAAAA3/e1#3": {Ref: "8F2AAAAAA3/e1#3", Runs: 1, Since: "2026-10-31"},
		"8F2BBBBBB6/e1#1": {Ref: "8F2BBBBBB6/e1#1", Runs: 3, Since: "2026-09-01"},
	}
	if len(got) != len(want) {
		t.Fatalf("rows %+v", got)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("row %s is %+v, want %+v", k, got[k], w)
		}
	}
	root := t.TempDir()
	if err := WriteUnresolved(root, got); err != nil {
		t.Fatal(err)
	}
	back, err := ReadUnresolved(root)
	if err != nil || len(back) != 3 || back["8F2AAAAAA3/e1#1"].From() != "8F2AAAAAA3/e1" {
		t.Errorf("read %+v, %v", back, err)
	}
}
