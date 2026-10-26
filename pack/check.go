package pack

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/audit"
	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/notation"
	"github.com/tamnd/math2-agent/prov"
)

// Checks are the pack checks of corpus K11.6, in order. Each check is hard: a volume that fails one does not build an edition.
var Checks = []string{"PC1", "PC2", "PC3", "PC4", "PC5", "PC6", "PC7", "PC8", "PC9", "PC10"}

// checker runs the checks on one document.
type checker struct {
	d        *Document
	findings map[string][]check.Finding
	checked  map[string]int
	skip     map[string]string // the reason of each check that did not run
	na       map[string]bool
}

// Check runs the pack checks PC1 to PC10 on the document (corpus K11.6). The report has one result for each check, in the order of Checks. Check runs no model and uses no network. PC9 builds each format two times.
func (d *Document) Check() *check.Report {
	c := &checker{d: d, findings: map[string][]check.Finding{}, checked: map[string]int{}, skip: map[string]string{}, na: map[string]bool{}}
	for _, p := range d.problems {
		if p.tag != "" {
			c.atNode(p.check, p.tag, 0, 0, "%s", p.msg)
		} else {
			c.atKey(p.check, p.key, "%s", p.msg)
		}
	}
	c.levels()
	c.order()
	c.closure()
	c.notation()
	c.conventions()
	c.links()
	c.glue()
	c.provenance()
	c.reproducible()
	c.exercises()
	r := &check.Report{Files: len(d.Items()) + 1}
	for _, id := range Checks {
		res := check.Result{Rule: id, State: check.Pass, Checked: c.checked[id], Findings: len(c.findings[id])}
		switch {
		case res.Findings > 0:
			res.State = check.Fail
		case c.skip[id] != "":
			res.State, res.Reason = check.Skip, c.skip[id]
		case c.na[id] || res.Checked == 0:
			res.State = check.NA
		}
		r.Results = append(r.Results, res)
		r.Findings = append(r.Findings, c.findings[id]...)
	}
	slices.SortStableFunc(r.Findings, func(a, b check.Finding) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Col, b.Col))
	})
	return r
}

// atKey reports a finding at a key of the volume file.
func (c *checker) atKey(id, key, format string, args ...any) {
	c.findings[id] = append(c.findings[id], check.Finding{Rule: id, Path: c.d.Volume.File, Line: max(c.d.Volume.Line[key], 1), Col: 1, Msg: fmt.Sprintf(format, args...)})
}

// atNode reports a finding at a node file. A line of 0 is the first line.
func (c *checker) atNode(id, t string, line, col int, format string, args ...any) {
	p := t
	if n := c.d.tree.nodes[t]; n != nil {
		p = n.Path
	}
	c.findings[id] = append(c.findings[id], check.Finding{Rule: id, Path: p, Line: max(line, 1), Col: max(col, 1), Msg: fmt.Sprintf(format, args...)})
}

// levels is PC1: each node of the volume has the writing level and the evidence level of the volume.
func (c *checker) levels() {
	want := c.d.Volume.Levels
	for _, it := range c.d.Items() {
		c.checked["PC1"]++
		if it.Writing < want.Writing {
			c.atNode("PC1", it.Tag, 0, 0, "%s is at %s, and the volume needs %s", it.Tag, it.Writing, want.Writing)
		}
		if it.Kind == "statement" && it.Role != "axiom" && it.Evidence < want.Evidence {
			c.atNode("PC1", it.Tag, 0, 0, "%s is at %s, and the volume needs %s", it.Tag, it.Evidence, want.Evidence)
		}
	}
}

// order is PC2: no item comes before a node that it uses, and no proof uses or links to an item after its statement.
func (c *checker) order() {
	d := c.d
	for _, it := range d.Items() {
		c.checked["PC2"]++
		n := d.tree.nodes[it.Tag]
		for _, u := range n.List("uses") {
			if h := d.homes[u]; h != nil && h.position > it.position {
				c.atNode("PC2", it.Tag, 0, 0, "%s uses %s, and %s %s comes after it", it.Tag, u, h.Label, h.Number)
			}
		}
		if !it.Proof {
			continue
		}
		for _, l := range check.Links(n) {
			if h := d.homes[l.Target]; h != nil && h.position > it.position && !h.Proof {
				c.atNode("PC2", it.Tag, l.Line, l.Col, "the proof links to %s %s, and it comes after the statement", h.Label, h.Number)
			}
		}
	}
}

// closure is PC3: each node that an item uses is in the volume or in assumes.
func (c *checker) closure() {
	d := c.d
	assumed := map[string]bool{}
	for _, a := range d.Volume.Assumes {
		assumed[a] = true
	}
	for _, it := range d.Items() {
		c.checked["PC3"]++
		for _, u := range d.tree.nodes[it.Tag].List("uses") {
			if d.homes[u] == nil && !assumed[u] && d.tree.nodes[u] != nil {
				c.atNode("PC3", it.Tag, 0, 0, "%s uses %s, and %s is not in the volume or in assumes", it.Tag, u, u)
			}
		}
	}
}

