package propose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/audit"
	"github.com/tamnd/math2-agent/prov"
)

// tree returns a git repository with the tree of testdata/tree in one commit.
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("testdata", "tree"))); err != nil {
		t.Fatal(err)
	}
	gitT(t, root, "init", "-q", "-b", "main")
	gitT(t, root, "add", "-A")
	gitT(t, root, "commit", "-q", "-m", "the tree")
	return root
}

func gitT(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// fake is a Host in memory.
type fake struct {
	tokens map[string]string // token to login
	pulls  []Pull
	opened []PullRequest
}

func (f *fake) User(_ context.Context, token string) (string, error) {
	if login, ok := f.tokens[token]; ok {
		return login, nil
	}
	return "", fmt.Errorf("%w. GitHub does not accept the token", ErrUnauthorized)
}

func (f *fake) Pulls(_ context.Context, _ string, since time.Time) ([]Pull, error) {
	var out []Pull
	for _, p := range f.pulls {
		if p.Opened.After(since) || p.Closed.After(since) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fake) Open(_ context.Context, _ string, pr PullRequest) (Pull, error) {
	f.opened = append(f.opened, pr)
	p := Pull{Number: 100 + len(f.opened), URL: fmt.Sprintf("https://github.com/tamnd/math2/pull/%d", 100+len(f.opened)), Branch: pr.Branch, Opened: now}
	f.pulls = append(f.pulls, p)
	return p, nil
}

var now = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)

func proposer(root string) (*Proposer, *fake) {
	f := &fake{tokens: map[string]string{"t-ada": "ada"}}
	return &Proposer{Root: root, Host: f, Now: func() time.Time { return now }}, f
}

const monoid = "---\nkind: concept\nname: Monoid\nmsc: [20M10]\n---\n\n## Summary\n\nA monoid is a set with an associative binary operation and an identity element for it.\n"

// applied writes the files of the last pull request of f to the tree and commits them, as a merge of the pull request does.
func applied(t *testing.T, root string, f *fake) {
	t.Helper()
	pr := f.opened[len(f.opened)-1]
	gitT(t, root, "checkout", "-q", "-b", pr.Branch)
	for path, data := range pr.Files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, root, "add", "-A")
	gitT(t, root, "commit", "-q", "-m", pr.Message)
}

