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
	"github.com/tamnd/math2-agent/models"
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

// run runs git in root and fails the test on an error. It turns off the automatic gc and maintenance, because a detached gc can still write to .git when the test removes its temporary directory.
func run(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=audit", "-c", "user.email=audit@example.com", "-c", "commit.gpgsign=false", "-c", "gc.auto=0", "-c", "maintenance.auto=false"}, args...)...)
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
// setup prepares the state of the other packages that the fixtures of a rule need.
// fixedDate gives the commits of a fixture the date 2026-10-01.
func fixedDate(t *testing.T) {
	t.Setenv("GIT_AUTHOR_DATE", "2026-10-01T12:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2026-10-01T12:00:00Z")
}

var setup = map[string]func(t *testing.T){
	// V04 compares the date of a review with the date of the HEAD commit, thus the commits of the fixtures have a fixed date.
	"V04": func(t *testing.T) {
		t.Setenv("GIT_AUTHOR_DATE", "2026-10-01T12:00:00Z")
		t.Setenv("GIT_COMMITTER_DATE", "2026-10-01T12:00:00Z")
	},
	// A02 and A04 compare a date with the date of the HEAD commit, thus the commits of the fixtures have a fixed date.
	"A02": fixedDate,
	"A04": fixedDate,
	// L12 compares the date of a reading with the date of the HEAD commit.
	"L12": fixedDate,
	// X03 counts the runs of the week that ends a week before the date of the HEAD commit.
	"X03": fixedDate,
	// The manifests of R05 name the commit of step 1.
	"R05": fixedDate,
	// The README of T10 names the commit of step 1 as the cut-over commit.
	"T10": fixedDate,
	// P06 compares the date of locator_unconfirmed with the date of the HEAD commit.
	"P06": fixedDate,
	// The manifest of R01 names the commit of step 1, thus the commits of the fixtures have a fixed date.
	"R01": func(t *testing.T) {
		t.Setenv("GIT_AUTHOR_DATE", "2026-10-01T12:00:00Z")
		t.Setenv("GIT_COMMITTER_DATE", "2026-10-01T12:00:00Z")
	},
	// The register has only models of one family, thus X09 adds a judge of another family.
	"X09": func(t *testing.T) {
		saved := modelRegister
		t.Cleanup(func() { modelRegister = saved })
		reg := *models.Default()
		reg.Models = append(slices.Clone(reg.Models), models.Model{ID: "judge-a", Family: "testfamily", Provider: "test"})
		modelRegister = func() *models.Register { return &reg }
	},
}

// options gives the options of a rule that needs more than the tree.
var options = map[string]func(opt *Options, root string){
	// D05 reads the analysis plane of a checkout of tamnd/arxiv, which the fixture has in checkout/.
	"D05": func(opt *Options, root string) { opt.Arxiv = filepath.Join(root, "checkout") },
	// The Y rules read a checkout of tamnd/wikipedia-math, which the fixture has in wikipedia/.
	"Y01": wikipediaCheckout,
	"Y02": wikipediaCheckout,
	"Y03": wikipediaCheckout,
	"Y08": wikipediaCheckout,
	"Y09": wikipediaCheckout,
	"Y10": wikipediaCheckout,
	"Y13": wikipediaCheckout,
	"Y14": wikipediaCheckout,
}

func wikipediaCheckout(opt *Options, root string) { opt.Wikipedia = filepath.Join(root, "wikipedia") }

func ruleOptions(id, root string) Options {
	opt := Options{Rules: []string{id}}
	if f := options[id]; f != nil {
		f(&opt, root)
	}
	return opt
}

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
			if f := setup[e.ID]; f != nil {
				f(t)
			}
			root := tree(t, filepath.Join(dir, "good"))
			good, err := Run(root, ruleOptions(e.ID, root))
			if err != nil {
				t.Fatal(err)
			}
			if r := result(t, good, e.ID); r.State != check.Pass || r.Checked == 0 {
				t.Errorf("good: the state is %s with %d checked, and it must be pass", r.State, r.Checked)
			}
			for _, f := range good.Findings {
				t.Errorf("good: %s", f)
			}
			root = tree(t, filepath.Join(dir, "bad"))
			bad, err := Run(root, ruleOptions(e.ID, root))
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
	if len(rules) != 119 {
		t.Errorf("the catalogue has %d rules, and spec 15.3 and corpus spec K3.7, K4.9, K5.14, K6.12, K7.10, K9.9, K10.7 and K13.9 and corpus arxiv AX5.8 and corpus wikipedia have 119", len(rules))
	}
	planes := map[byte]string{'T': Nodes, 'S': Nodes, 'B': Nodes, 'G': Graph, 'F': Formal, 'V': Formal, 'C': Compute, 'A': Align, 'L': Licence, 'P': Prov, 'R': Release, 'X': Agents, 'I': Inventory, 'D': Inventory, 'N': Notation, 'Y': Wikipedia}
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
		reason := "the tree has no release manifest in manifests/releases"
		if r.Rule == "R04" {
			reason = "the tree has no record of a restore drill in reports/restore"
		}
		if r.State != check.Skip || r.Reason != reason {
			t.Errorf("%s: the state is %s %q, and it must be skip with the reason %q", r.Rule, r.State, r.Reason, reason)
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

func TestGroupAndHard(t *testing.T) {
	root := filepath.Join("testdata", "T05", "good")
	rep, err := Run(root, Options{Group: "Y", Hard: true})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range rep.Results {
		ids = append(ids, r.Rule)
		reason := "not written yet"
		if r.Rule == "Y01" || r.Rule == "Y02" || r.Rule == "Y03" {
			reason = "no checkout of tamnd/wikipedia-math is given, so set -wikipedia to run the rule"
		}
		if r.State != check.Skip || r.Reason != reason {
			t.Errorf("%s: the state is %s %q, and it must be skip with the reason %q", r.Rule, r.State, r.Reason, reason)
		}
	}
	if !slices.Equal(ids, []string{"Y01", "Y02", "Y03", "Y04", "Y05", "Y06", "Y11", "Y15", "Y18"}) {
		t.Errorf("the hard rules of the group Y are %v", ids)
	}
	if _, err := Run(root, Options{Group: "Y", Plane: Graph}); err == nil {
		t.Error("the group Y and the plane graph select no rule, and Run gave no error")
	}
}
