package inventory

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/tag"
)

// Kinds returns the kinds of spec 04.2 that a mention or a row can have, and Unknown.
func Kinds() []string {
	return []string{"concept", "definition", "statement", "conjecture", "problem", "example", "counterexample", "object", "proof", "computation", "topic", "exercise", Unknown}
}

// Options controls one run of Cluster.
type Options struct {
	Day  time.Time                                    // the day of the run, for the history of the rows
	By   string                                       // the run or the person, for the history of the rows
	Used func(id string) bool                         // reports whether an identifier is a node tag or is in the register. A new row never gets one.
	Mint func(used func(string) bool) (string, error) // gives a new row identifier. Nil means tag.Mint with crypto/rand.
	// Signals are the facts outside the mentions for the formal rules of corpus K4.5 and the rule research-aligned. The zero value means that only the mentions of a row decide.
	Signals Signals
	// NameJudge is the dedup judge of the name pass (corpus arxiv AX8.2): it reports whether the research mention and the mention of another source with its name are not different. Nil means that the name pass joins nothing, and its matches stay candidates for the judge.
	NameJudge func(mention, target *Mention) bool
}

// Result is what one run of Cluster did.
type Result struct {
	Rows    []Row    // all the rows, in the order of their identifiers
	Added   []string // the identifiers of the new rows
	Merged  int      // the number of rows that the run merged into other rows
	Touched int      // the number of old rows that got new mentions
	Waiting int      // the number of groups of mentions that wait, because only models give them (rule I03)
	// Cited is the number of joins of pass 1 by a cited result: each join puts two groups into one group, because a mention states the result of another mention with the confidence high (corpus arxiv AX5.7 and AX8.2).
	Cited int
	// Hashed is the number of joins of the hash pass: each join puts two groups into one group, because they have items of one kind and one block with the same content hash (corpus arxiv AX8.2).
	Hashed int
	// Equivalent is the number of joins of pass 2: each join puts two groups of pass 1 into one group, because they have statements with the same canonical type.
	Equivalent int
	// Named is the number of joins of the name pass: each join puts a mention of the research layer into the group of a mention of another source with its name (corpus arxiv AX8.2 and AX8.4).
	Named   int
	Orphans int // the number of attached mentions whose target has no row
	// Formal is the number of rows whose outcome the formal rules changed: a new outcome formal_only, or formal_only open again because a test of corpus K4.5 now selects the row.
	Formal int
	// Aligned is the number of rows of the research layer that got the outcome aligned, or lost it, because of the aligned edges of the nodes.
	Aligned int
}

