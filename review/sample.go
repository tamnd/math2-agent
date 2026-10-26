package review

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

// The shares of the bindings that the re-review sample of a month draws (spec 10.8).
const (
	RateF2 = 0.01  // 1% of the bindings at F2
	RateF3 = 0.005 // 0.5% of the bindings at F3
)

// SamplesFile is the register of the re-review samples, from the root of the tree.
const SamplesFile = "registers/samples.reg"

// Sample is one record of registers/samples.reg: a binding that the re-review sample of a month drew, and the reviewer who does the re-review.
type Sample struct {
	Month    string          `json:"month"` // YYYY-MM
	Seed     string          `json:"seed"`  // the seed of the draw of the month
	Tag      string          `json:"tag"`
	Decl     string          `json:"decl"`     // the declaration that the reviews read: the adapter, or the declaration
	Level    status.Fidelity `json:"level"`    // the fidelity level at the draw, F2 or F3
	First    []string        `json:"first"`    // the ids of the reviews that counted at the draw
	Reviewer string          `json:"reviewer"` // the reviewer who does the re-review, or "" when no reviewer can do it
}

var monthRE = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// Draw draws the re-review sample of the month from the build: RateF2 of the bindings at F2 and RateF3 of the bindings at F3, at least one of each level that has a binding. The draw depends only on the seed, the month and the bindings, thus anyone can draw the same sample again.
//
// Each binding gets a score, the SHA-256 hash of the seed, the month, the tag and the declaration, and the draw takes the bindings with the lowest scores. Each binding then gets a reviewer of the register who did not write the binding and did not review it. Draw takes a reviewer with an area of the node first, then a reviewer with no affiliation of a first reviewer, then the reviewer with the fewest re-reviews of the month. A tie goes to the lowest score of the reviewer.
func Draw(in Input, reviewers map[string]status.Reviewer, month, seed string) ([]Sample, error) {
	if !monthRE.MatchString(month) {
		return nil, fmt.Errorf("review: %q is not a month, YYYY-MM", month)
	}
	if seed == "" || strings.ContainsAny(seed, " \t\n#") {
		return nil, fmt.Errorf("review: the seed %q is empty or has a space or a #", seed)
	}
	type drawn struct {
		Sample
		score  [32]byte
		msc    []string
		author string
		seen   []string // the reviewers of each review of the binding
	}
	msc := map[string][]string{}
	for _, n := range in.Nodes {
		msc[n.ID] = n.List("msc")
	}
	levels := map[status.Fidelity][]drawn{}
	for _, s := range in.Status {
		if s.Retired {
			continue
		}
		for _, b := range s.Bindings {
			if b.Fidelity != status.F2 && b.Fidelity != status.F3 {
				continue
			}
			d := drawn{Sample: Sample{Month: month, Seed: seed, Tag: s.Tag, Decl: b.Checked, Level: b.Fidelity, First: []string{}}, score: score(seed, month, s.Tag, b.Checked), msc: msc[s.Tag], author: in.Authors[status.Key{Tag: s.Tag, Decl: b.Decl}]}
			for _, r := range b.Reviews {
				if r.State == status.Counts {
					d.First = append(d.First, r.ID)
				}
				if r.Reviewer != "" {
					d.seen = append(d.seen, r.Reviewer)
				}
			}
			levels[b.Fidelity] = append(levels[b.Fidelity], d)
		}
	}
	var picked []drawn
	for _, l := range []struct {
		level status.Fidelity
		rate  float64
	}{{status.F2, RateF2}, {status.F3, RateF3}} {
		all := levels[l.level]
		slices.SortFunc(all, func(a, b drawn) int { return bytes.Compare(a.score[:], b.score[:]) })
		n := int(math.Ceil(l.rate * float64(len(all))))
		picked = append(picked, all[:n]...)
	}
	slices.SortFunc(picked, func(a, b drawn) int { return bytes.Compare(a.score[:], b.score[:]) })

	ids := make([]string, 0, len(reviewers))
	for id := range reviewers {
		if Person(id) {
			ids = append(ids, id)
		}
	}
	load := map[string]int{}
	out := make([]Sample, 0, len(picked))
	for _, d := range picked {
		type candidate struct {
			id          string
			area, indep bool
			score       [32]byte
		}
		var cs []candidate
		for _, id := range ids {
			if id == d.author || slices.Contains(d.seen, id) {
				continue
			}
			r := reviewers[id]
			c := candidate{id: id, area: inAreas(d.msc, r.Areas), indep: true, score: score(seed, month, d.Tag, d.Decl, id)}
			for _, f := range d.seen {
				if g, ok := reviewers[f]; ok && !r.Independent(g) {
					c.indep = false
				}
			}
			cs = append(cs, c)
		}
		if len(cs) > 0 {
			best := slices.MinFunc(cs, func(a, b candidate) int {
				return cmp.Or(compareTrue(a.area, b.area), compareTrue(a.indep, b.indep), cmp.Compare(load[a.id], load[b.id]), bytes.Compare(a.score[:], b.score[:]))
			})
			d.Reviewer = best.id
			load[best.id]++
		}
		out = append(out, d.Sample)
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
	h.Sum(s[:0])
	return s
}

// compareTrue orders true before false.
func compareTrue(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	}
	return 1
}

