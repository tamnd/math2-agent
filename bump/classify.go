package bump

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

// Kind is the kind of a break (spec 07.9).
type Kind string

// The kinds of a break.
const (
	Deprecated Kind = "deprecated" // a rename with a deprecation alias: the @[deprecated] attribute names the new declaration
	Renamed    Kind = "renamed"    // a rename without an alias: the old declaration is gone, and one declaration has its type hash
	Refactored Kind = "refactored" // the declaration is there, and its type hash changed
	Lost       Kind = "lost"       // the declaration is gone, and no declaration or more than one has its type hash
	Broken     Kind = "broken"     // the build of the Lake project fails
)

// kinds are the kinds in the order of the report.
var kinds = []Kind{Deprecated, Renamed, Refactored, Lost, Broken}

// Kinds returns the kinds of a break in the order of the report.
func Kinds() []Kind { return slices.Clone(kinds) }

// Mechanical tells if a person does not have to look at a break of the kind. The bump applies the rename of a deprecated or a renamed declaration, and that pull request can merge with no review (spec 07.9, step 3).
func (k Kind) Mechanical() bool { return k == Deprecated || k == Renamed }

// A Break is one change that the bump makes to a binding, or one error of the build.
type Break struct {
	Kind Kind
	Tag  string // the node of the binding. It is empty for a broken build.
	Old  string // the declaration with the old pins
	New  string // the new declaration of a rename
	File string // the file of an error of the build, from the Lake project
	Line int    // the line of an error of the build
	Text string // the message of Lean, or the candidates of a lost declaration
}

// Classify compares the extraction of the bound declarations with the old pins and with the new pins, and returns a break for each Lean binding of the nodes that changed. byHash gives the declarations of the new Mathlib for each type hash. It finds a rename without an alias. With a nil byHash, each declaration that is gone is lost.
func Classify(nodes []*node.Node, old, cur *build.Extraction, byHash map[string][]string) []Break {
	before, after := records(old), records(cur)
	var out []Break
	seen := map[[2]string]bool{}
	for _, n := range nodes {
		if _, retired := n.Meta["retired"]; retired {
			continue
		}
		for _, d := range decls(n) {
			if seen[[2]string{n.ID, d}] {
				continue
			}
			seen[[2]string{n.ID, d}] = true
			r, ok := after[d]
			if !ok {
				continue
			}
			b := Break{Tag: n.ID, Old: d}
			o, had := before[d]
			switch {
			case r.Deprecated != nil && r.Deprecated.NewName != "":
				b.Kind, b.New, b.Text = Deprecated, r.Deprecated.NewName, r.Deprecated.Text
			case r.Error != "" && had && o.Error == "":
				var cands []string
				for _, c := range byHash[o.TypeHash] {
					if c != d {
						cands = append(cands, c)
					}
				}
				if len(cands) == 1 {
					b.Kind, b.New = Renamed, cands[0]
				} else {
					b.Kind, b.Text = Lost, r.Error
					if len(cands) > 1 {
						b.Text = "the declarations with the old type hash are " + strings.Join(cands, ", ")
					}
				}
			case r.Error == "" && had && o.Error == "" && o.TypeHash != r.TypeHash:
				b.Kind = Refactored
			default:
				continue
			}
			out = append(out, b)
		}
	}
	sortBreaks(out)
	return out
}

// records returns the records of the extraction by declaration.
func records(x *build.Extraction) map[string]lean.Record {
	out := map[string]lean.Record{}
	if x != nil {
		for _, r := range x.Records {
			out[r.Decl] = r
		}
	}
	return out
}

// decls returns the declarations of the Lean bindings of the node. An adapter is Math2 code, and a break in it is a break of the build.
func decls(n *node.Node) []string {
	var out []string
	list, _ := n.Meta["formal"].([]any)
	for _, item := range list {
		m, _ := item.(map[string]any)
		if m["system"] != "lean4" {
			continue
		}
		if s, _ := m["decl"].(string); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// leanError matches an error of lake build, as in error: ./././Math2/Foo.lean:12:4: unknown identifier.
var leanError = regexp.MustCompile(`(?m)^error: (?:\./)*([^:\n]+\.lean):(\d+):(\d+): (.*)$`)

// Failures returns a break for each error of Lean in the output of a failed lake build. The file is relative to the Lake project.
func Failures(output string) []Break {
	var out []Break
	for _, m := range leanError.FindAllStringSubmatch(output, -1) {
		line, _ := strconv.Atoi(m[2])
		out = append(out, Break{Kind: Broken, File: m[1], Line: line, Text: strings.TrimSpace(m[4])})
	}
	sortBreaks(out)
	return slices.Compact(out)
}

// A Catchup is a Math2 declaration that the new Mathlib has with the same type hash (spec 07.9, step 6). The bump proposes to bind the node to Mathlib and to deprecate the Math2 copy.
type Catchup struct {
	Tag     string   // the node
	Decl    string   // the Math2 declaration
	Mathlib []string // the Mathlib declarations with the same type hash
}

// Upstream returns the catch-ups of the bound Math2 declarations of the extraction. byHash gives the declarations of the new Mathlib for each type hash.
func Upstream(x *build.Extraction, byHash map[string][]string) []Catchup {
	if x == nil {
		return nil
	}
	recs := records(x)
	var out []Catchup
	for _, b := range x.Bindings {
		r, ok := recs[b.Decl]
		if !ok || r.Error != "" || !strings.HasPrefix(b.Decl, "Math2.") {
			continue
		}
		var ms []string
		for _, d := range byHash[r.TypeHash] {
			if !strings.HasPrefix(d, "Math2.") {
				ms = append(ms, d)
			}
		}
		if len(ms) > 0 {
			out = append(out, Catchup{Tag: b.Tag, Decl: b.Decl, Mathlib: ms})
		}
	}
	slices.SortFunc(out, func(a, b Catchup) int { return strings.Compare(a.Tag+" "+a.Decl, b.Tag+" "+b.Decl) })
	return out
}

// ByHash returns the declarations of the library records for each type hash, in order of name.
func ByHash(decls []lean.Decl) map[string][]string {
	out := map[string][]string{}
	for _, d := range decls {
		if d.TypeHash != "" && d.Error == "" {
			out[d.TypeHash] = append(out[d.TypeHash], d.Decl)
		}
	}
	for _, v := range out {
		slices.Sort(v)
	}
	return out
}

// Stale returns the tags of the nodes whose fidelity level is lower in cur than in old, in tag order. A refactor makes the reviews of a binding stale (rule F02), thus the node drops to F0 and goes into the fidelity queue.
func Stale(old, cur []status.Status) []string {
	was := map[string]status.Fidelity{}
	for _, s := range old {
		was[s.Tag] = s.Fidelity
	}
	var out []string
	for _, s := range cur {
		if f, ok := was[s.Tag]; ok && s.Fidelity < f {
			out = append(out, s.Tag)
		}
	}
	slices.Sort(out)
	return out
}

// page is the number of library records that Library asks for in one request.
const page = 2000

// Library returns the library records of all the declarations under root in the modules that the session loaded, for example Mathlib. Give them to [ByHash].
func Library(s *lean.Session, root string) ([]lean.Decl, error) {
	var out []lean.Decl
	for {
		ds, total, err := s.Library(root, len(out), page)
		if err != nil {
			return nil, err
		}
		out = append(out, ds...)
		if len(out) >= total || len(ds) == 0 {
			return out, nil
		}
	}
}
