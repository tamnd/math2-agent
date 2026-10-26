package audit

import (
	"slices"

	"github.com/tamnd/math2-agent/models"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
)

// The rules X08 and X09 check the models of the agents against the model register of corpus K7.7 (package models).

// modelRegister gives the model register. A test can give another register.
var modelRegister = models.Default

// judges is the sanity check of corpus K9.3 whose verdicts come from judge models.
const judges = "judges"

// x08 checks that each line of a provenance log that an agent wrote names a model with an entry in the model register (rule X08, corpus K7.7). A line of an agent adds or changes a field and names the agent role. A person on the line does not change who wrote the value.
func x08(a *auditor) outcome {
	var o outcome
	reg := modelRegister()
	for _, n := range a.scopeNodes() {
		path := prov.Path(n.ID)
		data, _ := a.readFile(path)
		for _, l := range readLog(data) {
			if l.err != nil || l.line.Agent == "" || l.line.Op != prov.Add && l.line.Op != prov.Change {
				continue
			}
			o.checked++
			switch {
			case l.line.Model == "":
				o.add(path, l.n, 1, "the agent %s wrote %s of %s, and the line names no model", l.line.Agent, l.line.Field, n.ID)
			case reg.Lookup(l.line.Model) == nil:
				o.add(path, l.n, 1, "the agent %s wrote %s of %s with the model %s, which has no entry in the model register %s (corpus K7.7)", l.line.Agent, l.line.Field, n.ID, l.line.Model, models.File)
			}
		}
	}
	return o
}

// x09 checks that each judge verdict on an agent-written statement or proof is from a model of a family other than the family of the writer (rule X09, corpus K7.10). The verdicts are the results of the sanity check judges of corpus K9.3 in the record of the run that wrote the field. A result n/a has no verdict. When the register has no entry for the writer, rule X08 reports it.
func x09(a *auditor) outcome {
	var o outcome
	ws, reason := a.agentRuns(func(n *node.Node) []string {
		return slices.DeleteFunc(bodyFields(n), func(f string) bool { return f != "body.statement" && f != "body.proof" })
	})
	if reason != "" {
		o.reason = reason
		return o
	}
	reg := modelRegister()
	for _, w := range ws {
		writer := w.w.line.line.Model
		family := reg.Family(writer)
		if family == "" {
			continue
		}
		line, col := fieldPos(w.n, w.w.field)
		run := w.w.line.line.Run
		for _, s := range w.run.Sanity {
			if s.Node != w.n.ID || s.Field != w.w.field || s.Check != judges || s.Result == prov.SanityNA {
				continue
			}
			o.checked++
			switch {
			case s.Model == "":
				o.add(w.n.Path, line, col, "a judge verdict on %s of %s in the run %s names no model", w.w.field, w.n.ID, run)
			case reg.Lookup(s.Model) == nil:
				o.add(w.n.Path, line, col, "a judge verdict on %s of %s in the run %s is from the model %s, which has no entry in the model register %s", w.w.field, w.n.ID, run, s.Model, models.File)
			case reg.Family(s.Model) == family:
				o.add(w.n.Path, line, col, "a judge verdict on %s of %s in the run %s is from %s, and the writer %s is of the same family %s. Give the judge a model of another family (corpus K7.6)", w.w.field, w.n.ID, run, s.Model, writer, family)
			}
		}
	}
	return o
}
