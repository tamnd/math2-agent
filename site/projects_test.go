package site

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/build"
)

const finiteGroups = `name: Finite groups
goal: Prove each member of the topic Finite groups to E2.
topic: ZZZJE6
level: E2
budget: 5
roles: [prove]
members:
  - {id: github:octocat, role: owner}
  - {id: orcid:0000-0002-1825-0097, role: member}
publications:
  - {title: Notes on finite groups, doi: 10.1234/example, year: 2027}
`

// writeRecord writes a run record to the store.
func writeRecord(t *testing.T, store string, r agent.Record) {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, store, agent.RecordsDir+"/"+r.ID+".json", string(data))
}

func TestProjects(t *testing.T) {
	content := t.TempDir()
	if err := os.CopyFS(content, os.DirFS(fixtures)); err != nil {
		t.Fatal(err)
	}
	writeFile(t, content, "projects/finite-groups.yaml", finiteGroups)
	writeFile(t, content, "projects/nowhere.yaml", "name: Nowhere\ngoal: A project with a topic that is not in the graph.\ntopic: ZZZZZZ\nbudget: 0\nmembers:\n  - {id: github:octocat, role: owner}\n")
	store := t.TempDir()
	job := agent.Job{ID: "j-1", Role: "prove", Target: "Q529ES", OnBehalfOf: "project:finite-groups"}
	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	writeRecord(t, store, agent.Record{ID: "r-2026-09-30-0000000a", Job: job, Started: day("2026-09-30"), Outcome: "failed", Spend: agent.Spend{USD: 1.5}})
	writeRecord(t, store, agent.Record{ID: "r-2026-10-02-0000000b", Job: job, Started: day("2026-10-02"), Outcome: "merged", Spend: agent.Spend{USD: 3}})
	writeRecord(t, store, agent.Record{ID: "r-2026-10-03-0000000c", Job: job, Started: day("2026-10-03"), Outcome: "failed", Spend: agent.Spend{USD: 2.25}})
	writeRecord(t, store, agent.Record{ID: "r-2026-10-04-0000000d", Job: job, Started: day("2026-10-04"), Outcome: "failed", Spend: agent.Spend{USD: 9}, Shadow: true})
	other := job
	other.OnBehalfOf = "project:other"
	writeRecord(t, store, agent.Record{ID: "r-2026-10-05-0000000e", Job: other, Started: day("2026-10-05"), Outcome: "failed", Spend: agent.Spend{USD: 7}})

	opt := Options{Content: content, Out: filepath.Join(t.TempDir(), "site"), URL: "https://example.org/math2", Repo: "o/math2", AgentRepo: "o/math2-agent", Version: "test", Build: t.TempDir(), Store: store}
	if _, err := build.Run(context.Background(), build.Options{Root: content, Out: opt.Build}); err != nil {
		t.Fatal(err)
	}
	if err := Build(opt); err != nil {
		t.Fatal(err)
	}
	out := opt.Out

	list := readFile(t, out, "projects/index.html")
	for _, want := range []string{`href="/math2/projects/finite-groups/"`, `href="/math2/projects/nowhere/"`, "$5.00"} {
		if !strings.Contains(list, want) {
			t.Errorf("the list of projects has no %s", want)
		}
	}
	p := readFile(t, out, "projects/finite-groups/index.html")
	for _, want := range []string{
		"Prove each member of the topic Finite groups to E2.",
		`<h2 id="blueprint-h">Blueprint</h2>`,
		`class="box ev-e1 member"`,
		"below the goal",
		`<a href="https://github.com/octocat">octocat</a>`,
		"orcid:0000-0002-1825-0097, member",
		"<code>on_behalf_of: project:finite-groups</code>",
		"<tr><td>2026-10</td><td>2</td><td>$5.25 <span class=\"muted\">over the budget</span></td><td>$5.00</td></tr>",
		"<tr><td>2026-09</td><td>1</td><td>$1.50</td><td>$5.00</td></tr>",
		"<code>r-2026-10-02-0000000b</code>",
		`href="/math2/node/Q529ES/"`,
		`<a href="https://doi.org/10.1234/example">Notes on finite groups</a> (2027)`,
		`Cite this page as <a href="https://example.org/math2/projects/finite-groups/">`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("the page of the project has no %s", want)
		}
	}
	for _, not := range []string{"r-2026-10-04-0000000d", "r-2026-10-05-0000000e"} {
		if strings.Contains(p, not) {
			t.Errorf("the page of the project lists the run %s, a shadow run or a run of another project", not)
		}
	}
	if !strings.Contains(readFile(t, out, "projects/nowhere/index.html"), "The graph has no topic ZZZZZZ") {
		t.Error("the page of a project with no topic in the graph does not say so")
	}
	if !strings.Contains(readFile(t, out, "sitemap.xml"), "projects/finite-groups/") {
		t.Error("the sitemap has no page of a project")
	}

	var j projectJSON
	if err := json.Unmarshal([]byte(readFile(t, out, "api/projects/finite-groups.json")), &j); err != nil {
		t.Fatal(err)
	}
	if j.OnBehalfOf != "project:finite-groups" || j.Level != "E2" || len(j.Blueprint) == 0 || len(j.Tasks) == 0 || len(j.Months) != 2 || len(j.Publications) != 1 || j.Members[0].ID != "github:octocat" || j.Months[0].Month != "2026-10" || !j.Months[0].Over {
		t.Errorf("the JSON copy is %+v", j)
	}
	var index []struct{ ID string }
	if err := json.Unmarshal([]byte(readFile(t, out, "api/projects.json")), &index); err != nil || len(index) != 2 || index[0].ID != "finite-groups" {
		t.Errorf("api/projects.json is %+v, %v", index, err)
	}
}

func TestProjectsNone(t *testing.T) {
	out := buildSite(t, newContent(t), "")
	if !strings.Contains(readFile(t, out, "projects/index.html"), "has no projects yet") {
		t.Error("the list of projects of a tree with no projects does not say so")
	}
	if got := strings.TrimSpace(readFile(t, out, "api/projects.json")); got != "[]" {
		t.Errorf("api/projects.json is %s", got)
	}
}

func TestProjectsBad(t *testing.T) {
	content := newContent(t)
	writeFile(t, content, "projects/bad.yaml", "name: Bad\ngoal: No owner.\ntopic: ZZZJE6\nbudget: 1\nmembers:\n  - {id: github:octocat, role: member}\n")
	err := Build(Options{Content: content, Out: filepath.Join(t.TempDir(), "site"), Version: "test"})
	if err == nil || !strings.Contains(err.Error(), "has no owner") {
		t.Errorf("a project with no owner gives %v", err)
	}
}
