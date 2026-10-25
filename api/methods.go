package api

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/search"
	"github.com/tamnd/math2-agent/status"
	"github.com/tamnd/math2-agent/store"
)

// Method is one read method of the API.
type Method struct {
	Name    string  `json:"name"`
	Summary string  `json:"summary"`
	Params  []Param `json:"params"`

	run func(s *Service, a args) (result any, tags []string, err error)
}

// Param is a parameter of a method.
type Param struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // string, integer or boolean
	Required bool   `json:"required,omitempty"`
	Summary  string `json:"summary"`
}

// article is the name of each type with its article, for the messages.
var article = map[string]string{String: "a string", Integer: "an integer", Boolean: "a boolean"}

// The types of a Param.
const (
	String  = "string"
	Integer = "integer"
	Boolean = "boolean"
)

// Methods returns the read methods, in the order of spec 11.2.
func Methods() []Method { return slices.Clone(methods) }

func names() []string {
	var out []string
	for _, m := range methods {
		out = append(out, m.Name)
	}
	return out
}

var (
	ref = Param{Name: "ref", Type: String, Required: true, Summary: "a tag, an alias or an m2:// URI"}
	k   = Param{Name: "k", Type: Integer, Summary: "the number of results, from 1 to 100. The default is 10."}
)

