package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/wpcorpus"
)

func TestWPGold(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, wpcorpus.ManifestDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cs := []*wpcorpus.Candidate{
		{PageID: 1, Title: "Group_(mathematics)", Signals: wpcorpus.S1, CatDepth: -1},
		{PageID: 2, Title: "Pi", Signals: wpcorpus.S4, CatDepth: -1},
		{PageID: 3, Title: "Monster_group", Signals: wpcorpus.S5, CatDepth: -1},
	}
	for _, c := range cs {
		wpcorpus.Classify(c)
	}
	f, err := os.Create(filepath.Join(dir, "candidates.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	if err := wpcorpus.WriteCandidates(f, cs, "20261001", false); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	run := func(want int, args ...string) string {
		t.Helper()
		var out, errb bytes.Buffer
		if code := runWP(append([]string{"gold"}, args...), &out, &errb); code != want {
			t.Fatalf("%v: exit %d, %s", args, code, errb.String())
		}
		return out.String() + errb.String()
	}
	if s := run(ExitOK, "draw", "-corpus", root); !strings.Contains(s, "3 rows in") {
		t.Errorf("draw: %s", s)
	}
	if s := run(ExitFailure, "draw", "-corpus", root); !strings.Contains(s, "remove the file first") {
		t.Errorf("second draw: %s", s)
	}

	gold := filepath.Join(dir, "gold", "scope.tsv")
	b, err := os.ReadFile(gold)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	for i := 1; i < len(lines); i++ {
		mark := "math"
		if strings.Contains(lines[i], "Monster_group") {
			mark = "not-math"
		}
		lines[i] = strings.Replace(lines[i], "\t\t\t", "\t"+mark+"\ttest\t", 1)
	}
	if err := os.WriteFile(gold, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := run(ExitOK, "measure", "-corpus", root); !strings.Contains(s, "3 marked, precision 0.667, recall 1.000") {
		t.Errorf("measure: %s", s)
	}
	if _, err := os.Stat(filepath.Join(root, "reports", "gold-s.md")); err != nil {
		t.Error(err)
	}
	if s := run(ExitUsage, "frob"); !strings.Contains(s, "usage: m2 wp gold draw") {
		t.Errorf("usage: %s", s)
	}
}
