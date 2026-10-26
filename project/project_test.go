package project_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/project"
	"github.com/tamnd/math2-agent/status"
)

const fermat = "name: The Fermat numbers\ngoal: File a counterexample for each known factor of a Fermat number.\ntopic: T0PC1X\nlevel: E4\nbudget: 40\nroles: [explore, prove]\nmembers:\n  - {id: github:octocat, role: owner}\n  - {id: orcid:0000-0002-1825-0097, role: member}\n"

func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, text := range files {
		p := filepath.Join(root, project.Dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRead(t *testing.T) {
	ps, err := project.Read(t.TempDir())
	if err != nil || ps != nil {
		t.Errorf("a tree with no projects gives %v, %v", ps, err)
	}
	ps, err = project.Read(tree(t, map[string]string{"fermat.yaml": fermat, "README.md": "Not a project.\n", "aaa.yaml": strings.Replace(fermat, "level: E4\n", "", 1)}))
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || ps[0].ID != "aaa" || ps[1].ID != "fermat" {
		t.Fatalf("Read gives %d projects", len(ps))
	}
	p := ps[1]
	if p.Name != "The Fermat numbers" || p.Topic != "T0PC1X" || p.Level != status.Certified || p.Budget != 40 || len(p.Roles) != 2 || len(p.Members) != 2 || p.OnBehalfOf() != "project:fermat" {
		t.Errorf("the project is %+v", p)
	}
	if p.Publications != nil {
		t.Errorf("a project with no publications has %v", p.Publications)
	}
	if ps[0].Level != status.None {
		t.Errorf("a project with no level has the level %v", ps[0].Level)
	}

	for _, tc := range []struct{ name, file, text, want string }{
		{"an id", "Fermat.yaml", fermat, "is not a project id"},
		{"no name", "fermat.yaml", strings.Replace(fermat, "name: The Fermat numbers\n", "", 1), "has no name"},
		{"no goal", "fermat.yaml", strings.Replace(fermat, "goal: ", "goal: \"\"\nx: ", 1), "field x not found"},
		{"a topic", "fermat.yaml", strings.Replace(fermat, "T0PC1X", "fermat", 1), "is not a tag"},
		{"a level", "fermat.yaml", strings.Replace(fermat, "E4", "E9", 1), "not an evidence level"},
		{"a low level", "fermat.yaml", strings.Replace(fermat, "E4", "E0", 1), "give a level from E1 to E5"},
		{"a budget", "fermat.yaml", strings.Replace(fermat, "40", "-1", 1), "is not a number of dollars"},
		{"a role", "fermat.yaml", strings.Replace(fermat, "prove]", "harvest]", 1), `"harvest" is not a role`},
		{"a role twice", "fermat.yaml", strings.Replace(fermat, "prove]", "explore]", 1), "is in the list twice"},
		{"a person", "fermat.yaml", strings.Replace(fermat, "github:octocat", "octocat", 1), "is not a GitHub login or an ORCID iD"},
		{"a member role", "fermat.yaml", strings.Replace(fermat, "role: member", "role: chair", 1), `has the role "chair"`},
		{"a member twice", "fermat.yaml", strings.Replace(fermat, "orcid:0000-0002-1825-0097", "github:octocat", 1), "is in the list twice"},
		{"a publication title", "fermat.yaml", fermat + "publications:\n  - {doi: 10.1234/x}\n", "has no title"},
		{"a publication link", "fermat.yaml", fermat + "publications:\n  - {title: A paper}\n", "has no DOI and no URL"},
		{"a DOI", "fermat.yaml", fermat + "publications:\n  - {title: A paper, doi: 'https://doi.org/10.1234/x'}\n", "is not a DOI"},
		{"a URL", "fermat.yaml", fermat + "publications:\n  - {title: A paper, url: 'http://example.org/a'}\n", "does not start with https://"},
		{"no owner", "fermat.yaml", strings.Replace(fermat, "role: owner", "role: member", 1), "has no owner"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := project.Read(tree(t, map[string]string{tc.file: tc.text})); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Read gives %v, and it must say %q", err, tc.want)
			}
		})
	}
}

func TestPublications(t *testing.T) {
	ps, err := project.Read(tree(t, map[string]string{"fermat.yaml": fermat + "publications:\n  - {title: A paper, doi: 10.1234/x.y, year: 2027}\n  - {title: A talk, url: 'https://example.org/talk'}\n"}))
	if err != nil {
		t.Fatal(err)
	}
	w := ps[0].Publications
	if len(w) != 2 || w[0].Link() != "https://doi.org/10.1234/x.y" || w[0].Year != 2027 || w[1].Link() != "https://example.org/talk" {
		t.Errorf("the publications are %+v", w)
	}
}

func TestAllows(t *testing.T) {
	ps, err := project.Read(tree(t, map[string]string{"fermat.yaml": fermat}))
	if err != nil {
		t.Fatal(err)
	}
	p := project.Find(ps, "project:fermat")
	if p == nil || project.Find(ps, "project:other") != nil || project.Find(ps, "fermat") != nil {
		t.Fatal("Find does not find the project by its id in a job")
	}
	for _, tc := range []struct {
		job  agent.Job
		want string
	}{
		{agent.Job{Role: "explore", OnBehalfOf: "project:fermat"}, ""},
		{agent.Job{Role: "prove", OnBehalfOf: "project:fermat", RequestedBy: "github:octocat"}, ""},
		{agent.Job{Role: "prove", OnBehalfOf: "project:other"}, "runs on behalf of project:other"},
		{agent.Job{Role: "explain", OnBehalfOf: "project:fermat"}, "the role explain does not work for the project. Its roles are explore, prove"},
		{agent.Job{Role: "explore", OnBehalfOf: "project:fermat", RequestedBy: "github:someone"}, "github:someone requested the job and is not a member"},
	} {
		err := p.Allows(&tc.job)
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("Allows(%+v) gives %v, and it must say %q", tc.job, err, tc.want)
		}
	}
}

func TestSpent(t *testing.T) {
	p := &project.Project{ID: "fermat"}
	rec := func(id, who string, at time.Time, usd float64, shadow bool) *agent.Record {
		return &agent.Record{ID: id, Job: agent.Job{OnBehalfOf: who}, Started: at, Spend: agent.Spend{USD: usd}, Shadow: shadow}
	}
	oct := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	recs := []*agent.Record{
		rec("r-1", "project:fermat", oct, 1.5, false),
		rec("r-2", "project:fermat", oct.AddDate(0, 0, 20), 2, false),
		rec("r-3", "project:fermat", oct.AddDate(0, -1, 0), 4, false),
		rec("r-4", "project:fermat", oct, 8, true),
		rec("r-5", "project:other", oct, 16, false),
	}
	if runs := p.Runs(recs); len(runs) != 3 || runs[0].ID != "r-1" || runs[2].ID != "r-3" {
		t.Errorf("Runs gives %d runs", len(runs))
	}
	if usd := p.Spent(recs, oct.AddDate(0, 0, 10)); usd != 3.5 {
		t.Errorf("the spend of October is $%v", usd)
	}
}
