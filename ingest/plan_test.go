package ingest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
)

func TestReadPlan(t *testing.T) {
	hash := "blake3:" + strings.Repeat("0a", 32)
	cases := []struct {
		name, plan, problem string
	}{
		{"good", "imports:\n  - source: oeis\n  - source: mathlib_100\n    url: https://raw.githubusercontent.com/x/y/abc/100.yaml\n  - source: zbmath\n    snapshot: " + hash + "\n", ""},
		{"empty", "imports: []\n", "the plan has no imports"},
		{"unknown field", "imports:\n  - source: oeis\n    rate: 1\n", "field rate not found"},
		{"bad id", "imports:\n  - source: OEIS\n", `the source "OEIS" is not the id of a source`},
		{"twice", "imports:\n  - source: oeis\n  - source: oeis\n", "the source oeis has two imports"},
		{"url and snapshot", "imports:\n  - source: oeis\n    url: https://oeis.org/x\n    snapshot: " + hash + "\n", "has a URL and a snapshot"},
		{"not http", "imports:\n  - source: oeis\n    url: file:///etc/passwd\n", "is not an HTTP URL"},
		{"bad hash", "imports:\n  - source: oeis\n    snapshot: sha256:00\n", "is not a BLAKE3 hash"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "plan.yaml")
			if err := os.WriteFile(path, []byte(c.plan), 0o600); err != nil {
				t.Fatal(err)
			}
			p, err := ingest.ReadPlan(path)
			if c.problem == "" {
				if err != nil || len(p.Imports) != 3 || p.Imports[2].Snapshot != hash {
					t.Fatalf("ReadPlan gave %+v, %v", p, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.problem) {
				t.Fatalf("ReadPlan gave %v, want %q", err, c.problem)
			}
		})
	}
}