// notation is PC4: no two macros of the volume print the same glyph for two meanings, unless the volume chooses a variant.
func (c *checker) notation() {
	d := c.d
	reg := d.tree.reg
	if reg == nil {
		switch {
		case !errors.Is(d.tree.regErr, fs.ErrNotExist):
			c.skip["PC4"] = "the notation register does not load: " + d.tree.regErr.Error()
		case len(d.Volume.Notation) > 0:
			c.atKey("PC4", "notation", "the volume chooses variants, and the tree has no notation register")
		}
		return
	}
	byID := map[string]*notation.Entry{}
	for _, e := range reg.Entries {
		byID[e.ID()] = e
	}
	for _, id := range slices.Sorted(maps.Keys(d.Volume.Notation)) {
		c.checked["PC4"]++
		e := byID[id]
		name := d.Volume.Notation[id]
		switch {
		case e == nil:
			c.atKey("PC4", "notation", "the volume chooses a variant of %s, and the register has no such entry", id)
		case !slices.ContainsFunc(e.Variants, func(v notation.Variant) bool { return v.Name == name }):
			c.atKey("PC4", "notation", "the entry %s has no variant %s", id, name)
		}
	}
	uses := d.Notation
	for i := range uses {
		c.checked["PC4"]++
		a := byID[uses[i].ID]
		for j := i + 1; j < len(uses); j++ {
			b := byID[uses[j].ID]
			if a == nil || b == nil || uses[i].Variant != "" || uses[j].Variant != "" {
				continue
			}
			conflict := slices.Contains(a.Conflicts, b.ID()) || slices.Contains(b.Conflicts, a.ID())
			if g := notation.Glyph(uses[i].LaTeX); g != "" && g == notation.Glyph(uses[j].LaTeX) && a.Denotes != b.Denotes {
				conflict = true
			}
			if conflict {
				c.atKey("PC4", "notation", "%s and %s print the same symbol for two meanings. Choose a variant of one of them in notation", a.ID(), b.ID())
			}
		}
	}
}

// conventions is PC5: each node uses the option of the volume for each convention, or the volume names the node in differ.
func (c *checker) conventions() {
	d := c.d
	switch err := d.tree.convErr; {
	case err == nil:
	case !errors.Is(err, fs.ErrNotExist):
		c.skip["PC5"] = "the conventions register does not load: " + err.Error()
		return
	case len(d.Volume.Conventions) > 0:
		c.atKey("PC5", "conventions", "the volume chooses conventions, and the tree has no conventions register")
		return
	default:
		return
	}
	known := map[string]*notation.Convention{}
	for _, cv := range d.tree.conv {
		known[cv.ID] = cv
	}
	for _, vc := range d.Volume.Conventions {
		c.checked["PC5"]++
		cv := known[vc.Convention]
		switch {
		case cv == nil:
			c.atKey("PC5", "conventions", "the convention %s is not in the register", vc.Convention)
		case cv.Option(vc.Option) == nil:
			c.atKey("PC5", "conventions", "the convention %s has no option %s", vc.Convention, vc.Option)
		}
		for t := range vc.Differ {
			if d.homes[t] == nil {
				c.atKey("PC5", "conventions", "differ names %s, and it is not in the volume", t)
			}
		}
	}
	differ := map[string]map[string]string{}
	for _, vc := range d.Volume.Conventions {
		differ[vc.Convention] = vc.Differ
	}
	for _, u := range d.Conventions {
		c.checked["PC5"]++
		for _, df := range u.Differ {
			if _, ok := differ[u.ID][df.Tag]; !ok {
				c.atNode("PC5", df.Tag, 0, 0, "%s uses the option %s of %s, and the volume uses %s. Name the node in differ with the difference", df.Tag, cmp.Or(df.Option, "none"), u.ID, u.Option)
			}
		}
	}
}

// links is PC6: each link and each uses of the volume ends at a node.
func (c *checker) links() {
	c.checked["PC6"] += len(c.d.Items())
	for _, m := range c.d.missing {
		switch {
		case m.how == "outline":
			c.atKey("PC6", "outline", "the outline names %s, and it is not a node", m.to)
		case m.from == "":
			c.atKey("PC6", "targets", "%s is not a node", m.to)
		default:
			c.atNode("PC6", m.from, 0, 0, "%s %s %s, and it is not a node", m.from, m.how, m.to)
		}
	}
}

