// Package review makes the review packets of spec 10.6 and writes fidelity reviews.
//
// A packet holds what a reviewer reads for each Lean binding of a node: the informal statement and the Lean statement, with and without notation, each constant of the statement with its type and the nodes that bind it, the report of the F1 battery and the reviews that the binding has. [Make] builds a packet from the files of a build. [Packet.WriteText] shows it, with the two statements side by side.
//
// [Write] adds a review to the tree. It gives the review the next id of the node, writes it to reviews/XX/TAG.yaml and adds the id to the binding. Write checks the review before it changes a file. The tool never signs a review: the reviewer signs the commit with their own key.
//
// [Queue] gives the fidelity queue of spec 16.3: the bindings that wait for a review, with the nodes that most nodes use first.
//
// [Exports] gives the reviews of each reviewer, for the credit of spec 16.4. [Export.WriteText] writes them as text for a CV, and [Slug] gives the last segment of the stable link of a reviewer on the site.
package review

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/battery"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

// Packet is the review packet of one node.
type Packet struct {
	Tag           string    `json:"tag"`
	Kind          string    `json:"kind"`
	Name          string    `json:"name"`
	Section       string    `json:"section"`         // the name of the section that states the mathematics, for example Statement
	Statement     string    `json:"statement"`       // the text of that section
	StatementHash string    `json:"statement_hash"`  // the hash that a review records, as status.StatementHash gives it
	Stale         bool      `json:"stale,omitempty"` // the statement changed after the last build, so the build files are out of date
	Bindings      []Binding `json:"bindings"`
}

// Binding is the part of a packet for one Lean binding.
type Binding struct {
	Decl      string               `json:"decl"`
	Adapter   string               `json:"adapter,omitempty"`
	Checked   string               `json:"checked"` // the declaration that the reviewer reads: the adapter, or the declaration
	Match     string               `json:"match"`
	Author    string               `json:"author,omitempty"` // the author of the binding, from registers/bindings.reg
	Fidelity  status.Fidelity      `json:"fidelity"`
	TypeHash  string               `json:"type_hash"`
	Lean      string               `json:"lean"`     // the statement as Lean prints it
	LeanAll   string               `json:"lean_all"` // the statement with pp.all, without notation
	Error     string               `json:"error,omitempty"`
	Constants []Constant           `json:"constants"`
	Battery   *battery.Report      `json:"battery,omitempty"`
	Reviews   []status.ReviewState `json:"reviews"`
}

// Constant is a constant of a Lean statement.
type Constant struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"` // the type, when the build has a record of the constant
	// Structural is set for a constant of logic or of notation, or an instance, which needs no node (battery.Structural).
	Structural bool    `json:"structural,omitempty"`
	Nodes      []Bound `json:"nodes"` // the nodes that bind the constant
}

// Bound is a node that binds a constant.
type Bound struct {
	Tag      string          `json:"tag"`
	Name     string          `json:"name"`
	Fidelity status.Fidelity `json:"fidelity"`
}

// Input are the files of a build that a packet uses.
type Input struct {
	Nodes    []*node.Node
	Status   []status.Status        // status.json
	Records  map[string]lean.Record // the records of extract.json, by declaration
	Bindings []lean.Binding         // the bindings of the attribute @[m2 "TAG"], from extract.json
	Battery  []battery.Report       // battery.json
	Authors  map[status.Key]string  // the authors of the bindings, from registers/bindings.reg
}

// Make returns the packet of the node with the tag.
func Make(in Input, tag string) (Packet, error) {
	i := slices.IndexFunc(in.Nodes, func(n *node.Node) bool { return n.ID == tag })
	if i < 0 {
		return Packet{}, fmt.Errorf("review: no node has the tag %s", tag)
	}
	n := in.Nodes[i]
	j := slices.IndexFunc(in.Status, func(s status.Status) bool { return s.Tag == tag })
	if j < 0 {
		return Packet{}, fmt.Errorf("review: the build has no status of %s. Run m2 build", tag)
	}
	st := in.Status[j]
	p := Packet{Tag: tag, Kind: n.Kind(), Name: n.Name(), StatementHash: status.StatementHash(n), Bindings: []Binding{}}
	for _, sec := range check.Sections(n) {
		if slices.Contains(check.StatementSections(), sec.Name) {
			p.Section, p.Statement = sec.Name, strings.TrimSpace(sec.Text)
			break
		}
	}
	p.Stale = st.StatementHash != p.StatementHash

	fidelity := map[string]status.Status{}
	for _, s := range in.Status {
		fidelity[s.Tag] = s
	}
	bound := map[string][]string{}
	for _, s := range in.Status {
		for _, b := range s.Bindings {
			for _, d := range []string{b.Decl, b.Adapter} {
				if d != "" && !slices.Contains(bound[d], s.Tag) {
					bound[d] = append(bound[d], s.Tag)
				}
			}
		}
	}
	for _, b := range in.Bindings {
		if !slices.Contains(bound[b.Decl], b.Tag) {
			bound[b.Decl] = append(bound[b.Decl], b.Tag)
		}
	}
	reports := map[status.Key]battery.Report{}
	for _, r := range in.Battery {
		reports[status.Key{Tag: r.Tag, Decl: r.Decl}] = r
	}

	for _, b := range st.Bindings {
		pb := Binding{Decl: b.Decl, Adapter: b.Adapter, Checked: b.Checked, Match: b.Match, Author: in.Authors[status.Key{Tag: tag, Decl: b.Decl}], Fidelity: b.Fidelity, TypeHash: b.TypeHash, Constants: []Constant{}, Reviews: b.Reviews}
		if pb.Reviews == nil {
			pb.Reviews = []status.ReviewState{}
		}
		rec, ok := in.Records[b.Checked]
		switch {
		case !ok:
			pb.Error = "the build has no record of the declaration"
		case rec.Error != "":
			pb.Error = rec.Error
		default:
			pb.Lean, pb.LeanAll = rec.StatementPP, rec.StatementPPAll
			pb.TypeHash = cmp.Or(rec.TypeHash, pb.TypeHash)
			for _, c := range rec.UsesConstantsInStatement {
				pc := Constant{Name: c, Structural: battery.Structural(c), Nodes: []Bound{}}
				if r, ok := in.Records[c]; ok && r.Error == "" {
					pc.Type = r.StatementPP
				}
				tags := slices.Clone(bound[c])
				slices.Sort(tags)
				for _, t := range tags {
					pc.Nodes = append(pc.Nodes, Bound{Tag: t, Name: fidelity[t].Name, Fidelity: fidelity[t].Fidelity})
				}
				pb.Constants = append(pb.Constants, pc)
			}
		}
		if r, ok := reports[status.Key{Tag: tag, Decl: b.Checked}]; ok {
			pb.Battery = &r
		}
		p.Bindings = append(p.Bindings, pb)
	}
	return p, nil
}

