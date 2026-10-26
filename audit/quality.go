package audit

import (
	"bytes"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/sample"
	"github.com/tamnd/math2-agent/sources"
)

// The rules L05, P05, P06, X05, X06 and X07 check the text of agents against the gates of corpus K9. A field is agent-written when the last line of its provenance log that adds or changes it is of an agent with no person: a line on the field, on a field that contains it, or on the whole node. A person who confirms the field later does not make it the text of a person. The run of that line keeps the results of the gates in its summary in build/runs/: the copy check (L05), the sources that the run claimed (P05) and the sanity checks (X05).

// written is a field of a node that an agent wrote, with the line of the provenance log.
type written struct {
	field string
	line  logLine
}

// agentWritten returns the fields of n that an agent wrote, in the order of fields.
func (a *auditor) agentWritten(n *node.Node, fields []string) []written {
	data, _ := a.readFile(prov.Path(n.ID))
	log := readLog(data)
	var out []written
	for _, f := range fields {
		for _, l := range slices.Backward(log) {
			if l.err != nil || l.line.Op != prov.Add && l.line.Op != prov.Change || !prov.Under(f, l.line.Field) {
				continue
			}
			if l.line.Agent != "" && l.line.Human == "" {
				out = append(out, written{f, l})
			}
			break
		}
	}
	return out
}

// bodyFields returns the body fields of n that have text, sorted.
func bodyFields(n *node.Node) []string {
	var out []string
	for f, v := range prov.Fields(n) {
		if (f == "body" || strings.HasPrefix(f, "body.")) && strings.TrimSpace(v) != "" {
			out = append(out, f)
		}
	}
	slices.Sort(out)
	return out
}

// sourceFields returns the fields sources[0], sources[1] and on of n, one for each source.
func sourceFields(n *node.Node) []string {
	out := make([]string, len(metaMaps(n.Meta["sources"])))
	for i := range out {
		out[i] = fmt.Sprintf("sources[%d]", i)
	}
	return out
}

// itemPos returns the position of the item i of the list under the top-level key, or of its key sub when sub is not empty. It returns the position of the key when the list has no such item.
func itemPos(n *node.Node, key string, i int, sub string) (line, col int) {
	if n.Front == nil || len(n.Front.Content) == 0 {
		return 1, 1
	}
	m := n.Front.Content[0]
	for k := 0; k+1 < len(m.Content); k += 2 {
		if m.Content[k].Value != key || i >= len(m.Content[k+1].Content) {
			continue
		}
		it := m.Content[k+1].Content[i]
		for j := 0; sub != "" && j+1 < len(it.Content); j += 2 {
			if it.Content[j].Value == sub {
				return it.Content[j].Line, it.Content[j].Column
			}
		}
		return it.Line, it.Column
	}
	return keyPos(n, key)
}

// sourceID returns the system and the id of a source of a node: the key of the identifier and its value.
func sourceID(m map[string]any) (system, id string) {
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if k != "locator" && k != "note" && k != "locator_unconfirmed" {
			return k, fmt.Sprint(m[k])
		}
	}
	return "", ""
}

// agentRun is a field that an agent wrote, with the summary of its run.
type agentRun struct {
	n   *node.Node
	w   written
	run *prov.Run
}

// agentRuns returns the fields of the nodes in scope that an agent wrote in a run, with the summary of the run, in the order of the nodes and of the fields. A run that the artefact store does not have is not in the result: rule P02 reports it. The reason is not empty when the rule must skip.
func (a *auditor) agentRuns(fields func(*node.Node) []string) ([]agentRun, string) {
	var ws []agentRun
	for _, n := range a.scopeNodes() {
		for _, w := range a.agentWritten(n, fields(n)) {
			if w.line.line.Run != "" {
				ws = append(ws, agentRun{n: n, w: w})
			}
		}
	}
	if len(ws) == 0 {
		return nil, ""
	}
	runs, reason := a.runs()
	if reason != "" {
		return nil, reason
	}
	out := ws[:0]
	for _, w := range ws {
		if w.run = runs[w.w.line.line.Run]; w.run != nil {
			out = append(out, w)
		}
	}
	return out, ""
}

