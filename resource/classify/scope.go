package classify

import (
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/scope"
)

// Scope is the classifier of RS9.3 for scope.Apply.
type Scope struct {
	Model *Model
	Cited func(id string) []string // the MSC codes of the works that a resource cites. Nil means none
}

// Classify gives the outcome, the score of in and the MSC2 class of a resource.
func (s Scope) Classify(r *resource.Resource) (string, float64, string) {
	f := Features(r, s.cited(r.ID))
	v, score := s.Model.Decide(f)
	class := ""
	if v == resource.In {
		class, _ = s.Model.Class(f)
	}
	return v, score, class
}

// Version gives the version of the model.
func (s Scope) Version() string { return s.Model.Version() }

func (s Scope) cited(id string) []string {
	if s.Cited == nil {
		return nil
	}
	return s.Cited(id)
}

// Examples makes the training examples of RS9.3 from the live resources that a test decided: in by S1 or S3, border by a border rule, and out. It also gives the examples of the class model: the works in scope with an MSC code, labelled with the first two digits of the primary code.
func Examples(rs []resource.Resource, env scope.Env, cited func(string) []string) (labels, classes []Example) {
	for i := range rs {
		r := &rs[i]
		if !r.Live() || r.Scope != nil && r.Scope.Rule == scope.PersonRule {
			continue
		}
		var c []string
		if cited != nil {
			c = cited(r.ID)
		}
		d := scope.Decide(scope.FromResource(r), env)
		f := Features(r, c)
		switch {
		case d.Value == resource.In && (d.Rule == "S1" || d.Rule == "S3"):
			labels = append(labels, Example{ID: r.ID, Features: f, Label: resource.In})
			if len(r.MSC) > 0 && len(r.MSC[0].Code) >= 2 {
				classes = append(classes, Example{ID: r.ID, Features: f, Label: r.MSC[0].Code[:2]})
			}
		case d.Value == resource.Border || d.Value == resource.Out:
			labels = append(labels, Example{ID: r.ID, Features: f, Label: d.Value})
		}
	}
	return labels, classes
}
