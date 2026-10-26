package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// releaseTree copies the seed of the release package to a new git repository, builds it and commits it. The build has no Lake project, thus the test writes the pins into the build manifest.
func releaseTree(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on the PATH")
	}
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "m2", "GIT_COMMITTER_NAME": "m2", "GIT_AUTHOR_EMAIL": "m2@example.com", "GIT_COMMITTER_EMAIL": "m2@example.com"} {
		t.Setenv(k, v)
	}
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("..", "release", "testdata", "seed"))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/build/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitT(t, root, "init", "-q")
	gitT(t, root, "add", ".")
	gitT(t, root, "commit", "-q", "-m", "seed")
	if code, _, stderr := run("build", "-root", root); code != ExitOK {
		t.Fatalf("m2 build: exit code %d, stderr %q", code, stderr)
	}
	path := filepath.Join(root, "build", "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	m["lean"], m["mathlib"] = "leanprover/lean4:v4.35.0", strings.Repeat("ab", 20)
	if data, err = json.Marshal(m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func gitT(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root, "-c", "user.name=m2", "-c", "user.email=m2@example.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestReleaseCut(t *testing.T) {
	root := releaseTree(t)
	code, stdout, stderr := run("release", "cut", "-root", root, "-no-build", "-date", "2027-03-01")
	if code != ExitOK || !strings.Contains(stdout, "the release 2027.03") || !strings.Contains(stdout, "2 live nodes") {
		t.Fatalf("the first cut: exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if msg := gitT(t, root, "log", "-1", "--format=%s"); msg != "Cut the release 2027.03" {
		t.Errorf("the last commit is %q", msg)
	}
	if st := gitT(t, root, "status", "--porcelain"); st != "" {
		t.Errorf("the cut left changes:\n%s", st)
	}
	notes, err := os.ReadFile(filepath.Join(root, "manifests", "releases", "2027.03.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(notes), "This is the first release") || !strings.Contains(string(notes), "| nodes_live | 0 | 2 | +2 | 2 new nodes |") {
		t.Errorf("the notes are\n%s", notes)
	}
	levels, err := os.ReadFile(filepath.Join(root, "manifests", "releases", "2027.03.levels"))
	if err != nil || !strings.Contains(string(levels), "3C53TD E1 ") {
		t.Errorf("the levels are %q, %v", levels, err)
	}

	// A person writes the summary, and m2 release notes keeps it.
	edited := strings.Replace(string(notes), "<!-- Write the summary of the release here. m2 release notes keeps this section when it writes the notes again. -->", "The first release has a group and Lagrange's theorem.", 1)
	if err := os.WriteFile(filepath.Join(root, "manifests", "releases", "2027.03.md"), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run("release", "notes", "-root", root, "2027.03"); code != ExitOK {
		t.Fatalf("m2 release notes: exit code %d, stderr %q", code, stderr)
	}
	again, _ := os.ReadFile(filepath.Join(root, "manifests", "releases", "2027.03.md"))
	if string(again) != edited {
		t.Errorf("m2 release notes changed the notes:\n%s", again)
	}
	if code, stdout, _ := run("release", "notes", "-root", root, "-o", "-", "2027.03"); code != ExitOK || !strings.Contains(stdout, "## Counts") || strings.Contains(stdout, "Lagrange's theorem.") {
		t.Errorf("m2 release notes -o -: exit code %d, stdout %q", code, stdout)
	}
	gitT(t, root, "commit", "-q", "-am", "Write the summary of 2027.03")

	if code, stdout, stderr := run("release", "verify", "-root", root, "2027.03"); code != ExitOK || !strings.Contains(stdout, "graph: the rebuild gives the hash of the manifest") {
		t.Errorf("m2 release verify: exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	// The second run audits the tree. The seed has no Lean build, thus R02 fails, and the cut makes no tag.
	code, _, stderr = run("release", "cut", "-root", root, "-date", "2027-03-02")
	if code != ExitFailure || !strings.Contains(stderr, "the audit fails on the rules") || !strings.Contains(stderr, "R02") {
		t.Errorf("the second cut: exit code %d, stderr %q", code, stderr)
	}
	if tags := gitT(t, root, "tag"); tags != "" {
		t.Errorf("the failed cut made the tags %q", tags)
	}

	if err := os.WriteFile(filepath.Join(root, "nodes", "3C", "3C53TD.md"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitT(t, root, "commit", "-q", "-am", "Change a node")
	if code, _, stderr := run("release", "cut", "-root", root, "2027.03"); code != ExitFailure || !strings.Contains(stderr, "nodes/3C/3C53TD.md changed after the commit") {
		t.Errorf("a cut after a change of a node: exit code %d, stderr %q", code, stderr)
	}
}

func TestReleaseUsage(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"release"},
		{"release", "publish"},
		{"release", "cut", "-root", root, "2027-03"},
		{"release", "cut", "-root", root, "-date", "March", "2027.03"},
		{"release", "notes", "-root", root},
		{"release", "notes", "-root", root, "2027.03..next"},
		{"release", "verify", "-root", root, "-record", "ubuntu", "2027.03"},
		{"release", "verify", "-root", root},
	} {
		if code, _, _ := run(args...); code != ExitUsage {
			t.Errorf("m2 %v: exit code %d, and it must be %d", args, code, ExitUsage)
		}
	}
	if code, _, stderr := run("release", "cut", "-root", root); code != ExitFailure || !strings.Contains(stderr, "must be a git repository") {
		t.Errorf("a cut outside git: exit code %d, stderr %q", code, stderr)
	}
}
