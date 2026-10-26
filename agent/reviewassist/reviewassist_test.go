package reviewassist_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/reviewassist"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/workspace"
)

const lagrangePath = "nodes/3C5/3C53TD.md"

const lagrange = `---
tag: 3C53TD
kind: statement
role: theorem
name: Lagrange's theorem
formal:
  - system: lean4
    library: math2
    decl: Math2.Algebra.lagrange
    match: exact
---

## Statement

For each finite group G and each subgroup H of G, the order of H divides the order of G.
`

var records = map[string]lean.Record{
	"Math2.Algebra.lagrange": {
		Decl:                     "Math2.Algebra.lagrange",
		Kind:                     "theorem",
		TypeHash:                 "blake3:lagrange",
		StatementPP:              "∀ {G : Type u_1} [inst : Group G] (H : Subgroup G), Nat.card ↥H ∣ Nat.card G",
		StatementPPAll:           "∀ {G : Type u_1} [inst : Group.{u_1} G] (H : Subgroup.{u_1} G inst), @Dvd.dvd.{0} Nat Nat.instDvdNat (Nat.card.{u_1} (Subgroup.toSet H)) (Nat.card.{u_1} G)",
		UsesConstantsInStatement: []string{"Group", "Subgroup", "Nat.card", "Dvd.dvd"},
	},
}

// tree makes a git repository with the files in one commit, opens it as a workspace, and reads the input of the review packets as a build does.
func tree(t *testing.T, files map[string]string) (*workspace.Workspace, review.Input) {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	w, err := workspace.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	nodes, err := node.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	st := status.Compute(status.Input{Nodes: nodes, Records: records})
	return w, review.Input{Nodes: nodes, Status: st, Records: records}
}

func job() *agent.Job {
	return &agent.Job{ID: "j-2026-10-26-0001", Role: "review-assist", Target: "3C53TD", Goal: "the node has a review packet with the concerns of the critic", OnBehalfOf: "github:octocat"}
}

const by = "m2-agent/review-assist@0.1.0"

func TestFile(t *testing.T) {
	f := &reviewassist.File{Tag: "3C53TD", By: by, Packet: "3C53TD Lagrange's theorem\n", Concerns: []reviewassist.Concern{
		{Decl: "Math2.Algebra.lagrange", Class: "M-junk", Text: "Nat.card is 0 for an infinite group, and 0 divides only 0."},
		{Decl: "Math2.Algebra.lagrange", Class: "M-hyp-missing", Text: "the statement has no hypothesis that G is finite."},
	}}
	md := string(f.Markdown())
	for _, want := range []string{
		"# Review packet of 3C53TD\n\nm2-agent/review-assist@0.1.0 made this packet",
		"It is not a review. To review, run `m2 review 3C53TD`.",
		"## Packet\n\n````text\n3C53TD Lagrange's theorem\n````\n\n## Concerns\n\n- `Math2.Algebra.lagrange` M-junk: Nat.card is 0",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("the file has no %q:\n%s", want, md)
		}
	}
	g, err := reviewassist.ParseFile([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if g.Tag != f.Tag || g.By != f.By || g.Packet != f.Packet || len(g.Concerns) != 2 || g.Concerns[1] != f.Concerns[1] {
		t.Errorf("ParseFile gives %+v", g)
	}
	f.Concerns = nil
	if g, err := reviewassist.ParseFile(f.Markdown()); err != nil || len(g.Concerns) != 0 || !strings.Contains(string(f.Markdown()), "The critic found no concern.") {
		t.Errorf("a file with no concern gives %+v, %v", g, err)
	}
	if _, err := reviewassist.ParseFile([]byte("# Notes\n")); err == nil {
		t.Error("a file that is not a packet gives no error")
	}
	bad := strings.Replace(md, "- `Math2.Algebra.lagrange` M-junk:", "- Math2.Algebra.lagrange M-junk:", 1)
	if _, err := reviewassist.ParseFile([]byte(bad)); err == nil || !strings.Contains(err.Error(), "is not a concern") {
		t.Errorf("a bad concern gives %v", err)
	}
}

func TestCheck(t *testing.T) {
	_, in := tree(t, map[string]string{lagrangePath: lagrange})
	_, p, err := reviewassist.Packet(in, "3C53TD")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		c    reviewassist.Concern
		want string
	}{
		{reviewassist.Concern{Decl: "Math2.Algebra.lagrange", Class: "M-junk", Text: "Nat.card is 0 for an infinite group."}, ""},
		{reviewassist.Concern{Decl: "Subgroup.card_dvd_card", Class: "M-junk", Text: "a."}, "the packet has no binding of Subgroup.card_dvd_card. Its bindings are Math2.Algebra.lagrange"},
		{reviewassist.Concern{Decl: "Math2.Algebra.lagrange", Class: "M-wrong", Text: "a."}, "M-wrong is not a class of spec 10.5"},
		{reviewassist.Concern{Decl: "Math2.Algebra.lagrange", Class: "M-junk", Text: " "}, "has no reason"},
		{reviewassist.Concern{Decl: "Math2.Algebra.lagrange", Class: "M-junk", Text: strings.Repeat("a", reviewassist.MaxConcern+1)}, "more than 400 characters"},
	} {
		err := reviewassist.Check(p, c.c)
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("Check(%+v) gives %v, want %q", c.c, err, c.want)
		}
	}
}

