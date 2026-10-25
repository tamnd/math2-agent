package audit

import (
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
	"go.yaml.in/yaml/v3"
)

// The rules F01 to F05 and F07 to F09 read the files of the last m2 build in build/. A rule skips when a file that it needs does not exist.

// built holds the files of the last build.
type built struct {
	records   map[string]lean.Record   // the records of extract.json, by declaration
	statuses  map[string]status.Status // the statuses of status.json, by tag
	reviews   map[string]status.Review // the reviews of the tree, by id
	manifest  *build.Manifest          // nil when manifest.json does not exist
	noExtract string                   // the reason to skip a rule that needs extract.json
	noStatus  string                   // the reason to skip a rule that needs status.json
	noReviews string                   // the reason to skip a rule that needs the reviews
}

// lastBuild reads the files of the last build one time.
func (a *auditor) lastBuild() *built {
	if a.last != nil {
		return a.last
	}
	dir := filepath.Join(a.root, "build")
	skip := func(file string, err error) string {
		if errors.Is(err, fs.ErrNotExist) {
			return "build/" + file + " does not exist. Run m2 build first"
		}
		return err.Error()
	}
	b := &built{records: map[string]lean.Record{}, statuses: map[string]status.Status{}}
	if x, err := build.ReadExtraction(dir); err != nil {
		b.noExtract = skip(build.ExtractFile, err)
	} else {
		for _, r := range x.Records {
			b.records[r.Decl] = r
		}
	}
	if st, err := build.ReadStatus(dir); err != nil {
		b.noStatus = skip(build.StatusFile, err)
	} else {
		for _, s := range st {
			b.statuses[s.Tag] = s
		}
	}
	var err error
	if b.reviews, err = status.LoadReviews(a.root); err != nil {
		b.noReviews = err.Error()
	}
	if m, err := build.ReadManifest(dir); err == nil {
		b.manifest = m
	}
	a.last = b
	return b
}

// binding is one Lean binding in the front matter of a node.
type binding struct {
	decl, adapter, match string
	reviews              []string // the ids in fidelity_reviews
	line, col            int      // the position of the item in the formal list
}

// checked returns the declaration that the kernel checks: the adapter, or the declaration.
func (b binding) checked() string { return cmp.Or(b.adapter, b.decl) }

// candidate reports whether the binding can give E5: its match is exact, or stronger or variant with an adapter.
func (b binding) candidate() bool {
	return b.match == "exact" || ((b.match == "stronger" || b.match == "variant") && b.adapter != "")
}

// leanBindings returns the Lean bindings of n with their positions.
func leanBindings(n *node.Node) []binding {
	line, col := keyPos(n, "formal")
	var items []*yaml.Node
	if n.Front != nil && len(n.Front.Content) > 0 {
		m := n.Front.Content[0]
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == "formal" {
				items = m.Content[i+1].Content
			}
		}
	}
	list, _ := n.Meta["formal"].([]any)
	var out []binding
	for i, item := range list {
		m, _ := item.(map[string]any)
		if str(m["system"]) != "lean4" || str(m["decl"]) == "" {
			continue
		}
		b := binding{decl: str(m["decl"]), adapter: str(m["adapter"]), match: str(m["match"]), line: line, col: col}
		rs, _ := m["fidelity_reviews"].([]any)
		for _, r := range rs {
			b.reviews = append(b.reviews, str(r))
		}
		if i < len(items) {
			b.line, b.col = items[i].Line, items[i].Column
		}
		out = append(out, b)
	}
	return out
}

// bindingPos returns the position of the Lean binding of decl in n.
func bindingPos(n *node.Node, decl string) (line, col int) {
	for _, b := range leanBindings(n) {
		if b.decl == decl {
			return b.line, b.col
		}
	}
	return keyPos(n, "formal")
}

// f01 checks that each Lean binding resolves to a declaration at the pinned commit (rule F01). The extraction of the last build must have a record without an error for each declaration and adapter, and the build must be at the Mathlib commit that lean/lake-manifest.json pins.
func f01(a *auditor) outcome {
	var o outcome
	b := a.lastBuild()
	if b.noExtract != "" {
		o.reason = b.noExtract
		return o
	}
	if pinned := a.pinnedMathlib(); b.manifest != nil && pinned != "" && b.manifest.Mathlib != pinned {
		o.add("lean/lake-manifest.json", 1, 1, "the last build is at the Mathlib commit %s, and lean/lake-manifest.json pins %s. Run m2 build", cmp.Or(b.manifest.Mathlib, "none"), pinned)
	}
	for _, n := range a.scopeNodes() {
		if retired(n) {
			continue
		}
		for _, bd := range leanBindings(n) {
			for _, d := range []string{bd.decl, bd.adapter} {
				if d == "" {
					continue
				}
				o.checked++
				r, ok := b.records[d]
				switch {
				case !ok:
					o.add(n.Path, bd.line, bd.col, "build/%s has no record of %s. Run m2 build", build.ExtractFile, d)
				case r.Error != "":
					o.add(n.Path, bd.line, bd.col, "%s does not resolve at the pinned commit: %s. Correct the name, or record the rename in registers/bindings.reg", d, r.Error)
				}
			}
		}
	}
	return o
}