// l05 checks that each body field that an agent wrote has a copy check result in the record of its run, and that the result has no match that blocks (rule L05, corpus K9.4). A match with a source that is not open blocks. The last result of the node in the run counts, because the agent can run the check again after a repair. L05 is soft until the calibration of corpus K9.4 sets the thresholds of the copy check.
func l05(a *auditor) outcome {
	var o outcome
	ws, reason := a.agentRuns(bodyFields)
	if reason != "" {
		o.reason = reason
		return o
	}
	for _, w := range ws {
		o.checked++
		line, col := fieldPos(w.n, w.w.field)
		id := w.w.line.line.Run
		var cc *prov.CopyCheck
		for i := range slices.Backward(w.run.Copy) {
			if w.run.Copy[i].Node == w.n.ID {
				cc = &w.run.Copy[i]
				break
			}
		}
		if cc == nil {
			o.add(w.n.Path, line, col, "%s of %s is from the run %s, and the record of the run has no copy check result for %s. Run the copy check of corpus K9.4 before the pull request opens", w.w.field, w.n.ID, id, w.n.ID)
			continue
		}
		for _, m := range cc.Matches {
			if m.Field == w.w.field && m.Blocks() {
				o.add(w.n.Path, line, col, "the copy check of the run %s found that %s copies %s %s (class %s) at the bytes %d to %d. Write the text again", id, w.w.field, m.Source, m.URI, m.Class, m.Start, m.End)
			}
		}
	}
	return o
}

// p05 checks that each source that an agent added resolves to a record of the record plane, and that the record matches the claim of the run (rule P05, corpus K9.5). The run claims the title, the authors and the year of each source in its record. A source of a system that the record plane has no records of is not checked, because the audit cannot resolve it.
func p05(a *auditor) outcome {
	var o outcome
	ws, reason := a.agentRuns(sourceFields)
	if reason != "" {
		o.reason = reason
		return o
	}
	if len(ws) == 0 {
		return o
	}
	im := a.importsOf()
	if im.noRecords != "" {
		o.reason = im.noRecords
		return o
	}
	plane := &sources.Plane{}
	for _, r := range im.records {
		plane.Add(im.source[r], *r)
	}
	for _, w := range ws {
		var i int
		if _, err := fmt.Sscanf(w.w.field, "sources[%d]", &i); err != nil {
			continue
		}
		system, id := sourceID(metaMaps(w.n.Meta["sources"])[i])
		if !plane.Knows(system) {
			continue
		}
		o.checked++
		line, col := itemPos(w.n, "sources", i, "")
		run := w.w.line.line.Run
		var claim *prov.SourceClaim
		for k := range slices.Backward(w.run.Sources) {
			if c := w.run.Sources[k]; c.Node == w.n.ID && c.System == system && c.ID == id {
				claim = &w.run.Sources[k]
				break
			}
		}
		if claim == nil {
			o.add(w.n.Path, line, col, "the run %s added the source %s %s, and the record of the run has no claim for it. The run must confirm each source that it adds (corpus K9.5)", run, system, id)
			continue
		}
		res, err := sources.Confirm(plane, *claim)
		switch {
		case err != nil:
			o.add(w.n.Path, line, col, "the source %s %s: %v", system, id, err)
		case res.Verdict == sources.Fabricated:
			o.add(w.n.Path, line, col, "the run %s added the source %s %s, and the record plane has no record of it. The source is fabricated: remove it", run, system, id)
		case res.Verdict == sources.Mismatch:
			o.add(w.n.Path, line, col, "the run %s added the source %s %s, and the record does not match the claim of the run: %s", run, system, id, strings.Join(res.Problems, "; "))
		}
	}
	return o
}

// locatorAge is the age in days of a locator_unconfirmed flag that rule P06 reports.
const locatorAge = 180

// p06 reports each source whose locator nobody confirmed for more than locatorAge days (rule P06, corpus K9.5). The flag locator_unconfirmed gives the date when the source got it, and the age is measured to the date of the HEAD commit.
func p06(a *auditor) outcome {
	var o outcome
	type flag struct {
		n    *node.Node
		i    int
		date any
	}
	var flags []flag
	for _, n := range a.scopeNodes() {
		for i, m := range metaMaps(n.Meta["sources"]) {
			if d, ok := m["locator_unconfirmed"]; ok {
				flags = append(flags, flag{n, i, d})
			}
		}
	}
	if len(flags) == 0 {
		return o
	}
	if !isRepo(a.root) {
		o.reason = "the tree is not a git repository, thus the audit has no date to measure the age of a flag to"
		return o
	}
	head, err := headDate(a.root)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	for _, f := range flags {
		o.checked++
		line, col := itemPos(f.n, "sources", f.i, "locator_unconfirmed")
		system, id := sourceID(metaMaps(f.n.Meta["sources"])[f.i])
		d, ok := f.date.(time.Time)
		if !ok {
			if d, err = time.Parse(time.DateOnly, fmt.Sprint(f.date)); err != nil {
				o.add(f.n.Path, line, col, "the flag locator_unconfirmed of the source %s %s is %q, and it must be the date when the source got the flag, YYYY-MM-DD", system, id, fmt.Sprint(f.date))
				continue
			}
		}
		if days := int(head.Sub(d).Hours() / 24); days > locatorAge {
			o.add(f.n.Path, line, col, "nobody confirmed the locator of the source %s %s in %d days, since %s. Confirm the locator in the source, or remove the source", system, id, days, d.Format(time.DateOnly))
		}
	}
	return o
}

