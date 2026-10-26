// Package sample draws the sample audits of the corpus and measures their error rates (corpus K9.7).
//
// Gates and reviews catch errors one at a time. They cannot say how many errors remain. The sample audit can: each month, it draws a random sample of the merged nodes of each stratum, and auditors check the sample blind, against a written checklist. A stratum is a combination of kind, writer, model, prompt version and layer, thus each configuration of an agent gets its own error rate.
//
// The register registers/audits.reg keeps the audits, one on each line. Draw adds the audits of a month, an auditor writes the result of each one, and Rates gives the error rate of each stratum and property, with the upper end of its 95% interval. With no errors in n samples, the upper end is 3/n. With errors, it is the upper end of the Wilson interval.
package sample

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// File is the register of the sample audits, from the root of the tree.
const File = "registers/audits.reg"

// MinSample is the least number of audits that a stratum gets each month. A stratum with fewer merged nodes is audited in full.
const MinSample = 100

// NearShare is the share of a threshold at which the upper end of an error rate is near the threshold.
const NearShare = 0.8

// Z is the quantile of the normal distribution for the 95% interval.
const Z = 1.959964

// Stratum is a stratum of the sample audit (corpus K9.7).
type Stratum struct {
	Kind   string // the kind of the node, for example theorem
	Writer string // person, importer, or the agent role, for example prove
	Model  string // the model of the agent, or empty for a person or an importer
	Prompt string // the prompt version of the agent, or empty
	Layer  string // the layer of corpus K2.7, for example L2
}

// String returns the stratum as the register writes it: its parts with | between them, and - for an empty part.
func (s Stratum) String() string {
	parts := []string{s.Kind, s.Writer, s.Model, s.Prompt, s.Layer}
	for i, p := range parts {
		parts[i] = cmp.Or(p, "-")
	}
	return strings.Join(parts, "|")
}

// ParseStratum parses a stratum as String writes it.
func ParseStratum(s string) (Stratum, error) {
	p := strings.Split(s, "|")
	if len(p) != 5 {
		return Stratum{}, fmt.Errorf("sample: the stratum %q does not have the 5 parts kind|writer|model|prompt|layer", s)
	}
	for i := range p {
		if p[i] == "-" {
			p[i] = ""
		}
	}
	return Stratum{Kind: p[0], Writer: p[1], Model: p[2], Prompt: p[3], Layer: p[4]}, nil
}

// Property is a property of corpus K9.1 that the audit measures.
type Property string

// The properties, with their thresholds.
const (
	True     Property = "true"     // the statement is true and the proof proves it
	Faithful Property = "faithful" // the node says what its sources say, and the sources are right
	Readable Property = "readable" // the node follows its card and the house style
)

// Properties are the properties in the order of the reports.
var Properties = []Property{True, Faithful, Readable}

// Threshold returns the threshold of the error rate of the property: 1% for a false statement, 2% for an unfaithful statement or a wrong source, 5% for a style or card error.
func (p Property) Threshold() float64 {
	switch p {
	case True:
		return 0.01
	case Faithful:
		return 0.02
	case Readable:
		return 0.05
	}
	return 0
}

// Classes maps each error class of the checklist to its property. The classes of prose come from corpus K9.7.
var Classes = map[string]Property{
	"false_statement":       True,
	"wrong_definition":      True,
	"dropped_hypothesis":    True,
	"step_does_not_follow":  True,
	"unfaithful":            Faithful,
	"stronger_than_source":  Faithful,
	"wrong_attribution":     Faithful,
	"wrong_source":          Faithful,
	"convention_not_stated": Readable,
	"style":                 Readable,
	"card":                  Readable,
}

// The results of an audit that are not error classes.
const (
	Pending = "open" // the auditor did not do the audit yet
	Clean   = "ok"   // the auditor found no error
)

// Audit is one record of registers/audits.reg: a node that the sample of a month drew, with its result.
type Audit struct {
	Month   string   // YYYY-MM
	Seed    string   // the seed of the draw of the month
	Tag     string   // the tag of the node
	Stratum Stratum  // the stratum of the node at the draw
	Auditor string   // the auditor, or empty when nobody has the audit
	Errors  []string // the error classes that the auditor found
	Done    bool     // the auditor did the audit
}

