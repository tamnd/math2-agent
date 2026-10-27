package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateShard3(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"nodes/7K/7K3QX2.md", "nodes/~/~3fa94c01.md", "prov/7K/7K3QX2.jsonl", "reviews/G5/G5S1M9.yaml", "nodes/7K3/7K3QX3.md", "nodes/7K/notes.txt"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(p)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(p), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	code, out, errOut := run("migrate", "shard3", "-root", root)
	want := "nodes/7K/7K3QX2.md nodes/7K3/7K3QX2.md\nprov/7K/7K3QX2.jsonl prov/7K3/7K3QX2.jsonl\nreviews/G5/G5S1M9.yaml reviews/G5S/G5S1M9.yaml\n3 paths moved\n"
	if code != ExitOK || out != want {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, out, errOut)
	}
	for _, p := range []string{"nodes/7K3/7K3QX2.md", "nodes/~/~3fa94c01.md", "nodes/7K/notes.txt", "reviews/G5S/G5S1M9.yaml"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "reviews", "G5")); err == nil {
		t.Error("the empty shard reviews/G5 is still there")
	}
	if code, out, _ := run("migrate", "shard3", "-root", root); code != ExitOK || out != "0 paths moved\n" {
		t.Errorf("a second run gives exit code %d, stdout %q", code, out)
	}
	if code, _, errOut := run("migrate", "shard2"); code != ExitUsage || !strings.Contains(errOut, "usage") {
		t.Errorf("an unknown layout gives exit code %d, stderr %q", code, errOut)
	}
}

func TestMigrateShard3Clash(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"nodes/7K/7K3QX2.md", "nodes/7K3/7K3QX2.md"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(p)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(p), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if code, _, errOut := run("migrate", "shard3", "-root", root); code != ExitFailure || !strings.Contains(errOut, "have the same tag") {
		t.Errorf("a clash gives exit code %d, stderr %q", code, errOut)
	}
}

func TestMigrateNodeV2(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"schema/node.v1.json":   "{}",
		"nodes/EX6/EX6H3W.md":   "---\ntag: EX6H3W\nkind: example\nname: A large subgroup\nuses: [3C53TD]\n---\n\n## Example\n\nFind the order of $H$.\n\n## Remarks\n\nRole: exercise. About: 3C53TD. Difficulty: 2. Group: B. Check: exact.\n\n### Answer\n\n12\n",
		"prov/EX6/EX6H3W.jsonl": `{"field":"*","op":"add","agent":null,"human":"github:tamnd","at":"2026-10-01T09:00:00Z"}` + "\n",
	}
	for p, text := range files {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(p)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	code, out, errOut := run("migrate", "node.v2", "-root", root)
	if want := "nodes/EX6/EX6H3W.md 9 provenance lines\n1 node moved to node.v2\n"; code != ExitOK || out != want {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, out, errOut)
	}
	data, err := os.ReadFile(filepath.Join(root, "nodes/EX6/EX6H3W.md"))
	if err != nil || !strings.Contains(string(data), "kind: exercise\n") || !strings.Contains(string(data), "## Answer\n\n12\n") {
		t.Errorf("the node is\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(root, "schema/node.v2.json")); err != nil {
		t.Error(err)
	}
	if code, out, _ := run("migrate", "node.v2", "-root", root, "-q"); code != ExitOK || out != "0 nodes moved to node.v2\n" {
		t.Errorf("a second run gives exit code %d, stdout %q", code, out)
	}
}
