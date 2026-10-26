package site

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/project"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/status"
)

// projectView is the page of one project (spec 13.6): the goal, the blueprint, the tasks, the members, the spend of each month and the run log.
type projectView struct {
	*project.Project
	URL       string // the stable link of the page
	Page      string // the path of the page on the site
	JSONURL   string
	Topic     link
	Level     string // the evidence level of the goal, or ""
	Blueprint *dagView
	Tasks     []*taskItem
	Members   []memberItem
	Months    []monthItem // the newest month first
	Runs      []runItem   // in the order of their start
	HasStatus bool
	HasRuns   bool // the site has the run records
}

// A taskItem is a node of the blueprint that needs work.
type taskItem struct {
	*dagItem
	Why string
}

type memberItem struct {
	Who  link
	ID   string
	Role string
}

type monthItem struct {
	Month  string  `json:"month"` // YYYY-MM
	Runs   int     `json:"runs"`
	USD    float64 `json:"usd"`
	Budget float64 `json:"budget"`
	Over   bool    `json:"over"` // the runs of the month spent more than the budget
}

type runItem struct {
	ID, Date, Role, Outcome string
	Target                  link
	USD                     float64
}

// The reasons of the tasks of a project.
const (
	taskUnproved = "no proof"
	taskOpen     = "open"
	taskBelow    = "below the goal"
)

// loadProjects reads the projects of the tree and the run records of the store, and makes the page of each project.
func (b *builder) loadProjects() error {
	ps, err := project.Read(b.opt.Content)
	if err != nil {
		return fmt.Errorf("site: %w", err)
	}
	if len(ps) == 0 {
		return nil
	}
	var recs []*agent.Record
	if b.opt.Store != "" {
		if recs, err = agent.ReadRecords(b.opt.Store); err != nil {
			return fmt.Errorf("site: %w", err)
		}
	}
	f := b.frontierOf()
	for _, p := range ps {
		b.projects = append(b.projects, b.projectOf(p, f, recs))
	}
	return nil
}

func (b *builder) projectOf(p *project.Project, f frontier, recs []*agent.Record) *projectView {
	v := &projectView{
		Project: p, URL: b.abs("projects/" + p.ID + "/"), Page: b.url("projects/" + p.ID + "/"), JSONURL: b.url("api/projects/" + p.ID + ".json"),
		Topic: b.nodeLink(p.Topic), HasStatus: b.status != nil, HasRuns: b.opt.Store != "",
	}
	if p.Level != status.None {
		v.Level = p.Level.String()
	}
	if t, ok := b.nodes[p.Topic]; ok && t.Kind == "topic" {
		v.Blueprint = t.Blueprint
	}
	if v.Blueprint != nil {
		in := map[string]*dagItem{}
		for _, it := range v.Blueprint.Items {
			in[it.Tag] = it
		}
		for _, it := range f.Unproved {
			if in[it.Tag] != nil {
				v.Tasks = append(v.Tasks, &taskItem{it, taskUnproved})
			}
		}
		for _, it := range f.Open {
			if in[it.Tag] != nil {
				v.Tasks = append(v.Tasks, &taskItem{it, taskOpen})
			}
		}
		if p.Level != status.None && b.status != nil {
			for _, it := range v.Blueprint.Items {
				if s := b.statusOf(it.Tag); it.Member && (s == nil || s.Evidence < p.Level) && !slices.ContainsFunc(v.Tasks, func(t *taskItem) bool { return t.Tag == it.Tag }) {
					v.Tasks = append(v.Tasks, &taskItem{it, taskBelow})
				}
			}
		}
		slices.SortStableFunc(v.Tasks, func(x, y *taskItem) int { return cmp.Or(cmp.Compare(x.Text, y.Text), cmp.Compare(x.Tag, y.Tag)) })
	}
	for _, m := range p.Members {
		v.Members = append(v.Members, memberItem{byLink(prov.Line{Human: m.ID}), m.ID, m.Role})
	}
	months := map[string]*monthItem{}
	for _, r := range p.Runs(recs) {
		s := r.Started.UTC()
		target := link{Text: r.Job.Target}
		if r.Job.Target != "" {
			target = b.nodeLink(r.Job.Target)
		}
		v.Runs = append(v.Runs, runItem{ID: r.ID, Date: s.Format("2006-01-02"), Role: r.Job.Role, Outcome: r.Outcome, Target: target, USD: r.Spend.USD})
		key := s.Format("2006-01")
		m := months[key]
		if m == nil {
			m = &monthItem{Month: key, Budget: p.Budget}
			months[key] = m
		}
		m.Runs++
		m.USD += r.Spend.USD
		m.Over = m.USD > p.Budget
	}
	for _, m := range months {
		v.Months = append(v.Months, *m)
	}
	slices.SortFunc(v.Months, func(x, y monthItem) int { return cmp.Compare(y.Month, x.Month) })
	return v
}

