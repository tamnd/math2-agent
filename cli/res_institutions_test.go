package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource/institutions"
)

func TestResInstitutions(t *testing.T) {
	root := t.TempDir()
	if code, _, _ := run("res", "institutions", "-root", root); code != ExitUsage {
		t.Errorf("no -ror gave exit %d, want %d", code, ExitUsage)
	}
	dump := filepath.Join(t.TempDir(), "ror.json")
	data := `[{"id":"https://ror.org/0aaaaaa01","status":"active","types":["education"],"names":[{"value":"University A","types":["ror_display"]}]}]`
	if err := os.WriteFile(dump, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("res", "institutions", "-root", root, "-ror", dump, "-wikidata", "none")
	if code != ExitOK || !strings.Contains(stdout, "read 1 organizations of ROR, 1 of them active") || !strings.Contains(stdout, "I4: 1") {
		t.Fatalf("m2 res institutions = %d\n%s\n%s", code, stdout, stderr)
	}
	if ins, err := institutions.Read(root); err != nil || len(ins) != 1+len(institutions.Platforms) {
		t.Errorf("the register has %d records: %v", len(ins), err)
	}
}
