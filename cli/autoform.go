package cli

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/formalize"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/autoform"
	"github.com/tamnd/math2-agent/battery"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/models"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/workspace"
)

const autoformUsage = "m2 autoform [-root DIR] [-out DIR] [-store DIR] [-work DIR] [-rungs LIST] [-on-behalf-of ID] [-budget USD] [-branch NAME] [-pr] [-sandbox IMAGE | -no-sandbox] [-json] <tag>"

// autoformDir is the directory of the reports of m2 autoform in the store.
const autoformDir = "autoform"

// openPull pushes the branch of the work tree and opens the pull request with the body file. The tests replace it.
var openPull = func(ctx context.Context, dir, branch, title, body string) (string, error) {
	push := exec.CommandContext(ctx, "git", "-C", dir, "push", "-q", "-u", "origin", branch) // #nosec G204 -- the arguments are fixed here, and no shell reads them.
	if b, err := push.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git push: %w: %s", err, strings.TrimSpace(string(b)))
	}
	pr := exec.CommandContext(ctx, "gh", "pr", "create", "--head", branch, "--title", title, "--body-file", body) // #nosec G204 -- as above.
	pr.Dir = dir
	b, err := pr.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr create: %w: %s", err, strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}

func runAutoform(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 autoform", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree, a git repository")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	store := flags.String("store", "", "the directory of the run records and of the reports. Empty means the output directory, where the rules P02 and P03 read the summaries.")
	work := flags.String("work", "", "the directory of the work trees of the candidates. Empty means a new directory in the temporary directory.")
	rungList := flags.String("rungs", "0,1", "the rungs of the ladder of formalize, one candidate for each, in order. A rung can occur more than one time.")
	behalf := flags.String("on-behalf-of", os.Getenv("M2_ON_BEHALF_OF"), "the person, the project or the schedule on whose behalf the runs work, for example github:octocat. The default is $M2_ON_BEHALF_OF.")
	usd := flags.Float64("budget", 0, "the budget of each run in dollars. Zero means the budget of the role.")
	branch := flags.String("branch", "", "the branch of the winning candidate. Empty means autoform/ and the tag.")
	pr := flags.Bool("pr", false, "push the branch to origin and open the pull request with gh")
	asJSON := flags.Bool("json", false, "write the result as JSON")
	box := sandboxFlags(flags)
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 1 {
		fmt.Fprintf(stderr, "Usage: %s\n", autoformUsage)
		return ExitUsage
	}
	target := flags.Arg(0)
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 autoform: %v\n", err)
		return ExitFailure
	}
	p := tools.NodePath(target)
	if p == "" {
		return fail(fmt.Errorf("%q is not a tag or a provisional id", target))
	}
	if _, err := os.Stat(filepath.Join(*root, filepath.FromSlash(p))); err != nil {
		return fail(fmt.Errorf("the tree has no node %s: %w", target, err))
	}
	r, err := agent.LoadRole(prompts.FS, "formalize")
	if err != nil {
		return fail(err)
	}
	var rungs []int
	for s := range strings.SplitSeq(*rungList, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 0 || n >= len(r.Ladder) {
			return fail(fmt.Errorf("the rung %q is not a rung of the ladder of formalize, from 0 to %d", s, len(r.Ladder)-1))
		}
		rungs = append(rungs, n)
	}
	b, err := box(*root)
	if err != nil {
		return fail(err)
	}
	model, err := newModel()
	if err != nil {
		return fail(err)
	}
	if *out == "" {
		*out = filepath.Join(*root, "build")
	}
	if *store == "" {
		*store = *out
	}
	svc, err := api.Open(api.Config{Root: *root, Out: *out})
	if errors.Is(err, fs.ErrNotExist) {
		return fail(fmt.Errorf("%w. Run m2 build first", err))
	}
	if err != nil {
		return fail(err)
	}
	defer func() { _ = svc.Close() }()
	nodes, err := readNodes(*root)
	if err != nil {
		return fail(err)
	}

	rnd := make([]byte, 4)
	_, _ = rand.Read(rnd)
	id := "j-" + time.Now().UTC().Format("2006-01-02") + "-" + hex.EncodeToString(rnd)
	if *work == "" {
		*work = filepath.Join(os.TempDir(), "m2-autoform", id)
	}
	ctx := context.Background()
	rs := &runSetup{root: *root, out: *out, store: *store, svc: svc, model: model, roles: map[string]*agent.Role{"formalize": r}, box: b}
	var (
		cands []autoform.Candidate
		recs  []*agent.Record
		trees []*workspace.Workspace
	)
	defer func() {
		for _, ws := range trees {
			if ws != nil {
				_ = ws.Close()
			}
		}
	}()
	for i, rung := range rungs {
		job := &agent.Job{ID: fmt.Sprintf("%sc%d", id, i+1), Role: "formalize", Target: target, Goal: goals["formalize"], OnBehalfOf: *behalf}
		job.Budget.USD = *usd
		if err := job.Validate(); err != nil {
			return fail(err)
		}
		rec, ws, changes, runErr := rs.run(ctx, job, rung, filepath.Join(*work, fmt.Sprintf("c%d", i+1)))
		trees = append(trees, ws)
		recs = append(recs, rec)
		if rec == nil {
			return fail(fmt.Errorf("candidate %d: %w", i+1, runErr))
		}
		c := autoform.Candidate{Run: rec.ID, Rung: rung, Outcome: rec.Outcome, Reason: rec.Reason}
		if ms := rec.Models(); len(ms) > 0 {
			c.Model = ms[0]
			c.Family = models.Default().Family(c.Model)
		}
		switch {
		case runErr != nil:
			c.Reason = runErr.Error()
		case rec.Outcome == agent.Success:
			if err := candidate(ctx, &c, ws, changes, target, b); err != nil {
				return fail(fmt.Errorf("candidate %d: %w", i+1, err))
			}
		}
		cands = append(cands, c)
	}
	res := autoform.Select(target, cands)
	res.Unbound = autoform.Unbound(nodes, target)
	dir := filepath.Join(*store, autoformDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fail(err)
	}
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return fail(err)
	}
	report := filepath.Join(dir, id+".md")
	if err := os.WriteFile(filepath.Join(dir, id+".json"), append(data, '\n'), 0o600); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(report, []byte(res.Markdown()), 0o600); err != nil {
		return fail(err)
	}
	name, url := "", ""
	if res.Winner >= 0 {
		name = cmp.Or(*branch, "autoform/"+target)
		w := res.Winner
		title := fmt.Sprintf("formal: bind %s to %s with m2 autoform", target, res.Candidates[w].Decl)
		if err := commitWinner(ctx, trees[w], recs[w], r, target, name, title); err != nil {
			return fail(err)
		}
		if *pr {
			if url, err = openPull(ctx, trees[w].Dir, name, title, report); err != nil {
				return fail(err)
			}
		}
	}
	if *asJSON {
		fmt.Fprintf(stdout, "%s\n", data)
	} else {
		writeAutoform(stdout, res, report)
		switch {
		case url != "":
			fmt.Fprintf(stdout, "the pull request is %s\n", url)
		case name != "":
			fmt.Fprintf(stdout, "the winner is on the branch %s. Open the pull request with: git -C %s push -u origin %s && gh pr create --head %s --body-file %s\n", name, trees[res.Winner].Dir, name, name, report)
		}
		fmt.Fprintf(stdout, "Remove the work trees with: git -C %s worktree remove --force DIR, for each directory in %s\n", *root, *work)
	}
	if res.Winner < 0 {
		return ExitFailure
	}
	return ExitOK
}

