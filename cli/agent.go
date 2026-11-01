package cli

import (
	"bytes"
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
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/align"
	"github.com/tamnd/math2-agent/agent/bump"
	"github.com/tamnd/math2-agent/agent/curate"
	"github.com/tamnd/math2-agent/agent/enumerate"
	"github.com/tamnd/math2-agent/agent/exercise"
	"github.com/tamnd/math2-agent/agent/explain"
	"github.com/tamnd/math2-agent/agent/explore"
	"github.com/tamnd/math2-agent/agent/formalize"
	"github.com/tamnd/math2-agent/agent/informalize"
	"github.com/tamnd/math2-agent/agent/prove"
	"github.com/tamnd/math2-agent/agent/reviewassist"
	"github.com/tamnd/math2-agent/agent/tools"
	"github.com/tamnd/math2-agent/agent/understand"
	"github.com/tamnd/math2-agent/agent/write"
	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/card"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/compute"
	"github.com/tamnd/math2-agent/copycheck"
	axImport "github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
	llmcmd "github.com/tamnd/math2-agent/llm/command"
	"github.com/tamnd/math2-agent/models"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/novelty"
	"github.com/tamnd/math2-agent/project"
	"github.com/tamnd/math2-agent/prompts"
	"github.com/tamnd/math2-agent/sandbox"
	"github.com/tamnd/math2-agent/search"
	"github.com/tamnd/math2-agent/workspace"
)

const agentUsage = "m2 agent run [-root DIR] [-out DIR] [-store DIR] [-work DIR] [-job FILE] [-target T] [-goal G] [-on-behalf-of ID] [-budget USD] [-rung N] [-native-decide] [-type-change] [-web] [-arxiv DIR] [-sandbox IMAGE | -no-sandbox] [-json] align|bump|curate|enumerate|exercise|explain|explore|formalize|informalize|prove|review-assist|understand|write"

// goals are the goals of a job that gives none.
var goals = map[string]string{
	"align":         "the node has a proposal of each crosswalk edge that the records give, or a Mathlib binding with the type hash of its Math2 binding",
	"bump":          "each Lean binding of the node resolves in the pinned Mathlib and is not deprecated",
	"curate":        "the record has a node in the words of math2 that states the same result, or the node has the right uses, about and msc",
	"enumerate":     "the inventory has a mention of a model list for each item of the cell that the model knows, with its sources",
	"explain":       "the node has an L2 explanation marked agent, with each claim grounded in a cited node",
	"explore":       "a conjecture or a counterexample about the node, checked by a computation that verifies, with its novelty check",
	"formalize":     "the node has a Lean binding that passes the checks of the run",
	"exercise":      "the node has an exercise with a task, an answer, a solution in steps and a check",
	"informalize":   "the declaration has a node that states it in words, in the house style, with an exact binding to it",
	"prove":         "the statement of the node has a Lean proof",
	"understand":    "each item of the paper that the pass reads has a record of the pass",
	"write":         "the node has each section, example and proof that its card asks for at the next writing level, in the house style",
	"review-assist": "the node has a review packet in reports/review with the packet of the last build and the concerns of the critic",
}

// leanRoles are the roles whose brief or goal builds the Lean library. The other roles need no Lean and no sandbox.
var leanRoles = []string{"bump", "formalize", "informalize", "prove"}

// newModel makes the client of the model service. The tests replace it with a scripted model.
var newModel = func() (llm.Model, error) {
	if s := os.Getenv(llmcmd.Env); s != "" {
		backends, err := llmcmd.Parse(s)
		if err != nil {
			return nil, err
		}
		return llmcmd.New(backends...), nil
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return nil, errors.New("set ANTHROPIC_API_KEY to the key of the Claude API, or " + llmcmd.Env + " to the clients of your subscriptions, for example codex,claude")
	}
	return claude.New(), nil
}

// CheckEnv names the client of the model of the checks of understand, a backend of llm/command with its model, for example codex:gpt-6-sol.
const CheckEnv = "M2_LLM_CHECK"

// newCheck makes the model of the checks of understand from $M2_LLM_CHECK, and gives its id. The model must have an entry in the register, of a family other than the family of the model of the pass (rule X09). With no $M2_LLM_CHECK it gives no model. The tests replace it.
var newCheck = func(pass string) (llm.Model, string, error) {
	s := os.Getenv(CheckEnv)
	if s == "" {
		return nil, "", nil
	}
	backends, err := llmcmd.Parse(s)
	if err != nil {
		return nil, "", err
	}
	if len(backends) != 1 || backends[0].Model == "" {
		return nil, "", fmt.Errorf("%s is one backend with its model, for example codex:gpt-6-sol, not %q", CheckEnv, s)
	}
	id := backends[0].Model
	if err := checkFamily(pass, id); err != nil {
		return nil, "", err
	}
	return llmcmd.New(backends...), id, nil
}

