package audit

import (
	"path/filepath"
	"regexp"
	"slices"

	"github.com/tamnd/math2-agent/tag"
)

var commitRE = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// t05 checks that the registers agree with the node files (rule T05). Each record of registers/tags.reg has a tag, a commit and a kind, and the records are sorted with no tag two times. Each tagged node file has a record with its kind. Each record has a node file or a record in registers/retired.reg. Each retired node and each record of registers/retired.reg has a record in registers/tags.reg, and each retired node has a record in registers/retired.reg. Each alias names one node: the assignments of registers/aliases.reg that are in effect and the aliases fields of the nodes give no alias two tags.
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
	t05Aliases(a, &o)
	return o
}

// t05Aliases checks that each alias names one node. The first place that gives an alias owns it, and the register comes before the nodes, which are in tag order.
func t05Aliases(a *auditor, o *outcome) {
	const reg = "registers/aliases.reg"
	aliases, err := tag.ReadAliases(filepath.Join(a.root, "registers", "aliases.reg"))
	if err != nil {
		o.add(reg, 0, 0, "%v", err)
		return
	}
	owner := map[string]string{} // alias to tag
	where := map[string]string{} // alias to the place that gives it
	for _, al := range aliases {
		if al.Until == tag.Current {
			owner[al.Name], where[al.Name] = al.Tag, reg
		}
	}
	for _, n := range a.nodes {
		for _, alias := range n.List("aliases") {
			if !tag.IsAlias(alias) {
				continue
			}
			o.checked++
			old, ok := owner[alias]
			if !ok {
				owner[alias], where[alias] = n.ID, n.Path
				continue
			}
			if old != n.ID {
				line, col := keyPos(n, "aliases")
				o.add(n.Path, line, col, "the alias %s names %s in %s, and an alias names one node", alias, old, where[alias])
			}
		}
	}
}