// readNodes parses each node file of the tree. A file that does not parse is not in the result, because m2 check reports it.
func readNodes(root string) ([]*node.Node, error) {
	files, err := check.ReadTree(root)
	if err != nil {
		return nil, err
	}
	var out []*node.Node
	for _, f := range files {
		if n, err := node.Parse(f.Path, f.Data); err == nil {
			out = append(out, n)
		}
	}
	return out, nil
}

// candidate fills the statement of the candidate c from the work tree of a run that reached its goal: the new binding of the target, the hash of its statement, the Lean files of the run and the report of the battery. The battery runs with no prover and no model, thus the checks 2, 7, 8 and 9 do not run.
func candidate(ctx context.Context, c *autoform.Candidate, ws *workspace.Workspace, changes []workspace.Change, target string, box *tools.Box) error {
	added, _, err := formalize.Added(target, changes)
	if err != nil {
		c.Reason = err.Error()
		return nil
	}
	c.Decl, c.Match = added[0].Decl, added[0].Match
	nodes, err := readNodes(ws.Dir)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(nodes, func(n *node.Node) bool { return n.ID == target })
	if i < 0 {
		return fmt.Errorf("the work tree has no node %s", target)
	}
	checked := c.Decl
	list, _ := nodes[i].Meta["formal"].([]any)
	for _, item := range list {
		if m, _ := item.(map[string]any); m["decl"] == c.Decl {
			if a, _ := m["adapter"].(string); a != "" {
				checked = a
			}
		}
	}
	env, err := newLean(ctx, ws.Dir, box)
	if err != nil {
		return err
	}
	recs, err := env.Extract(ctx, []string{checked}, true)
	if err != nil {
		return err
	}
	in := battery.Input{Nodes: nodes, Records: map[string]lean.Record{}}
	for _, r := range recs {
		in.Records[r.Decl] = r
		if r.Decl == checked {
			c.Hash = autoform.Key(r)
		}
	}
	reports, err := battery.Run(ctx, in, battery.Options{})
	if err != nil {
		return err
	}
	if k := slices.IndexFunc(reports, func(r battery.Report) bool { return r.Tag == target && r.Decl == checked }); k >= 0 {
		c.Battery = &reports[k]
	}
	var src strings.Builder
	for _, ch := range changes {
		if strings.HasPrefix(ch.Path, "lean/") && strings.HasSuffix(ch.Path, ".lean") && ch.New != nil {
			fmt.Fprintf(&src, "-- %s\n%s\n", ch.Path, ch.New)
		}
	}
	const limit = 8 << 10
	c.Lean = src.String()
	if len(c.Lean) > limit {
		c.Lean = c.Lean[:limit] + "\n-- the rest is in the work tree"
	}
	return nil
}