// Member is a merged node that the draw can take.
type Member struct {
	Tag     string
	Stratum Stratum
}

var monthRE = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// Draw draws the sample of the month: n members of each stratum, or MinSample when n is smaller, and each member of a stratum with fewer members. The draw depends only on the seed, the month and the members, thus anyone can draw the same sample again. Each member gets a score, the SHA-256 hash of the seed, the month and the tag, and the draw takes the members with the lowest scores. The result is in the order of the strata, then of the scores.
//
// A stratum with fewer than 1,000 merges in a month pools its samples over three months, and the caller then gives the members of the three months.
func Draw(members []Member, month, seed string, n int) ([]Audit, error) {
	if !monthRE.MatchString(month) {
		return nil, fmt.Errorf("sample: %q is not a month, YYYY-MM", month)
	}
	if seed == "" || strings.ContainsAny(seed, " \t\n#") {
		return nil, fmt.Errorf("sample: the seed %q is empty or has a space or a #", seed)
	}
	n = max(n, MinSample)
	type scored struct {
		Member
		score [32]byte
	}
	by := map[string][]scored{}
	seen := map[string]bool{}
	for _, m := range members {
		if seen[m.Tag] {
			continue
		}
		seen[m.Tag] = true
		k := m.Stratum.String()
		by[k] = append(by[k], scored{m, score(seed, month, m.Tag)})
	}
	var out []Audit
	for _, k := range slices.Sorted(maps.Keys(by)) {
		all := by[k]
		slices.SortFunc(all, func(a, b scored) int { return bytes.Compare(a.score[:], b.score[:]) })
		for _, m := range all[:min(n, len(all))] {
			out = append(out, Audit{Month: month, Seed: seed, Tag: m.Tag, Stratum: m.Stratum})
		}
	}
	return out, nil
}

// score returns the SHA-256 hash of the parts, with a zero byte after each part.
func score(parts ...string) [32]byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	var s [32]byte
	copy(s[:], h.Sum(nil))
	return s
}

// header is the first line of a new register.
const header = "# month seed tag stratum auditor result\n"

// Load reads registers/audits.reg in the tree root. A later record of the same month and tag replaces the earlier one, for example with the result of the audit. The result is in the order of the first record of each audit. A tree without the register has no audits.
//
// A record has the fields month, seed, tag, stratum, auditor and result, with spaces between them. The auditor is - when nobody has the audit. The result is open for an audit that is not done, ok for an audit with no error, or the error classes with commas between them.
func Load(root string) ([]Audit, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(File))) // #nosec G304 -- the caller names the root of the tree.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse parses the text of the register, as Load does.
func Parse(data []byte) ([]Audit, error) {
	var out []Audit
	at := map[[2]string]int{}
	n := 0
	for line := range strings.Lines(string(data)) {
		n++
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if len(f) != 6 {
			return nil, fmt.Errorf("%s: line %d has %d fields, and a record has the fields month, seed, tag, stratum, auditor and result", File, n, len(f))
		}
		a := Audit{Month: f[0], Seed: f[1], Tag: f[2]}
		if !monthRE.MatchString(a.Month) {
			return nil, fmt.Errorf("%s: line %d: %q is not a month, YYYY-MM", File, n, a.Month)
		}
		s, err := ParseStratum(f[3])
		if err != nil {
			return nil, fmt.Errorf("%s: line %d: %w", File, n, err)
		}
		a.Stratum = s
		if f[4] != "-" {
			a.Auditor = f[4]
		}
		switch f[5] {
		case Pending:
		case Clean:
			a.Done = true
		default:
			a.Done = true
			for c := range strings.SplitSeq(f[5], ",") {
				if _, ok := Classes[c]; !ok {
					return nil, fmt.Errorf("%s: line %d: %q is not an error class", File, n, c)
				}
				a.Errors = append(a.Errors, c)
			}
		}
		k := [2]string{a.Month, a.Tag}
		if i, ok := at[k]; ok {
			out[i] = a
			continue
		}
		at[k] = len(out)
		out = append(out, a)
	}
	return out, nil
}