var methods = []Method{
	{
		Name: "resolve", Summary: "Find the nodes that an identifier names: a tag, an alias, an m2:// URI, a Lean declaration or an external id such as a Wikidata QID or an OEIS A-number.",
		Params: []Param{{Name: "ref", Type: String, Required: true, Summary: "the identifier"}},
		run:    (*Service).resolve,
	},
	{
		Name: "get", Summary: "Get a node: its front matter, its body and its computed status.",
		Params: []Param{ref},
		run:    (*Service).get,
	},
	{
		Name: "status", Summary: "Get the status that the build computed for a node: the evidence level, the fidelity level and the reasons.",
		Params: []Param{ref},
		run: func(s *Service, a args) (any, []string, error) {
			t, err := s.tag(a.str("ref"))
			if err != nil {
				return nil, nil, err
			}
			st, _ := s.store.Status(t)
			return st, []string{t}, nil
		},
	},
	{
		Name: "get_formal", Summary: "Get the Lean bindings of a node, with their axioms, their fidelity, their reviews and their replay.",
		Params: []Param{ref},
		run: func(s *Service, a args) (any, []string, error) {
			t, err := s.tag(a.str("ref"))
			if err != nil {
				return nil, nil, err
			}
			st, _ := s.store.Status(t)
			return formal{Tag: t, Fidelity: st.Fidelity, Bindings: nonNil(st.Bindings)}, []string{t}, nil
		},
	},
	{
		Name: "neighbours", Summary: "Get the nodes and the edges near a node.",
		Params: []Param{ref, {Name: "depth", Type: Integer, Summary: "the number of edges from the node, from 0 to 3. The default is 1."}},
		run: func(s *Service, a args) (any, []string, error) {
			t, err := s.tag(a.str("ref"))
			if err != nil {
				return nil, nil, err
			}
			d := a.int("depth", 1)
			if d < 0 || d > 3 {
				return nil, nil, invalid("the depth %d is not from 0 to 3", d)
			}
			nb, err := s.store.Neighbourhood(t, d)
			return nb, nb.Nodes, notFoundOr(err)
		},
	},
	{
		Name: "closure", Summary: "Get the best alternative of the proofs of a node, with its trust profile and its axioms.",
		Params: []Param{ref, {Name: "strategy", Type: String, Summary: "min-depth or min-size. The default is min-depth."}},
		run: func(s *Service, a args) (any, []string, error) {
			t, err := s.tag(a.str("ref"))
			if err != nil {
				return nil, nil, err
			}
			m := graph.MinDepth
			switch a.str("strategy") {
			case "", "min-depth":
			case "min-size":
				m = graph.MinSize
			default:
				return nil, nil, invalid("the strategy %q is not min-depth or min-size", a.str("strategy"))
			}
			c, err := s.store.Closure(t, m)
			return c, c.Nodes, notFoundOr(err)
		},
	},
	{
		Name: "rdeps", Summary: "Get the nodes that use a node, and the nodes that lose each proof when the node is false.",
		Params: []Param{ref},
		run: func(s *Service, a args) (any, []string, error) {
			t, err := s.tag(a.str("ref"))
			if err != nil {
				return nil, nil, err
			}
			d, err := s.store.Dependents(t)
			return d, append(append([]string{t}, d.Needs...), d.Uses...), notFoundOr(err)
		},
	},
	{
		Name: "path", Summary: "Get the shortest learning path from one node to another.",
		Params: []Param{
			{Name: "from", Type: String, Required: true, Summary: "the node to start from: a tag, an alias or an m2:// URI"},
			{Name: "to", Type: String, Required: true, Summary: "the node to learn: a tag, an alias or an m2:// URI"},
		},
		run: func(s *Service, a args) (any, []string, error) {
			from, err := s.tag(a.str("from"))
			if err != nil {
				return nil, nil, err
			}
			to, err := s.tag(a.str("to"))
			if err != nil {
				return nil, nil, err
			}
			p, err := s.store.Path(from, to)
			return p, p, notFoundOr(err)
		},
	},
	{
		Name: "frontier", Summary: "Get the formal statements with no proof, or the open problems with the statements that bound them.",
		Params: []Param{
			{Name: "open", Type: Boolean, Summary: "the open problems, not the formal statements with no proof"},
			{Name: "msc", Type: String, Summary: "only the nodes with an MSC code that starts with this"},
			{Name: "topic", Type: String, Summary: "only the nodes that are part of this topic: a tag, an alias or an m2:// URI"},
			{Name: "evidence", Type: String, Summary: "only the nodes at this evidence level, from E0 to E5"},
		},
		run: func(s *Service, a args) (any, []string, error) {
			f := store.Filter{Open: a.bool("open"), MSC: a.str("msc")}
			if t := a.str("topic"); t != "" {
				var err error
				if f.Topic, err = s.tag(t); err != nil {
					return nil, nil, err
				}
			}
			if e := a.str("evidence"); e != "" {
				if err := f.Evidence.UnmarshalText([]byte(e)); err != nil || f.Evidence == status.None {
					return nil, nil, invalid("the evidence level %q is not one of E0 to E5", e)
				}
			}
			es, err := s.store.Frontier(f)
			var tags []string
			for _, e := range es {
				tags = append(tags, e.Tag)
			}
			return nonNil(es), tags, err
		},
	},
	{
		Name: "search", Summary: "Search the nodes with free text, LaTeX, a Lean type or an identifier. Each result gives the channels that found it.",
		Params: []Param{
			{Name: "q", Type: String, Required: true, Summary: "the query"}, k,
			{Name: "channel", Type: String, Summary: "the channels to run, separated by commas: " + strings.Join(search.Channels(), ", ") + ". The default is all of them."},
			{Name: "kind", Type: String, Summary: "only nodes of these kinds, separated by commas"},
			{Name: "open", Type: Boolean, Summary: "rank the open problems and the weak evidence first"},
		},
		run: func(s *Service, a args) (any, []string, error) {
			return s.search(a, split(a.str("channel")))
		},
	},
	{
		Name: "search_formal", Summary: "Search the nodes and the Mathlib declarations with a Lean type pattern, a Lean goal or a list of constants.",
		Params: []Param{{Name: "q", Type: String, Required: true, Summary: "the Lean type, the goal or the constants"}, k},
		run: func(s *Service, a args) (any, []string, error) {
			return s.search(a, []string{search.Formal})
		},
	},
	{
		Name: "search_formula", Summary: "Search the nodes by the structure of a LaTeX formula. The names of the variables do not matter.",
		Params: []Param{{Name: "q", Type: String, Required: true, Summary: "the formula in LaTeX"}, k},
		run: func(s *Service, a args) (any, []string, error) {
			return s.search(a, []string{search.Formula})
		},
	},
	{
		Name: "similar", Summary: "Get the nodes whose text is near the text of a node.",
		Params: []Param{ref, k},
		run:    (*Service).similar,
	},
	{
		Name: "premises", Summary: "Rank the Lean declarations that can help to prove a goal.",
		Params: []Param{
			{Name: "goal", Type: String, Summary: "the goal, as Lean prints it, with the hypotheses before ⊢"},
			{Name: "tag", Type: String, Summary: "the node of the goal. The declarations of the nodes that it uses rank higher."}, k,
		},
		run: func(s *Service, a args) (any, []string, error) {
			n, err := a.k()
			if err != nil {
				return nil, nil, err
			}
			g := search.Goal{Goal: a.str("goal"), K: n}
			if t := a.str("tag"); t != "" {
				if g.Tag, err = s.tag(t); err != nil {
					return nil, nil, err
				}
			}
			if strings.TrimSpace(g.Goal) == "" && g.Tag == "" {
				return nil, nil, invalid("give a goal, a tag or both")
			}
			ps, err := s.index.Premises(g)
			var tags []string
			for _, p := range ps {
				tags = append(tags, p.Tags...)
			}
			return nonNil(ps), tags, err
		},
	},
}

// formal is the result of get_formal.
type formal struct {
	Tag      string           `json:"tag"`
	Fidelity status.Fidelity  `json:"fidelity"`
	Bindings []status.Binding `json:"bindings"`
}

// resolved is the result of resolve.
type resolved struct {
	Ref   string   `json:"ref"`
	Tags  []string `json:"tags"`
	Match string   `json:"match"` // store for a tag, an alias or a URI, identifier for an id that the search index resolves
}