// commitWinner adds the provenance lines of the run to the log of the target in the work tree of the winner, and commits the changes of the run on a new branch.
func commitWinner(ctx context.Context, ws *workspace.Workspace, rec *agent.Record, r *agent.Role, target, branch, title string) error {
	changes, err := ws.Changes(ctx)
	if err != nil {
		return err
	}
	p := tools.NodePath(target)
	before := nodeAt(ws.Dir, "HEAD", p)
	after := nodeAt(ws.Dir, "", p)
	model := ""
	if ms := rec.Models(); len(ms) > 0 {
		model = ms[0]
	}
	who := prov.Line{Agent: rec.Agent, Model: model, Prompt: r.Prompt(), Run: rec.ID, At: time.Now().UTC().Format(time.RFC3339)}
	if err := prov.Append(ws.Dir, target, prov.Changes(before, after, who)...); err != nil {
		return err
	}
	paths := []string{prov.Path(target)}
	for _, c := range changes {
		paths = append(paths, c.Path)
	}
	msg := fmt.Sprintf("%s\n\nThe formalize run %s of %s wrote the binding. The report of m2 autoform has the other candidates and the battery.\n", title, rec.ID, rec.Agent)
	for _, args := range [][]string{
		{"checkout", "-q", "-b", branch},
		append([]string{"add", "-A", "--"}, paths...),
		{"commit", "-q", "-m", msg},
	} {
		if _, err := git(ws.Dir, args...); err != nil {
			return err
		}
	}
	return nil
}

// writeAutoform writes the result as text: one line for each candidate, then the winner and the report.
func writeAutoform(w io.Writer, res *autoform.Result, report string) {
	passed := 0
	for _, c := range res.Candidates {
		if c.Passes() {
			passed++
		}
	}
	fmt.Fprintf(w, "autoform %s: %d candidates, %d pass the battery, %d groups\n", res.Tag, len(res.Candidates), passed, len(res.Groups))
	for i, c := range res.Candidates {
		state := c.Outcome
		switch {
		case c.Passes():
			state = "pass"
		case c.Battery != nil:
			state = "fails the battery"
		case c.Reason != "":
			state += ": " + c.Reason
		}
		fmt.Fprintf(w, "  c%d %s %s %s %s\n", i+1, c.Run, cmp.Or(c.Model, "-"), cmp.Or(c.Decl, "-"), state)
	}
	if res.Winner >= 0 {
		fmt.Fprintf(w, "the winner is c%d, %s at F0\n", res.Winner+1, res.Candidates[res.Winner].Decl)
	} else {
		fmt.Fprintln(w, "no candidate passes the battery")
	}
	if res.Split() {
		fmt.Fprintf(w, "the candidates split into %d groups. The report shows the dissenting candidates.\n", len(res.Groups))
	}
	if len(res.Unbound) > 0 {
		fmt.Fprintf(w, "the node uses %s with no Lean binding\n", strings.Join(res.Unbound, ", "))
	}
	fmt.Fprintf(w, "the report is %s\n", report)
}
