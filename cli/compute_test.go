package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/compute"
	"github.com/tamnd/math2-agent/sandbox"
)

// echo is a sandbox that runs no program. It writes the entry file to stdout.
type echo struct{}

func (echo) Run(_ context.Context, s sandbox.Spec) (*sandbox.Result, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, strings.TrimPrefix(s.Command[len(s.Command)-1], sandbox.WorkDir+"/")))
	if err != nil {
		return nil, err
	}
	return &sandbox.Result{Stdout: data, Runtime: "echo", Network: "none", Duration: time.Second}, nil
}

func TestCompute(t *testing.T) {
	old := newSandbox
	newSandbox = func(context.Context, string) (sandbox.Sandbox, error) { return echo{}, nil }
	t.Cleanup(func() { newSandbox = old })

	digest := "sha256:" + strings.Repeat("a", 64) // not the digest of a real image
	src := "print(42)\n"
	root := t.TempDir()
	for p, data := range map[string]string{
		"manifests/images.yaml":  "images:\n  - name: m2-python\n    amd64: " + digest + "\n    arm64: " + digest + "\n",
		"compute/T1TEST/main.py": src,
		"nodes/T1T/T1TEST.md":    "---\ntag: T1TEST\nkind: computation\nname: A test\nclaim: x\nengine:\n  image: m2-python@" + digest + "\n  entry: main.py\noutput:\n  hash: " + compute.Hash([]byte(src)) + "\n---\n\n## Description\n\nA test.\n",
		"nodes/T2T/T2TEST.md":    "---\ntag: T2TEST\nkind: statement\nname: Not a computation\n---\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	code, stdout, stderr := run("compute", "verify", "-root", root, "T1TEST")
	if code != ExitOK || !strings.Contains(stdout, "T1TEST on ") || !strings.Contains(stdout, ": verified") {
		t.Fatalf("verify: exit code %d\n%s%s", code, stdout, stderr)
	}
	code, stdout, stderr = run("compute", "run", "-root", root, "-json", "-all")
	var runs []compute.Run
	if code != ExitOK || json.Unmarshal([]byte(stdout), &runs) != nil || len(runs) != 1 || runs[0].Output != compute.Hash([]byte(src)) {
		t.Fatalf("run -json -all: exit code %d\n%s%s", code, stdout, stderr)
	}
	for _, args := range [][]string{
		{"compute"}, {"compute", "go"}, {"compute", "run", "-root", root}, {"compute", "run", "-root", root, "-all", "T1TEST"},
	} {
		if code, _, _ := run(args...); code != ExitUsage {
			t.Errorf("%v: exit code %d, want %d", args, code, ExitUsage)
		}
	}
	for _, tag := range []string{"T2TEST", "T9NONE"} {
		if code, _, _ := run("compute", "run", "-root", root, tag); code != ExitFailure {
			t.Errorf("%s: exit code %d, want %d", tag, code, ExitFailure)
		}
	}
	if code, _, stderr := run("compute", "run", "-root", t.TempDir(), "-all"); code != ExitFailure || !strings.Contains(stderr, "images.yaml does not exist") {
		t.Errorf("a tree with no manifest: exit code %d, %s", code, stderr)
	}
}