func (s *Service) resolve(a args) (any, []string, error) {
	r := a.str("ref")
	if tags, err := s.store.Resolve(r); err == nil {
		return resolved{Ref: r, Tags: tags, Match: "store"}, tags, nil
	}
	rs, err := s.index.Search(search.Query{Text: r, K: 100, Channels: []string{search.Identifier}})
	if err != nil {
		return nil, nil, err
	}
	var tags []string
	for _, x := range rs {
		if x.Tag != "" {
			tags = append(tags, x.Tag)
		}
	}
	if len(tags) == 0 {
		return nil, nil, &Error{Code: NotFound, Message: fmt.Sprintf("math2 holds no node for %q", r)}
	}
	slices.Sort(tags)
	return resolved{Ref: r, Tags: tags, Match: search.Identifier}, tags, nil
}

// got is the result of get.
type got struct {
	Tag    string         `json:"tag"`
	Kind   string         `json:"kind"`
	Name   string         `json:"name"`
	Hash   string         `json:"hash"`
	Front  map[string]any `json:"front"`
	Body   string         `json:"body"`
	Status status.Status  `json:"status"`
}

func (s *Service) get(a args) (any, []string, error) {
	t, err := s.tag(a.str("ref"))
	if err != nil {
		return nil, nil, err
	}
	n, err := s.file(t)
	if err != nil {
		return nil, nil, err
	}
	g, _ := s.store.Node(t)
	st, _ := s.store.Status(t)
	return got{Tag: t, Kind: g.Kind, Name: g.Name, Hash: g.Hash, Front: n.Meta, Body: n.Body, Status: st}, []string{t}, nil
}

func (s *Service) search(a args, channels []string) (any, []string, error) {
	n, err := a.k()
	if err != nil {
		return nil, nil, err
	}
	rs, err := s.index.Search(search.Query{Text: a.str("q"), K: n, Channels: channels, Kinds: split(a.str("kind")), Open: a.bool("open")})
	if err != nil {
		return nil, nil, invalid("%s", strings.TrimPrefix(err.Error(), "search: "))
	}
	return nonNil(rs), resultTags(rs), nil
}

func (s *Service) similar(a args) (any, []string, error) {
	t, err := s.tag(a.str("ref"))
	if err != nil {
		return nil, nil, err
	}
	n, err := a.k()
	if err != nil {
		return nil, nil, err
	}
	f, err := s.file(t)
	if err != nil {
		return nil, nil, err
	}
	rs, err := s.index.Search(search.Query{Text: f.Name() + "\n" + f.Body, K: n + 1, Channels: []string{search.Embedding}})
	if err != nil {
		return nil, nil, err
	}
	rs = slices.DeleteFunc(rs, func(r search.Result) bool { return r.Tag == t })
	if len(rs) > n {
		rs = rs[:n]
	}
	return nonNil(rs), resultTags(rs), nil
}

func resultTags(rs []search.Result) []string {
	var tags []string
	for _, r := range rs {
		tags = append(tags, r.Tag)
	}
	return tags
}

// nonNil returns an empty list for nil, so that a result with no items is [] in JSON and not null.
func nonNil[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}

func notFoundOr(err error) error {
	if err == nil {
		return nil
	}
	return notFound(err)
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// args are the checked parameters of a call.
type args map[string]any

// args checks the parameters against the method, and converts each value to the type of its parameter.
func (m Method) args(params map[string]any) (args, error) {
	a := args{}
	for name, v := range params {
		i := slices.IndexFunc(m.Params, func(p Param) bool { return p.Name == name })
		if i < 0 {
			return nil, invalid("%s has no parameter %q", m.Name, name)
		}
		p := m.Params[i]
		x, ok := convert(p.Type, v)
		if !ok {
			return nil, invalid("the parameter %s of %s must be %s, not %v", name, m.Name, article[p.Type], v)
		}
		a[name] = x
	}
	for _, p := range m.Params {
		if _, ok := a[p.Name]; p.Required && (!ok || a[p.Name] == "") {
			return nil, invalid("%s needs the parameter %s", m.Name, p.Name)
		}
	}
	return a, nil
}

func convert(typ string, v any) (any, bool) {
	switch typ {
	case String:
		s, ok := v.(string)
		return s, ok
	case Boolean:
		switch x := v.(type) {
		case bool:
			return x, true
		case string:
			b, err := strconv.ParseBool(x)
			return b, err == nil
		}
	case Integer:
		var s string
		switch x := v.(type) {
		case string:
			s = x
		case json.Number:
			s = x.String()
		case float64:
			if x != float64(int(x)) {
				return nil, false
			}
			return int(x), true
		case int:
			return x, true
		default:
			return nil, false
		}
		n, err := strconv.Atoi(s)
		return n, err == nil
	}
	return nil, false
}

func (a args) str(name string) string {
	s, _ := a[name].(string)
	return s
}

func (a args) bool(name string) bool {
	b, _ := a[name].(bool)
	return b
}

func (a args) int(name string, def int) int {
	if n, ok := a[name].(int); ok {
		return n
	}
	return def
}

// k returns the number of results.
func (a args) k() (int, error) {
	n := a.int("k", 10)
	if n < 1 || n > 100 {
		return 0, invalid("k is %d, and it must be from 1 to 100", n)
	}
	return n, nil
}
