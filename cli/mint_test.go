package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/tag"
)

const (
	mintConcept = "---\ntag: ~00000001\nkind: concept\nname: Group\n---\n\n## Summary\n\nA group is a set with an associative operation, an identity and inverses. See also [[subgroups|~00000002]].\n"
	mintSub     = "---\ntag: ~00000002\nkind: concept\nname: Subgroup\nuses: [~00000001]\n---\n\n## Summary\n\nA subgroup is a subset of a [[group|~00000001]] that is a group under the same operation.\n"
)

func writeFile(t *testing.T, root, rel, text string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMintTwoNodes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "nodes/~/~00000001.md", mintConcept)
	writeFile(t, root, "nodes/~/~00000002.md", mintSub)
	if code, stdout, _ := run("check", "-root", root, "-q"); code != ExitOK {
		t.Fatalf("the provisional tree fails m2 check:\n%s", stdout)
	}

	code, stdout, stderr := run("mint", "-root", root, "-commit", "c0ffee1")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout must have 2 lines:\n%s", stdout)
	}
	var want []string
	for i, l := range lines {
		f := strings.Fields(l)
		if len(f) != 3 || f[0] != []string{"~00000001", "~00000002"}[i] || !tag.IsTag(f[1]) || f[2] != "concept" {
			t.Fatalf("line %d is %q", i+1, l)
		}
		want = append(want, f[1]+" c0ffee1 concept")
		if _, err := os.Stat(filepath.Join(root, "nodes", tag.Shard(f[1]), f[1]+".md")); err != nil {
			t.Error(err)
		}
	}
	reg, err := os.ReadFile(filepath.Join(root, "registers", "tags.reg"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimSpace(string(reg)), "\n"); len(got) != 2 || !strings.Contains(string(reg), want[0]) || !strings.Contains(string(reg), want[1]) {
		t.Errorf("registers/tags.reg is:\n%s", reg)
	}
	if left, _ := filepath.Glob(filepath.Join(root, "nodes", "~", "*")); len(left) != 0 {
		t.Errorf("nodes/~ still has %v", left)
	}
	if code, stdout, _ := run("check", "-root", root, "-q"); code != ExitOK {
		t.Errorf("m2 check fails after m2 mint:\n%s", stdout)
	}
	if code, stdout, _ := run("fmt", "-check", "-root", root); code != ExitOK {
		t.Errorf("m2 mint left files that are not canonical:\n%s", stdout)
	}
	if code, stdout, _ := run("mint", "-root", root, "-commit", "c0ffee2"); code != ExitOK || stdout != "" {
		t.Errorf("a second m2 mint: exit code %d, stdout %q", code, stdout)
	}
}

func TestMintSeed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "nodes/~/~00000001.md", mintConcept)
	writeFile(t, root, "nodes/~/~00000002.md", mintSub)
	code, stdout, stderr := run("mint", "-root", root, "-commit", "c0ffee1", "-seed")
	if code != ExitOK {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout must have 2 lines:\n%s", stdout)
	}
	for _, l := range lines {
		if f := strings.Fields(l); len(f) != 3 || !tag.IsSeed(f[1]) {
			t.Errorf("the line %q has no seed tag", l)
		}
	}
	if code, stdout, _ := run("check", "-root", root, "-q"); code != ExitOK {
		t.Errorf("m2 check fails after m2 mint -seed:\n%s", stdout)
	}
}

func TestMintUnknownProvisional(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "nodes/~/~00000002.md", mintSub)
	code, _, stderr := run("mint", "-root", root, "-commit", "c0ffee1")
	if code != ExitFailure || !strings.Contains(stderr, "rule T03") || !strings.Contains(stderr, "~00000001") {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "nodes", "~", "~00000002.md")); err != nil {
		t.Error("m2 mint moved a file after an error")
	}
	if _, err := os.Stat(filepath.Join(root, "registers", "tags.reg")); err == nil {
		t.Error("m2 mint wrote the register after an error")
	}
}

func TestMintUsage(t *testing.T) {
	root := t.TempDir()
	if code, _, _ := run("mint", "-root", root, "extra"); code != ExitUsage {
		t.Errorf("an argument gave exit code %d, want %d", code, ExitUsage)
	}
	if code, _, _ := run("mint", "-root", root, "-commit", "a b"); code != ExitUsage {
		t.Errorf("a commit with a space gave exit code %d, want %d", code, ExitUsage)
	}
	if code, _, stderr := run("mint", "-root", root); code != ExitFailure || !strings.Contains(stderr, "-commit") {
		t.Errorf("a tree with no git: exit code %d, stderr %q", code, stderr)
	}
}
