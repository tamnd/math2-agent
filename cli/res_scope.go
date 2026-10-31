package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/queue"
	"github.com/tamnd/math2-agent/resource"
	resclassify "github.com/tamnd/math2-agent/resource/classify"
	resscope "github.com/tamnd/math2-agent/resource/scope"
	"github.com/tamnd/math2-agent/resource/venues"
)

const resScopeUsage = "m2 res scope [-root DIR] [-env FILE] [-day YYYY-MM-DD] [-fit] [-no-model]"

// ResScopeQueue is the file of the queue res-scope in the tree.
const ResScopeQueue = "build/resources/queues/res-scope.md"

func runResScope(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res scope", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	envPath := fs.String("env", "", "a JSON file with the registers of the scope tests")
	day := fs.String("day", time.Now().UTC().Format(time.DateOnly), "the date of the run")
	fit := fs.Bool("fit", false, "fit the classifier on the resources that a test decides, set its thresholds, and write "+resclassify.ModelFile)
	noModel := fs.Bool("no-model", false, "do not use the classifier")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: "+resScopeUsage)
		return ExitUsage
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "m2 res scope: %v\n", err)
		return ExitFailure
	}
	var env resscope.Env
	if *envPath != "" {
		var err error
		if env, err = readEnv(*envPath); err != nil {
			return fail(err)
		}
	}
	if err := addVenues(*root, &env); err != nil {
		return fail(err)
	}
	rs, err := resource.ReadResources(*root)
	if err != nil {
		return fail(err)
	}
	ds, err := resscope.ReadDecisions(*root)
	if err != nil {
		return fail(err)
	}
	modelPath := filepath.Join(*root, resclassify.ModelFile)
	var model *resclassify.Model
	if *fit {
		if model, err = fitScope(*root, rs, env, stdout); err != nil {
			return fail(err)
		}
		if err := model.Write(modelPath); err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "wrote %s, version %s\n", resclassify.ModelFile, model.Version())
	} else if !*noModel {
		if model, err = resclassify.Read(modelPath); err != nil {
			return fail(err)
		}
	}
	texts, err := resource.ReadTexts(*root)
	if err != nil {
		return fail(err)
	}
	in := resscope.Input{Env: env, Decisions: ds, By: "m2 " + version + " scope/1", Day: *day, Signals: curriculumSignal(texts)}
	if model != nil && !*noModel {
		in.Classifier = resclassify.Scope{Model: model}
	}
	c, items, err := resscope.Apply(rs, in)
	if err != nil {
		return fail(err)
	}
	if err := resource.WriteResources(*root, rs); err != nil {
		return fail(err)
	}
	var buf bytes.Buffer
	for _, it := range items {
		line, err := json.Marshal(queue.Item{Target: it.ID, Name: it.Reason, Goal: "the resource " + it.ID + " has a scope outcome", Priority: 1 - it.Score})
		if err != nil {
			return fail(err)
		}
		buf.Write(append(line, '\n'))
	}
	if err := writeOut(filepath.Join(*root, ResScopeQueue), buf.Bytes()); err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "in %d, border %d, out %d, deferred %d, changed %d, open %d, queue %d\n", c.ByValue[resource.In], c.ByValue[resource.Border], c.ByValue[resource.Out], c.ByValue[resource.Deferred], c.Changed, c.Open, len(items))
	return ExitOK
}

// goldLine is one record of the gold set of RS9.3: a resource and the label that a person gave.
type goldLine struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	By    string `json:"by"`
}

// fitScope fits the classifier. It sets the thresholds on the gold set when the tree has one, and else on a held-out tenth of the examples, chosen by a hash of the id.
func fitScope(root string, rs []resource.Resource, env resscope.Env, stdout io.Writer) (*resclassify.Model, error) {
	labels, classes := resclassify.Examples(rs, env, nil)
	if len(labels) == 0 {
		return nil, errors.New("no resource has a scope that a test decides, so the classifier has no examples")
	}
	gold, err := readGold(root, rs)
	if err != nil {
		return nil, err
	}
	train := labels
	source := resclassify.GoldFile
	if len(gold) == 0 {
		source = "a held-out tenth of the examples"
		train = nil
		for _, e := range labels {
			if h := sha256.Sum256([]byte(e.ID)); h[0] < 26 {
				gold = append(gold, e)
			} else {
				train = append(train, e)
			}
		}
	}
	m := resclassify.Fit(train)
	m.Classes = resclassify.Fit(classes)
	ev := m.Tune(gold)
	met := "meets"
	if !ev.Met {
		met = "does not meet"
	}
	fmt.Fprintf(stdout, "fitted on %d examples and %d classes; on %s (%d records): high %.3f, low %.3f, precision %.3f, recall %.3f, middle %d; it %s the targets of RS9.3\n", len(train), len(m.Classes.Labels), source, ev.Total, ev.High, ev.Low, ev.Precision, ev.Recall, ev.Middle, met)
	return m, nil
}

func readGold(root string, rs []resource.Resource) ([]resclassify.Example, error) {
	recs, err := mdrec.Read(filepath.Join(root, resclassify.GoldFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	byID := map[string]*resource.Resource{}
	for i := range rs {
		byID[rs[i].ID] = &rs[i]
	}
	var out []resclassify.Example
	for n, rec := range recs {
		var g goldLine
		if err := json.Unmarshal(rec, &g); err != nil || g.ID == "" || g.Label == "" {
			return nil, fmt.Errorf("%s: record %d: want an id and a label", resclassify.GoldFile, n+1)
		}
		r := byID[g.ID]
		if r == nil {
			return nil, fmt.Errorf("%s: record %d: no resource has the id %s", resclassify.GoldFile, n+1, g.ID)
		}
		out = append(out, resclassify.Example{ID: g.ID, Features: resclassify.Features(r, nil), Label: g.Label})
	}
	return out, nil
}

// curriculumSignal sets the signal of test S5 on each resource that a curriculum names (RS5.6). It gives nil when no curriculum names a text.
func curriculumSignal(texts []resource.Text) func(*resource.Resource, *resscope.Record) {
	named := map[string]bool{}
	for _, t := range texts {
		if id, _, _ := strings.Cut(t.Resource, "/"); id != "" {
			named[id] = true
		}
	}
	if len(named) == 0 {
		return nil
	}
	return func(r *resource.Resource, rec *resscope.Record) {
		if named[r.ID] {
			rec.Curriculum = true
		}
	}
}

// addVenues adds the scopes of the venue register of the tree to the environment. A venue that the environment file names keeps the scope of the file.
func addVenues(root string, env *resscope.Env) error {
	vs, err := venues.Read(root)
	if err != nil {
		return err
	}
	for id, sc := range venues.Scopes(vs) {
		if env.Venues == nil {
			env.Venues = map[string]string{}
		}
		if _, ok := env.Venues[id]; !ok {
			env.Venues[id] = sc
		}
	}
	return nil
}