// Cluster runs the first two passes of corpus spec K4.3 on the mentions. Pass 1 joins two mentions with the same identifier, or with identifiers that a crosswalk links. It also joins two mentions of one kind and one block with the same content hash, which is the same item copied from paper to paper (corpus arxiv AX8.2). The block of a mention is its MSC codes at two digits (AX8.1), and two mentions with no code are in one block. Pass 2 joins two statements of formal libraries with the same canonical type hash: Lean then proves each from the other with the identity, so they are one statement. Pass 2 never joins definitions, because two definitions with one type, such as two functions from ℕ to ℕ, are not one item. A row that a pass merged names the pass as its rule: identifiers, hash or equivalence. Mentions that a row holds stay in one row, so Cluster never splits a row. Only a curator splits a row. When one group of mentions touches more than one row, Cluster keeps one row and gives the others the outcome merged.
//
// Cluster gives each new row the outcome of the first rule of Rules that matches it. Then it applies the formal rules of corpus K4.5 to each row of a formal declaration, old or new, that is open or formal_only: such a row stays open while a test selects it for a node, and gets the outcome formal_only when no test does. Last, a row of the research layer that is open or a record by a rule gets the outcome aligned when a node has an aligned edge to one of its mentions (the rule research-aligned). A group of mentions that only models give gets no row (rule I03). The mentions of the group wait until a source confirms them.
func Cluster(rows []Row, ms []Mention, opt Options) (*Result, error) {
	mint := opt.Mint
	if mint == nil {
		mint = func(used func(string) bool) (string, error) { return tag.Mint(nil, used) }
	}
	day := opt.Day.UTC().Format(time.DateOnly)
	byID := map[string]*Mention{}
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	rows = slices.Clone(rows)
	sortRows(rows)
	u := unionFind{}
	owner := map[string]int{} // the index of the row of each mention
	for i, r := range rows {
		if r.Outcome != nil && r.Outcome.Value == OutcomeMerged {
			continue
		}
		var first string
		for _, id := range r.Mentions {
			if byID[id] == nil {
				continue
			}
			owner[id] = i
			if first == "" {
				first = id
			} else {
				u.union(first, id)
			}
		}
	}
	joinIdentifiers(u, ms)
	cited := joinCited(u, ms, byID)
	pass1 := u.roots(ms)
	res := &Result{Cited: cited, Hashed: joinHash(u, ms)}
	hashed := u.roots(ms)
	res.Equivalent = joinEquivalent(u, ms)
	equivalent := u.roots(ms)
	res.Named = joinNames(u, ms, opt.NameJudge)
	groups := map[string][]string{}
	for i := range ms {
		if ms[i].Attached() {
			continue
		}
		r := u.find(ms[i].ID)
		groups[r] = append(groups[r], ms[i].ID)
	}
	added := map[string]bool{}
	used := func(id string) bool { return added[id] || opt.Used != nil && opt.Used(id) }
	for _, root := range slices.Sorted(maps.Keys(groups)) {
		ids := groups[root]
		slices.Sort(ids)
		var touched []int
		for _, id := range ids {
			if i, ok := owner[id]; ok && !slices.Contains(touched, i) {
				touched = append(touched, i)
			}
		}
		if len(touched) == 0 {
			if onlyModels(ids, byID) {
				res.Waiting++
				continue
			}
			id, err := mint(used)
			if err != nil {
				return nil, err
			}
			added[id] = true
			res.Added = append(res.Added, id)
			r := Row{ID: id, Mentions: ids, History: []Event{{Op: "add", At: day, By: opt.By}}}
			derive(&r, byID)
			applyRules(&r, byID, opt.By)
			rows = append(rows, r)
			continue
		}
		slices.SortFunc(touched, func(a, b int) int { return keepFirst(&rows[a], &rows[b]) })
		keep := &rows[touched[0]]
		before := slices.Clone(keep.Mentions)
		for _, i := range touched[1:] {
			r := &rows[i]
			a, b := firstIn(r, byID), firstIn(keep, byID)
			rule, why := "identifiers", "a shared identifier"
			switch {
			case equivalent[a] != equivalent[b]:
				rule, why = "name", "an equal name"
			case hashed[a] != hashed[b]:
				rule, why = "equivalence", "an equal formal statement"
			case pass1[a] != pass1[b]:
				rule, why = "hash", "an equal content hash"
			}
			r.Mentions = nil
			r.Outcome = &Outcome{Value: OutcomeMerged, Target: []string{keep.ID}, Rule: rule, By: opt.By}
			r.History = append(r.History, Event{Op: "merge", At: day, By: opt.By, Note: "merged into " + keep.ID + " by " + why})
			keep.History = append(keep.History, Event{Op: "merge", At: day, By: opt.By, Note: "took the mentions of " + r.ID})
			res.Merged++
		}
		var gone []string // the mentions of the row that are not in this import
		for _, id := range keep.Mentions {
			if byID[id] == nil {
				gone = append(gone, id)
			}
		}
		keep.Mentions = append(gone, ids...)
		slices.Sort(keep.Mentions)
		keep.Mentions = slices.Compact(keep.Mentions)
		if !slices.Equal(before, keep.Mentions) {
			res.Touched++
		}
		derive(keep, byID)
		applyRules(keep, byID, opt.By)
	}
	res.Orphans = attach(rows, ms, byID)
	for i := range rows {
		if applyFormal(&rows[i], byID, opt.Signals, day, opt.By) {
			res.Formal++
		}
		if applyAligned(&rows[i], byID, opt.Signals, day, opt.By) {
			res.Aligned++
		}
	}
	sortRows(rows)
	slices.Sort(res.Added)
	res.Rows = rows
	return res, nil
}

// firstIn returns the first mention of the row that is in this import.
func firstIn(r *Row, byID map[string]*Mention) string {
	for _, id := range r.Mentions {
		if byID[id] != nil {
			return id
		}
	}
	return ""
}

// unionFind holds groups of mentions. The root of a group is its smallest mention, so the groups do not depend on the order of the joins.
type unionFind map[string]string

func (u unionFind) find(x string) string {
	p, ok := u[x]
	if !ok || p == x {
		u[x] = x
		return x
	}
	r := u.find(p)
	u[x] = r
	return r
}

// union joins the groups of a and b. It reports whether they were two groups.
func (u unionFind) union(a, b string) bool {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return false
	}
	u[max(ra, rb)] = min(ra, rb)
	return true
}

// roots returns the root of the group of each mention.
func (u unionFind) roots(ms []Mention) map[string]string {
	out := make(map[string]string, len(ms))
	for i := range ms {
		out[ms[i].ID] = u.find(ms[i].ID)
	}
	return out
}

