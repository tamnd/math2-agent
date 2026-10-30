package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if code, _, stderr := run("res", "import"); code != ExitNotWritten || !strings.Contains(stderr, "RS-M2") {
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
	writeFile(t, in, "bad.jsonl", `{"native_id":"1","colour":"red"}`+"\n")
	if code, _, stderr := run("res", "census", "-in", in, "-root", root); code != ExitFailure || !strings.Contains(stderr, "bad.jsonl:1") {
		t.Errorf("exit code %d, stderr %q for an unknown field", code, stderr)
	}
}
