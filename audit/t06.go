package audit

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tamnd/math2-agent/tag"
)

var (
	mscAliasRE   = regexp.MustCompile(`^[0-9]{2}(-xx|[a-z](xx|[0-9]{2}))$`)
	oeisAliasRE  = regexp.MustCompile(`^a[0-9]{6}$`)
	arxivAliasRE = regexp.MustCompile(`^([0-9]{4}-[0-9]{4,5}(v[0-9]+)?|[a-z]+(-[a-z]+)*-[0-9]{7}(v[0-9]+)?)$`)
)

// crockford maps the letters that Crockford base32 reads as digits.
var crockford = strings.NewReplacer("o", "0", "i", "1", "l", "1")

// t06 checks that no alias looks like a tag, an MSC code, an OEIS A-number or an arXiv id (rule T06). Crockford base32 does not use case, and it reads o as 0 and i and l as 1. Thus the alias 7k3qx2 and the alias 7k3qxl look like the tag 7K3QX2. An alias has no dot and no slash, so an arXiv id looks like an alias when a hyphen is in the place of the dot or of the slash.
func t06(a *auditor) outcome {
	var o outcome
	const reg = "registers/aliases.reg"
	aliases, err := tag.ReadAliases(filepath.Join(a.root, "registers", "aliases.reg"))
	if err != nil {
		o.add(reg, 0, 0, "%v", err)
		return o
	}
	recs, err := readRegister(a, reg)
	if err != nil {
		o.reason = err.Error()
		return o
	}
	for i, al := range aliases {
		o.checked++
		line := recs[i].line
		switch name := al.Name; {
		case tag.IsTag(strings.ToUpper(crockford.Replace(name))):
			o.add(reg, line, 1, "the alias %s looks like the tag %s", name, strings.ToUpper(crockford.Replace(name)))
		case mscAliasRE.MatchString(name):
			o.add(reg, line, 1, "the alias %s looks like an MSC code", name)
		case oeisAliasRE.MatchString(name):
			o.add(reg, line, 1, "the alias %s looks like an OEIS A-number", name)
		case arxivAliasRE.MatchString(name):
			o.add(reg, line, 1, "the alias %s looks like an arXiv id", name)
		}
	}
	return o
}
