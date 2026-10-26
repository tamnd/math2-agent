package cli

import (
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
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/formalize"
	"github.com/tamnd/math2-agent/agent/prove"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/sandbox"
	"github.com/tamnd/math2-agent/workspace"
)

const agentUsage = "m2 agent run [-root DIR] [-out DIR] [-store DIR] [-work DIR] [-job FILE] [-target T] [-goal G] [-on-behalf-of ID] [-budget USD] [-rung N] [-native-decide] [-sandbox IMAGE | -no-sandbox] [-json] formalize|prove"

// goals are the goals of a job that gives none.
var goals = map[string]string{
	"formalize": "the node has a Lean binding that passes the checks of the run",
	"prove":     "the statement of the node has a Lean proof",
}

// newModel makes the client of the model service. The tests replace it with a scripted model.
var newModel = func() (llm.Model, error) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return nil, errors.New("set ANTHROPIC_API_KEY to the key of the Claude API")
	}
	return claude.New(), nil
}

// newLean makes the Lean environment of a work tree, in the sandbox box when it is not nil. The tests replace it.
var newLean = func(ctx context.Context, tree string, box *tools.Box) (tools.Lean, error) {
	if box != nil {
		return &tools.Lake{Tree: tree, Exe: boxExtract, Box: box}, nil
	}
	exe, err := lean.Install(ctx, filepath.Join(tree, "lean"), "")
	if err != nil {
		return nil, err
	}
	return &tools.Lake{Tree: tree, Exe: exe}, nil
}

