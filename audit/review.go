package audit

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/status"
	"go.yaml.in/yaml/v3"
)

// review is one fidelity review in a file below reviews/, with its position.
type review struct {
	status.Review
	path      string // the path of the file, for example reviews/3C/3C53TD.yaml
	tag       string // the tag in the file name
	line, col int    // the position of the review in the file
}

// readReviews reads each review file in scope. A file that does not parse gives a finding in bad.
func (a *auditor) readReviews() (out []review, bad []review, err error) {
	dir := filepath.Join(a.root, "reviews")
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == dir {
				return filepath.SkipDir
			}
			return err
		}
		rel, _ := filepath.Rel(a.root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() || filepath.Ext(rel) != ".yaml" || !a.inScope(rel) {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 G122 -- the path comes from a walk of the working tree, which the caller owns.
		if err != nil {
			return err
		}
		tag := strings.TrimSuffix(filepath.Base(rel), ".yaml")
		var doc yaml.Node
		if err := yaml.Unmarshal(data, &doc); err != nil {
			bad = append(bad, review{Review: status.Review{Notes: err.Error()}, path: rel, tag: tag, line: 1, col: 1})
			return nil
		}
		var items []*yaml.Node
		if len(doc.Content) > 0 {
			m := doc.Content[0]
			for i := 0; i+1 < len(m.Content); i += 2 {
				if m.Content[i].Value == "reviews" {
					items = m.Content[i+1].Content
				}
			}
		}
		for _, it := range items {
			r := review{path: rel, tag: tag, line: it.Line, col: it.Column}
			if err := it.Decode(&r.Review); err != nil {
				r.Notes = err.Error()
				bad = append(bad, r)
				continue
			}
			out = append(out, r)
		}
		return nil
	})
	return out, bad, err
}

var hashSyntax = regexp.MustCompile(`^blake3:[0-9a-f]{64}$`)

// v01 checks that each review is bound to the informal statement hash and to the elaborated-type hash of a binding (rule V01). The review has both hashes, and a Lean binding of its node lists it in fidelity_reviews and checks the declaration that the review read.
func v01(a *auditor) outcome {
	var o outcome
	reviews, bad, err := a.readReviews()
	if err != nil {
		o.reason = err.Error()
		return o
	}
	for _, r := range bad {
		o.checked++
		o.add(r.path, r.line, r.col, "the review does not parse: %s", r.Notes)
	}
	nodes := map[string]int{}
	for i, n := range a.nodes {
		nodes[n.ID] = i
	}
	for _, r := range reviews {
		o.checked++
		id := cmp.Or(r.ID, "without an id")
		for _, h := range []struct{ key, value string }{{"statement_hash", r.StatementHash}, {"type_hash", r.TypeHash}} {
			if !hashSyntax.MatchString(h.value) {
				o.add(r.path, r.line, r.col, "the review %s has the %s %q. Write the hash as blake3: and 64 lowercase hex digits", id, h.key, h.value)
			}
		}
		i, ok := nodes[r.tag]
		if !ok {
			o.add(r.path, r.line, r.col, "the review %s is in the file of %s, and no node has the tag %s", id, r.tag, r.tag)
			continue
		}
		n := a.nodes[i]
		var lists []binding
		for _, b := range leanBindings(n) {
			if slices.Contains(b.reviews, r.ID) {
				lists = append(lists, b)
			}
		}
		switch {
		case len(lists) == 0:
			o.add(r.path, r.line, r.col, "no Lean binding of %s lists the review %s in fidelity_reviews. Add the id to the binding that the review read", r.tag, id)
		case !slices.ContainsFunc(lists, func(b binding) bool { return b.checked() == r.Decl }):
			o.add(r.path, r.line, r.col, "the review %s read %s, and the binding that lists it checks %s", id, cmp.Or(r.Decl, "no declaration"), lists[0].checked())
		}
	}
	return o
}

// agentPrefixes start the ids that name an agent or a run, and never a person.
var agentPrefixes = []string{"agent:", "run:", "bot:"}

// v03 checks that no agent is a reviewer (rule V03). A reviewer is a person with an ORCID iD or a GitHub login, and a GitHub login that ends in [bot] is an app. The second part of the rule, on the runs that a reviewer started, needs the run records of M5.
func v03(a *auditor) outcome {
	var o outcome
	reviews, _, err := a.readReviews()
	if err != nil {
		o.reason = err.Error()
		return o
	}
	for _, r := range reviews {
		o.checked++
		who := r.Reviewer
		switch {
		case slices.ContainsFunc(agentPrefixes, func(p string) bool { return strings.HasPrefix(who, p) }),
			strings.HasPrefix(who, "github:") && strings.HasSuffix(who, "[bot]"):
			o.add(r.path, r.line, r.col, "the reviewer of %s is %s, which is an agent. Only a person can sign a fidelity review", r.ID, who)
		case !strings.HasPrefix(who, "orcid:") && !strings.HasPrefix(who, "github:"):
			o.add(r.path, r.line, r.col, "the reviewer of %s is %q. Write orcid: and an ORCID iD, or github: and a GitHub login", r.ID, who)
		}
	}
	return o
}

