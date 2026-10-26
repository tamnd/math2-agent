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
