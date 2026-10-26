// Package orchestrator schedules the jobs of the agent roles with budgets and the escalation ladder (spec 11.4 and 11.5).
//
// An Orchestrator keeps one queue for each role. Add puts a job in the queue of its role. A job with the target and the goal of a job that is in the queue or that runs merges into that job. Drain starts runs with the Dispatch function until no job can start, and it waits for each run that it started.
//
// The money limits are in a Config, the file orchestrator.yaml that people review and commit each month. Each role has a monthly budget, and each project or schedule can have a monthly cap. A run gets the least of the money that is left to its job, to its role in the month and to its project or schedule in the month. While a run goes, the orchestrator reserves that money, thus two runs at the same time cannot use the same dollars. The run itself stops before it goes over its budget (see [agent.Runner.Run]). When the money of a role or a project is used up, its jobs wait for the next month.
//
// The queues share fairly: the orchestrator takes the roles in turn, and in a role it takes first the project or schedule that spent the least in the month. In the queue of a project, the job with the highest priority goes first.
//
// A job starts on the cheapest rung of the ladder of its role that has a recorded success. The first version counts each target of the role as a similar target. When a run of the job fails, the next run of the job is on the next rung. When an attempt on the same target, goal and rung failed in the retry time, the orchestrator skips that rung. A job ends when a run reaches the goal, when the model reports that it is stuck, when the ladder or the budget of the job is used up, or when a run cannot start.
//
// Each job names the person, the project or the schedule on whose behalf it runs (rule P03), and Add refuses a job that does not.
//
// A major bump of a role version needs a shadow week (spec 12.8). Shadow gives the orchestrator the candidate version of a role. After each run of a job of the role, the candidate runs the same job on the same rung, and the system uses only the output of the current version. The weekly agent report compares the two.
//
// The orchestrator knows the past attempts from the run records in the artefact store. ReadAttempts reads them. Thus a new orchestrator after a restart counts the same spend and the same failures.
package orchestrator

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
)

// DefaultRetry is the retry time of a Config that gives none.
const DefaultRetry = 7 * 24 * time.Hour

// minRun is the least budget in dollars with which a run starts. A run with less money cannot do a useful turn.
const minRun = 0.05

// Config is the configuration of the orchestrator, the file orchestrator.yaml (spec 11.5).
type Config struct {
	Roles      map[string]float64 `yaml:"roles"`                  // the monthly budget of each role in US dollars. A role that is not in the map gets no run.
	OnBehalfOf map[string]float64 `yaml:"on_behalf_of,omitempty"` // the monthly cap of a project or a schedule in US dollars. An id that is not in the map has only the budget of the role as its cap.
	Retry      agent.Duration     `yaml:"retry,omitempty"`        // the time in which the orchestrator does not try a target, goal and rung again after a failed attempt. Zero means DefaultRetry.
	Workers    int                `yaml:"workers,omitempty"`      // the most runs at the same time. Zero means 1.
	Shadow     map[string]string  `yaml:"shadow,omitempty"`       // the directory of the candidate version of a role in a shadow week, by role. The files of the role are in <dir>/<role>/, as in prompts/.
}

// ReadConfig reads the configuration from the YAML file at the path.
func ReadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the caller names the configuration file.
	if err != nil {
		return nil, err
	}
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// Cap sets the monthly cap of the project or the schedule id to usd, unless the configuration gives it a lower cap. The budget of a project file (spec 13.6) is such a cap: the owner of the project sets it, and orchestrator.yaml can give a lower one.
func (c *Config) Cap(id string, usd float64) {
	if old, ok := c.OnBehalfOf[id]; ok && old <= usd {
		return
	}
	if c.OnBehalfOf == nil {
		c.OnBehalfOf = map[string]float64{}
	}
	c.OnBehalfOf[id] = usd
}