// x05 checks that each statement and proof that an agent wrote has the results of the sanity checks of corpus K9.3 in the record of its run (rule X05). Each check needs a result for the node and the field: pass, fail or n/a. A failure does not fail the rule: the gate is soft, and the failure opens a job for a person.
func x05(a *auditor) outcome {
	var o outcome
	ws, reason := a.agentRuns(func(n *node.Node) []string {
		return slices.DeleteFunc(bodyFields(n), func(f string) bool { return f != "body.statement" && f != "body.proof" })
	})
	if reason != "" {
		o.reason = reason
		return o
	}
	for _, w := range ws {
		o.checked++
		line, col := fieldPos(w.n, w.w.field)
		run := w.w.line.line.Run
		have := map[string]bool{}
		for _, s := range w.run.Sanity {
			if s.Node != w.n.ID || s.Field != w.w.field {
				continue
			}
			switch {
			case !slices.Contains(prov.SanityChecks, s.Check):
				o.add(w.n.Path, line, col, "the record of the run %s has the sanity check %q for %s, and corpus K9.3 has no such check", run, s.Check, w.w.field)
			case s.Result != prov.SanityPass && s.Result != prov.SanityFail && s.Result != prov.SanityNA:
				o.add(w.n.Path, line, col, "the record of the run %s has the result %q of the sanity check %s for %s. Give pass, fail or n/a", run, s.Result, s.Check, w.w.field)
			default:
				have[s.Check] = true
			}
		}
		var missing []string
		for _, c := range prov.SanityChecks {
			if !have[c] {
				missing = append(missing, c)
			}
		}
		if len(missing) > 0 {
			o.add(w.n.Path, line, col, "%s of %s is from the run %s, and the record of the run has no result of the sanity checks %s (corpus K9.3)", w.w.field, w.n.ID, run, strings.Join(missing, ", "))
		}
	}
	return o
}

// x06 reports each stratum of the sample audit whose error rate is near its threshold or above it (rule X06, corpus K9.7). The state of a rate comes from the upper end of its 95% interval (package sample). The rates are of the audits of the last month of registers/audits.reg. A stratum with fewer than sample.MinSample audits in that month pools the audits of the last three months.
func x06(a *auditor) outcome {
	var o outcome
	data, err := a.readFile(sample.File)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	if data == nil {
		o.reason = sample.File + " does not exist, thus no sample audit has been drawn"
		return o
	}
	audits, err := sample.Parse(data)
	if err != nil {
		o.add(sample.File, 0, 0, "%v", err)
		return o
	}
	last := ""
	for _, x := range audits {
		if x.Done {
			last = max(last, x.Month)
		}
	}
	if last == "" {
		return o
	}
	first := monthsBefore(last, 2)
	inMonth := map[sample.Stratum]int{}
	for _, x := range audits {
		if x.Done && x.Month == last {
			inMonth[x.Stratum]++
		}
	}
	var window []sample.Audit
	for _, x := range audits {
		if x.Month == last || x.Month >= first && x.Month < last && inMonth[x.Stratum] < sample.MinSample {
			window = append(window, x)
		}
	}
	for _, r := range sample.Rates(window) {
		o.checked++
		if r.State != sample.Near && r.State != sample.Above {
			continue
		}
		line := 1
		if i := bytes.LastIndex(data, []byte(" "+r.Stratum.String()+" ")); i >= 0 {
			line = bytes.Count(data[:i], []byte("\n")) + 1
		}
		o.add(sample.File, line, 1, "in the stratum %s, %d of %d audits to %s found an error of the property %s. The upper end of the interval of the error rate is %.1f%%, %s the threshold of %.0f%%", r.Stratum, r.Errors, r.Audits, last, r.Property, 100*r.Upper, map[sample.State]string{sample.Near: "near", sample.Above: "above"}[r.State], 100*r.Property.Threshold())
	}
	return o
}

