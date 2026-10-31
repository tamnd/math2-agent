package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent/understand"
)

func TestCorpusUnderstandReview(t *testing.T) {
	root := t.TempDir()
	rec := `"ref":"2401.00001v1#x","by":"agent:m2-agent/understand@0.5.0","model":"m","class":"open","pass":"unfold"`
	files := map[string]string{
		"records/understand/arxiv/unfold/AAA.jsonl": `{"id":"ax://paper/2401.00001#T1",` + rec + `,"unfold":{"unfolded":"x","hash":"h1","verdict":{"model":"c","verdict":"equivalent"}}}` + "\n" +
			`{"id":"ax://paper/2401.00001#T2",` + rec + `,"queue":"unfold-review","unfold":{"unfolded":"x","hash":"h2","verdict":{"model":"c","verdict":"weaker"}}}` + "\n",
		understand.GoldUnfoldFile: `{"item":"ax://paper/2401.00001#T1","hash":"h1","verdict":"equivalent","by":"github:a","on":"2026-10-31"}` + "\n",
	}
	for name, data := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr := run("corpus", "understand", "review", "-root", root, "-items")
	if code != ExitOK || !strings.Contains(stdout, "unfold  unfold-review  1") || !strings.Contains(stdout, "ax://paper/2401.00001#T2") {
		t.Errorf("review gives %d, %q, %q", code, stdout, stderr)
	}
	code, stdout, stderr = run("corpus", "understand", "gold", "-root", root)
	if code != ExitOK || !strings.Contains(stdout, "passed forms that are equivalent") {
		t.Errorf("gold gives %d, %q, %q", code, stdout, stderr)
	}
	if code, _, _ = run("corpus", "understand", "nothing"); code != ExitUsage {
		t.Errorf("an unknown command gives %d", code)
	}
}
