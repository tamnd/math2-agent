package arxiv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReferences(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{
		"metadata/2401.jsonl": `{"id":"2401.00001","versions":[{"version":1,"licence":"arxiv-1.0"},{"version":2,"licence":"arxiv-1.0"}]}` + "\n" + `{"id":"2401.00002","versions":[{"version":1,"licence":"cc-by"}]}` + "\n",
		"analysis/objects/2401/2401.00001.jsonl": `{"tag":"OLD1","paper":"2401.00001","version":"1","local":"thm-1","path":"source","minhash":"AAAA"}` + "\n" +
			`{"tag":"THM1","paper":"2401.00001","version":"2","local":"thm-1","path":"source","minhash":"BBBB"}` + "\n" +
			`{"paper":"2401.00001","version":"2","local":"thm-2","path":"source","minhash":"CCCC"}` + "\n",
		"content/en/2401/2401.00002/01_intro.md": "An open text.\n",
		"content/en/2401/2401.00002/02_end.md":   "The end.\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := References(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("the references are %+v", rs)
	}
	if r := rs[0]; r.URI != "arxiv:2401.00002" || r.Class != "open" || r.Text != "An open text.\n\n\nThe end.\n\n\n" || r.MinHash != "" {
		t.Errorf("the text is %+v", r)
	}
	if r := rs[1]; r.URI != "ax://paper/2401.00001#THM1" || r.Class != "record" || r.Licence != "arxiv-1.0" || r.MinHash != "BBBB" || r.Text != "" {
		t.Errorf("the item is %+v", r)
	}
}