// checkFamily reports an error unless the register has both models, in two families (rule X09).
func checkFamily(pass, check string) error {
	reg := models.Default()
	for _, id := range []string{pass, check} {
		if err := reg.Check(id); err != nil {
			return err
		}
	}
	if f := reg.Family(check); f == reg.Family(pass) {
		return fmt.Errorf("the check model %s is of the family %s, as the model %s of the pass is, and rule X09 asks for another family", check, f, pass)
	}
	return nil
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
	if len(args) > 0 && args[0] == "dataset" {
		return runAgentDataset(args[1:], stdout, stderr)
	}
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintf(stderr, "Usage: %s\n       %s\n       %s\n", agentUsage, agentReportUsage, agentDatasetUsage)
		return ExitUsage
	}
	flags := flag.NewFlagSet("m2 agent run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree, a git repository")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	store := flags.String("store", "", "the directory of the run records. Empty means the output directory, where the rules P02 and P03 read the summaries.")
	work := flags.String("work", "", "the directory of the new work tree of the run. Empty means a new directory in the temporary directory.")
	jobFile := flags.String("job", "", "a job file in YAML (spec 11.4). The flags -target, -goal, -on-behalf-of, -budget, -native-decide and -type-change change its fields.")
	target := flags.String("target", "", "the tag or the provisional id of the target node")
	goal := flags.String("goal", "", "the goal of the job. Empty means the goal of the role.")
	behalf := flags.String("on-behalf-of", os.Getenv("M2_ON_BEHALF_OF"), "the person, the project or the schedule on whose behalf the run works, for example github:octocat. The default is $M2_ON_BEHALF_OF.")
	usd := flags.Float64("budget", 0, "the budget of the run in dollars. Zero means the budget of the job, or of the role.")
	rung := flags.Int("rung", 0, "the rung of the escalation ladder of the role, from 0")
	native := flags.Bool("native-decide", false, "permit a proof that uses native_decide. Rule F04 records each use.")
	typeChange := flags.Bool("type-change", false, "permit a bump rebind to a declaration with another type hash")
	web := flags.Bool("web", false, "with explore, the novelty check also searches zbMATH Open, arXiv, MathOverflow and the OEIS on the web")
	arxiv := flags.String("arxiv", os.Getenv("ARXIV_CORPUS"), "with understand, the checkout of tamnd/arxiv with the analysis plane; with write, the checkout whose content plane and fingerprints are the reference corpus of the copy check, and whose open proofs the brief gives. The default is $ARXIV_CORPUS.")
	asJSON := flags.Bool("json", false, "write the summary of the run as JSON")
	box := sandboxFlags(flags)
	if err := flags.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 1 {
		fmt.Fprintf(stderr, "m2 agent run: give one role: %s\n", strings.Join(slices.Sorted(maps.Keys(goals)), ", "))
		return ExitUsage
	}
	role := flags.Arg(0)
	if _, ok := goals[role]; !ok {
		fmt.Fprintf(stderr, "m2 agent run: the role %s is not written yet. The roles are %s.\n", role, strings.Join(slices.Sorted(maps.Keys(goals)), ", "))
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
		case "type-change":
			job.Constraints.TypeChange = *typeChange
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
	var b *tools.Box
	if slices.Contains(leanRoles, role) {
		var err error
		if b, err = box(*root); err != nil {
			return fail(err)
		}
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
	rs := &runSetup{root: *root, out: *out, store: *store, svc: svc, model: model, roles: map[string]*agent.Role{role: r}, box: b, web: *web, arxiv: *arxiv}
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
	root   string // the math2 working tree
	out    string // the output directory of m2 build. Empty means build/ in the tree.
	store  string // the directory of the run records
	svc    *api.Service
	model  llm.Model
	roles  map[string]*agent.Role
	box    *tools.Box // the sandbox of the Lean programs, or nil
	shadow bool       // the runs are shadow runs of candidate role versions
	web    bool       // the novelty check of explore searches the web
	arxiv  string     // the checkout of tamnd/arxiv that understand reads
}

// run does one run of the job on the rung in a new work tree at dir, and writes the record of the run to the store. It returns the record, the work tree and its changes. A record with an error is a run that the model service stopped.
func (rs *runSetup) run(ctx context.Context, job *agent.Job, rung int, dir string) (*agent.Record, *workspace.Workspace, []workspace.Change, error) {
	r := rs.roles[job.Role]
	if r == nil {
		return nil, nil, nil, fmt.Errorf("the role %s is not written yet", job.Role)
	}
	if id, ok := strings.CutPrefix(job.OnBehalfOf, "project:"); ok {
		ps, err := project.Read(rs.root)
		if err != nil {
			return nil, nil, nil, err
		}
		p := project.Find(ps, job.OnBehalfOf)
		if p == nil {
			return nil, nil, nil, fmt.Errorf("the job runs on behalf of %s, and the tree has no %s/%s.yaml", job.OnBehalfOf, project.Dir, id)
		}
		if err := p.Allows(job); err != nil {
			return nil, nil, nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o750); err != nil {
		return nil, nil, nil, err
	}
	ws, err := workspace.Add(ctx, rs.root, dir, "HEAD")
	if err != nil {
		return nil, nil, nil, err
	}
	var env tools.Lean
	if slices.Contains(leanRoles, job.Role) {
		if err := linkPackages(rs.root, ws.Dir, rs.box != nil); err != nil {
			return nil, ws, nil, err
		}
		if env, err = newLean(ctx, ws.Dir, rs.box); err != nil {
			return nil, ws, nil, err
		}
	}
	var (
		brief  string
		check  agent.Goal
		before agent.Goal // the goal before the first turn, for a role whose goal calls no model
		meter  *agent.Meter
		extra  []agent.Tool
		nv     *explore.Novelty
	)
	if slices.Contains([]string{"align", "curate", "enumerate", "explain", "explore", "understand"}, job.Role) && (rung < 0 || rung >= len(r.Ladder)) {
		return nil, ws, nil, fmt.Errorf("the ladder of %s has no rung %d", r.Name, rung)
	}
	switch job.Role {
	case "align":
		records := &curate.Records{Root: rs.root}
		if brief, err = align.Brief(ws, job); err != nil {
			return nil, ws, nil, err
		}
		// A tree with no build has no status, and the goal refuses each Mathlib binding.
		st, err := build.ReadStatus(cmp.Or(rs.out, filepath.Join(rs.root, "build")))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, ws, nil, err
		}
		meter = &agent.Meter{Model: rs.model}
		check = align.Goal(ws, job, records, st, meter, r.Ladder[rung].Model, r.Checks["judge"])
		extra = []agent.Tool{curate.Read(records), align.Propose(ws, records, "m2-agent/align@"+r.Version)}
	case "bump":
		// A binding can name any declaration of Mathlib, and the Math2 library imports only a part of it.
		if l, ok := env.(*tools.Lake); ok {
			l.Modules = []string{"Mathlib", "Math2"}
		}
		if brief, err = bump.Brief(ctx, ws, env, job); err != nil {
			return nil, ws, nil, err
		}
		check = bump.Goal(ws, env, job)
		extra = []agent.Tool{bump.Rebind(ws)}
	case "review-assist":
		// The packet comes from the build of the tree, and the nodes come from the work tree.
		in, err := reviewInput(ws.Dir, cmp.Or(rs.out, filepath.Join(rs.root, "build")))
		if err != nil {
			return nil, ws, nil, err
		}
		if brief, err = reviewassist.Brief(ws, in, job); err != nil {
			return nil, ws, nil, err
		}
		check = reviewassist.Goal(ws, in, job)
		extra = []agent.Tool{reviewassist.Write(ws, in, "m2-agent/review-assist@"+r.Version)}
	case "curate":
		// The records are in build/records of the tree, which git does not track, thus they are not in the work tree.
		records := &curate.Records{Root: rs.root}
		if brief, err = curate.Brief(ws, job, records); err != nil {
			return nil, ws, nil, err
		}
		meter = &agent.Meter{Model: rs.model}
		check = curate.Goal(ws, job, records, meter, r.Ladder[rung].Model, r.Checks["judge"])
		extra = []agent.Tool{curate.Read(records), curate.Propose(ws, "m2-agent/curate@"+r.Version)}
	case "enumerate":
		rows, err := inventory.ReadRows(rs.root)
		if err != nil {
			return nil, ws, nil, err
		}
		ms, err := inventory.ReadMentions(rs.root)
		if err != nil {
			return nil, ws, nil, err
		}
		cell, err := enumerate.NewCell(job.Target, rows, ms)
		if err != nil {
			return nil, ws, nil, err
		}
		model := r.Ladder[rung].Model
		family := models.Default().Family(model)
		if family == "" {
			return nil, ws, nil, models.Default().Check(model)
		}
		brief = enumerate.Brief(job, cell)
		check = enumerate.Goal(ws, job)
		extra = []agent.Tool{enumerate.List(ws, cell, family, model, "agent:m2-agent/enumerate@"+r.Version)}
	case "understand":
		if rs.arxiv == "" {
			return nil, ws, nil, errors.New("understand reads the analysis plane of tamnd/arxiv. Set -arxiv or $ARXIV_CORPUS to its checkout")
		}
		t, err := understand.ParseTarget(job.Target)
		if err != nil {
			return nil, ws, nil, err
		}
		p, err := understand.Load(rs.arxiv, t)
		if err != nil {
			return nil, ws, nil, err
		}
		if t.Pass == understand.Prose {
			text, err := proseText(ctx, rs.arxiv, p.ID, p.Version)
			if err != nil {
				return nil, ws, nil, err
			}
			if err := p.SetText(text); err != nil {
				return nil, ws, nil, err
			}
		}
		// The checks of prose and unfold need a model of a family other than the family of the pass (rule X09). With no such model in $M2_LLM_CHECK, p.Check and p.UnfoldCheck stay nil, and each prose item and each unfolding goes to its review queue.
		if t.Pass == understand.Prose || t.Pass == understand.Unfold {
			cm, id, err := newCheck(r.Ladder[rung].Model)
			if err != nil {
				return nil, ws, nil, err
			}
			if cm != nil {
				p.Check = understand.CheckWith(cm, id, r.Checks["prose"])
				p.UnfoldCheck = understand.UnfoldCheckWith(cm, id, r.Checks["unfold"])
			}
		}
		by := "agent:m2-agent/understand@" + r.Version
		brief = understand.Brief(job, p, r.Passes[t.Pass])
		check = understand.Goal(ws, job, p, by)
		before = check // the goal reads the records only, thus a paper with no item of the pass needs no model
		extra = understand.Tools(ws, p, r.Ladder[rung].Model, by)
	case "explore":
		var runner *compute.Runner
		var done func()
		if runner, nv, done, err = rs.explore(ctx, dir); err != nil {
			return nil, ws, nil, err
		}
		defer done()
		if brief, err = explore.Brief(ws, job, runner); err != nil {
			return nil, ws, nil, err
		}
		meter = &agent.Meter{Model: rs.model}
		nv.Judge = explore.Judge(meter, r.Ladder[rung].Model, r.Checks["judge"])
		check = explore.Goal(ws, job, runner, nv)
		extra = []agent.Tool{explore.Run(ws, runner), explore.Lookup(nv)}
	case "explain":
		if brief, err = explain.Brief(ws, job); err != nil {
			return nil, ws, nil, err
		}
		meter = &agent.Meter{Model: rs.model}
		check = explain.Goal(ws, job, meter, r.Ladder[rung].Model, r.Checks["ground"])
	case "write":
		cards, err := card.Load(ws.Dir)
		if err != nil {
			return nil, ws, nil, err
		}
		if brief, err = write.Brief(ws, job, cards, rs.arxiv); err != nil {
			return nil, ws, nil, err
		}
		// With -arxiv, the copy check runs in the loop against the arXiv reference corpus (corpus arxiv AX9.5). Otherwise the tree has no reference corpus yet, thus the copy check runs in CI and not in the loop.
		var corpus *copycheck.Corpus
		if rs.arxiv != "" {
			if corpus, err = arxivCorpus(rs.arxiv); err != nil {
				return nil, ws, nil, err
			}
		}
		check = write.Goal(ws, job, cards, corpus)
	case "exercise":
		if brief, err = exercise.Brief(ws, job); err != nil {
			return nil, ws, nil, err
		}
		// The solver must be a model of a family other than the family of the writer, and the model register has no such model yet. Thus each exercise goes to a person.
		check = exercise.Goal(ws, job, nil, "", r.Agent())
	case "informalize":
		// The target can be any declaration of Mathlib, and the Math2 library imports only a part of it. The docstring comes from the records in build/records of the tree.
		if l, ok := env.(*tools.Lake); ok {
			l.Modules = []string{"Mathlib", "Math2"}
		}
		d, err := informalize.Load(ctx, ws, env, &curate.Records{Root: rs.root}, job.Target)
		if err != nil {
			return nil, ws, nil, err
		}
		if brief, err = informalize.Brief(ws, job, d); err != nil {
			return nil, ws, nil, err
		}
		check = informalize.Goal(ws, job, d)
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
	ts, err := tools.Profile(r, rs.svc, ws, env, extra...)
	if err != nil {
		return nil, ws, nil, err
	}
	rn := agent.Runner{Role: r, Model: rs.model, Tools: ts, Goal: check, Before: before, Meter: meter}
	rec, runErr := rn.Run(ctx, job, rung, brief)
	if rec == nil {
		return nil, ws, nil, runErr
	}
	rec.Shadow = rs.shadow
	if nv != nil {
		rec.Novelty = nv.Results()
	}
	if _, err := agent.WriteRecord(rs.store, rec); err != nil && len(rec.Turns) > 0 {
		return rec, ws, nil, errors.Join(runErr, err)
	}
	changes, err := ws.Changes(ctx)
	if err != nil {
		return rec, ws, nil, err
	}
	return rec, ws, changes, runErr
}

// explore makes the runner of the computations of an explore run in the work tree at dir, and the searches of its novelty check. The runs of the computations go next to the work tree, because they are not changes of the run. The searches are the graph, when the build has a search index, the record plane, and the web when rs.web is set. done closes the search index.
func (rs *runSetup) explore(ctx context.Context, dir string) (*compute.Runner, *explore.Novelty, func(), error) {
	images, err := compute.ReadImages(rs.root)
	if err != nil {
		return nil, nil, nil, err
	}
	sb, err := newSandbox(ctx, "")
	if err != nil {
		return nil, nil, nil, err
	}
	runner := &compute.Runner{Images: images, Sandbox: sb, Out: dir + ".compute"}
	nv := &explore.Novelty{}
	done := func() {}
	ix, err := search.Open(filepath.Join(cmp.Or(rs.out, filepath.Join(rs.root, "build")), build.SearchFile), nil)
	switch {
	case err == nil:
		done = func() { _ = ix.Close() }
		files, err := check.ReadTree(rs.root)
		if err != nil {
			done()
			return nil, nil, nil, err
		}
		g := &novelty.Graph{Index: ix}
		for _, f := range files {
			if n, err := node.Parse(f.Path, f.Data); err == nil {
				g.Nodes = append(g.Nodes, n)
			}
		}
		nv.Searchers = append(nv.Searchers, g)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, nil, nil, err
	}
	plane, err := novelty.RecordPlane(rs.root, nil)
	if err != nil {
		done()
		return nil, nil, nil, err
	}
	nv.Searchers = append(nv.Searchers, plane...)
	if rs.web {
		w := novelty.Web{UserAgent: "m2/" + version + " (+https://github.com/tamnd/math2-agent)"}
		nv.Searchers = append(nv.Searchers, &novelty.ZbMATH{Web: w}, &novelty.ArXiv{Web: w}, &novelty.StackExchange{Web: w, Site: "mathoverflow"}, &novelty.OEIS{Web: w})
	}
	return runner, nv, done, nil
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

// arxivCorpus reads the reference corpus of the copy check from the checkout of tamnd/arxiv: the texts of the content plane, and the fingerprint of each item, which is the only trace of the text of a closed version (corpus arxiv AX9.5).
func arxivCorpus(root string) (*copycheck.Corpus, error) {
	refs, err := axImport.References(root)
	if err != nil {
		return nil, err
	}
	texts := make([]copycheck.Text, 0, len(refs))
	for _, r := range refs {
		texts = append(texts, copycheck.Text{Source: "arxiv", URI: r.URI, Licence: r.Licence, Class: ingest.Class(r.Class), Text: r.Text, MinHash: r.MinHash})
	}
	return copycheck.New(texts, nil), nil
}

// proseText runs ax prose of arxiv-reader on the version, with the checkout of tamnd/arxiv as its corpus, and reads the running text that it prints. ax prose reads the e-print that the pipeline cached and never fetches from arXiv. The binary is $AX, or ax on the path.
func proseText(ctx context.Context, corpus, id string, version int) (*understand.Text, error) {
	bin := cmp.Or(os.Getenv("AX"), "ax")
	cmd := exec.CommandContext(ctx, bin, "prose", fmt.Sprintf("%sv%d", id, version)) // #nosec G204 G702 -- the binary is the one that the operator names, and the arguments are an arXiv id
	cmd.Env = append(os.Environ(), "ARXIV_CORPUS="+corpus)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("the pass prose reads the running text with %s prose: %w: %s", bin, err, strings.TrimSpace(stderr.String()))
	}
	var t understand.Text
	if err := json.Unmarshal(out, &t); err != nil {
		return nil, fmt.Errorf("%s prose printed no running text: %w", bin, err)
	}
	return &t, nil
}
