// Package project reads the projects of a math2 tree (spec 13.6).
//
// A project is a campaign with people attached. It is the file projects/ID.yaml in the tree, where ID is the id of the project, for example fermat:
//
//	name: The Fermat numbers
//	goal: File a counterexample for each known factor of a Fermat number.
//	topic: T0PC1X
//	level: E4
//	budget: 40
//	roles: [explore, prove]
//	members:
//	  - {id: github:octocat, role: owner}
//
// The goal is the topic node and the evidence level that its members must reach, and the text says it in words. The blueprint of the project is the closure of the members of the topic, and its tasks are the frontier items inside the blueprint. The budget is the most money in US dollars that the agent runs of the project can spend in a month. A job runs for the project when it runs on behalf of project:ID, and only the agent roles of the project work for it. A person who requests a job for the project must be a member. The members of the project review the file, as they review a node.
package project

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/tag"
)

// Dir is the directory of the projects in the math2 tree.
const Dir = "projects"

// The roles of a member of a project.
const (
	Owner  = "owner"  // an owner sets the goal and the budget
	Member = "member" // a member can request agent work inside the budget
)

// A Project is one project of the tree.
type Project struct {
	ID      string       `yaml:"-"` // the name of the file without .yaml
	Name    string       `yaml:"name"`
	Goal    string       `yaml:"goal"`
	Topic   string       `yaml:"topic"`           // the tag of the topic node of the goal
	Level   status.Level `yaml:"level,omitempty"` // the evidence level that the members of the topic must reach, or none
	Budget  float64      `yaml:"budget"`          // the most US dollars that the agent runs of the project can spend in a month
	Roles   []string     `yaml:"roles"`           // the agent roles that work for the project. No role means no agent work.
	Members []Person     `yaml:"members"`
}

// A Person is a member of a project.
type Person struct {
	ID   string `yaml:"id"`   // a GitHub login or an ORCID iD, for example github:octocat
	Role string `yaml:"role"` // Owner or Member
}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// OnBehalfOf returns the id of the project in a job, project:ID.
func (p *Project) OnBehalfOf() string { return "project:" + p.ID }

// Validate reports the first problem of the project: an id that is not in the form of a project id, no name, no goal or no topic tag, a level below E1, a negative budget, a role that is not in spec 12.1, or members with no owner, a person id that is not valid, a role that is not owner or member, or a person twice.
func (p *Project) Validate() error {
	switch {
	case !idRe.MatchString(p.ID):
		return fmt.Errorf("project: %q is not a project id: use lower-case letters, digits and hyphens", p.ID)
	case strings.TrimSpace(p.Name) == "":
		return fmt.Errorf("project %s: the project has no name", p.ID)
	case strings.TrimSpace(p.Goal) == "":
		return fmt.Errorf("project %s: the project has no goal", p.ID)
	case !tag.IsTag(p.Topic):
		return fmt.Errorf("project %s: the topic %q is not a tag", p.ID, p.Topic)
	case p.Level != status.None && p.Level < status.Claimed:
		return fmt.Errorf("project %s: the level %s is not a goal: give a level from E1 to E5", p.ID, p.Level)
	case p.Budget < 0 || math.IsNaN(p.Budget) || math.IsInf(p.Budget, 0):
		return fmt.Errorf("project %s: the budget %v is not a number of dollars", p.ID, p.Budget)
	}
	for i, r := range p.Roles {
		if !slices.Contains(agent.Roles(), r) {
			return fmt.Errorf("project %s: %q is not a role of spec 12.1", p.ID, r)
		}
		if slices.Contains(p.Roles[:i], r) {
			return fmt.Errorf("project %s: the role %s is in the list twice", p.ID, r)
		}
	}
	owners := 0
	for i, m := range p.Members {
		switch {
		case !prov.IsPerson(m.ID):
			return fmt.Errorf("project %s: the member %q is not a GitHub login or an ORCID iD, such as github:octocat", p.ID, m.ID)
		case m.Role != Owner && m.Role != Member:
			return fmt.Errorf("project %s: the member %s has the role %q, and a member is an owner or a member", p.ID, m.ID, m.Role)
		case slices.ContainsFunc(p.Members[:i], func(o Person) bool { return o.ID == m.ID }):
			return fmt.Errorf("project %s: the member %s is in the list twice", p.ID, m.ID)
		}
		if m.Role == Owner {
			owners++
		}
	}
	if owners == 0 {
		return fmt.Errorf("project %s: the project has no owner", p.ID)
	}
	return nil
}

// Read reads and validates each project in root/projects, in the order of their ids. A tree with no projects gives none.
func Read(root string) ([]*Project, error) {
	dir := filepath.Join(root, Dir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	var out []*Project
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".yaml")
		if !ok || e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- a file of the projects directory of the tree.
		if err != nil {
			return nil, fmt.Errorf("project: %w", err)
		}
		p := &Project{ID: id}
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(p); err != nil {
			return nil, fmt.Errorf("project: %s/%s: %w", Dir, e.Name(), err)
		}
		if err := p.Validate(); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// Find returns the project of the id of a job, project:ID, or nil when no project has the id.
func Find(ps []*Project, onBehalfOf string) *Project {
	for _, p := range ps {
		if p.OnBehalfOf() == onBehalfOf {
			return p
		}
	}
	return nil
}

// Allows reports why the project does not allow the job, or nil when it allows it. The job must run on behalf of the project, its role must be a role of the project, and a person who requested it must be a member.
func (p *Project) Allows(job *agent.Job) error {
	switch {
	case job.OnBehalfOf != p.OnBehalfOf():
		return fmt.Errorf("project %s: the job runs on behalf of %s", p.ID, job.OnBehalfOf)
	case !slices.Contains(p.Roles, job.Role):
		return fmt.Errorf("project %s: the role %s does not work for the project. Its roles are %s", p.ID, job.Role, roles(p.Roles))
	case job.RequestedBy != "" && prov.IsPerson(job.RequestedBy) && !slices.ContainsFunc(p.Members, func(m Person) bool { return m.ID == job.RequestedBy }):
		return fmt.Errorf("project %s: %s requested the job and is not a member of the project", p.ID, job.RequestedBy)
	}
	return nil
}

func roles(rs []string) string {
	if len(rs) == 0 {
		return "none"
	}
	return strings.Join(rs, ", ")
}

// Runs returns the run records of the jobs on behalf of the project, in the order of the records. The runs of a shadow week are not runs of the project, because the system uses none of their output.
func (p *Project) Runs(recs []*agent.Record) []*agent.Record {
	var out []*agent.Record
	for _, r := range recs {
		if r.Job.OnBehalfOf == p.OnBehalfOf() && !r.Shadow {
			out = append(out, r)
		}
	}
	return out
}

// Spent returns the US dollars of the runs of the project that started in the month of t in UTC.
func (p *Project) Spent(recs []*agent.Record, t time.Time) float64 {
	t = t.UTC()
	usd := 0.0
	for _, r := range p.Runs(recs) {
		if s := r.Started.UTC(); s.Year() == t.Year() && s.Month() == t.Month() {
			usd += r.Spend.USD
		}
	}
	return usd
}