// Binding returns the binding of the packet whose declaration or adapter is decl. An empty decl names the only binding.
func (p Packet) Binding(decl string) (Binding, error) {
	if decl == "" {
		switch len(p.Bindings) {
		case 1:
			return p.Bindings[0], nil
		case 0:
			return Binding{}, fmt.Errorf("review: %s has no Lean binding", p.Tag)
		}
		return Binding{}, fmt.Errorf("review: %s has %d Lean bindings. Name the declaration", p.Tag, len(p.Bindings))
	}
	for _, b := range p.Bindings {
		if b.Decl == decl || b.Adapter == decl {
			return b, nil
		}
	}
	return Binding{}, fmt.Errorf("review: %s has no Lean binding of %s", p.Tag, decl)
}

var (
	orcidRE  = regexp.MustCompile(`^orcid:[0-9]{4}-[0-9]{4}-[0-9]{4}-[0-9]{3}[0-9X]$`)
	githubRE = regexp.MustCompile(`^github:[A-Za-z0-9](-?[A-Za-z0-9])*$`)
)

// Person reports whether id names a person as a reviewer: orcid: and an ORCID iD, or github: and a GitHub login. An agent or a bot is not a person (rule V03).
func Person(id string) bool {
	return orcidRE.MatchString(id) || githubRE.MatchString(id)
}

// Fill completes the review r of the binding b of the packet p and checks it. Fill sets the declaration, the two hashes and, when it is empty, the date. The reviewer must be a person who is not the author of the binding, and the verdict must have its conventions or its class. Fill does not set the id.
func Fill(p Packet, b Binding, r status.Review) (status.Review, error) {
	if p.Stale {
		return r, fmt.Errorf("review: the statement of %s changed after the last build. Run m2 build", p.Tag)
	}
	if b.Error != "" || b.TypeHash == "" {
		return r, fmt.Errorf("review: the build has no type hash of %s. Run m2 build", b.Checked)
	}
	if p.StatementHash == "" {
		return r, fmt.Errorf("review: %s has no statement section", p.Tag)
	}
	r.Decl, r.StatementHash, r.TypeHash = b.Checked, p.StatementHash, b.TypeHash
	r.Date = cmp.Or(r.Date, time.Now().UTC().Format(time.DateOnly))
	if _, err := time.Parse(time.DateOnly, r.Date); err != nil {
		return r, fmt.Errorf("review: the date %q is not YYYY-MM-DD", r.Date)
	}
	switch {
	case !Person(r.Reviewer):
		return r, fmt.Errorf("review: the reviewer %q is not a person. Give orcid: and an ORCID iD, or github: and a GitHub login. An agent cannot review", r.Reviewer)
	case r.Reviewer == b.Author:
		return r, fmt.Errorf("review: %s wrote the binding of %s, and the author of a binding cannot review it", r.Reviewer, b.Decl)
	}
	switch r.Verdict {
	case status.Faithful:
		if len(r.Conventions) > 0 || r.Class != "" {
			return r, fmt.Errorf("review: the verdict %s has no conventions and no class", r.Verdict)
		}
	case status.FaithfulWithConventions:
		if len(r.Conventions) == 0 || r.Class != "" {
			return r, fmt.Errorf("review: the verdict %s needs the conventions, and it has no class", r.Verdict)
		}
	case status.NotFaithful:
		if !slices.Contains(battery.Classes(), r.Class) || len(r.Conventions) > 0 {
			return r, fmt.Errorf("review: the verdict %s needs one class of %s, and it has no conventions", r.Verdict, strings.Join(battery.Classes(), ", "))
		}
	default:
		return r, fmt.Errorf("review: the verdict %q is not %s, %s or %s", r.Verdict, status.Faithful, status.FaithfulWithConventions, status.NotFaithful)
	}
	r.Notes = strings.TrimSpace(r.Notes)
	return r, nil
}