// monthsBefore returns the month k months before the month m, YYYY-MM.
func monthsBefore(m string, k int) string {
	t, err := time.Parse("2006-01", m)
	if err != nil {
		return m
	}
	return t.AddDate(0, -k, 0).Format("2006-01")
}

// The names of theorems that rule X07 finds: "Lagrange's theorem", "the Cauchy-Schwarz inequality" and "the theorem of Cauchy and Kovalevskaya".
var (
	theoremKind  = `(?:theorem|lemma|inequality|criterion|conjecture|formula|principle)s?`
	personName   = `\p{Lu}\p{L}+(?:[-–]\p{Lu}\p{L}+)*`
	theoremNames = []*regexp.Regexp{
		regexp.MustCompile(`(` + personName + `)['’]s ` + theoremKind + `\b`),
		regexp.MustCompile(`\b[Tt]he (` + personName + `) ` + theoremKind + `\b`),
		regexp.MustCompile(`\b[Tt]he ` + theoremKind + ` of (` + personName + `(?: and ` + personName + `)?)`),
	}
	// notProse is the text that is not prose for X07: formulas, code and links. A name in the text of a link resolves to the target of the link.
	parenthesis = regexp.MustCompile(`\([^)]*\)`)
	notProse    = regexp.MustCompile(`\$\$[\s\S]*?\$\$|\$[^$\n]*\$|\\\([\s\S]*?\\\)|\\\[[\s\S]*?\\\]|` + "`[^`\n]*`" + `|\[\[[^\]]*\]\]|\[[^\]]*\]\([^)]*\)`)
)

// nameKey returns the words of a name of a theorem that identify it, sorted, in lower case and with no accents: "Lagrange's theorem" and "the theorem of Lagrange" both give "lagrange theorem". The text in parentheses is not part of the name.
func nameKey(s string) string {
	s = parenthesis.ReplaceAllString(s, " ")
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var out []string
	for _, w := range words {
		switch w {
		case "the", "of", "s", "and", "a":
			continue
		}
		out = append(out, strings.TrimSuffix(w, "s"))
	}
	slices.Sort(out)
	return strings.Join(out, " ")
}

// x07 finds the names of theorems in the body fields that an agent wrote that resolve to no node and to no source (rule X07, corpus K9.3). A name resolves when it is the name or an alias of a node, when it is in the text of a link, or when the note or the locator of a source of the node names each person of the name. Models invent theorems that match what they want to prove.
func x07(a *auditor) outcome {
	var o outcome
	known := map[string]bool{}
	for _, n := range a.nodes {
		if name := str(n.Meta["name"]); name != "" {
			known[nameKey(name)] = true
		}
		for _, alias := range metaList(n.Meta["aliases"]) {
			known[nameKey(alias)] = true
		}
	}
	for _, n := range a.scopeNodes() {
		ws := a.agentWritten(n, bodyFields(n))
		if len(ws) == 0 {
			continue
		}
		var cited strings.Builder
		for _, m := range metaMaps(n.Meta["sources"]) {
			cited.WriteString(strings.ToLower(str(m["note"]) + " " + str(m["locator"]) + " "))
		}
		text := prov.Fields(n)
		for _, w := range ws {
			o.checked++
			prose := notProse.ReplaceAllString(text[w.field], " ")
			seen := map[string]bool{}
			for _, re := range theoremNames {
				for _, m := range re.FindAllStringSubmatch(prose, -1) {
					key := nameKey(m[0])
					if known[key] || seen[key] || citedNames(cited.String(), m[1]) {
						continue
					}
					seen[key] = true
					line, col := fieldPos(n, w.field)
					o.add(n.Path, line, col, "%s of %s names %q, and no node has the name and no source of the node names it. Link the theorem, or give its source", w.field, n.ID, m[0])
				}
			}
		}
	}
	return o
}

// citedNames reports whether the text of the sources has each person of the name of a theorem.
func citedNames(cited, persons string) bool {
	words := strings.FieldsFunc(strings.ToLower(persons), func(r rune) bool { return !unicode.IsLetter(r) })
	return slices.ContainsFunc(words, func(w string) bool { return w != "and" }) && !slices.ContainsFunc(words, func(w string) bool {
		return w != "and" && !strings.Contains(cited, w)
	})
}

// metaList returns the strings of a list of the front matter.
func metaList(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