// Append adds the audits to registers/audits.reg in the tree root. It makes the register when the tree has none.
func Append(root string, audits []Audit) error {
	path := filepath.Join(root, filepath.FromSlash(File))
	old, err := os.ReadFile(path) // #nosec G304 -- the caller names the root of the tree.
	switch {
	case errors.Is(err, fs.ErrNotExist):
		old = []byte(header)
	case err != nil:
		return err
	}
	var b bytes.Buffer
	b.Write(old)
	if len(old) > 0 && old[len(old)-1] != '\n' {
		b.WriteByte('\n')
	}
	for _, a := range audits {
		fmt.Fprintf(&b, "%s %s %s %s %s %s\n", a.Month, a.Seed, a.Tag, a.Stratum, cmp.Or(a.Auditor, "-"), a.result())
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o600)
}

// result returns the result field of the audit.
func (a Audit) result() string {
	switch {
	case !a.Done:
		return Pending
	case len(a.Errors) == 0:
		return Clean
	}
	return strings.Join(a.Errors, ",")
}

// State is the state of an error rate against its threshold.
type State string

// The states.
const (
	Below State = "below" // the upper end of the interval is under NearShare of the threshold
	Short State = "short" // the sample has no error, but it is too small to show that the rate is under the threshold
	Near  State = "near"  // the upper end of the interval is NearShare of the threshold or more, and not above it
	Above State = "above" // the upper end of the interval passes the threshold: the configuration stops
)

// Rate is the error rate of one property in one stratum.
type Rate struct {
	Stratum  Stratum
	Property Property
	Audits   int     // the audits that are done
	Errors   int     // the audits with an error of the property
	Upper    float64 // the upper end of the 95% interval of the rate
	State    State
}

// Need returns the number of audits with no errors that show a rate under the threshold of the property.
func Need(p Property) int {
	return int(math.Ceil(3 / p.Threshold()))
}

// Rates returns the error rate of each property in each stratum of the audits that are done, in the order of the strata and of Properties. The caller gives the audits of the window to measure, for example of one month, or of three months for a small stratum.
func Rates(audits []Audit) []Rate {
	type key struct {
		s Stratum
		p Property
	}
	done := map[Stratum]int{}
	bad := map[key]int{}
	for _, a := range audits {
		if !a.Done {
			continue
		}
		done[a.Stratum]++
		props := map[Property]bool{}
		for _, c := range a.Errors {
			props[Classes[c]] = true
		}
		for p := range props {
			bad[key{a.Stratum, p}]++
		}
	}
	strata := slices.Collect(maps.Keys(done))
	slices.SortFunc(strata, func(a, b Stratum) int { return strings.Compare(a.String(), b.String()) })
	var out []Rate
	for _, s := range strata {
		for _, p := range Properties {
			r := Rate{Stratum: s, Property: p, Audits: done[s], Errors: bad[key{s, p}]}
			r.Upper = Upper(r.Errors, r.Audits)
			t := p.Threshold()
			switch {
			case r.Upper > t && r.Errors == 0:
				r.State = Short
			case r.Upper > t:
				r.State = Above
			case r.Upper >= NearShare*t:
				r.State = Near
			default:
				r.State = Below
			}
			out = append(out, r)
		}
	}
	return out
}

// Upper returns the upper end of the 95% interval of an error rate with bad errors in n samples: 3/n with no errors, and the upper end of the Wilson interval with errors. It is 1 when n is 0.
func Upper(bad, n int) float64 {
	if n <= 0 {
		return 1
	}
	if bad == 0 {
		return min(1, 3/float64(n))
	}
	_, hi := Wilson(bad, n)
	return hi
}

// Wilson returns the Wilson score interval at 95% of an error rate with bad errors in n samples.
func Wilson(bad, n int) (lo, hi float64) {
	if n <= 0 {
		return 0, 1
	}
	p := float64(bad) / float64(n)
	nn := float64(n)
	z2 := Z * Z
	d := 1 + z2/nn
	c := (p + z2/(2*nn)) / d
	h := Z * math.Sqrt(p*(1-p)/nn+z2/(4*nn*nn)) / d
	return max(0, c-h), min(1, c+h)
}