// joinIdentifiers is pass 1: it joins the mentions that have a key in common.
func joinIdentifiers(u unionFind, ms []Mention) {
	owner := map[string]string{}
	for i := range ms {
		m := &ms[i]
		if m.Attached() {
			continue
		}
		u.find(m.ID)
		for _, k := range Keys(m) {
			if o, ok := owner[k]; ok {
				u.union(o, m.ID)
			} else {
				owner[k] = m.ID
			}
		}
	}
}

// joinCited joins a mention to the mention whose result it states, when the cited result has the confidence high and the target is a mention of this run (corpus arxiv AX5.7 and AX8.2). Only a mention with the origin cited states the result: a result of the paper that cites one in its body is a new result. The kinds must agree, or be statement and conjecture, which share a block (AX8.1). It returns the number of joins.
func joinCited(u unionFind, ms []Mention, byID map[string]*Mention) int {
	n := 0
	for i := range ms {
		m := &ms[i]
		if m.Attached() || m.Origin != "cited" {
			continue
		}
		for _, c := range m.CitesResult {
			t := byID[c.To]
			if c.Confidence != "high" || t == nil || t == m || t.Attached() || !sameBlockKind(m.Kind, t.Kind) {
				continue
			}
			if u.union(m.ID, t.ID) {
				n++
			}
		}
	}
	return n
}

func sameBlockKind(a, b string) bool {
	claim := func(k string) bool { return k == "statement" || k == "conjecture" }
	return a == b || claim(a) && claim(b)
}

// joinHash joins the mentions of one kind and one block with the same content hash. Two mentions of one paper never join by hash: two items of a paper with one text, as "Conjecture 1 implies Conjecture 2" with other targets, are two items. It returns the number of joins.
func joinHash(u unionFind, ms []Mention) int {
	n := 0
	owner := map[string][]*Mention{} // the first mention of each paper with the key
	for i := range ms {
		m := &ms[i]
		if m.Hash == "" || m.Attached() {
			continue
		}
		for _, b := range Blocks(m) {
			k := m.Kind + " " + b + " " + m.Hash
			joined := false
			for _, o := range owner[k] {
				if IsResearch(m.Source) && IsResearch(o.Source) && paperOf(o) == paperOf(m) {
					continue
				}
				if u.union(o.ID, m.ID) {
					n++
				}
				joined = true
				break
			}
			if !joined {
				owner[k] = append(owner[k], m)
			}
		}
	}
	return n
}