// inAreas tells if an MSC2020 code of a node is in one of the areas.
func inAreas(codes, areas []string) bool {
	for _, a := range areas {
		a = strings.TrimRight(a, "xX-")
		if a != "" && slices.ContainsFunc(codes, func(c string) bool { return strings.HasPrefix(c, a) }) {
			return true
		}
	}
	return false
}

// LoadSamples reads registers/samples.reg in the tree root. A later record of the same month, tag and declaration replaces the earlier one, for example to give the re-review to a different reviewer. The result is in the order of the first record of each binding. A tree without the register has no samples.
func LoadSamples(root string) ([]Sample, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(SamplesFile))) // #nosec G304 -- the caller names the root of the tree.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Sample
	at := map[[3]string]int{}
	n := 0
	for line := range strings.Lines(string(data)) {
		n++
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if len(f) != 7 {
			return nil, fmt.Errorf("%s: line %d has %d fields, and a record has the fields month, seed, tag, decl, level, first and reviewer", SamplesFile, n, len(f))
		}
		s := Sample{Month: f[0], Seed: f[1], Tag: f[2], Decl: f[3], First: []string{}}
		if err := s.Level.UnmarshalText([]byte(f[4])); err != nil || (s.Level != status.F2 && s.Level != status.F3) {
			return nil, fmt.Errorf("%s: line %d: the level %q is not F2 or F3", SamplesFile, n, f[4])
		}
		if !monthRE.MatchString(s.Month) {
			return nil, fmt.Errorf("%s: line %d: %q is not a month, YYYY-MM", SamplesFile, n, s.Month)
		}
		if f[5] != "-" {
			s.First = strings.Split(f[5], ",")
		}
		if f[6] != "-" {
			s.Reviewer = f[6]
		}
		k := [3]string{s.Month, s.Tag, s.Decl}
		if i, ok := at[k]; ok {
			out[i] = s
			continue
		}
		at[k] = len(out)
		out = append(out, s)
	}
	return out, nil
}

// AppendSamples adds the samples to registers/samples.reg in the tree root. It makes the register when the tree has none.
func AppendSamples(root string, samples []Sample) error {
	path := filepath.Join(root, filepath.FromSlash(SamplesFile))
	old, err := os.ReadFile(path) // #nosec G304 -- the caller names the root of the tree.
	switch {
	case errors.Is(err, fs.ErrNotExist):
		old = []byte("# month seed tag decl level first reviewer\n")
	case err != nil:
		return err
	}
	var b bytes.Buffer
	b.Write(old)
	if len(old) > 0 && old[len(old)-1] != '\n' {
		b.WriteByte('\n')
	}
	for _, s := range samples {
		first := strings.Join(s.First, ",")
		fmt.Fprintf(&b, "%s %s %s %s %s %s %s\n", s.Month, s.Seed, s.Tag, s.Decl, s.Level, cmp.Or(first, "-"), cmp.Or(s.Reviewer, "-"))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o600)
}

