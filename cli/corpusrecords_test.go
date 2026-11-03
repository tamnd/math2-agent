package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusRecords(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	for name, data := range map[string]string{
		"records/mentions/arxiv/AAA.jsonl": `{"id":"ax://paper/2401.00001#R1","source":"arxiv","kind":"statement"}` + "\n",
		"records/inventory/200.jsonl":      `{"id":"200AAAAAAA","mentions":["ax://paper/2401.00001#R1"],"kind":"statement","layer":"research","spread":1}` + "\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if code, _, _ := run("corpus", "records", "export", "-root", root); code != ExitUsage {
		t.Errorf("export with no -out gave exit %d, want %d", code, ExitUsage)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"export", "-root", root, "-out", out}, "wrote 2 shards"},
		{[]string{"pin", "-root", root, "-out", out, "-commit", "0123abcd"}, "tamnd/math2-records at 0123abcd, 2 shards"},
		{[]string{"keep", "-root", root}, "removed 1 files of arXiv mentions and understand records, and 1 research rows"},
	} {
		code, stdout, stderr := run(append([]string{"corpus", "records"}, c.args...)...)
		if code != ExitOK || !strings.Contains(stdout, c.want) {
			t.Errorf("m2 corpus records %v = %d\n%s\n%s", c.args, code, stdout, stderr)
		}
	}
	if code, _, stderr := run("corpus", "import", "-source", "arxiv", "-corpus", out, "-root", root, "-commit", "0123abcd"); code != ExitFailure || !strings.Contains(stderr, "unpack") {
		t.Errorf("import on a packed tree = %d\n%s", code, stderr)
	}
	if code, stdout, stderr := run("corpus", "records", "unpack", "-root", root, "-out", out); code != ExitOK || !strings.Contains(stdout, "added 1 files and 1 research rows") {
		t.Errorf("unpack = %d\n%s\n%s", code, stdout, stderr)
	}
}
