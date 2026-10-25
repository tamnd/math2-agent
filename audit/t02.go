package audit

import (
	"github.com/tamnd/math2-agent/tag"
)

// t02 checks that no two node files have the same tag, and that no tag of a node or of registers/tags.reg has a form that Mint excludes (rule T02). The tag of a node file is the tag in its file name and the tag in its tag field.
func t02(a *auditor) outcome {
	var o outcome
	first := map[string]string{} // the tag and the first file that has it
	claim := func(id, path string) {
		if p, ok := first[id]; ok && p != path {
			o.add(path, 1, 1, "the tag %s is also the tag of %s", id, p)
			return
		}
		first[id] = path
	}
	for _, f := range a.all {
		o.checked++
		id := trimExt(f.Path)
		if !tag.IsTag(id) {
			continue
		}
		claim(id, f.Path)
		if tag.Excluded(id) {
			o.add(f.Path, 1, 1, "the tag %s has a form that Mint excludes: a number or an MSC code", id)
		}
	}
	for _, n := range a.nodes {
		if t := n.Get("tag"); t != n.ID && tag.IsTag(t) {
			claim(t, n.Path)
		}
	}
	const reg = "registers/tags.reg"
	recs, err := readRegister(a, reg)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	for _, r := range recs {
		o.checked++
		if id := r.fields[0]; tag.IsTag(id) && tag.Excluded(id) {
			o.add(reg, r.line, 1, "the tag %s has a form that Mint excludes: a number or an MSC code", id)
		}
	}
	return o
}
