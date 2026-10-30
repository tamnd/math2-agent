package audit

import (
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
	"github.com/tamnd/math2-agent/tag"
)

// resTree is the catalogue of resources that the H rules read: the records of records/resources/ and the register of registers/resources/.
type resTree struct {
	recs []resRecord
	bad  []resLine // the lines that do not parse
	reg  []resource.RegisterLine
	err  error

	cat    *ingest.Catalogue // manifests/sources.yaml, read when a rule needs it
	catErr error
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
	files, err := mdrec.Glob(filepath.Join(a.root, resource.Dir, "*"+mdrec.Ext))
	if err != nil {
		t.err = err
		return t
	}
	for _, path := range files {
		name := filepath.Base(path)
		if !resource.IsShard(name) {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(resource.Dir, filepath.Base(mdrec.Which(path))))
		recs, err := mdrec.Read(path)
		if err != nil {
			t.err = err
			return t
		}
		for n, rec := range recs {
			r, err := resource.Parse(rec)
			if err != nil {
				t.bad = append(t.bad, resLine{rel, n + 1, err})
				continue
			}
			t.recs = append(t.recs, resRecord{r, rel, n + 1})
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
		case resource.Shard(r.ID) != strings.TrimSuffix(strings.TrimSuffix(filepath.Base(r.file), mdrec.Ext), mdrec.Old):
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

// h04 checks that no strong id belongs to two live resources at the same level (rule H04, RS3.5). The level of an id is the place where the record holds it: the work, an expression or a manifestation. An ISBN is strong only when the titles of the two resources match. A finding is at the second resource, and it names the first.
func h04(a *auditor) outcome {
	t := a.resources()
	if o, done := t.skip(); done {
		return o
	}
	var o outcome
	strong := map[string]bool{"isbn": true}
	for _, s := range cluster.Strong {
		strong[s] = true
	}
	type owner struct {
		r     *resRecord
		title string
	}
	seen := map[string]owner{}
	for i := range t.recs {
		r := &t.recs[i]
		if !r.Live() {
			continue
		}
		title := cluster.Fold(r.Title)
		mine := map[string]bool{}
		check := func(level string, m resource.IDs) {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				if !strong[k] {
					continue
				}
				for _, v := range m[k] {
					if n, err := ids.Normalize(k, v); err == nil {
						v = n
					}
					key := level + " " + k + ":" + v
					if mine[key] {
						continue
					}
					mine[key] = true
					o.checked++
					first, ok := seen[key]
					switch {
					case !ok:
						seen[key] = owner{r, title}
					case k == "isbn" && first.title != title:
					default:
						o.add(r.file, r.line, 1, "the resource %s has the %s id %s:%s of the live resource %s. Merge the two resources, or correct the id", r.ID, level, k, v, first.r.ID)
					}
				}
			}
		}
		check("work", r.IDs)
		for _, e := range r.Expressions {
			check("expression", e.IDs)
			for _, m := range e.Manifestations {
				check("manifestation", m.IDs)
			}
		}
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "no live resource has a strong id"
	}
	return o
}

// citationOnly is the source of the records that only a reference of another resource gives (RS11.5). Such a record does not count for rule H05.
const citationOnly = "refs"

// catalogue reads manifests/sources.yaml the first time that a rule needs it.
func (t *resTree) catalogue(root string) (*ingest.Catalogue, error) {
	if t.cat == nil && t.catErr == nil {
		t.cat, t.catErr = ingest.ReadCatalogue(root)
	}
	return t.cat, t.catErr
}

// h05 checks that each resource has at least one catalogue record from a source that a person approved (rule H05, RS1.2). A record of the source refs does not count, because a citation alone does not confirm a work.
func h05(a *auditor) outcome {
	t := a.resources()
	if o, done := t.skip(); done {
		return o
	}
	var o outcome
	cat, err := t.catalogue(a.root)
	if err != nil {
		o.add(ingest.CatalogueFile, 0, 0, "%v", err)
		return o
	}
	for _, r := range t.recs {
		if !r.Live() {
			continue
		}
		o.checked++
		ok := false
		for _, c := range r.Catalogue {
			if s := cat.Source(c.Source); c.Source != citationOnly && s != nil && s.ApprovedBy != "" {
				ok = true
				break
			}
		}
		if !ok {
			o.add(r.file, r.line, 1, "the resource %s has no catalogue record from a source that a person approved in %s", r.ID, ingest.CatalogueFile)
		}
	}
	return o
}

// h07 checks that each duplicate resource points to a live resource (rule H07, RS10.6). A duplicate that points to a duplicate is a chain of two, and the build makes each chain one long.
func h07(a *auditor) outcome {
	t := a.resources()
	if o, done := t.skip(); done {
		return o
	}
	var o outcome
	byID := map[string]*resRecord{}
	for i := range t.recs {
		byID[t.recs[i].ID] = &t.recs[i]
	}
	for _, r := range t.recs {
		if r.Live() {
			continue
		}
		o.checked++
		target, ok := byID[r.Scope.Target]
		switch {
		case r.Scope.Target == "":
			o.add(r.file, r.line, 1, "the duplicate resource %s has no target", r.ID)
		case r.Scope.Target == r.ID:
			o.add(r.file, r.line, 1, "the duplicate resource %s points to itself", r.ID)
		case !ok:
			o.add(r.file, r.line, 1, "the duplicate resource %s points to %s, which is not in the catalogue", r.ID, r.Scope.Target)
		case !target.Live():
			o.add(r.file, r.line, 1, "the duplicate resource %s points to the duplicate %s. Point it to %s, the live resource", r.ID, target.ID, target.Scope.Target)
		}
	}
	if o.checked == 0 {
		o.reason = "the catalogue has no duplicate resource"
	}
	return o
}

// h10 checks that each catalogue record names its source and the snapshot of that source (rule H10, RS12.2). The source is an entry of manifests/sources.yaml, and the snapshot is one of the snapshots of the entry.
func h10(a *auditor) outcome {
	t := a.resources()
	if o, done := t.skip(); done {
		return o
	}
	var o outcome
	cat, err := t.catalogue(a.root)
	if err != nil {
		o.add(ingest.CatalogueFile, 0, 0, "%v", err)
		return o
	}
	for _, r := range t.recs {
		for i, c := range r.Catalogue {
			o.checked++
			s := cat.Source(c.Source)
			switch {
			case c.Source == "":
				o.add(r.file, r.line, 1, "the catalogue record %d of the resource %s has no source", i+1, r.ID)
			case c.Snapshot == "":
				o.add(r.file, r.line, 1, "the catalogue record %d of the resource %s has no snapshot of %s", i+1, r.ID, c.Source)
			case c.Source == citationOnly:
			case s == nil:
				o.add(r.file, r.line, 1, "the catalogue record %d of the resource %s names the source %s, which is not in %s", i+1, r.ID, c.Source, ingest.CatalogueFile)
			case s.Snapshot(c.Snapshot) == nil:
				o.add(r.file, r.line, 1, "the catalogue record %d of the resource %s names the snapshot %s, which is not a snapshot of %s", i+1, r.ID, c.Snapshot, c.Source)
			}
		}
	}
	return o
}

// h11 reports each live resource with the outcome deferred whose date has passed (rule H11, RS9.6). The queue res-scope holds it until a person or a test decides it.
func h11(a *auditor) outcome {
	t := a.resources()
	if o, done := t.skip(); done {
		return o
	}
	var o outcome
	today := time.Now().UTC().Format(time.DateOnly)
	for _, r := range t.recs {
		if r.Scope == nil || r.Scope.Value != resource.Deferred {
			continue
		}
		o.checked++
		if r.Scope.Until <= today {
			o.add(r.file, r.line, 1, "the resource %s is deferred, and its date %s has passed. Decide it in the queue res-scope", r.ID, r.Scope.Until)
		}
	}
	if o.checked == 0 {
		o.reason = "no resource is deferred"
	}
	return o
}

// BannedDomainsFile is the ban list of RS14.5: one domain on each line, and a line that starts with # is a comment.
const BannedDomainsFile = "manifests/banned-domains.txt"

// banned are the shadow libraries of RS14.5. The rule bans them also when the ban list does not name them.
var banned = []string{"libgen.rs", "libgen.is", "libgen.li", "libgen.st", "library.lol", "sci-hub.se", "sci-hub.ru", "sci-hub.st", "z-lib.org", "z-library.sk", "singlelogin.re", "annas-archive.org", "annas-archive.se", "annas-archive.li"}

// bannedKeys are the id keys and the source names of the shadow libraries.
var bannedKeys = map[string]bool{"libgen": true, "scihub": true, "sci_hub": true, "zlib": true, "zlibrary": true, "annas_archive": true}

// bannedHost reports whether the host of the link is a domain of the list or a subdomain of one.
func bannedHost(link string, domains []string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || u.Host == "" {
		return "", false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return d, true
		}
	}
	return "", false
}

// h16 checks that no source of manifests/sources.yaml, no catalogue record and no link of a record points to a shadow library, and that no record has an id key of one (rule H16, RS14.5).
func h16(a *auditor) outcome {
	var o outcome
	domains := slices.Clone(banned)
	data, err := a.readFile(BannedDomainsFile)
	if err != nil {
		o.add(BannedDomainsFile, 0, 0, "%v", err)
		return o
	}
	for line := range strings.Lines(string(data)) {
		if d := strings.ToLower(strings.TrimSpace(line)); d != "" && !strings.HasPrefix(d, "#") {
			domains = append(domains, strings.TrimPrefix(d, "www."))
		}
	}
	t := a.resources()
	cat, err := t.catalogue(a.root)
	if err != nil {
		o.add(ingest.CatalogueFile, 0, 0, "%v", err)
		return o
	}
	for _, s := range cat.Sources {
		o.checked++
		links := []string{s.Home}
		for _, sn := range s.Snapshots {
			links = append(links, sn.URL)
		}
		if bannedKeys[s.ID] {
			o.add(ingest.CatalogueFile, 0, 0, "the source %s is a shadow library. math2 never uses one", s.ID)
			continue
		}
		for _, l := range links {
			if d, ok := bannedHost(l, domains); ok {
				o.add(ingest.CatalogueFile, 0, 0, "the source %s points to %s, a shadow library. math2 never uses one", s.ID, d)
				break
			}
		}
	}
	if t.err != nil {
		o.add(resource.Dir, 0, 0, "%v", t.err)
		return o
	}
	for _, r := range t.recs {
		o.checked++
		for _, c := range r.Catalogue {
			if bannedKeys[c.Source] {
				o.add(r.file, r.line, 1, "the resource %s has a catalogue record of the shadow library %s", r.ID, c.Source)
			}
		}
		keys := func(m resource.IDs) {
			for _, k := range slices.Sorted(maps.Keys(m)) {
				if bannedKeys[k] {
					o.add(r.file, r.line, 1, "the resource %s has the id key %s of a shadow library", r.ID, k)
				}
			}
		}
		keys(r.IDs)
		for _, e := range r.Expressions {
			keys(e.IDs)
			for _, m := range e.Manifestations {
				keys(m.IDs)
				for _, l := range m.Links {
					if d, ok := bannedHost(l, domains); ok {
						o.add(r.file, r.line, 1, "the resource %s has a link to %s, a shadow library", r.ID, d)
					}
				}
			}
		}
	}
	if o.checked == 0 {
		o.reason = "the tree has no source and no resource"
	}
	return o
}

// h15 reports each work that a curriculum names and that has no resource (rule H15, curriculum closure of RS11.6). A text with no resource id, a text whose id is not in the catalogue and a text that names a duplicate are findings. The target is 0.
func h15(a *auditor) outcome {
	var o outcome
	texts, err := resource.ReadTexts(a.root)
	if err != nil {
		o.add(resource.CurriculaDir, 0, 0, "%v", err)
		return o
	}
	if len(texts) == 0 {
		o.reason = "no curriculum names a text"
		return o
	}
	t := a.resources()
	if t.err != nil {
		o.add(resource.Dir, 0, 0, "%v", t.err)
		return o
	}
	byID := map[string]*resRecord{}
	for i := range t.recs {
		byID[t.recs[i].ID] = &t.recs[i]
	}
	for _, x := range texts {
		o.checked++
		id, _, _ := strings.Cut(x.Resource, "/")
		r, ok := byID[id]
		switch {
		case x.Resource == "":
			o.add(x.File, x.Line, 1, "the text %q has no resource id. Find its resource, or add one at T1", x.Name)
		case !ok:
			o.add(x.File, x.Line, 1, "the text %q names the resource %s, which is not in the catalogue", x.Name, id)
		case !r.Live():
			o.add(x.File, x.Line, 1, "the text %q names the duplicate %s. Name %s, the live resource", x.Name, id, r.Scope.Target)
		}
	}
	return o
}
