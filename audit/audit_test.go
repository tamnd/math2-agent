package audit

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/check"
)

var update = flag.Bool("update", false, "write the want.txt file of each bad fixture")

// tree returns the root of the fixture tree at dir. When dir has numbered directories 1, 2 and more, tree makes a git repository with one commit for each of them, in order, and returns the repository.
func tree(t *testing.T, dir string) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "1")); err != nil {
		return dir
	}
	root := t.TempDir()
	run(t, root, "init", "-q", "-b", "main")
	for i := 1; ; i++ {
		step := filepath.Join(dir, itoa(i))
		if _, err := os.Stat(step); err != nil {
			break
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Name() != ".git" {
				if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := os.CopyFS(root, os.DirFS(step)); err != nil {
			t.Fatal(err)
		}
		commit(t, root, "step "+itoa(i))
	}
	return root
}

func itoa(i int) string { return string(rune('0' + i)) }

// run runs git in root and fails the test on an error.
func run(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=audit", "-c", "user.email=audit@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func commit(t *testing.T, root, msg string) {
	t.Helper()
	run(t, root, "add", "-A")
	run(t, root, "commit", "-q", "--allow-empty", "-m", msg)
}

func result(t *testing.T, rep *check.Report, id string) check.Result {
	t.Helper()
	for _, r := range rep.Results {
		if r.Rule == id {
			return r
		}
	}
	t.Fatalf("the report has no result for %s", id)
	return check.Result{}
}

// TestFixtures checks that each written rule passes on its good fixture and finds the problems of want.txt in its bad fixture (spec 15.3).
func TestFixtures(t *testing.T) {
	for _, e := range catalogue {
		dir := filepath.Join("testdata", e.ID)
		_, err := os.Stat(dir)
		if e.run == nil {
			if err == nil {
				t.Errorf("%s has fixtures, and the rule is not written", e.ID)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s has no fixture pair in %s", e.ID, dir)
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			good, err := Run(tree(t, filepath.Join(dir, "good")), Options{Rules: []string{e.ID}})
			if err != nil {
				t.Fatal(err)
			}
			if r := result(t, good, e.ID); r.State != check.Pass || r.Checked == 0 {
				t.Errorf("good: the state is %s with %d checked, and it must be pass", r.State, r.Checked)
			}
			for _, f := range good.Findings {
				t.Errorf("good: %s", f)
			}
			bad, err := Run(tree(t, filepath.Join(dir, "bad")), Options{Rules: []string{e.ID}})
			if err != nil {
				t.Fatal(err)
			}
			wantState := check.Found
			if e.Hard {
				wantState = check.Fail
			}
			if r := result(t, bad, e.ID); r.State != wantState {
				t.Errorf("bad: the state is %s, and it must be %s", r.State, wantState)
			}
			var got strings.Builder
			for _, f := range bad.Findings {
				got.WriteString(f.String() + "\n")
			}
			golden := filepath.Join(dir, "want.txt")
			if *update {
				if err := os.WriteFile(golden, []byte(got.String()), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != string(want) {
				t.Errorf("bad: the findings are\n%s\nand %s has\n%s", got.String(), golden, want)
			}
		})
	}
}

func TestCatalogue(t *testing.T) {
	rules := Rules()
	if len(rules) != 65 {
		t.Errorf("the catalogue has %d rules, and spec 15.3 has 65", len(rules))
	}
	planes := map[byte]string{'T': Nodes, 'S': Nodes, 'B': Nodes, 'G': Graph, 'F': Formal, 'V': Formal, 'C': Compute, 'A': Align, 'L': Licence, 'P': Prov, 'R': Release, 'X': Agents}
	id := regexp.MustCompile(`^[A-Z][0-9]{2}$`)
	seen := map[string]bool{}
	for _, r := range rules {
		if !id.MatchString(r.ID) || seen[r.ID] {
			t.Errorf("%q is not a new rule id", r.ID)
		}
		seen[r.ID] = true
		if planes[r.ID[0]] != r.Plane {
			t.Errorf("%s has the plane %s, and the letter gives %s", r.ID, r.Plane, planes[r.ID[0]])
		}
	}
}

func TestSelect(t *testing.T) {
	root := filepath.Join("testdata", "T05", "good")
	for _, opt := range []Options{{Plane: "trust"}, {Rules: []string{"Z99"}}, {Plane: Graph, Rules: []string{"T05"}}} {
		if _, err := Run(root, opt); err == nil {
			t.Errorf("%+v: Run gave no error", opt)
		}
	}
	rep, err := Run(root, Options{Plane: Release})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range rep.Results {
		ids = append(ids, r.Rule)
		if r.State != check.Skip || r.Reason != "not written yet" {
			t.Errorf("%s: the state is %s %q, and it must be skip because the rule is not written", r.Rule, r.State, r.Reason)
		}
	}
	if !slices.Equal(ids, []string{"R01", "R02", "R03", "R04", "R05"}) {
		t.Errorf("the release plane has the rules %v", ids)
	}
}

func TestNoHistory(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("testdata", "T05", "good"))); err != nil {
		t.Fatal(err)
	}
	rep, err := Run(root, Options{Rules: []string{"T01"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := result(t, rep, "T01"); r.State != check.Skip || r.Reason == "" {
		t.Errorf("the state is %s %q, and it must be skip with a reason", r.State, r.Reason)
	}
}

// TestPullRequest checks the scope of an audit with a base revision, and the provisional ids of rule T03 in a pull request.
func TestPullRequest(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("testdata", "T03", "good"))); err != nil {
		t.Fatal(err)
	}
	run(t, root, "init", "-q", "-b", "main")
	commit(t, root, "main")
	run(t, root, "checkout", "-q", "-b", "change")
	for _, dir := range []string{"nodes/~", "topics"} {
		if err := os.CopyFS(filepath.Join(root, dir), os.DirFS(filepath.Join("testdata", "T03", "bad", dir))); err != nil {
			t.Fatal(err)
		}
	}
	commit(t, root, "add a node")

	rep, err := Run(root, Options{Plane: Nodes, Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 1 {
		t.Errorf("the scope has %d node files, and the pull request changed 1", rep.Files)
	}
	for _, f := range rep.Findings {
		t.Errorf("%s", f)
	}
	if r := result(t, rep, "T03"); r.State != check.Pass || r.Checked != 2 {
		t.Errorf("T03: the state is %s with %d checked, and it must be pass with 2", r.State, r.Checked)
	}

	// An untracked file that refers to the provisional id of another pull request.
	if err := os.WriteFile(filepath.Join(root, "topics", "other.yaml"), []byte("members: [~0123abcd]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err = Run(root, Options{Rules: []string{"T03"}, Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if r := result(t, rep, "T03"); r.State != check.Fail || len(rep.Findings) != 1 || rep.Findings[0].Path != "topics/other.yaml" {
		t.Errorf("T03: the state is %s with the findings %v, and it must fail on topics/other.yaml", r.State, rep.Findings)
	}

	// The full audit of the same tree finds each provisional id.
	rep, err = Run(root, Options{Rules: []string{"T03"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 3 {
		t.Errorf("the full audit has %d T03 findings, and the tree has 3 provisional ids", len(rep.Findings))
	}

	if _, err := Run(root, Options{Base: "--help"}); err == nil {
		t.Error("Run accepted a base revision that starts with -")
	}
}
