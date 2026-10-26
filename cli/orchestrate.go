package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/orchestrator"
	"github.com/tamnd/math2-agent/prompts"
)

const orchestrateUsage = "m2 orchestrate [-root DIR] [-out DIR] [-store DIR] [-config FILE] [-queue DIR] [-work DIR] [-once] [-interval D] [-json]"

func runOrchestrate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("m2 orchestrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "the math2 working tree, a git repository")
	out := flags.String("out", "", "the output directory of m2 build. Empty means build/ in the tree.")
	store := flags.String("store", "", "the directory of the run records. Empty means the output directory.")
	configFile := flags.String("config", "", "the configuration with the budgets. Empty means orchestrator.yaml in the tree.")
	queue := flags.String("queue", "", "the directory of the job files in YAML. Empty means queue/ in the store. A job that ends goes to done/ in this directory.")
	work := flags.String("work", "", "the directory of the work trees of the runs. Empty means a new directory in the temporary directory.")
	once := flags.Bool("once", false, "run the jobs in the queue until no job can start, then stop")
	interval := flags.Duration("interval", 5*time.Minute, "the time between two reads of the queue")
	asJSON := flags.Bool("json", false, "write the report of each pass as JSON")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "Usage: %s\n", orchestrateUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 orchestrate: %v\n", err)
		return ExitFailure
	}
	if *out == "" {
		*out = filepath.Join(*root, "build")
	}
	if *store == "" {
		*store = *out
	}
	if *configFile == "" {
		*configFile = filepath.Join(*root, "orchestrator.yaml")
	}
	if *queue == "" {
		*queue = filepath.Join(*store, "queue")
	}
	if *work == "" {
		*work = filepath.Join(os.TempDir(), "m2-agent")
	}
	config, err := orchestrator.ReadConfig(*configFile)
	if err != nil {
		return fail(err)
	}
	var roles []*agent.Role
	rs := &runSetup{root: *root, store: *store, roles: map[string]*agent.Role{}}
	for _, name := range slices.Sorted(maps.Keys(goals)) {
		r, err := agent.LoadRole(prompts.FS, name)
		if err != nil {
			return fail(err)
		}
		roles = append(roles, r)
		rs.roles[name] = r
	}
	if rs.model, err = newModel(); err != nil {
		return fail(err)
	}
	rs.svc, err = api.Open(api.Config{Root: *root, Out: *out})
	if errors.Is(err, fs.ErrNotExist) {
		return fail(fmt.Errorf("%w. Run m2 build first", err))
	}
	if err != nil {
		return fail(err)
	}
	defer func() { _ = rs.svc.Close() }()
	attempts, err := orchestrator.ReadAttempts(*store)
	if err != nil {
		return fail(err)
	}
	dispatch := func(ctx context.Context, job *agent.Job, rung int) (*agent.Record, error) {
		dir := filepath.Join(*work, fmt.Sprintf("%s-%d", job.ID, rung))
		rec, ws, _, err := rs.run(ctx, job, rung, dir)
		if ws != nil {
			_ = ws.Close()
			if rec == nil || rec.Outcome != agent.Success {
				removeWorkTree(ctx, *root, ws.Dir)
			}
		}
		return rec, err
	}
	o := orchestrator.New(config, roles, dispatch, attempts, nil)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	files := map[string]string{} // the job file of each job id in the queues
	code := ExitOK
	for {
		if err := readQueue(o, *queue, files, stdout); err != nil {
			return fail(err)
		}
		rep := o.Drain(ctx)
		for _, d := range rep.Done {
			if err := moveDone(*queue, files, d); err != nil {
				return fail(err)
			}
		}
		if len(rep.Over()) > 0 {
			code = ExitFailure
		}
		writeOrchestration(stdout, rep, *asJSON)
		if *once || ctx.Err() != nil {
			return code
		}
		select {
		case <-ctx.Done():
			return code
		case <-time.After(*interval):
		}
	}
}

// readQueue adds each job file of the queue directory that is not in files to the orchestrator. A job with the target and the goal of a job in the queues goes to done/ as merged.
func readQueue(o *orchestrator.Orchestrator, queue string, files map[string]string, stdout io.Writer) error {
	paths, err := filepath.Glob(filepath.Join(queue, "*.yaml"))
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, f := range files {
		known[f] = true
	}
	for _, p := range paths {
		if known[p] {
			continue
		}
		data, err := os.ReadFile(p) // #nosec G304 -- the file is a job file in the queue directory.
		if err != nil {
			return err
		}
		job := &agent.Job{}
		if err := yaml.Unmarshal(data, job); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		id, err := o.Add(job)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		files[job.ID] = p
		if id != job.ID {
			fmt.Fprintf(stdout, "the job %s merges into %s\n", job.ID, id)
			if err := moveDone(queue, files, orchestrator.Done{Job: *job, Outcome: "merged", Reason: "the job " + id + " has the same target and goal"}); err != nil {
				return err
			}
		}
	}
	return nil
}

// moveDone writes the end of the job to done/<id>.json in the queue directory and removes the job file.
func moveDone(queue string, files map[string]string, d orchestrator.Done) error {
	dir := filepath.Join(queue, "done")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, d.Job.ID+".json"), append(data, '\n'), 0o600); err != nil {
		return err
	}
	if f, ok := files[d.Job.ID]; ok {
		delete(files, d.Job.ID)
		return os.Remove(f)
	}
	return nil
}

func writeOrchestration(w io.Writer, rep *orchestrator.Report, asJSON bool) {
	if asJSON {
		data, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Fprintf(w, "%s\n", data)
		return
	}
	for _, a := range rep.Runs {
		fmt.Fprintf(w, "run %s of job %s on %s: %s, $%.4f of $%.2f, %s\n", a.Run, a.Job, a.Rung.Model, a.Outcome, a.USD, a.Budget, a.Reason)
	}
	for _, d := range rep.Done {
		fmt.Fprintf(w, "job %s ends: %s, %s\n", d.Job.ID, d.Outcome, d.Reason)
	}
	for _, x := range rep.Waiting {
		fmt.Fprintf(w, "job %s waits: %s\n", x.Job.ID, x.Why)
	}
	for _, a := range rep.Over() {
		fmt.Fprintf(w, "the run %s spent $%.4f, more than its budget of $%.2f\n", a.Run, a.USD, a.Budget)
	}
	if len(rep.Runs)+len(rep.Done)+len(rep.Waiting) == 0 {
		fmt.Fprintln(w, "the queues are empty")
	}
}

// removeWorkTree removes the work tree of a run that did not reach its goal. The run record keeps what the run did.
func removeWorkTree(ctx context.Context, root, dir string) {
	_ = exec.CommandContext(ctx, "git", "-C", root, "worktree", "remove", "--force", dir).Run() // #nosec G204 -- the orchestrator made the directory, and no shell reads the arguments.
}