func TestOpenNode(t *testing.T) {
	root := tree(t)
	p, f := proposer(root)
	c, err := NewNode(root, []byte(monoid))
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Open(context.Background(), "t-ada", c, "The monoid is the next concept of the algebra list.")
	if err != nil {
		t.Fatal(err)
	}
	if res.Number != 101 || res.User != "github:ada" || len(res.Files) != 2 || res.Opened != 1 || res.Cap != 10 {
		t.Errorf("the result is %+v", res)
	}
	pr := f.opened[0]
	if pr.User != "ada" || pr.Base != "main" || pr.Commit != gitT(t, root, "rev-parse", "HEAD") || Method(pr.Branch) != MethodNode {
		t.Errorf("the pull request is %+v", pr)
	}
	if !strings.Contains(pr.Body, "> The monoid is the next concept") || !strings.Contains(pr.Body, "`prov/~/"+c.Tag+".jsonl`") {
		t.Errorf("the body is\n%s", pr.Body)
	}
	if pr.Title != "Add the concept Monoid" {
		t.Errorf("the title is %q", pr.Title)
	}
	lines, err := prov.Parse(strings.NewReader(string(pr.Files[prov.Path(c.Tag)])))
	if err != nil || len(lines) != 1 || lines[0].Field != prov.Whole || lines[0].Op != prov.Add || lines[0].Human != "github:ada" || lines[0].At != "2026-11-02T10:00:00Z" {
		t.Errorf("the provenance is %+v, %v", lines, err)
	}
	if got, err := os.ReadDir(filepath.Join(root, "nodes")); err != nil || len(got) != 2 {
		t.Errorf("Open changed the tree: %v %v", got, err)
	}
	if wts := gitT(t, root, "worktree", "list"); strings.Count(wts, "\n") != 0 {
		t.Errorf("Open left a worktree:\n%s", wts)
	}

	// The tree of the pull request passes the audit of a pull request, as CI runs it.
	applied(t, root, f)
	rep, err := audit.Run(root, audit.Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed() {
		t.Errorf("the pull request fails the audit: %v", rep.Findings)
	}
}

func TestOpenEdits(t *testing.T) {
	tests := []struct {
		name  string
		make  func(root string) (Change, error)
		title string
		want  []string
	}{
		{"edit", func(root string) (Change, error) {
			data, err := os.ReadFile(filepath.Join(root, "nodes", "G5", "G5S1M9.md"))
			if err != nil {
				return Change{}, err
			}
			return Edit(root, "G5S1M9", []byte(strings.Replace(string(data), "number of elements.", "number of its elements.", 1)))
		}, "Edit G5S1M9 Group", []string{"number of its elements."}},
		{"binding", func(root string) (Change, error) {
			return Bind(root, "3C53TD", Binding{Decl: "Subgroup.card_subgroup_dvd_card", Match: "exact"})
		}, "Bind 3C53TD to Subgroup.card_subgroup_dvd_card", []string{"formal:", "  - system: lean4\n    library: mathlib\n    decl: Subgroup.card_subgroup_dvd_card\n    match: exact\n"}},
		{"same_as", func(root string) (Change, error) {
			return Align(root, "G5S1M9", Alignment{Edge: "same_as", System: "wikidata", ID: "Q83478"})
		}, "Align G5S1M9 with wikidata Q83478", []string{"same_as:\n  wikidata: Q83478"}},
		{"close_to", func(root string) (Change, error) {
			return Align(root, "G5S1M9", Alignment{Edge: "close_to", System: "nlab", ID: "group", Note: "the nLab page is about groups in any category"})
		}, "Align G5S1M9 with nlab group", []string{"close_to:", "note: the nLab page is about groups in any category"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tree(t)
			p, f := proposer(root)
			c, err := tt.make(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.Open(context.Background(), "t-ada", c, ""); err != nil {
				t.Fatal(err)
			}
			pr := f.opened[0]
			if pr.Title != tt.title || strings.Contains(pr.Body, "note of the author") {
				t.Errorf("the title is %q and the body is\n%s", pr.Title, pr.Body)
			}
			text := string(pr.Files["nodes/"+c.Tag[:2]+"/"+c.Tag+".md"])
			for _, w := range tt.want {
				if !strings.Contains(text, w) {
					t.Errorf("the node file has no %q:\n%s", w, text)
				}
			}
			if log := string(pr.Files[prov.Path(c.Tag)]); !strings.Contains(log, `"op":"`) || !strings.Contains(log, `"human":"github:ada"`) {
				t.Errorf("the log is %s", log)
			}
			applied(t, root, f)
			rep, err := audit.Run(root, audit.Options{Base: "main"})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Failed() {
				t.Errorf("the pull request fails the audit: %v", rep.Findings)
			}
		})
	}
}

func TestOpenAudit(t *testing.T) {
	root := tree(t)
	p, f := proposer(root)
	// The node uses a tag that no node has, thus a hard rule fails.
	c, err := NewNode(root, []byte("---\nkind: concept\nname: Ring\nmsc: [16-01]\n---\n\n## Summary\n\nA ring is an abelian [[group|ZZZZZZ]] with a second operation.\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Open(context.Background(), "t-ada", c, "")
	var ae *AuditError
	if !errors.As(err, &ae) || !strings.Contains(err.Error(), "ZZZZZZ") {
		t.Fatalf("the error is %v", err)
	}
	if len(f.opened) != 0 {
		t.Error("Open opened a pull request that fails the audit")
	}
}

func TestOpenLimit(t *testing.T) {
	root := tree(t)
	p, f := proposer(root)
	p.Limit = Limit{Multiple: 2, Floor: 2}
	f.pulls = []Pull{
		{Branch: Branch + MethodNode + "/a", Opened: now.Add(-2 * 24 * time.Hour)},
		{Branch: Branch + MethodNode + "/b", Opened: now.Add(-3 * 24 * time.Hour)},
		{Branch: Branch + MethodEdit + "/c", Opened: now.Add(-24 * time.Hour)},
		{Branch: "fix-typo", Opened: now.Add(-24 * time.Hour)},
	}
	c, err := NewNode(root, []byte(monoid))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Open(context.Background(), "t-ada", c, ""); !errors.Is(err, ErrLimit) || !strings.Contains(err.Error(), "cap is 2") {
		t.Fatalf("the error is %v", err)
	}
	// Six pull requests of the method were closed in four weeks. The cap is 2 × 6 / 4 = 3.
	for i := range 6 {
		f.pulls = append(f.pulls, Pull{Branch: fmt.Sprintf("%s%s/old%d", Branch, MethodNode, i), Opened: now.Add(-20 * 24 * time.Hour), Closed: now.Add(-10 * 24 * time.Hour)})
	}
	res, err := p.Open(context.Background(), "t-ada", c, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Opened != 3 || res.Cap != 3 {
		t.Errorf("opened %d with the cap %d", res.Opened, res.Cap)
	}
	if _, err := p.Open(context.Background(), "t-ada", c, ""); !errors.Is(err, ErrLimit) {
		t.Errorf("the fourth pull request in a week: %v", err)
	}
}

func TestOpenErrors(t *testing.T) {
	root := tree(t)
	p, _ := proposer(root)
	c, err := NewNode(root, []byte(monoid))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "t-unknown"} {
		if _, err := p.Open(context.Background(), token, c, ""); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("the token %q: %v", token, err)
		}
	}
	if _, err := p.Open(context.Background(), "t-ada", Change{Method: "delete"}, ""); err == nil {
		t.Error("Open accepted a change of no method")
	}
	bad := Change{Method: MethodNode, Tag: "~0123abcd", Files: map[string][]byte{"../x.md": []byte("x")}}
	if _, err := p.Open(context.Background(), "t-ada", bad, ""); err == nil {
		t.Error("Open accepted a path out of the tree")
	}
}

func TestChangeErrors(t *testing.T) {
	root := tree(t)
	group, err := os.ReadFile(filepath.Join(root, "nodes", "G5", "G5S1M9.md"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		err  func() error
		want string
	}{
		{"node with a tag", func() error {
			_, err := NewNode(root, []byte(strings.Replace(monoid, "kind:", "tag: 7K3QX2\nkind:", 1)))
			return err
		}, "the mint workflow gives it a tag"},
		{"node of no kind", func() error {
			_, err := NewNode(root, []byte(strings.Replace(monoid, "concept", "idea", 1)))
			return err
		}, `"idea" is not a kind`},
		{"node that is not a file", func() error { _, err := NewNode(root, []byte("A monoid.")); return err }, "not a node file"},
		{"edit of no node", func() error { _, err := Edit(root, "ZZZZZZ", group); return err }, "no node has the tag ZZZZZZ"},
		{"edit of a bad tag", func() error { _, err := Edit(root, "../G5", group); return err }, "is not a tag"},
		{"edit of the tag", func() error { _, err := Edit(root, "3C53TD", group); return err }, "keeps the tag 3C53TD"},
		{"edit of the kind", func() error {
			_, err := Edit(root, "G5S1M9", []byte(strings.Replace(string(group), "kind: concept", "kind: definition", 1)))
			return err
		}, "keeps the kind concept"},
		{"edit with no change", func() error { _, err := Edit(root, "G5S1M9", group); return err }, "does not change"},
		{"binding of a bad name", func() error { _, err := Bind(root, "3C53TD", Binding{Decl: "a b", Match: "exact"}); return err }, "not the name of a Lean declaration"},
		{"binding of a bad match", func() error { _, err := Bind(root, "3C53TD", Binding{Decl: "Nat.add", Match: "same"}); return err }, `"same" is not`},
		{"exact binding with an adapter", func() error {
			_, err := Bind(root, "3C53TD", Binding{Decl: "Nat.add", Match: "exact", Adapter: "Math2.Match.x"})
			return err
		}, "has no adapter"},
		{"alignment of a bad edge", func() error { _, err := Align(root, "G5S1M9", Alignment{Edge: "uses", ID: "x"}); return err }, `"uses" is not`},
		{"close_to with no note", func() error {
			_, err := Align(root, "G5S1M9", Alignment{Edge: "close_to", System: "nlab", ID: "group"})
			return err
		}, "a note"},
		{"aligned with a system", func() error {
			_, err := Align(root, "G5S1M9", Alignment{Edge: "aligned", System: "arxiv", ID: "m2://arxiv/1", Match: "exact"})
			return err
		}, "no system"},
	}
	for _, tt := range tests {
		if err := tt.err(); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: the error is %v, and it must have %q", tt.name, err, tt.want)
		}
	}

	// A second binding to the same declaration.
	c, err := Bind(root, "3C53TD", Binding{Decl: "Nat.add", Match: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range c.Files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Bind(root, "3C53TD", Binding{Decl: "Nat.add", Match: "weaker"}); err == nil || !strings.Contains(err.Error(), "has a binding to Nat.add") {
		t.Errorf("the second binding: %v", err)
	}
}

func TestLimit(t *testing.T) {
	for _, tt := range []struct {
		l      Limit
		closed int
		want   int
	}{
		{Limit{}, 0, 10},
		{Limit{}, 40, 20},
		{Limit{Multiple: 1.5, Floor: 1}, 9, 4},
		{Limit{Multiple: 1, Floor: 5}, 8, 5},
	} {
		if got := tt.l.Cap(tt.closed); got != tt.want {
			t.Errorf("%+v.Cap(%d) = %d, want %d", tt.l, tt.closed, got, tt.want)
		}
	}
}

func TestMethod(t *testing.T) {
	for branch, want := range map[string]string{
		"m2-api/propose_node/0123abcd-ff00aa11": MethodNode,
		"m2-api/propose_edit/G5S1M9-00":         MethodEdit,
		"m2-api/delete/x":                       "",
		"propose_node/x":                        "",
	} {
		if got := Method(branch); got != want {
			t.Errorf("Method(%q) = %q, want %q", branch, got, want)
		}
	}
}

func TestResultJSON(t *testing.T) {
	data, err := json.Marshal(Result{Pull: Pull{Number: 7, Branch: "b", Opened: now}, User: "github:ada"})
	if err != nil {
		t.Fatal(err)
	}
	if s := string(data); !strings.Contains(s, `"number":7`) || strings.Contains(s, `"closed"`) {
		t.Errorf("the JSON is %s", s)
	}
}
