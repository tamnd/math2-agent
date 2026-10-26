package prov

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/node"
)

// The two lines of spec 04.8.
const specLog = `{"field":"formal[0]","op":"add","agent":"m2-agent/formalize@0.9.1","model":"claude-opus-5-5","run":"r-2026-11-02-8812","human":null,"commit":"c4f1e9a","at":"2026-11-02T09:13:44Z"}
{"field":"formal[0].fidelity_reviews","op":"add","agent":null,"human":"orcid:0000-0002-1825-0097","commit":"e01b7d2","at":"2026-11-04T16:20:11Z"}
`

func TestParseSpec(t *testing.T) {
	lines, err := Parse(strings.NewReader(specLog + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Line{
		{Field: "formal[0]", Op: Add, Agent: "m2-agent/formalize@0.9.1", Model: "claude-opus-5-5", Run: "r-2026-11-02-8812", Commit: "c4f1e9a", At: "2026-11-02T09:13:44Z"},
		{Field: "formal[0].fidelity_reviews", Op: Add, Human: "orcid:0000-0002-1825-0097", Commit: "e01b7d2", At: "2026-11-04T16:20:11Z"},
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("got %+v", lines)
	}
	for i, l := range lines {
		if err := l.Validate(); err != nil {
			t.Errorf("line %d: %v", i+1, err)
		}
		b, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(b); got != strings.Split(specLog, "\n")[i] {
			t.Errorf("line %d is written as\n%s", i+1, got)
		}
	}
	if _, err := Parse(strings.NewReader("{}\nnot json\n")); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("a line that is not JSON gives %v", err)
	}
}

func TestValidate(t *testing.T) {
	ok := Line{Field: "name", Op: Change, Human: "github:writer", At: "2026-09-01T10:00:00Z"}
	agent := Line{Field: "name", Op: Change, Agent: "m2-agent/explain@1.2.0", Run: "r-2026-09-01-1", At: "2026-09-01T10:00:00Z"}
	for _, l := range []Line{ok, agent} {
		if err := l.Validate(); err != nil {
			t.Errorf("%+v: %v", l, err)
		}
	}
	for name, edit := range map[string]func(*Line){
		"no field":         func(l *Line) { l.Field = "" },
		"unknown op":       func(l *Line) { l.Op = "edit" },
		"no one":           func(l *Line) { l.Human = "" },
		"not a person":     func(l *Line) { l.Human = "writer" },
		"agent confirms":   func(l *Line) { l.Op, l.Human, l.Agent, l.Run = Confirm, "", agent.Agent, agent.Run },
		"agent no run":     func(l *Line) { l.Agent = agent.Agent },
		"agent bad form":   func(l *Line) { l.Agent, l.Run = "formalize", agent.Run },
		"run bad form":     func(l *Line) { l.Run = "r-1" },
		"time not RFC3339": func(l *Line) { l.At = "2026-09-01" },
	} {
		l := ok
		edit(&l)
		if l.Validate() == nil {
			t.Errorf("%s: the line is valid", name)
		}
	}
}

func TestAppendRead(t *testing.T) {
	root := t.TempDir()
	if lines, err := Read(root, "7K3QX2"); lines != nil || err != nil {
		t.Fatalf("a tag with no log gives %v, %v", lines, err)
	}
	a := Line{Field: Whole, Op: Add, Human: "github:writer", At: "2026-09-01T10:00:00Z"}
	b := Line{Field: "name", Op: Change, Human: "github:writer", At: "2026-09-02T10:00:00Z"}
	if err := Append(root, "7K3QX2", a); err != nil {
		t.Fatal(err)
	}
	if err := Append(root, "7K3QX2", b); err != nil {
		t.Fatal(err)
	}
	if err := Append(root, "7K3QX2", Line{Field: "name"}); err == nil {
		t.Error("Append wrote a line that is not valid")
	}
	got, err := Read(root, "7K3QX2")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []Line{a, b}) {
		t.Errorf("got %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "prov", "7K3", "7K3QX2.jsonl")); err != nil {
		t.Error(err)
	}
	all, err := ReadAll(root)
	if err != nil || len(all) != 1 || len(all["7K3QX2"]) != 2 {
		t.Errorf("ReadAll gives %v, %v", all, err)
	}
}

func TestCovers(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"same_as.wikidata", "same_as", true},
		{"same_as", "same_as.wikidata", true},
		{"formal", "formal[0]", true},
		{"*", "body.statement", true},
		{"same_as.oeis", "same_as.wikidata", false},
		{"name", "names", false},
		{"body.explanations.L1", "body.explanations.L2", false},
	} {
		if got := Covers(c.a, c.b); got != c.want {
			t.Errorf("Covers(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func parse(t *testing.T, text string) *node.Node {
	t.Helper()
	n, err := node.Parse("nodes/7K3/7K3QX2.md", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestChanges(t *testing.T) {
	old := parse(t, "---\ntag: 7K3QX2\nkind: statement\nname: A name\nmsc: [20D60]\n---\n\n## Statement\n\nThe text.\n\n## Explanations\n\n### L1 agent\n\nOne.\n\n### L2 human\n\nTwo.\n")
	after := parse(t, "---\ntag: 7K3QX2\nkind: statement\nname: A new name\nsame_as:\n  wikidata: Q1\n---\n\n## Statement\n\nThe text.\n\n## Explanations\n\n### L1 human\n\nOne.\n\n### L2 human\n\nTwo, longer.\n")
	who := Line{Human: "github:writer", At: "2026-09-01T10:00:00Z"}
	var got []string
	for _, l := range Changes(old, after, who) {
		if l.Human != who.Human || l.At != who.At {
			t.Errorf("%s does not take the person and the time", l.Field)
		}
		got = append(got, l.Op+" "+l.Field)
	}
	want := []string{"change body.explanations.L1", "change body.explanations.L2", "remove msc", "change name", "add same_as"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	if c := Changes(nil, after, who); len(c) != 1 || c[0].Field != Whole || c[0].Op != Add {
		t.Errorf("a new node gives %+v", c)
	}
	if c := Changes(old, nil, who); len(c) != 1 || c[0].Op != Remove {
		t.Errorf("a node that is gone gives %+v", c)
	}
	if c := Changes(old, old, who); len(c) != 0 {
		t.Errorf("no change gives %+v", c)
	}
}

func TestFields(t *testing.T) {
	n := parse(t, "---\ntag: 7K3QX2\nkind: statement\n---\n\nText before.\n\n## Statement\n\nThe text.\n\n## Explanations\n\nAn opening.\n\n### L3 agent\n\nThree.\n")
	f := Fields(n)
	for k, v := range map[string]string{"body": "Text before.", "body.statement": "The text.", "body.explanations": "An opening.", "body.explanations.L3": "### L3 agent\n\nThree."} {
		if f[k] != v {
			t.Errorf("%s is %q", k, f[k])
		}
	}
}

const (
	explain = "m2-agent/explain@1.2.0"
	person  = "orcid:0000-0002-1825-0097"
)

func TestUnconfirmedAndQuarantined(t *testing.T) {
	log := []Line{
		{Field: Whole, Op: Add, Agent: explain, Run: "r-2026-09-01-1"},
		{Field: "body.explanations.L1", Op: Add, Agent: explain, Run: "r-2026-09-01-1"},
		{Field: "body.explanations.L2", Op: Add, Agent: explain, Run: "r-2026-09-01-1"},
		{Field: "body.explanations.L1", Op: Quarantine, Human: person},
		{Field: "body.explanations.L2", Op: Quarantine, Human: person},
		{Field: "body.explanations.L2", Op: Confirm, Human: person},
	}
	u := Unconfirmed(log)
	if _, ok := u["body.explanations.L1"]; !ok || len(u) != 2 {
		t.Errorf("Unconfirmed gives %v", u)
	}
	q := Quarantined(log)
	if _, ok := q["body.explanations.L1"]; !ok || len(q) != 1 {
		t.Errorf("Quarantined gives %v", q)
	}
	if l, ok := Author(log, "body.explanations.L1"); !ok || l.Agent != explain {
		t.Errorf("the author of L1 is %+v", l)
	}
	if l, _ := Author(log, "body.explanations.L2"); l.Human != person {
		t.Errorf("the author of L2 is %+v", l)
	}
	if l, _ := Author(log, "name"); l.Field != Whole {
		t.Errorf("the author of name is %+v", l)
	}
}

func TestProduced(t *testing.T) {
	logs := map[string][]Line{
		"7K3QX2": {
			{Field: "body.explanations.L1", Op: Add, Agent: explain, Model: "claude-opus-5-5", Prompt: "explain/7", Run: "r-2026-09-01-1"},
			{Field: "body.explanations.L2", Op: Add, Agent: explain, Run: "r-2026-09-01-2"},
			{Field: "body.explanations.L3", Op: Add, Agent: explain, Run: "r-2026-09-01-2"},
			{Field: "body.explanations.L3", Op: Change, Human: person},
		},
		"3C53TD": {{Field: "body.statement", Op: Add, Agent: explain, Model: "claude-sonnet-5-5", Prompt: "explain/7", Run: "r-2026-09-01-3"}},
	}
	runs := map[string]*Run{"r-2026-09-01-2": {Models: []string{"claude-opus-5-5"}, Prompt: "explain/7"}}
	var got []string
	for _, h := range Produced(logs, runs, "claude-opus-5-5", "explain/7") {
		got = append(got, h.Tag+" "+h.Line.Field)
	}
	if want := []string{"7K3QX2 body.explanations.L1", "7K3QX2 body.explanations.L2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	if h := Produced(logs, nil, "claude-opus-5-5", "explain/8"); len(h) != 0 {
		t.Errorf("another prompt gives %v", h)
	}
}

func TestCredit(t *testing.T) {
	logs := map[string][]Line{
		"7K3QX2": {
			{Field: Whole, Op: Add, Human: person, At: "2026-01-05T10:00:00Z"},
			{Field: "formal[0]", Op: Add, Agent: "m2-agent/formalize@0.9.1", Run: "r-2026-02-01-1", At: "2026-02-01T10:00:00Z"},
			{Field: "same_as.wikidata", Op: Add, Human: person, At: "2026-03-01T10:00:00Z"},
			{Field: "body.statement", Op: Change, Human: "github:other", At: "2026-03-02T10:00:00Z"},
			{Field: "formal[0].fidelity_reviews", Op: Add, Human: person, At: "2027-01-02T10:00:00Z"},
		},
	}
	runs := map[string]*Run{"r-2026-02-01-1": {OnBehalfOf: person}}
	from, to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	var got []string
	for _, c := range Credit(logs, runs, person, from, to) {
		got = append(got, c.Role+" "+c.Field+" "+c.Run)
	}
	want := []string{"Data curation same_as.wikidata ", "Formal analysis formal[0] r-2026-02-01-1", OriginalDraft + " * "}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	if c := Credit(logs, nil, person, time.Time{}, time.Time{}); len(c) != 3 {
		t.Errorf("with no runs and no bounds the person has %d contributions, and must have 3", len(c))
	}
}

func TestRuns(t *testing.T) {
	dir := t.TempDir()
	r := &Run{ID: "r-2026-09-01-1", Agent: explain, Prompt: "explain/7", Models: []string{"claude-opus-5-5"}, OnBehalfOf: "schedule:nightly", Started: "2026-09-01T09:00:00Z", Outcome: "success"}
	if err := WriteRun(dir, r); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRun(dir, r.ID)
	if err != nil || !reflect.DeepEqual(got, r) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := ReadRun(dir, "r-2026-09-01-2"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a run that is not there gives %v", err)
	}
	if _, err := ReadRun(dir, "../x"); err == nil {
		t.Error("ReadRun read a path outside the directory")
	}
	all, err := ReadRuns(dir)
	if err != nil || len(all) != 1 {
		t.Errorf("ReadRuns gives %v, %v", all, err)
	}
	if _, err := ReadRuns(filepath.Join(dir, "none")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a directory that is not there gives %v", err)
	}
	bad := *r
	bad.OnBehalfOf = explain
	if WriteRun(dir, &bad) == nil {
		t.Error("WriteRun wrote a run that acted on behalf of an agent")
	}
}

func TestHide(t *testing.T) {
	n := parse(t, "---\ntag: 7K3QX2\nkind: statement\n---\n\nText before.\n\n## Statement\n\nThe text.\n\n## Explanations\n\nAn opening.\n\n### L1 agent\n\nOne.\n\n### L2 agent\n\nTwo.\n")
	hide := func(fields ...string) string {
		return Hide(n, func(f string) bool { return slices.Contains(fields, f) }, "Hidden.")
	}
	if got := hide(); got != n.Body {
		t.Errorf("no hidden field gives %q", got)
	}
	want := "\nText before.\n\n## Statement\n\nThe text.\n\n## Explanations\n\nAn opening.\n\n### L1 agent\n\nHidden.\n\n### L2 agent\n\nTwo.\n"
	if got := hide("body.explanations.L1"); got != want {
		t.Errorf("hide L1 gives\n%q\nwant\n%q", got, want)
	}
	want = "\nHidden.\n\n## Statement\n\nHidden.\n\n## Explanations\n\nAn opening.\n\n### L1 agent\n\nOne.\n\n### L2 agent\n\nHidden.\n"
	if got := hide("body", "body.statement", "body.explanations.L2"); got != want {
		t.Errorf("hide three fields gives\n%q\nwant\n%q", got, want)
	}
}