func runAgent(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "report" {
		return runAgentReport(args[1:], stdout, stderr)
	}
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintf(stderr, "Usage: %s\n       %s\n", agentUsage, agentReportUsage)
		return ExitUsage
	}
	flags := flag.NewFlagSet("m2 agent run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree, a git repository")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	store := flags.String("store", "", "the directory of the run records. Empty means the output directory, where the rules P02 and P03 read the summaries.")
	work := flags.String("work", "", "the directory of the new work tree of the run. Empty means a new directory in the temporary directory.")
	jobFile := flags.String("job", "", "a job file in YAML (spec 11.4). The flags -target, -goal, -on-behalf-of, -budget and -native-decide change its fields.")
	target := flags.String("target", "", "the tag or the provisional id of the target node")
	goal := flags.String("goal", "", "the goal of the job. Empty means the goal of the role.")
	behalf := flags.String("on-behalf-of", os.Getenv("M2_ON_BEHALF_OF"), "the person, the project or the schedule on whose behalf the run works, for example github:octocat. The default is $M2_ON_BEHALF_OF.")
	usd := flags.Float64("budget", 0, "the budget of the run in dollars. Zero means the budget of the job, or of the role.")
	rung := flags.Int("rung", 0, "the rung of the escalation ladder of the role, from 0")
	native := flags.Bool("native-decide", false, "permit a proof that uses native_decide. Rule F04 records each use.")
	asJSON := flags.Bool("json", false, "write the summary of the run as JSON")
	box := sandboxFlags(flags)
	if err := flags.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "m2 agent run: give one role: formalize or prove")
		return ExitUsage
	}
	role := flags.Arg(0)
	if _, ok := goals[role]; !ok {
		fmt.Fprintf(stderr, "m2 agent run: the role %s is not written yet. The roles are formalize and prove.\n", role)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 agent run: %v\n", err)
		return ExitFailure
	}

	job := &agent.Job{}
	if *jobFile != "" {
		data, err := os.ReadFile(*jobFile)
		if err != nil {
			return fail(err)
		}
		if err := yaml.Unmarshal(data, job); err != nil {
			return fail(fmt.Errorf("%s: %w", *jobFile, err))
		}
		if job.Role != "" && job.Role != role {
			return fail(fmt.Errorf("the job %s is for the role %s", job.ID, job.Role))
		}
	}
	job.Role = role
	if job.ID == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		job.ID = "j-" + time.Now().UTC().Format("2006-01-02") + "-" + hex.EncodeToString(b)
	}
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "target":
			job.Target = *target
		case "goal":
			job.Goal = *goal
		case "on-behalf-of":
			job.OnBehalfOf = *behalf
		case "budget":
			job.Budget.USD = *usd
		case "native-decide":
			job.Constraints.NativeDecide = *native
		}
	})
	if job.OnBehalfOf == "" {
		job.OnBehalfOf = *behalf
	}
	if job.Goal == "" {
		job.Goal = goals[role]
	}
	if err := job.Validate(); err != nil {
		return fail(err)
	}
	b, err := box(*root)
	if err != nil {
		return fail(err)
	}
	r, err := agent.LoadRole(prompts.FS, role)
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

	if *work == "" {
		*work = filepath.Join(os.TempDir(), "m2-agent", job.ID)
	}
	rs := &runSetup{root: *root, store: *store, svc: svc, model: model, roles: map[string]*agent.Role{role: r}, box: b}
	rec, ws, changes, err := rs.run(context.Background(), job, *rung, *work)
	if ws != nil {
		defer func() { _ = ws.Close() }()
	}
	if err != nil {
		return fail(err)
	}
	if *asJSON {
		files := []string{}
		for _, c := range changes {
			files = append(files, c.Path)
		}
		data, _ := json.MarshalIndent(map[string]any{
			"run": rec.ID, "job": job.ID, "agent": rec.Agent, "outcome": rec.Outcome, "reason": rec.Reason,
			"models": rec.Models(), "usd": rec.Spend.USD, "turns": len(rec.Turns), "work_tree": ws.Dir, "changes": files,
		}, "", "  ")
		fmt.Fprintf(stdout, "%s\n", data)
	} else {
		fmt.Fprintf(stdout, "run %s of job %s: %s, %s\n", rec.ID, job.ID, rec.Outcome, rec.Reason)
		fmt.Fprintf(stdout, "%s, %d turns, $%.4f, models %s\n", rec.Agent, len(rec.Turns), rec.Spend.USD, strings.Join(rec.Models(), ", "))
		fmt.Fprintf(stdout, "the work tree is %s, with %d changed files\n", ws.Dir, len(changes))
		for _, c := range changes {
			fmt.Fprintf(stdout, "  %s\n", c.Path)
		}
		fmt.Fprintf(stdout, "Remove the work tree with: git -C %s worktree remove --force %s\n", *root, ws.Dir)
	}
	if rec.Outcome != agent.Success {
		return ExitFailure
	}
	return ExitOK
}

// runSetup is what the runs of m2 agent run and m2 orchestrate share.
type runSetup struct {
	root  string // the math2 working tree
	store string // the directory of the run records
	svc   *api.Service
	model llm.Model
	roles map[string]*agent.Role
	box   *tools.Box // the sandbox of the Lean programs, or nil
}

// run does one run of the job on the rung in a new work tree at dir, and writes the record of the run to the store. It returns the record, the work tree and its changes. A record with an error is a run that the model service stopped.
func (rs *runSetup) run(ctx context.Context, job *agent.Job, rung int, dir string) (*agent.Record, *workspace.Workspace, []workspace.Change, error) {
	r := rs.roles[job.Role]
	if r == nil {
		return nil, nil, nil, fmt.Errorf("the role %s is not written yet", job.Role)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o750); err != nil {
		return nil, nil, nil, err
	}
	ws, err := workspace.Add(ctx, rs.root, dir, "HEAD")
	if err != nil {
		return nil, nil, nil, err
	}
	if err := linkPackages(rs.root, ws.Dir, rs.box != nil); err != nil {
		return nil, ws, nil, err
	}
	env, err := newLean(ctx, ws.Dir, rs.box)
	if err != nil {
		return nil, ws, nil, err
	}
	var (
		brief string
		check agent.Goal
	)
	switch job.Role {
	case "prove":
		pin, err := prove.NewPin(ctx, ws, env, job)
		if err != nil {
			return nil, ws, nil, err
		}
		if brief, err = prove.Brief(ws, job, pin); err != nil {
			return nil, ws, nil, err
		}
		check = prove.Goal(ws, env, job, pin)
	case "formalize":
		if brief, err = formalize.Brief(ws, job); err != nil {
			return nil, ws, nil, err
		}
		check = formalize.Goal(ws, env, job)
	}
	ts, err := tools.Profile(r, rs.svc, ws, env)
	if err != nil {
		return nil, ws, nil, err
	}
	rn := agent.Runner{Role: r, Model: rs.model, Tools: ts, Goal: check}
	rec, runErr := rn.Run(ctx, job, rung, brief)
	if rec == nil {
		return nil, ws, nil, runErr
	}
	if _, err := agent.WriteRecord(rs.store, rec); err != nil && len(rec.Turns) > 0 {
		return rec, ws, nil, err
	}
	changes, err := ws.Changes(ctx)
	if err != nil {
		return rec, ws, nil, err
	}
	return rec, ws, changes, runErr
}