// Blocks returns the blocks of a mention: its MSC codes at two digits, or "" when it has no code (corpus arxiv AX8.1).
func Blocks(m *Mention) []string {
	var out []string
	for _, c := range m.MSC {
		if len(c) >= 2 && !slices.Contains(out, c[:2]) {
			out = append(out, c[:2])
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// joinEquivalent is pass 2: it joins the statements with the same canonical type hash. It returns the number of joins.
func joinEquivalent(u unionFind, ms []Mention) int {
	n := 0
	owner := map[string]string{}
	for i := range ms {
		m := &ms[i]
		if m.Kind != "statement" || m.TypeHash == "" {
			continue
		}
		if o, ok := owner[m.TypeHash]; !ok {
			owner[m.TypeHash] = m.ID
		} else if u.union(o, m.ID) {
			n++
		}
	}
	return n
}

// Passes returns the group of each mention after each pass of Cluster, with no rows, in the order of PassNames: after the identifiers, after the hashes too, and after the formal statements too. A group is named by its smallest mention.
func Passes(ms []Mention) []map[string]string {
	u := unionFind{}
	joinIdentifiers(u, ms)
	byID := map[string]*Mention{}
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	joinCited(u, ms, byID)
	p1 := u.roots(ms)
	joinHash(u, ms)
	p2 := u.roots(ms)
	joinEquivalent(u, ms)
	return []map[string]string{p1, p2, u.roots(ms)}
}

// keepFirst orders the rows of one group: the row to keep comes first. A row with an outcome other than merged comes first, then the row with more mentions, then the row with the smaller identifier.
func keepFirst(a, b *Row) int {
	// A node keeps its row, and so does a row of another layer that a paper joins: the paper adds spread to it (corpus arxiv AX8.4).
	na, nb := a.Outcome != nil && a.Outcome.Value == OutcomeNode, b.Outcome != nil && b.Outcome.Value == OutcomeNode
	if na != nb {
		if na {
			return -1
		}
		return 1
	}
	if ra, rb := a.Layer == Research, b.Layer == Research; ra != rb {
		if rb {
			return -1
		}
		return 1
	}
	ha, hb := a.Outcome != nil, b.Outcome != nil
	if ha != hb {
		if ha {
			return -1
		}
		return 1
	}
	if c := cmp.Compare(len(b.Mentions), len(a.Mentions)); c != 0 {
		return c
	}
	return cmp.Compare(a.ID, b.ID)
}

// onlyModels reports whether each mention was made by a model.
func onlyModels(ids []string, byID map[string]*Mention) bool {
	for _, id := range ids {
		if !FromModel(byID[id]) {
			return false
		}
	}
	return true
}

// FromModel reports whether a model made the mention: its importer is a run of the agent.
func FromModel(m *Mention) bool { return strings.HasPrefix(m.By, "agent") }

// Keys returns the identifier keys of a mention: its native id in the system of its source, and each id that it gives in other systems. Two mentions with a key in common are one item.
func Keys(m *Mention) []string {
	var out []string
	if m.NativeID != "" {
		out = append(out, key(SystemOf(m.Source), m.NativeID))
	}
	for _, sys := range slices.Sorted(maps.Keys(m.Same)) {
		for _, id := range m.Same[sys] {
			out = append(out, key(sys, id))
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// SystemOf returns the crosswalk system of the ids of a source.
func SystemOf(source string) string {
	if s, ok := sourceSystems[source]; ok {
		return s
	}
	return source
}

var sourceSystems = map[string]string{
	"erdosproblems": "erdos",
}

// titleSystems have page titles as ids. Their keys ignore the case of the first letter and read _ as a space.
var titleSystems = map[string]bool{"nlab": true, "wikipedia": true, "proofwiki": true, "eom": true}

func key(sys, id string) string {
	id = strings.TrimSpace(id)
	if titleSystems[sys] {
		id = strings.ReplaceAll(id, "_", " ")
		if id != "" {
			id = strings.ToLower(id[:1]) + id[1:]
		}
	}
	return sys + ":" + id
}

// namePriority orders the sources whose names a row prefers. A source that is not in the list comes after them.
var namePriority = []string{"wikidata", "wikipedia", "proofwiki", "nlab", "planetmath", "oeis", "erdosproblems", "stacks", "lmfdb", "mathlib", "formal_conjectures"}

// derive computes the fields of a row from its mentions: the kind, the name, the MSC codes, the layer and the spread.
func derive(r *Row, byID map[string]*Mention) {
	var ms []*Mention
	for _, id := range r.Mentions {
		if m := byID[id]; m != nil {
			ms = append(ms, m)
		}
	}
	if len(ms) == 0 {
		return
	}
	rank := func(m *Mention) int {
		if i := slices.Index(namePriority, m.Source); i >= 0 {
			return i
		}
		return len(namePriority)
	}
	slices.SortStableFunc(ms, func(a, b *Mention) int { return cmp.Compare(rank(a), rank(b)) })
	r.Name, r.Kind = "", Unknown
	votes := map[string]int{}
	sources := map[string]bool{}
	var msc []string
	for _, m := range ms {
		if r.Name == "" && m.Name != "" {
			r.Name = m.Name
		}
		if m.Kind != "" && m.Kind != Unknown {
			votes[m.Kind]++
		}
		sources[m.Source] = true
		msc = append(msc, m.MSC...)
	}
	best := 0
	for _, k := range Kinds() {
		if votes[k] > best {
			r.Kind, best = k, votes[k]
		}
	}
	slices.Sort(msc)
	r.MSC = slices.Compact(msc)
	r.Spread = len(sources)
	r.Layer = layerOf(slices.Sorted(maps.Keys(sources)))
}

// attach lists each attached mention in the row of its target: a proof in the field proofs, and a claim inside a proof in the field local (corpus arxiv AX5.2). The target of a claim is the target of the proof that holds it. It returns the number of attached mentions with no row to go to.
func attach(rows []Row, ms []Mention, byID map[string]*Mention) int {
	rowOf := map[string]int{}
	for i := range rows {
		rows[i].Proofs, rows[i].Local = nil, nil
		if rows[i].Outcome != nil && rows[i].Outcome.Value == OutcomeMerged {
			continue
		}
		for _, id := range rows[i].Mentions {
			rowOf[id] = i
		}
	}
	orphans := 0
	for i := range ms {
		m := &ms[i]
		if !m.Attached() {
			continue
		}
		to := m.AttachedTo
		for n := 0; n < 8; n++ {
			t := byID[to]
			if t == nil || !t.Attached() {
				break
			}
			to = t.AttachedTo
		}
		r, ok := rowOf[to]
		if !ok {
			orphans++
			continue
		}
		if m.Kind == "proof" {
			rows[r].Proofs = append(rows[r].Proofs, m.ID)
		} else {
			rows[r].Local = append(rows[r].Local, m.ID)
		}
	}
	for i := range rows {
		slices.Sort(rows[i].Proofs)
		slices.Sort(rows[i].Local)
	}
	return orphans
}