// reviewAge is the age after which rule V04 asks for a new review.
const reviewAge = 24 // months

// v04 reports each review older than 24 months on a node whose neighbourhood changed after the review (rule V04). The neighbourhood is the node and the nodes in its uses. The age is from the date of the HEAD commit, thus the result of a commit does not change with the day of the audit.
func v04(a *auditor) outcome {
	var o outcome
	if !isRepo(a.root) {
		o.reason = "the tree is not a git repository"
		return o
	}
	reviews, _, err := a.readReviews()
	if err != nil {
		o.reason = err.Error()
		return o
	}
	head, err := git(a.root, "log", "-1", "--format=%cs")
	if err != nil {
		o.reason = err.Error()
		return o
	}
	now, err := time.Parse(time.DateOnly, strings.TrimSpace(head))
	if err != nil {
		o.reason = err.Error()
		return o
	}
	var changed map[string]string
	byID := map[string]string{}
	for _, n := range a.nodes {
		byID[n.ID] = n.Path
	}
	for _, r := range reviews {
		date, err := time.Parse(time.DateOnly, r.Date)
		if err != nil {
			continue // V01 needs the date in the format of status.Review
		}
		o.checked++
		if !date.AddDate(0, reviewAge, 0).Before(now) {
			continue
		}
		if changed == nil {
			if changed, err = lastChanges(a.root); err != nil {
				o.reason = err.Error()
				return o
			}
		}
		path, ok := byID[r.tag]
		if !ok {
			continue
		}
		hood := []string{path}
		for _, n := range a.nodes {
			if n.Path == path {
				for _, u := range n.List("uses") {
					if p, ok := byID[u]; ok {
						hood = append(hood, p)
					}
				}
			}
		}
		for _, p := range hood {
			if changed[p] > r.Date {
				o.add(r.path, r.line, r.col, "the review %s of %s is from %s, which is more than %d months before the HEAD commit, and %s changed after the review. Ask for a new review", r.ID, r.tag, r.Date, reviewAge, p)
				break
			}
		}
	}
	return o
}

// lastChanges returns the date of the last commit that changed each node file, as YYYY-MM-DD.
func lastChanges(root string) (map[string]string, error) {
	out, err := git(root, "log", "--format=%x00%cs", "--name-only", "--relative", "--no-renames", "--", "nodes")
	if err != nil {
		return nil, err
	}
	last := map[string]string{}
	for entry := range strings.SplitSeq(out, "\x00") {
		lines := strings.Split(strings.TrimSpace(entry), "\n")
		if len(lines) < 2 {
			continue
		}
		for _, p := range lines[1:] {
			if _, ok := last[p]; !ok && p != "" {
				last[p] = lines[0]
			}
		}
	}
	return last, nil
}

// v02 checks the independence of each binding at F3 in status.json (rule V02). Two of the reviews that count must have reviewers with a record in registers/reviewers.reg and no shared affiliation, and neither reviewer can be the author of the binding in registers/bindings.reg.
func v02(a *auditor) outcome {
	var o outcome
	b := a.lastBuild()
	if reason := cmp.Or(b.noStatus, b.noReviews); reason != "" {
		o.reason = reason
		return o
	}
	reviewers, err := status.LoadReviewers(a.root)
	if err != nil {
		o.add("registers/reviewers.reg", 1, 1, "%v", err)
		return o
	}
	authors, err := status.LoadAuthors(a.root)
	if err != nil {
		o.add("registers/bindings.reg", 1, 1, "%v", err)
		return o
	}
	for _, n := range a.scopeNodes() {
		s, ok := b.statuses[n.ID]
		if !ok || retired(n) {
			continue
		}
		for _, sb := range s.Bindings {
			if sb.Fidelity != status.F3 {
				continue
			}
			o.checked++
			line, col := bindingPos(n, sb.Decl)
			author := authors[status.Key{Tag: n.ID, Decl: sb.Decl}]
			var ok []status.Reviewer
			for _, rs := range sb.Reviews {
				if rs.State != status.Counts {
					continue
				}
				who := b.reviews[rs.ID].Reviewer
				r, registered := reviewers[who]
				switch {
				case !registered:
					o.add(n.Path, line, col, "the reviewer %s of %s has no record in registers/reviewers.reg", who, rs.ID)
				case who == author:
					o.add(n.Path, line, col, "the reviewer %s of %s is the author of the binding %s", who, rs.ID, sb.Decl)
				default:
					ok = append(ok, r)
				}
			}
			if !independent(ok) {
				o.add(n.Path, line, col, "build/%s gives F3 to %s, and it has no two independent reviews. Two reviewers must share no affiliation, and neither can be the author. Run m2 build", build.StatusFile, sb.Checked)
			}
		}
	}
	return o
}

// independent reports whether two of the reviewers are independent.
func independent(rs []status.Reviewer) bool {
	for i := range rs {
		for _, r := range rs[i+1:] {
			if rs[i].Independent(r) {
				return true
			}
		}
	}
	return false
}