// pinnedMathlib returns the Mathlib commit in lean/lake-manifest.json, or "" when the tree has no such file.
func (a *auditor) pinnedMathlib() string {
	data, err := a.readFile("lean/lake-manifest.json")
	if err != nil || data == nil {
		return ""
	}
	var lm struct {
		Packages []struct {
			Name string `json:"name"`
			Rev  string `json:"rev"`
		} `json:"packages"`
	}
	if json.Unmarshal(data, &lm) != nil {
		return ""
	}
	for _, p := range lm.Packages {
		if p.Name == "mathlib" {
			return p.Rev
		}
	}
	return ""
}

// f02 checks that the status counts no stale review (rule F02). A review that status.json counts must have read the declaration that the kernel checks, and its two hashes must be the hashes of the build.
func f02(a *auditor) outcome {
	var o outcome
	b := a.lastBuild()
	if reason := cmp.Or(b.noStatus, b.noReviews); reason != "" {
		o.reason = reason
		return o
	}
	for _, n := range a.scopeNodes() {
		s, ok := b.statuses[n.ID]
		if !ok || retired(n) {
			continue
		}
		for _, sb := range s.Bindings {
			for _, rs := range sb.Reviews {
				o.checked++
				r, ok := b.reviews[rs.ID]
				if !ok || rs.State != status.Counts {
					continue
				}
				line, col := bindingPos(n, sb.Decl)
				switch {
				case r.Decl != sb.Checked:
					o.add(n.Path, line, col, "build/%s counts the review %s for %s, and the review read %s. Run m2 build", build.StatusFile, r.ID, sb.Checked, r.Decl)
				case r.TypeHash != sb.TypeHash:
					o.add(n.Path, line, col, "build/%s counts the review %s, and the review records the type hash %s. The build gives %s, thus the review is stale. Run m2 build", build.StatusFile, r.ID, r.TypeHash, sb.TypeHash)
				case r.StatementHash != status.StatementHash(n):
					o.add(n.Path, line, col, "build/%s counts the review %s, and the statement section changed after the review, thus the review is stale. Run m2 build", build.StatusFile, r.ID)
				}
			}
		}
	}
	return o
}

// f03 checks the axioms of each E5 candidate (rule F03). A candidate can use the three standard axioms and the named hypotheses in Math2.Hypotheses. A named hypothesis makes the result conditional, and the status gives the flag conditional.
func f03(a *auditor) outcome {
	var o outcome
	b := a.lastBuild()
	if b.noExtract != "" {
		o.reason = b.noExtract
		return o
	}
	for _, n := range a.scopeNodes() {
		if retired(n) {
			continue
		}
		for _, bd := range leanBindings(n) {
			r, ok := b.records[bd.checked()]
			if !bd.candidate() || !ok || r.Error != "" || r.Kind != "theorem" {
				continue
			}
			o.checked++
			var bad []string
			for _, ax := range r.Axioms {
				if !slices.Contains(lean.StandardAxioms, ax) && !strings.HasPrefix(ax, status.HypothesisPrefix) {
					bad = append(bad, ax)
				}
			}
			switch {
			case slices.Contains(bad, "sorryAx"):
				o.add(n.Path, bd.line, bd.col, "the E5 candidate %s uses sorry. Complete the proof, or set match: partial for a statement without a proof", r.Decl)
			case len(bad) > 0:
				o.add(n.Path, bd.line, bd.col, "the E5 candidate %s uses the axioms %s. E5 allows only %s and the named hypotheses in %s", r.Decl, strings.Join(bad, ", "), strings.Join(lean.StandardAxioms, ", "), strings.TrimSuffix(status.HypothesisPrefix, "."))
			}
		}
	}
	return o
}

// f04 reports each bound declaration whose closure has native_decide axioms (rule F04). Such a binding is at most E4.
func f04(a *auditor) outcome {
	var o outcome
	b := a.lastBuild()
	if b.noExtract != "" {
		o.reason = b.noExtract
		return o
	}
	for _, n := range a.scopeNodes() {
		if retired(n) {
			continue
		}
		for _, bd := range leanBindings(n) {
			r, ok := b.records[bd.checked()]
			if !ok || r.Error != "" {
				continue
			}
			o.checked++
			if k := len(r.NativeDecideAxioms); k == 1 {
				o.add(n.Path, bd.line, bd.col, "the closure of %s has the native_decide axiom %s. The level is at most E4", r.Decl, r.NativeDecideAxioms[0])
			} else if k > 1 {
				o.add(n.Path, bd.line, bd.col, "the closure of %s has %d native_decide axioms, for example %s. The level is at most E4", r.Decl, k, r.NativeDecideAxioms[0])
			}
		}
	}
	return o
}