// linkPackages links lean/.lake/packages of the tree into the new work tree. Thus the work tree uses the Mathlib build of the tree, and the run does not get Mathlib again. For a run in the sandbox, the link goes to the read-only mount of the packages in the container, and each directory of lean/ in the work tree becomes writable for the user of the container.
func linkPackages(root, tree string, boxed bool) error {
	src, err := filepath.Abs(filepath.Join(root, "lean", ".lake", "packages"))
	if err != nil {
		return err
	}
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) && !boxed {
		return nil
	}
	dir := filepath.Join(tree, "lean", ".lake")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if !boxed {
		return os.Symlink(src, filepath.Join(dir, "packages"))
	}
	if err := os.Symlink(sandbox.WorkDir, filepath.Join(dir, "packages")); err != nil {
		return err
	}
	r, err := os.OpenRoot(filepath.Join(tree, "lean"))
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	return fs.WalkDir(r.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		return r.Chmod(p, 0o777) // #nosec G302 -- the work tree is new, and the container runs as the user 65534.
	})
}

// boxExtract is the path of M2Extract in the image of the Lean sandbox.
const boxExtract = "/usr/local/bin/m2extract"

// The limits of a container of the Lean sandbox. Mathlib needs much memory.
const (
	boxCPU    = 4
	boxMemory = 8 << 30
)

// sandboxFlags adds the flags -sandbox and -no-sandbox to the flag set. After the parse, the function that it returns gives the sandbox of the Lean programs for the tree root, or nil for -no-sandbox. One of the two flags must be set, because the Lean files that a model writes can run programs when Lean elaborates them.
func sandboxFlags(flags *flag.FlagSet) func(root string) (*tools.Box, error) {
	image := flags.String("sandbox", os.Getenv("M2_LEAN_IMAGE"), "the image of the Lean sandbox by digest, for example ghcr.io/tamnd/m2-lean@sha256:... The default is $M2_LEAN_IMAGE.")
	none := flags.Bool("no-sandbox", false, "run lake, Lean and M2Extract on the host. Use it only for a run that you watch.")
	return func(root string) (*tools.Box, error) {
		switch {
		case *none:
			return nil, nil
		case *image == "":
			return nil, errors.New("give the image of the Lean sandbox with -sandbox, or run on the host with -no-sandbox")
		case !sandbox.ValidImage(*image):
			return nil, fmt.Errorf("the image %q has no digest", *image)
		}
		packages, err := filepath.Abs(filepath.Join(root, "lean", ".lake", "packages"))
		if err != nil {
			return nil, err
		}
		if packages, err = filepath.EvalSymlinks(packages); err != nil {
			return nil, fmt.Errorf("the sandbox needs the Lake packages of the tree: %w", err)
		}
		return &tools.Box{Docker: &sandbox.Docker{}, Image: *image, Packages: packages, CPU: boxCPU, Memory: boxMemory}, nil
	}
}