// Validate reports the first problem of the configuration: a role that is not in spec 12.1, an id that is not a project or a schedule, a shadow week of a role with no budget or with no directory, or a budget, a retry time or a number of workers that is negative.
func (c *Config) Validate() error {
	for _, r := range slices.Sorted(maps.Keys(c.Roles)) {
		if !slices.Contains(agent.Roles(), r) {
			return fmt.Errorf("orchestrator: %s is not a role", r)
		}
		if c.Roles[r] < 0 || math.IsNaN(c.Roles[r]) {
			return fmt.Errorf("orchestrator: the budget of %s is negative", r)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(c.OnBehalfOf)) {
		if !strings.HasPrefix(id, "project:") && !strings.HasPrefix(id, "schedule:") {
			return fmt.Errorf("orchestrator: %s is not a project or a schedule", id)
		}
		if c.OnBehalfOf[id] < 0 || math.IsNaN(c.OnBehalfOf[id]) {
			return fmt.Errorf("orchestrator: the cap of %s is negative", id)
		}
	}
	for _, r := range slices.Sorted(maps.Keys(c.Shadow)) {
		if _, ok := c.Roles[r]; !ok {
			return fmt.Errorf("orchestrator: the shadow week of %s needs a budget of the role", r)
		}
		if c.Shadow[r] == "" {
			return fmt.Errorf("orchestrator: the shadow week of %s names no directory", r)
		}
	}
	if c.Retry < 0 || c.Workers < 0 {
		return errors.New("orchestrator: the retry time and the number of workers cannot be negative")
	}
	return nil
}

// Attempt is one run of a job, as the orchestrator counts it.
type Attempt struct {
	Run        string     `json:"run"`
	Job        string     `json:"job"`
	Role       string     `json:"role"`
	Target     string     `json:"target"`
	Goal       string     `json:"goal"`
	OnBehalfOf string     `json:"on_behalf_of"`
	Rung       agent.Rung `json:"rung"`
	Started    time.Time  `json:"started"`
	Outcome    string     `json:"outcome"`
	Reason     string     `json:"reason"`
	USD        float64    `json:"usd"`              // the spend of the run
	Budget     float64    `json:"budget"`           // the budget in dollars that the run had
	Shadow     bool       `json:"shadow,omitempty"` // the run is a shadow run of a candidate role version
}

// AttemptOf returns the attempt of the record of a run.
func AttemptOf(r *agent.Record) Attempt {
	return Attempt{
		Run: r.ID, Job: r.Job.ID, Role: r.Job.Role, Target: r.Job.Target, Goal: r.Job.Goal, OnBehalfOf: r.Job.OnBehalfOf,
		Rung: r.Rung, Started: r.Started, Outcome: r.Outcome, Reason: r.Reason, USD: r.Spend.USD, Budget: r.Job.Budget.USD,
		Shadow: r.Shadow,
	}
}

// ReadAttempts reads the attempt of each run record in store/records, in the order of their start.
func ReadAttempts(store string) ([]Attempt, error) {
	recs, err := agent.ReadRecords(store)
	if err != nil {
		return nil, err
	}
	out := make([]Attempt, 0, len(recs))
	for _, r := range recs {
		out = append(out, AttemptOf(r))
	}
	return out, nil
}

// Dispatch runs a job on a rung of the ladder of its role and returns the record of the run. The orchestrator sets the budget in dollars of the job to the budget of the run. An error with no record means that the run did not start.
type Dispatch func(ctx context.Context, job *agent.Job, rung int) (*agent.Record, error)

// Done is a job that ended.
type Done struct {
	Job     agent.Job `json:"job"`
	Outcome string    `json:"outcome"` // success or failure
	Reason  string    `json:"reason"`
}

// Waiting is a job that cannot start now.
type Waiting struct {
	Job agent.Job `json:"job"`
	Why string    `json:"why"`
}

// Report is what one Drain did.
type Report struct {
	Runs    []Attempt `json:"runs"`          // the runs that Drain started, in the order of their end
	Done    []Done    `json:"done"`          // the jobs that ended
	Waiting []Waiting `json:"waiting"`       // the jobs that are in the queues at the end
	Shadows []string  `json:"shadow_errors"` // the shadow runs that did not run or that the model service stopped, with the reason
}

// Over returns the runs that spent more than their budget. It is empty when the budgets hold.
func (r *Report) Over() []Attempt {
	var out []Attempt
	for _, a := range r.Runs {
		if a.Budget > 0 && a.USD > a.Budget+1e-9 {
			out = append(out, a)
		}
	}
	return out
}

// entry is a job in a queue.
type entry struct {
	job     agent.Job
	seq     int
	running bool
}

// plan is the next run of a job.
type plan struct {
	e    *entry
	rung int
	usd  float64
}

// shadowRun is a shadow run that waits or runs.
type shadowRun struct {
	job      agent.Job
	rung     int
	usd      float64
	dispatch Dispatch
}

// Orchestrator is the scheduler of agent jobs. Make one with New.
type Orchestrator struct {
	config   *Config
	roles    map[string]*agent.Role
	dispatch Dispatch
	now      func() time.Time

	mu       sync.Mutex
	attempts []Attempt
	queues   map[string][]*entry // the queue of each role
	reserved map[string]float64  // the money that each running job holds, by job id
	seq      int
	turn     int // the role whose turn it is, as an index in the sorted roles

	shadows map[string]Dispatch // the dispatch of the candidate version of each role in a shadow week
	pending []*shadowRun        // the shadow runs that wait, in order
	held    map[string]float64  // the money that the running shadow runs of each role hold
}

// New returns an orchestrator with the configuration, the roles that it can run, the dispatch function and the past attempts. A nil now means time.Now.
func New(config *Config, roles []*agent.Role, dispatch Dispatch, attempts []Attempt, now func() time.Time) *Orchestrator {
	o := &Orchestrator{
		config: config, roles: map[string]*agent.Role{}, dispatch: dispatch, now: now,
		attempts: slices.Clone(attempts), queues: map[string][]*entry{}, reserved: map[string]float64{},
		shadows: map[string]Dispatch{}, held: map[string]float64{},
	}
	if now == nil {
		o.now = time.Now
	}
	for _, r := range roles {
		o.roles[r.Name] = r
	}
	return o
}

// Shadow runs the candidate version of the role in a shadow week (spec 12.8). After each run of a job of the role, the orchestrator runs the job again with dispatch, on the same rung and with the same budget, when the money of the role in the month allows it. The record that dispatch returns is of the candidate version, with Shadow set, and the system uses none of its output. The spend of a shadow run counts in the budget of the role, and not in the budget of the job or in the cap of its project. A shadow run does not move the ladder and does not end a job.
func (o *Orchestrator) Shadow(role string, dispatch Dispatch) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.shadows[role] = dispatch
}

