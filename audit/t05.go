package audit

import (
	"regexp"
	"slices"

	"github.com/tamnd/math2-agent/tag"
)

var commitRE = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// t05 checks that the registers agree with the node files (rule T05). Each record of registers/tags.reg has a tag, a commit and a kind, and the records are sorted with no tag two times. Each tagged node file has a record with its kind. Each record has a node file or a record in registers/retired.reg. Each retired node and each record of registers/retired.reg has a record in registers/tags.reg, and each retired node has a record in registers/retired.reg.
func t05(a *auditor) outcome {
	var o outcome
	const tags, retired = "registers/tags.reg", "registers/retired.reg"
	recs, err := readRegister(a, tags)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	kinds := a.schema.Kinds()
	registered := map[string]string{} // tag to kind
	previous := ""
	for _, r := range recs {
		o.checked++
		if len(r.fields) != 3 {
			o.add(tags, r.line, 1, "the record has %d fields, and a record has 3: tag, commit, kind", len(r.fields))
			continue
		}
		t, commit, kind := r.fields[0], r.fields[1], r.fields[2]
		if !tag.IsTag(t) {
			o.add(tags, r.line, 1, "%q is not a tag", t)
		}
		if !commitRE.MatchString(commit) {
			o.add(tags, r.line, 1, "%q is not a commit", commit)
		}
		if !slices.Contains(kinds, kind) {
			o.add(tags, r.line, 1, "%q is not a kind", kind)
		}
		if _, ok := registered[t]; ok {
			o.add(tags, r.line, 1, "the tag %s has two records", t)
		} else if t < previous {
			o.add(tags, r.line, 1, "the records are not sorted by tag")
		}
		registered[t] = kind
		previous = t
	}

	rrecs, err := readRegister(a, retired)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	inRetired := map[string]bool{}
	for _, r := range rrecs {
		o.checked++
		t := r.fields[0]
		inRetired[t] = true
		switch {
		case !tag.IsTag(t):
			o.add(retired, r.line, 1, "%q is not a tag", t)
		case registered[t] == "":
			o.add(retired, r.line, 1, "the tag %s is not in %s", t, tags)
		}
	}

	files := map[string]bool{}
	for _, f := range a.all {
		if t := trimExt(f.Path); tag.IsTag(t) {
			files[t] = true
		}
	}
	for _, n := range a.nodes {
		if !tag.IsTag(n.ID) {
			continue
		}
		o.checked++
		kind, ok := registered[n.ID]
		switch {
		case !ok:
			o.add(n.Path, 1, 1, "the tag %s is not in %s. Only the mint workflow gives tags", n.ID, tags)
		case kind != n.Kind() && slices.Contains(kinds, n.Kind()):
			o.add(n.Path, 1, 1, "the kind is %s, and %s gives %s", n.Kind(), tags, kind)
		}
		if _, ok := n.Meta["retired"]; ok && !inRetired[n.ID] {
			o.add(n.Path, 1, 1, "the node is retired, and %s has no record of it", retired)
		}
	}
	for _, r := range recs {
		if t := r.fields[0]; len(r.fields) == 3 && tag.IsTag(t) && !files[t] && !inRetired[t] {
			o.add(tags, r.line, 1, "the tag %s has no node file and no record in %s", t, retired)
		}
	}
	return o
}
