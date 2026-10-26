package audit

import (
	"regexp"

	"github.com/tamnd/math2-agent/tag"
)

// longRE is a value with the form of a tag of ten characters, with no test of its check character.
var longRE = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{10}$`)

// t09 checks that each tag of ten characters has a valid check character (rule T09, corpus spec K3.3). The values are the records of registers/tags.reg, the tags of the node files and the ids of the inventory rows. A tag of six characters of the legacy set has no check character.
func t09(a *auditor) outcome {
	var o outcome
	value := func(v, file string, line int) {
		if !longRE.MatchString(v) {
			return
		}
		o.checked++
		if want := tag.Check(v[:9]); v[9] != want {
			o.add(file, line, 1, "the check character of %s is %c, and it must be %c. A typing error in the value can give this", v, v[9], want)
		}
	}
	recs, err := readRegister(a, tagsReg)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	for _, r := range recs {
		value(r.fields[0], tagsReg, r.line)
	}
	for _, n := range a.nodes {
		value(n.ID, n.Path, 1)
	}
	if t := a.inventory(); t.err == nil {
		for _, r := range t.rows {
			value(r.ID, r.file, r.line)
		}
	}
	return o
}