// The states of a re-review.
const (
	Unassigned = "unassigned" // no reviewer can do the re-review
	Open       = "open"       // the reviewer did not do the re-review yet
	Agrees     = "agrees"     // the re-review is faithful or faithful-with-conventions, as the first reviews
	Disagrees  = "disagrees"  // the re-review is not-faithful
)

// Result is the re-review of one sample.
type Result struct {
	Sample
	Area   string `json:"area"`             // the top-level MSC2020 class of the node, for example 20, or - when the node has no code
	State  string `json:"state"`            // unassigned, open, agrees or disagrees
	Review string `json:"review,omitempty"` // the id of the re-review
	Class  string `json:"class,omitempty"`  // the class of the fault of a re-review that disagrees
}

// Rate is the disagreement of the re-reviews of one level, in one area or in all areas.
type Rate struct {
	Level     status.Fidelity `json:"level"`
	Area      string          `json:"area"`      // the top-level MSC2020 class, or "all"
	Drawn     int             `json:"drawn"`     // the samples
	Done      int             `json:"done"`      // the samples with a re-review
	Disagrees int             `json:"disagrees"` // the re-reviews that disagree
}

// Share returns the share of the re-reviews that disagree, from 0 to 1. It is NaN when no re-review is done.
func (r Rate) Share() float64 {
	if r.Done == 0 {
		return math.NaN()
	}
	return float64(r.Disagrees) / float64(r.Done)
}

// Measure finds the re-review of each sample in the reviews, and computes the disagreement of each level, in all areas and in each area. The re-review of a sample is the first review of the binding by the reviewer of the sample, from the first day of the month or later, that is not a first review. A review that read a different declaration is not a re-review.
//
// The rates are in the order of the level, with the rate of all areas first and then the areas in order.
func Measure(samples []Sample, reviews map[string]status.Review, nodes []*node.Node) ([]Result, []Rate) {
	area := map[string]string{}
	for _, n := range nodes {
		if codes := n.List("msc"); len(codes) > 0 && len(codes[0]) >= 2 {
			area[n.ID] = codes[0][:2]
		}
	}
	byTag := map[string][]status.Review{}
	for _, r := range reviews {
		t := Tag(r.ID)
		byTag[t] = append(byTag[t], r)
	}
	for _, rs := range byTag {
		slices.SortFunc(rs, func(a, b status.Review) int {
			return cmp.Or(strings.Compare(a.Date, b.Date), strings.Compare(a.ID, b.ID))
		})
	}
	results := make([]Result, 0, len(samples))
	rates := map[[2]string]*Rate{}
	rate := func(l status.Fidelity, a string) *Rate {
		k := [2]string{l.String(), a}
		if rates[k] == nil {
			rates[k] = &Rate{Level: l, Area: a}
		}
		return rates[k]
	}
	for _, s := range samples {
		res := Result{Sample: s, Area: cmp.Or(area[s.Tag], "-"), State: Open}
		if s.Reviewer == "" {
			res.State = Unassigned
		}
		for _, r := range byTag[s.Tag] {
			if s.Reviewer == "" || r.Reviewer != s.Reviewer || r.Decl != s.Decl || r.Date < s.Month+"-01" || slices.Contains(s.First, r.ID) {
				continue
			}
			res.Review, res.State = r.ID, Agrees
			if r.Verdict == status.NotFaithful {
				res.State, res.Class = Disagrees, r.Class
			}
			break
		}
		for _, x := range []*Rate{rate(s.Level, "all"), rate(s.Level, res.Area)} {
			x.Drawn++
			if res.State == Agrees || res.State == Disagrees {
				x.Done++
			}
			if res.State == Disagrees {
				x.Disagrees++
			}
		}
		results = append(results, res)
	}
	out := make([]Rate, 0, len(rates))
	for _, r := range rates {
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b Rate) int {
		return cmp.Or(cmp.Compare(a.Level, b.Level), compareTrue(a.Area == "all", b.Area == "all"), strings.Compare(a.Area, b.Area))
	})
	return results, out
}