func TestBrief(t *testing.T) {
	ws, in := tree(t, map[string]string{lagrangePath: lagrange, reviewassist.Path("3C53TD"): "# Review packet of 3C53TD\n"})
	brief, err := reviewassist.Brief(ws, in, job())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Job j-2026-10-26-0001: prepare the review packet of the target 3C53TD, a statement node,",
		`<data source="m2 review 3C53TD" licence="CC-BY-SA-4.0">`,
		"Nat.card ↥H ∣ Nat.card G",
		"The tree has a packet of an earlier run. Your packet replaces it:",
		`<data source="reports/review/3C53TD.md"`,
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief has no %q:\n%s", want, brief)
		}
	}
	ws, in = tree(t, map[string]string{lagrangePath: "---\ntag: 3C53TD\nkind: statement\nname: Lagrange's theorem\n---\n\n## Statement\n\nThe order.\n"})
	if _, err := reviewassist.Brief(ws, in, job()); err == nil || !strings.Contains(err.Error(), "has no Lean binding") {
		t.Errorf("a node with no binding gives %v", err)
	}
}

var u = llm.Usage{Input: 1000, Output: 200}

func write(concerns string) *llm.Response {
	return llmtest.Call("write_packet", map[string]string{"concerns": concerns}, u)
}

func TestRun(t *testing.T) {
	ws, in := tree(t, map[string]string{lagrangePath: lagrange})
	role, err := agent.LoadRole(prompts.FS, "review-assist")
	if err != nil {
		t.Fatal(err)
	}
	j := job()
	brief, err := reviewassist.Brief(ws, in, j)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tools.Profile(role, nil, ws, nil, reviewassist.Write(ws, in, by))
	if err != nil {
		t.Fatal(err)
	}
	model := llmtest.New(
		llmtest.Call("read_file", map[string]string{"path": lagrangePath}, u),
		write("Math2.Algebra.lagrange M-faith: the statement is right."),
		write("Math2.Algebra.lagrange M-hyp-missing: the statement has no hypothesis that G is finite, and Nat.card is 0 for an infinite group.\n`Math2.Algebra.lagrange` M-junk: Nat.card G is 0 when G is infinite, thus the divisibility holds for a trivial reason."),
	)
	rn := agent.Runner{Role: role, Model: model, Tools: ts, Goal: reviewassist.Goal(ws, in, j)}
	rec, err := rn.Run(context.Background(), j, 0, brief)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Outcome != agent.Success || len(rec.Turns) != 3 {
		t.Fatalf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
	}
	if got := rec.Turns[0].Why; !strings.Contains(got, "the run has not written the packet reports/review/3C53TD.md") {
		t.Errorf("after no packet the goal says %q", got)
	}
	if r := rec.Turns[1].Results[0]; !r.IsError || !strings.Contains(r.Content, "M-faith is not a class") {
		t.Errorf("a bad class gives %+v", r)
	}
	if r := rec.Turns[2].Results[0]; r.IsError || r.Content != "Wrote the packet of 3C53TD with 2 concerns to reports/review/3C53TD.md." {
		t.Errorf("the packet gives %+v", r)
	}
	data, err := ws.Read(reviewassist.Path("3C53TD"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := reviewassist.ParseFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.By != by || len(f.Concerns) != 2 || f.Concerns[1].Class != "M-junk" || !strings.Contains(f.Packet, "Nat.card ↥H ∣ Nat.card G") {
		t.Errorf("the packet is %+v", f)
	}
}

func TestGoal(t *testing.T) {
	ws, in := tree(t, map[string]string{lagrangePath: lagrange})
	goal := reviewassist.Goal(ws, in, job())
	check := func(text, want string) {
		t.Helper()
		if err := ws.Write(reviewassist.Path("3C53TD"), []byte(text)); err != nil {
			t.Fatal(err)
		}
		ok, why, err := goal(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if want == "" && !ok || want != "" && (ok || !strings.Contains(why, want)) {
			t.Errorf("the goal gives %v, %q, want %q", ok, why, want)
		}
	}
	text, _, err := reviewassist.Packet(in, "3C53TD")
	if err != nil {
		t.Fatal(err)
	}
	f := &reviewassist.File{Tag: "3C53TD", By: by, Packet: text}
	check(string(f.Markdown()), "")
	f.Packet = strings.Replace(text, "Nat.card G", "Nat.card H", 1)
	check(string(f.Markdown()), "is not the packet of the build")
	f.Packet = text
	f.Concerns = []reviewassist.Concern{{Decl: "Group", Class: "M-def", Text: "a."}}
	check(string(f.Markdown()), "the packet has no binding of Group")
	check("# Notes\n", "is not a review packet")
	if err := ws.Write(lagrangePath, []byte(strings.Replace(lagrange, "match: exact", "match: weaker", 1))); err != nil {
		t.Fatal(err)
	}
	f.Concerns = nil
	check(string(f.Markdown()), "review-assist writes only the review packet of the target")
}
