package audit

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/ids"
	"github.com/tamnd/math2-agent/tag"
)

// resTree is the catalogue of resources that the H rules read: the records of records/resources/ and the register of registers/resources/.
type resTree struct {
	recs []resRecord
	bad  []resLine // the lines that do not parse
	reg  []resource.RegisterLine
	err  error
}

type resRecord struct {
	resource.Resource
	file string
	line int
}

type resLine struct {
	file string
	line int
	err  error
}

// resources reads the catalogue of resources the first time that a rule needs it.
func (a *auditor) resources() *resTree {
	if a.res != nil {
		return a.res
	}
	t := &resTree{}
	a.res = t
	t.reg, t.err = resource.ReadRegister(a.root)
	if t.err != nil {
		return t
	}
	dir := filepath.Join(a.root, resource.Dir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return t
	}
	if err != nil {
		t.err = err
		return t
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(resource.Dir, e.Name()))
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- the path comes from a listing of the working tree, which the caller owns.
		if err != nil {
			t.err = err
			return t
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(nil, 64<<20)
		for n := 1; sc.Scan(); n++ {
			if len(bytes.TrimSpace(sc.Bytes())) == 0 {
				continue
			}
			r, err := resource.Parse(sc.Bytes())
			if err != nil {
				t.bad = append(t.bad, resLine{rel, n, err})
				continue
			}
			t.recs = append(t.recs, resRecord{r, rel, n})
		}
		if err := sc.Err(); err != nil {
			t.err = err
			return t
		}
	}
	return t
}

// skip returns the outcome of a rule when the catalogue cannot be read, or when it has no record and no register line.
func (t *resTree) skip() (outcome, bool) {
	var o outcome
	if t.err != nil {
		o.add(resource.Dir, 0, 0, "%v", t.err)
		return o, true
	}
	if len(t.recs) == 0 && len(t.bad) == 0 && len(t.reg) == 0 {
		o.reason = "the tree has no catalogue of resources"
		return o, true
	}
	return o, false
}

// h01 checks that the resource register, the tag register and the inventory register have no value in common (rule H01, corpus resources RS3.1). The three registers share the one id space of K3.6, thus a value names one thing. A finding is at the line of the resource register.
func h01(a *auditor) outcome {
	t := a.resources()
	if o, done := t.skip(); done {
		return o
	}
	var o outcome
	tags, err := tag.ReadUsed(filepath.Join(a.root, tagsReg), filepath.Join(a.root, "registers", "retired.reg"))
	if err != nil {
		o.add(tagsReg, 0, 0, "%v", err)
		return o
	}
	rows, err := inventory.ReadRegister(a.root)
	if err != nil {
		o.add(inventory.RegisterDir, 0, 0, "%v", err)
		return o
	}
	for _, l := range t.reg {
		o.checked++
		switch {
		case tags[l.ID]:
			o.add(l.File, l.Line, 1, "the resource id %s is also a tag. A resource id, a tag and an inventory id never have the same value", l.ID)
		case rows[l.ID]:
			o.add(l.File, l.Line, 1, "the resource id %s is also an inventory id. A resource id, a tag and an inventory id never have the same value", l.ID)
		}
	}
	return o
}

// h02 checks that each resource id has a valid check character and each record has a line in the register (rule H02, RS3.1). A provisional id is a finding, because m2 res mint changes it before merge.
func h02(a *auditor) outcome {
	t := a.resources()
	if o, done := t.skip(); done {
		return o
	}
	var o outcome
	have := map[string]bool{}
	for _, l := range t.reg {
		o.checked++
		if !tag.IsTag(l.ID) {
			o.add(l.File, l.Line, 1, "the register line %s does not have a resource id with a valid check character", l.ID)
		}
		have[l.ID] = true
	}
	for _, r := range t.recs {
		o.checked++
		switch {
		case tag.IsProvisional(r.ID):
			o.add(r.file, r.line, 1, "the resource %s has a provisional id. Run m2 res mint before merge", r.ID)
		case !tag.IsTag(r.ID):
			o.add(r.file, r.line, 1, "the resource %s does not have a resource id with a valid check character", r.ID)
		case !have[r.ID]:
			o.add(r.file, r.line, 1, "the resource %s has no line in %s", r.ID, resource.RegisterDir)
		case resource.Shard(r.ID)+".jsonl" != filepath.Base(r.file):
			o.add(r.file, r.line, 1, "the resource %s is in the shard of another id", r.ID)
		}
	}
	return o
}

// h03 checks that each external id of a resource is in the normal form of its system and passes its check digit (rule H03, RS3.3). It reads the ids of the work, of each creator, of each expression and of each manifestation.
func h03(a *auditor) outcome {
	t := a.resources()
	if o, done := t.skip(); done {
		return o
	}
	var o outcome
	for _, r := range t.recs {
		check := func(where string, m resource.IDs) {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				for _, v := range m[k] {
					o.checked++
					if err := ids.Check(k, v); err != nil {
						o.add(r.file, r.line, 1, "the resource %s has the id %s %q in %s: %s", r.ID, k, v, where, strings.TrimPrefix(err.Error(), "ids: "))
					}
				}
			}
		}
		check("ids", r.IDs)
		for i, c := range r.Creators {
			check("creator "+strconv.Itoa(i+1), c.IDs)
		}
		for _, e := range r.Expressions {
			check(e.Key, e.IDs)
			for _, c := range e.Contributors {
				check(e.Key+" contributor", c.IDs)
			}
			for _, m := range e.Manifestations {
				check(e.Key+"/"+m.Key, m.IDs)
			}
		}
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "no resource has an external id"
	}
	return o
}

// h06 checks the fields of each record (rule H06, RS2.4) with resource.Check, and reports each line that does not parse.
func h06(a *auditor) outcome {
	t := a.resources()
	if o, done := t.skip(); done {
		return o
	}
	var o outcome
	for _, b := range t.bad {
		o.checked++
		o.add(b.file, b.line, 1, "the line is not a resource record: %v", b.err)
	}
	for i := range t.recs {
		r := &t.recs[i]
		o.checked++
		for _, p := range resource.Check(&r.Resource) {
			o.add(r.file, r.line, 1, "the resource %s: %s", r.ID, p)
		}
	}
	return o
}