// Add puts the job in the queue of its role and returns the id of the job that does the work. That is the id of the job, or the id of a job with the same role, target and goal that is in the queue or that runs. Add refuses a job that is not valid, for example a job that names no person, project or schedule (rule P03), and a job of a role that the orchestrator cannot run.
func (o *Orchestrator) Add(job *agent.Job) (string, error) {
	if err := job.Validate(); err != nil {
		return "", err
	}
	if o.roles[job.Role] == nil {
		return "", fmt.Errorf("orchestrator: the orchestrator cannot run the role %s", job.Role)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, e := range o.queues[job.Role] {
		if e.job.ID == job.ID {
			return "", fmt.Errorf("orchestrator: the job %s is in the queue", job.ID)
		}
		if e.job.Target == job.Target && e.job.Goal == job.Goal {
			return e.job.ID, nil
		}
	}
	o.seq++
	o.queues[job.Role] = append(o.queues[job.Role], &entry{job: *job, seq: o.seq})
	return job.ID, nil
}

// Drain starts runs until no job in the queues can start, waits for each run, and reports what it did. A job that waits stays in its queue. When the context ends, Drain starts no new run.
func (o *Orchestrator) Drain(ctx context.Context) *Report {
	rep := &Report{Runs: []Attempt{}, Done: []Done{}, Waiting: []Waiting{}, Shadows: []string{}}
	type result struct {
		p      plan
		job    agent.Job
		shadow *shadowRun
		rec    *agent.Record
		err    error
	}
	results := make(chan result)
	workers := max(o.config.Workers, 1)
	running := 0
	for {
		for running < workers && ctx.Err() == nil {
			if s := o.nextShadow(rep); s != nil {
				running++
				go func() {
					rec, err := s.dispatch(ctx, &s.job, s.rung)
					results <- result{shadow: s, rec: rec, err: err}
				}()
				continue
			}
			p, ok := o.next(rep)
			if !ok {
				break
			}
			job := p.e.job
			job.Budget.USD = p.usd
			running++
			go func() {
				rec, err := o.dispatch(ctx, &job, p.rung)
				results <- result{p: p, job: job, rec: rec, err: err}
			}()
		}
		if running == 0 {
			break
		}
		r := <-results
		running--
		if r.shadow != nil {
			o.finishShadow(rep, r.shadow, r.rec, r.err)
		} else {
			o.finish(rep, r.p, r.job, r.rec, r.err)
		}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, role := range slices.Sorted(maps.Keys(o.queues)) {
		for _, e := range o.queues[role] {
			_, why, _ := o.plan(e)
			if why == "" {
				why = "the context ended"
			}
			rep.Waiting = append(rep.Waiting, Waiting{Job: e.job, Why: why})
		}
	}
	return rep
}

// Queued returns the jobs in the queues, by role and then in the order of Add.
func (o *Orchestrator) Queued() []agent.Job {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []agent.Job
	for _, role := range slices.Sorted(maps.Keys(o.queues)) {
		for _, e := range o.queues[role] {
			out = append(out, e.job)
		}
	}
	return out
}

// next selects the next run and reserves its money. It removes each job that ends on the way and reports it.
func (o *Orchestrator) next(rep *Report) (plan, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	roles := slices.Sorted(maps.Keys(o.roles))
	for i := range roles {
		role := roles[(o.turn+i)%len(roles)]
		for _, e := range o.order(role) {
			p, why, end := o.plan(e)
			if end != "" {
				o.remove(e)
				rep.Done = append(rep.Done, Done{Job: e.job, Outcome: agent.Failure, Reason: end})
				continue
			}
			if why != "" {
				continue
			}
			o.turn = (o.turn + i + 1) % len(roles)
			e.running = true
			o.reserved[e.job.ID] = p.usd
			return p, true
		}
	}
	return plan{}, false
}

// nextShadow takes the next shadow run that the money of its role allows and holds its money. It reports each shadow run that cannot run.
func (o *Orchestrator) nextShadow(rep *Report) *shadowRun {
	o.mu.Lock()
	defer o.mu.Unlock()
	for len(o.pending) > 0 {
		s := o.pending[0]
		o.pending = o.pending[1:]
		usd := min(s.usd, o.config.Roles[s.job.Role]-o.spentRole(s.job.Role))
		if usd < minRun {
			rep.Shadows = append(rep.Shadows, fmt.Sprintf("the shadow run of the job %s on rung %d: the money of %s for the month is used up", s.job.ID, s.rung, s.job.Role))
			continue
		}
		s.usd, s.job.Budget.USD = usd, usd
		o.held[s.job.Role] += usd
		return s
	}
	return nil
}

// order returns the jobs of the queue of the role that do not run, in the order in which they can start: the project or the schedule that spent the least in the month first, then the highest priority, then the order of Add.
func (o *Orchestrator) order(role string) []*entry {
	var out []*entry
	spent := map[string]float64{}
	for _, e := range o.queues[role] {
		if !e.running {
			out = append(out, e)
			spent[e.job.OnBehalfOf] = o.spentBy(e.job.OnBehalfOf)
		}
	}
	slices.SortStableFunc(out, func(a, b *entry) int {
		return cmp.Or(
			cmp.Compare(spent[a.job.OnBehalfOf], spent[b.job.OnBehalfOf]),
			strings.Compare(a.job.OnBehalfOf, b.job.OnBehalfOf),
			-cmp.Compare(a.job.Priority, b.job.Priority),
			cmp.Compare(a.seq, b.seq),
		)
	})
	return out
}

// plan returns the next run of the job. When the job cannot start now, why says why. When the job ends, end says why.
func (o *Orchestrator) plan(e *entry) (p plan, why, end string) {
	job := &e.job
	role := o.roles[job.Role]
	if e.running {
		return plan{}, "it runs", ""
	}
	monthly, ok := o.config.Roles[job.Role]
	if !ok {
		return plan{}, fmt.Sprintf("orchestrator.yaml gives no budget to the role %s", job.Role), ""
	}
	total := cmp.Or(job.Budget.USD, role.Budget.USD)
	if total <= 0 {
		return plan{}, "", "the job and its role give no budget in dollars"
	}
	var mine []Attempt
	for _, a := range o.attempts {
		if a.Job == job.ID && !a.Shadow {
			mine = append(mine, a)
		}
	}
	left := total
	for _, a := range mine {
		left -= a.USD
	}
	if left < minRun {
		return plan{}, "", fmt.Sprintf("the budget of $%.2f of the job is used up", total)
	}
	rung, wait := o.rung(job, role, mine)
	if rung < 0 {
		if wait != "" {
			return plan{}, wait, ""
		}
		return plan{}, "", "each rung of the ladder that the job allows failed"
	}
	usd := min(left, monthly-o.spentRole(job.Role))
	if c, ok := o.config.OnBehalfOf[job.OnBehalfOf]; ok {
		usd = min(usd, c-o.spentBy(job.OnBehalfOf))
	}
	if usd < minRun {
		return plan{}, fmt.Sprintf("the money of %s or of %s for the month is used up", job.Role, job.OnBehalfOf), ""
	}
	return plan{e: e, rung: rung, usd: usd}, "", ""
}

// rung returns the next rung of the ladder for the job, or -1. With -1, wait is not empty when a rung waits for the retry time.
func (o *Orchestrator) rung(job *agent.Job, role *agent.Role, mine []Attempt) (int, string) {
	var allowed []int
	for i, g := range role.Ladder {
		if len(job.Constraints.Models) == 0 || slices.Contains(job.Constraints.Models, g.Model) {
			allowed = append(allowed, i)
		}
	}
	if len(allowed) == 0 {
		return -1, ""
	}
	// The job starts at the cheapest allowed rung with a recorded success of the role.
	start := allowed[0]
	for _, i := range allowed {
		if slices.ContainsFunc(o.attempts, func(a Attempt) bool {
			return a.Role == job.Role && !a.Shadow && a.Outcome == agent.Success && a.Rung == role.Ladder[i]
		}) {
			start = i
			break
		}
	}
	retry := cmp.Or(time.Duration(o.config.Retry), DefaultRetry)
	now := o.now()
	wait := ""
	for _, i := range allowed {
		if i < start || slices.ContainsFunc(mine, func(a Attempt) bool { return a.Rung == role.Ladder[i] }) {
			continue
		}
		recent := slices.IndexFunc(o.attempts, func(a Attempt) bool {
			return a.Role == job.Role && !a.Shadow && a.Target == job.Target && a.Goal == job.Goal && a.Rung == role.Ladder[i] && a.Outcome != agent.Success && now.Sub(a.Started) < retry
		})
		if recent >= 0 {
			if wait == "" {
				a := o.attempts[recent]
				wait = fmt.Sprintf("the run %s with %s failed on %s, and the retry time is %s", a.Run, a.Rung.Model, a.Started.UTC().Format(time.DateOnly), retry)
			}
			continue
		}
		return i, ""
	}
	return -1, wait
}

// spentRole returns the spend of the role in the month of now, with the shadow runs and with the money that its running jobs and shadow runs hold.
func (o *Orchestrator) spentRole(role string) float64 {
	return o.held[role] + o.spent(func(a *Attempt) bool { return a.Role == role }, func(e *entry) bool { return e.job.Role == role })
}

// spentBy returns the spend of the project or the schedule in the month of now, with the money that its running jobs hold. The shadow runs do not count.
func (o *Orchestrator) spentBy(id string) float64 {
	return o.spent(func(a *Attempt) bool { return a.OnBehalfOf == id && !a.Shadow }, func(e *entry) bool { return e.job.OnBehalfOf == id })
}

func (o *Orchestrator) spent(attempt func(*Attempt) bool, job func(*entry) bool) float64 {
	now := o.now().UTC()
	usd := 0.0
	for i := range o.attempts {
		a := &o.attempts[i]
		s := a.Started.UTC()
		if s.Year() == now.Year() && s.Month() == now.Month() && attempt(a) {
			usd += a.USD
		}
	}
	for _, q := range o.queues {
		for _, e := range q {
			if e.running && job(e) {
				usd += o.reserved[e.job.ID]
			}
		}
	}
	return usd
}

// finish records the end of a run and ends its job when the run reached the goal, when the model reported that it is stuck, or when the run did not start.
func (o *Orchestrator) finish(rep *Report, p plan, job agent.Job, rec *agent.Record, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	p.e.running = false
	delete(o.reserved, job.ID)
	if rec != nil {
		a := AttemptOf(rec)
		a.Budget = p.usd
		o.attempts = append(o.attempts, a)
		rep.Runs = append(rep.Runs, a)
		if d := o.shadows[job.Role]; d != nil {
			o.pending = append(o.pending, &shadowRun{job: job, rung: p.rung, usd: p.usd, dispatch: d})
		}
	}
	done := func(outcome, reason string) {
		o.remove(p.e)
		rep.Done = append(rep.Done, Done{Job: p.e.job, Outcome: outcome, Reason: reason})
	}
	switch {
	case err != nil:
		done(agent.Failure, "the run failed: "+err.Error())
	case rec.Outcome == agent.Success:
		done(agent.Success, rec.Reason)
	case strings.HasPrefix(rec.Reason, "stuck: "):
		done(agent.Failure, rec.Reason)
	}
}

// finishShadow records the end of a shadow run.
func (o *Orchestrator) finishShadow(rep *Report, s *shadowRun, rec *agent.Record, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.held[s.job.Role] -= s.usd
	if rec != nil {
		a := AttemptOf(rec)
		a.Budget, a.Shadow = s.usd, true
		o.attempts = append(o.attempts, a)
		rep.Runs = append(rep.Runs, a)
	}
	if err != nil {
		rep.Shadows = append(rep.Shadows, fmt.Sprintf("the shadow run of the job %s on rung %d: %v", s.job.ID, s.rung, err))
	}
}

// remove takes the entry out of its queue.
func (o *Orchestrator) remove(e *entry) {
	q := o.queues[e.job.Role]
	o.queues[e.job.Role] = slices.DeleteFunc(q, func(x *entry) bool { return x == e })
	if len(o.queues[e.job.Role]) == 0 {
		delete(o.queues, e.job.Role)
	}
}
