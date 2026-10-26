package audit

import (
	"github.com/tamnd/math2-agent/tag"
)

const tagsReg = "registers/tags.reg"

// t08 checks that no value is both a tag and an inventory row (rule T08, corpus spec K3.6). Tags and row ids come from one space of values, thus a value names one thing. The tags are the records of registers/tags.reg and the node files. A row with the value of a tag is a finding at the row.
func t08(a *auditor) outcome {
	t := a.inventory()
	if o, done := t.skip(); done {
		return o
	}
	var o outcome
	recs, err := readRegister(a, tagsReg)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	tags := map[string]bool{}
	for _, r := range recs {
		tags[r.fields[0]] = true
	}
	for _, n := range a.nodes {
		if tag.IsTag(n.ID) {
			tags[n.ID] = true
		}
	}
	for _, r := range t.rows {
		o.checked++
		if tags[r.ID] {
			o.add(r.file, r.line, 1, "the row %s has the value of a tag. A tag and a row never have the same value", r.ID)
		}
	}
	return o
}
