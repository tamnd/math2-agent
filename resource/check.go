package resource

import (
	"fmt"
	"regexp"
	"strings"
)

// Problem is one fault in a resource record. Field names the field as a JSON path, for example expressions[0].manifestations[1].access.
type Problem struct {
	Field   string
	Message string
}

func (p Problem) String() string { return p.Field + ": " + p.Message }

var (
	keyRE  = regexp.MustCompile(`^[em][1-9][0-9]*$`)
	dateRE = regexp.MustCompile(`^[0-9]{4}(-[0-9]{2}(-[0-9]{2})?)?$`)
	dayRE  = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	mscRE  = regexp.MustCompile(`^[0-9]{2}(-XX|[A-Z]xx|[A-Z][0-9]{2}|-[0-9]{2})?$`)
)

// Check gives each fault in the fields of the record that rule H06 checks (RS2.4): the required fields, the values from the lists of this package, the local keys and the dates. It does not check the external ids (rule H03) or the register (rule H02).
func Check(r *Resource) []Problem {
	var ps []Problem
	add := func(field, format string, args ...any) {
		ps = append(ps, Problem{field, fmt.Sprintf(format, args...)})
	}
	if r.ID == "" {
		add("id", "the field is missing")
	}
	switch {
	case r.Kind == "":
		add("kind", "the field is missing")
	case !valid(Kinds, r.Kind):
		add("kind", "%q is not a kind of RS2.3", r.Kind)
	}
	for i, g := range r.Genre {
		if !valid(Genres, g) {
			add(fmt.Sprintf("genre[%d]", i), "%q is not a genre of RS2.3", g)
		}
	}
	if strings.TrimSpace(r.Title) == "" {
		add("title", "the field is missing")
	}
	if r.Date != "" && !dateRE.MatchString(r.Date) {
		add("date", "%q is not a date of the form YYYY, YYYY-MM or YYYY-MM-DD", r.Date)
	}
	if r.Level != "" && r.Kind != "thesis" {
		add("level", "only a thesis has a level")
	}
	creators(add, "creators", r.Creators)
	for i, c := range r.MSC {
		if !mscRE.MatchString(c.Code) {
			add(fmt.Sprintf("msc[%d].code", i), "%q is not an MSC2020 code", c.Code)
		}
		if c.From == "" {
			add(fmt.Sprintf("msc[%d].from", i), "the field is missing")
		}
	}
	keys := map[string]bool{}
	for i := range r.Expressions {
		e := &r.Expressions[i]
		f := fmt.Sprintf("expressions[%d]", i)
		localKey(add, f, e.Key, 'e', keys)
		if !valid(ExpressionKinds, e.Kind) {
			add(f+".kind", "%q is not a kind of expression", e.Kind)
		}
		if e.Date != "" && !dateRE.MatchString(e.Date) {
			add(f+".date", "%q is not a date", e.Date)
		}
		creators(add, f+".contributors", e.Contributors)
		if e.Access != nil {
			access(add, f+".access", e.Access)
		}
		mkeys := map[string]bool{}
		for j := range e.Manifestations {
			m := &e.Manifestations[j]
			g := fmt.Sprintf("%s.manifestations[%d]", f, j)
			localKey(add, g, m.Key, 'm', mkeys)
			if !valid(ManifestationKinds, m.Kind) {
				add(g+".kind", "%q is not a kind of manifestation", m.Kind)
			}
			if m.Date != "" && !dateRE.MatchString(m.Date) {
				add(g+".date", "%q is not a date", m.Date)
			}
			switch {
			case m.Access != nil:
				access(add, g+".access", m.Access)
			case len(m.Links) > 0 && e.Access == nil:
				add(g+".access", "a manifestation with a link needs an access, on itself or on its expression")
			}
		}
	}
	for i, rel := range r.Relations {
		f := fmt.Sprintf("relations[%d]", i)
		if !valid(Relations, rel.Kind) {
			add(f+".kind", "%q is not a relation of RS2.7", rel.Kind)
		}
		if rel.To == "" {
			add(f+".to", "the field is missing")
		}
		if rel.From != "" && !localRef(r, rel.From) {
			add(f+".from", "%q names no expression or manifestation of the resource", rel.From)
		}
	}
	if s := r.Scope; s != nil {
		switch {
		case !valid(Outcomes, s.Value):
			add("scope.value", "%q is not an outcome of RS9.6", s.Value)
		case s.Value == Duplicate && s.Target == "":
			add("scope.target", "a duplicate needs the id of the live resource")
		case s.Value == Duplicate && s.Target == r.ID:
			add("scope.target", "a resource cannot be a duplicate of itself")
		case s.Value == Deferred && !dayRE.MatchString(s.Until):
			add("scope.until", "a deferred decision needs a date of the form YYYY-MM-DD")
		case s.Value == Deferred && s.Reason == "":
			add("scope.reason", "a deferred decision needs a reason")
		}
		if s.By == "" {
			add("scope.by", "the field is missing")
		}
	}
	if len(r.Catalogue) == 0 {
		add("catalogue", "a resource needs at least one record of a catalogue")
	}
	for i, c := range r.Catalogue {
		f := fmt.Sprintf("catalogue[%d]", i)
		if c.Source == "" || c.Snapshot == "" || c.NativeID == "" {
			add(f, "a record of a catalogue needs a source, a snapshot and a native id")
		}
		if !valid(Levels, c.Level) {
			add(f+".level", "%q is not a level of RS2.2", c.Level)
		}
	}
	if len(r.History) == 0 {
		add("history", "a resource needs at least the event that added it")
	}
	for i, ev := range r.History {
		f := fmt.Sprintf("history[%d]", i)
		if ev.Op == "" || ev.By == "" || !dayRE.MatchString(ev.At) {
			add(f, "an event needs an op, a date of the form YYYY-MM-DD and a by")
		}
	}
	return ps
}

func creators(add func(string, string, ...any), field string, cs []Creator) {
	for i, c := range cs {
		f := fmt.Sprintf("%s[%d]", field, i)
		if strings.TrimSpace(c.Name) == "" {
			add(f+".name", "the field is missing")
		}
		if !valid(Roles, c.Role) {
			add(f+".role", "%q is not a role of RS2.5", c.Role)
		}
	}
}

func access(add func(string, string, ...any), field string, a *Access) {
	if !valid(Classes, a.Class) {
		add(field+".class", "%q is not an access class of spec 9.3", a.Class)
	}
	if a.Text != "" && !valid(Texts, a.Text) {
		add(field+".text", "%q is not a value of RS2.6", a.Text)
	}
	if a.Class == "open" && a.LicenceRead == nil && a.PublicDomain == nil {
		add(field+".licence_read", "the open class needs a reading of the licence at the source, or the public-domain answers")
	}
}

func localKey(add func(string, string, ...any), field, key string, prefix byte, seen map[string]bool) {
	switch {
	case !keyRE.MatchString(key) || key[0] != prefix:
		add(field+".key", "%q is not a key of the form %c1, %c2, and so on", key, prefix, prefix)
	case seen[key]:
		add(field+".key", "two have the key %s", key)
	}
	seen[key] = true
}

// localRef reports whether ref, as e3 or e3/m1, names an expression or a manifestation of r.
func localRef(r *Resource, ref string) bool {
	ek, mk, hasM := strings.Cut(ref, "/")
	e := r.Expression(ek)
	if e == nil {
		return false
	}
	if !hasM {
		return true
	}
	for _, m := range e.Manifestations {
		if m.Key == mk {
			return true
		}
	}
	return false
}
