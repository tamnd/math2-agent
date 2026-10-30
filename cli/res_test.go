package cli

import (
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
	if code, _, stderr := run("res", "census"); code != ExitNotWritten || !strings.Contains(stderr, "RS-M1") {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if code, _, _ := run("res", "nothing"); code != ExitUsage {
		t.Fatalf("exit code %d for an unknown subcommand", code)
	}
	if code, _, _ := run("res"); code != ExitUsage {
		t.Fatalf("exit code %d with no subcommand", code)
	}
}