// mathGlue are the patterns of mathematics in glue. Glue motivates, connects and summarizes. A definition, a statement or a proof is a node (corpus K11.3).
var mathGlue = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^\s*(\*\*|_)?(Theorem|Lemma|Proposition|Corollary|Definition|Proof)\b`),
	regexp.MustCompile(`(?i)\bwe (define|call|prove)\b`),
	regexp.MustCompile(`(?i)\bis (called|defined (as|to be|by))\b`),
	regexp.MustCompile(`(?i)\bif and only if\b`),
	regexp.MustCompile(`\$\$`),
	regexp.MustCompile(`(?i)\bq\.?e\.?d\b|∎`),
}

// glue is PC7: the glue has no mathematics, it has a writer, and a person confirmed the glue of an agent.
func (c *checker) glue() {
	for _, g := range c.d.Volume.Glue {
		c.checked["PC7"]++
		where := "the glue at " + g.At
		for _, re := range mathGlue {
			if m := re.FindString(g.Text); m != "" {
				c.atKey("PC7", "glue", "%s has mathematics (%q). Put a definition, a statement or a proof in a node, and link to it", where, strings.TrimSpace(m))
				break
			}
		}
		switch {
		case g.Writer == "":
			c.atKey("PC7", "glue", "%s has no writer", where)
		case !prov.IsPerson(g.Writer) && !prov.IsAgent(g.Writer):
			c.atKey("PC7", "glue", "%s has the writer %q, and it is not a person or an agent", where, g.Writer)
		case g.Agent() && !prov.IsRun(g.Run):
			c.atKey("PC7", "glue", "an agent wrote %s, and run is %q, not a run id", where, g.Run)
		case g.Agent() && !prov.IsPerson(g.Confirmed):
			c.atKey("PC7", "glue", "an agent wrote %s, and no person confirmed it. Add confirmed with the editor", where)
		}
	}
}

// provenance is PC8: the nodes of the volume pass the provenance rules L02 and L03.
func (c *checker) provenance() {
	r, err := audit.Run(c.d.tree.root, audit.Options{Rules: []string{"L02", "L03"}})
	if err != nil {
		c.skip["PC8"] = "the audit does not run: " + err.Error()
		return
	}
	paths := map[string]bool{}
	for _, it := range c.d.Items() {
		paths[it.Path] = true
	}
	ran := false
	for _, res := range r.Results {
		switch res.State {
		case check.Skip:
			c.skip["PC8"] = cmp.Or(c.skip["PC8"], res.Rule+": "+res.Reason)
		case check.NA:
		default:
			ran = true
		}
	}
	if ran {
		delete(c.skip, "PC8")
	}
	c.checked["PC8"] = len(paths)
	for _, f := range r.Findings {
		if paths[f.Path] {
			c.findings["PC8"] = append(c.findings["PC8"], check.Finding{Rule: "PC8", Path: f.Path, Line: f.Line, Col: f.Col, Msg: f.Rule + ": " + f.Msg})
		}
	}
	if !ran && c.skip["PC8"] == "" {
		c.na["PC8"] = true
	}
}

// reproducible is PC9: each format builds the same bytes two times, and the lock file of a pinned release is up to date.
func (c *checker) reproducible() {
	d := c.d
	for _, f := range d.Volume.formats() {
		c.checked["PC9"]++
		if f != HTML {
			c.atKey("PC9", "formats", "the format %s has no back end yet", f)
			continue
		}
		a, err := d.HTML()
		if err != nil {
			c.atKey("PC9", "formats", "the HTML does not build: %v", err)
			continue
		}
		b, err := d.HTML()
		if err != nil || !sameFiles(a, b) {
			c.atKey("PC9", "formats", "two builds of the HTML are not the same")
		}
	}
	if d.Volume.Release == "" {
		return
	}
	c.checked["PC9"]++
	want, err := d.Lock().Marshal()
	if err != nil {
		c.atKey("PC9", "release", "the lock does not encode: %v", err)
		return
	}
	got, err := ReadLock(d.tree.root, d.Volume.ID)
	switch {
	case err != nil:
		c.atKey("PC9", "release", "the lock file does not read: %v", err)
	case got == nil:
		c.atKey("PC9", "release", "the volume pins %s, and %s does not exist. Run m2 pack", d.Volume.Release, LockFile(d.Volume.ID))
	case !bytes.Equal(got, want):
		c.atKey("PC9", "release", "%s is not up to date. Run m2 pack", LockFile(d.Volume.ID))
	}
}

func sameFiles(a, b map[string][]byte) bool {
	return maps.EqualFunc(a, b, bytes.Equal)
}

// exercises is PC10: each section has the exercises that the volume asks for, at the difficulties that it asks for.
func (c *checker) exercises() {
	want := c.d.Volume.Exercises
	if want.PerSection == 0 {
		c.na["PC10"] = true
		return
	}
	lo, hi := 1, 5
	if len(want.Difficulty) == 2 {
		lo, hi = want.Difficulty[0], want.Difficulty[1]
	}
	for _, ch := range c.d.Chapters {
		for _, s := range ch.Sections {
			c.checked["PC10"]++
			n := 0
			for _, it := range s.Items {
				if it.Kind != "exercise" {
					continue
				}
				if v, err := strconv.Atoi(c.d.tree.nodes[it.Tag].Get("difficulty")); err == nil && v >= lo && v <= hi {
					n++
				}
			}
			if n < want.PerSection {
				c.atKey("PC10", "exercises", "the section %s %s has %d exercises of difficulty %d to %d, and the volume asks for %d", s.Number, s.Title, n, lo, hi, want.PerSection)
			}
		}
	}
}