// projectJSON is the agent view of a project.
type projectJSON struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Goal       string           `json:"goal"`
	Topic      string           `json:"topic"`
	Level      string           `json:"level,omitempty"`
	Budget     float64          `json:"budget"`
	Roles      []string         `json:"roles"`
	Members    []project.Person `json:"members"`
	OnBehalfOf string           `json:"on_behalf_of"`
	Blueprint  []string         `json:"blueprint"`
	Tasks      []jsonTask       `json:"tasks"`
	Months     []monthItem      `json:"months"`
	URL        string           `json:"url"`
	Source     string           `json:"source"`
}

type jsonTask struct {
	Tag string `json:"tag"`
	Why string `json:"why"`
}

func (b *builder) projectJSON(v *projectView) projectJSON {
	j := projectJSON{
		ID: v.ID, Name: v.Name, Goal: v.Goal, Topic: v.Project.Topic, Level: v.Level, Budget: v.Budget, Roles: v.Roles, Members: v.Project.Members, OnBehalfOf: v.OnBehalfOf(),
		Blueprint: []string{}, Tasks: []jsonTask{}, Months: v.Months, URL: v.URL,
		Source: "https://github.com/" + b.opt.Repo + "/blob/main/" + project.Dir + "/" + v.ID + ".yaml",
	}
	if j.Roles == nil {
		j.Roles = []string{}
	}
	if j.Months == nil {
		j.Months = []monthItem{}
	}
	if v.Blueprint != nil {
		for _, it := range v.Blueprint.Items {
			j.Blueprint = append(j.Blueprint, it.Tag)
		}
	}
	for _, t := range v.Tasks {
		j.Tasks = append(j.Tasks, jsonTask{t.Tag, t.Why})
	}
	return j
}

// writeProjects writes the list of the projects and the page and the JSON copy of each one.
func (b *builder) writeProjects() error {
	type entry struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Topic string `json:"topic"`
		URL   string `json:"url"`
		JSON  string `json:"json"`
	}
	index := []entry{}
	for _, v := range b.projects {
		if err := b.writeJSON("api/projects/"+v.ID+".json", b.projectJSON(v)); err != nil {
			return err
		}
		p := page{Title: v.Name, Description: "The project " + v.Name + " of Math 2.0: " + v.Goal, Section: "projects", JSONURL: v.JSONURL}
		if err := b.writePage("projects/"+v.ID, "project", v, p); err != nil {
			return err
		}
		index = append(index, entry{v.ID, v.Name, v.Project.Topic, v.URL, b.abs("api/projects/" + v.ID + ".json")})
	}
	if err := b.writeJSON("api/projects.json", index); err != nil {
		return err
	}
	return b.writePage("projects", "projects", b.projects, page{Title: "Projects", Description: "The projects of Math 2.0: campaigns with a goal, a budget for agent runs and the people who run them.", Section: "projects", JSONURL: b.url("api/projects.json")})
}