// f05 checks that each binding at E5 in status.json passed the replay, with the version of each checker recorded (rule F05). E5 needs two kernels.
func f05(a *auditor) outcome {
	var o outcome
	b := a.lastBuild()
	if b.noStatus != "" {
		o.reason = b.noStatus
		return o
	}
	for _, n := range a.scopeNodes() {
		s, ok := b.statuses[n.ID]
		if !ok || retired(n) {
			continue
		}
		for _, sb := range s.Bindings {
			if sb.Level != status.Kernel {
				continue
			}
			o.checked++
			line, col := bindingPos(n, sb.Decl)
			switch {
			case sb.Replay == nil || !sb.Replay.Passed:
				o.add(n.Path, line, col, "the binding %s is at E5 in build/%s, and it has no passed replay. Run m2 build", sb.Checked, build.StatusFile)
			case versions(sb.Replay.Checkers) < 2:
				o.add(n.Path, line, col, "the binding %s is at E5. E5 needs the versions of two kernels, and the replay records %d", sb.Checked, versions(sb.Replay.Checkers))
			}
		}
	}
	return o
}

// versions counts the checkers with a version.
func versions(checkers map[string]string) int {
	k := 0
	for _, v := range checkers {
		if v != "" {
			k++
		}
	}
	return k
}

// f07 checks that no statement-only binding has a level in status.json (rule F07).
func f07(a *auditor) outcome {
	var o outcome
	b := a.lastBuild()
	if b.noStatus != "" {
		o.reason = b.noStatus
		return o
	}
	for _, n := range a.scopeNodes() {
		s, ok := b.statuses[n.ID]
		if !ok || retired(n) {
			continue
		}
		for _, sb := range s.Bindings {
			if sb.Match != "partial" {
				continue
			}
			o.checked++
			if sb.Level != status.None {
				line, col := bindingPos(n, sb.Decl)
				o.add(n.Path, line, col, "the statement-only binding %s (match: partial) is at %s in build/%s. A statement without a proof never gives E4 or E5. Run m2 build", sb.Checked, sb.Level, build.StatusFile)
			}
		}
	}
	return o
}

// f08 reports each bound declaration and adapter that Mathlib deprecates, with the new name (rule F08).
func f08(a *auditor) outcome {
	var o outcome
	b := a.lastBuild()
	if b.noExtract != "" {
		o.reason = b.noExtract
		return o
	}
	for _, n := range a.scopeNodes() {
		if retired(n) {
			continue
		}
		for _, bd := range leanBindings(n) {
			for _, d := range []string{bd.decl, bd.adapter} {
				r, ok := b.records[d]
				if d == "" || !ok || r.Error != "" {
					continue
				}
				o.checked++
				dep := r.Deprecated
				if dep == nil {
					continue
				}
				since := ""
				if dep.Since != "" {
					since = " since " + dep.Since
				}
				if dep.NewName != "" {
					o.add(n.Path, bd.line, bd.col, "%s is deprecated%s. Bind %s, and record the rename in registers/bindings.reg", d, since, dep.NewName)
				} else {
					o.add(n.Path, bd.line, bd.col, "%s is deprecated%s, and Mathlib gives no new name: %s", d, since, cmp.Or(dep.Text, "no message"))
				}
			}
		}
	}
	return o
}

// f09 checks that each binding at F1 or higher in status.json passes the checks for degenerate statements in extract.json (rule F09, spec 10.4). The hypotheses must not prove False, and automation must not prove the negation. The rule does not fail on a statement that automation proves, because a trivial statement can be the correct one. Spec 10.4 puts such a statement in the review packet.
func f09(a *auditor) outcome {
	var o outcome
	b := a.lastBuild()
	if reason := cmp.Or(b.noExtract, b.noStatus); reason != "" {
		o.reason = reason
		return o
	}
	for _, n := range a.scopeNodes() {
		s, ok := b.statuses[n.ID]
		if !ok || retired(n) {
			continue
		}
		for _, sb := range s.Bindings {
			r, ok := b.records[sb.Checked]
			if sb.Fidelity < status.F1 || !ok || r.Error != "" || r.Kind != "theorem" {
				continue
			}
			o.checked++
			line, col := bindingPos(n, sb.Decl)
			switch c := r.Checks; {
			case c == nil:
				o.add(n.Path, line, col, "build/%s has no checks for %s. Run m2 build", build.ExtractFile, sb.Checked)
			case c.Vacuous != "":
				o.add(n.Path, line, col, "%s proves False from the hypotheses of %s, thus the statement is vacuous. Correct the hypotheses", c.Vacuous, sb.Checked)
			case c.Refuted != "":
				o.add(n.Path, line, col, "%s proves the negation of %s, thus the statement is false. Correct the statement", c.Refuted, sb.Checked)
			}
		}
	}
	return o
}
