package audit

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/node"
)

// The thresholds of the licence rules. A node that shares copyShared fingerprints with an item shares a passage of at least ingest.Shingle+copyShared-1 words with it, and usually more. Rule L04 reports a body when similarShare of its fingerprints, and at least similarShared, are in a cited item.
const (
	copyShared    = 4
	similarShared = 2
	similarShare  = 0.3
)

// imports holds the catalogue, the import manifests and the records of the tree.
type imports struct {
	cat       *ingest.Catalogue
	catData   []byte
	catErr    error
	manifests []imported
	records   []*ingest.Record          // the records of each manifest whose records file has the hash of the manifest
	source    map[*ingest.Record]string // the source of each record
	byItem    map[[2]string]*ingest.Record
	byPrint   map[uint32][]*ingest.Record
	noRecords string // the reason to skip a rule that needs the records
}

// imported is one import manifest.
type imported struct {
	path string
	m    *ingest.Manifest
	err  error
}

// importsOf reads the catalogue, the import manifests and the records one time.
func (a *auditor) importsOf() *imports {
	if a.imp != nil {
		return a.imp
	}
	im := &imports{source: map[*ingest.Record]string{}, byItem: map[[2]string]*ingest.Record{}, byPrint: map[uint32][]*ingest.Record{}}
	a.imp = im
	im.catData, im.catErr = a.readFile(ingest.CatalogueFile)
	if im.catErr == nil {
		im.cat, im.catErr = ingest.ReadCatalogue(a.root)
	}
	paths, _ := filepath.Glob(filepath.Join(a.root, ingest.ManifestDir, "*.yaml"))
	for _, p := range paths {
		m, err := ingest.ReadManifest(p)
		im.manifests = append(im.manifests, imported{path: ingest.ManifestDir + "/" + filepath.Base(p), m: m, err: err})
	}
	for _, mf := range im.manifests {
		if mf.m == nil || mf.m.Source != strings.TrimSuffix(filepath.Base(mf.path), ".yaml") {
			continue
		}
		recs, hash, err := ingest.ReadRecords(a.root, mf.m.Source)
		if err != nil || hash != mf.m.Hash {
			continue // L01 reports it
		}
		for i := range recs {
			r := &recs[i]
			im.records = append(im.records, r)
			im.source[r] = mf.m.Source
			im.byItem[[2]string{mf.m.Source, r.ID}] = r
			for _, f := range r.Fingerprints {
				im.byPrint[f] = append(im.byPrint[f], r)
			}
		}
	}
	if len(im.records) == 0 {
		im.noRecords = ingest.RecordsDir + " has no records that agree with an import manifest. Run m2 ingest first"
	}
	return im
}

// licenceOf returns the licence of a record: its own licence, or the licence of its source.
func (im *imports) licenceOf(r *ingest.Record) string {
	if r.Licence != "" {
		return r.Licence
	}
	if im.cat != nil {
		if s := im.cat.Source(im.source[r]); s != nil {
			return s.Licence
		}
	}
	return ""
}

// copies returns the records that share at least copyShared fingerprints with prints, in the order of their sources and ids, with the number of shared fingerprints.
func (im *imports) copies(prints []uint32) ([]*ingest.Record, map[*ingest.Record]int) {
	n := map[*ingest.Record]int{}
	for _, f := range prints {
		for _, r := range im.byPrint[f] {
			n[r]++
		}
	}
	var out []*ingest.Record
	for r, k := range n {
		if k >= copyShared {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(x, y *ingest.Record) int {
		return strings.Compare(im.source[x]+"\x00"+x.ID, im.source[y]+"\x00"+y.ID)
	})
	return out, n
}

// cited returns the records that a node names in sources, same_as or close_to.
func (im *imports) cited(g *graph.Graph, n *node.Node) []*ingest.Record {
	var out []*ingest.Record
	for _, t := range []string{graph.Source, graph.SameAs, graph.CloseTo} {
		for _, e := range g.Out(t, n.ID) {
			if r := im.byItem[[2]string{e.System, e.To}]; r != nil && !slices.Contains(out, r) {
				out = append(out, r)
			}
		}
	}
	return out
}

// catLine returns the line of the catalogue entry with the id.
func (im *imports) catLine(id string) int {
	for i, l := range bytes.Split(im.catData, []byte("\n")) {
		if f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(string(l)), "- ")); len(f) == 2 && f[0] == "id:" && f[1] == id {
			return i + 1
		}
	}
	return 1
}

// l01 checks that each imported text names a source and the licence of the source (rule L01). Each catalogue entry must name a licence and a class that agrees with it. Each import manifest must name a source and a snapshot of the catalogue, and the records must be the records of the manifest. A record stores text only when its class lets math2 store it.
func l01(a *auditor) outcome {
	var o outcome
	im := a.importsOf()
	const catFile = ingest.CatalogueFile
	switch {
	case im.catErr != nil:
		o.checked++
		o.add(catFile, 1, 1, "%v", im.catErr)
	case im.cat != nil:
		seen := map[string]bool{}
		for _, s := range im.cat.Sources {
			o.checked++
			line := im.catLine(s.ID)
			if seen[s.ID] {
				o.add(catFile, line, 1, "the source %s has two entries", s.ID)
			}
			seen[s.ID] = true
			for _, p := range s.Problems() {
				o.add(catFile, line, 1, "%s: %s", s.ID, p)
			}
		}
	}
	for _, mf := range im.manifests {
		o.checked++
		if mf.err != nil {
			o.add(mf.path, 1, 1, "%v", mf.err)
			continue
		}
		m := mf.m
		id := strings.TrimSuffix(filepath.Base(mf.path), ".yaml")
		var src *ingest.Source
		if im.cat != nil {
			src = im.cat.Source(m.Source)
		}
		switch {
		case m.Source != id:
			o.add(mf.path, 1, 1, "the manifest names the source %q, and the file name gives %s", m.Source, id)
			continue
		case src == nil:
			o.add(mf.path, 1, 1, "the source %s has no entry in %s", id, catFile)
			continue
		case src.Snapshot(m.Snapshot) == nil:
			o.add(mf.path, 1, 1, "the snapshot %s is not a snapshot of %s in %s", m.Snapshot, id, catFile)
		case m.Adapter == "":
			o.add(mf.path, 1, 1, "the manifest names no adapter")
		}
		total := 0
		for c, k := range m.Classes {
			if !c.Valid() {
				o.add(mf.path, 1, 1, "%q is not an access class", c)
			}
			total += k
		}
		if total != m.Records {
			o.add(mf.path, 1, 1, "the classes count %d records, and the manifest has %d", total, m.Records)
		}
		o.findings = append(o.findings, recordFindings(a, src, m)...)
	}
	return o
}

// recordFindings checks the records file of one manifest. A file that does not exist is not a finding, because the records are derived data.
func recordFindings(a *auditor, src *ingest.Source, m *ingest.Manifest) []check.Finding {
	var o outcome
	path := ingest.RecordsDir + "/" + m.Source + ".jsonl"
	recs, hash, err := ingest.ReadRecords(a.root, m.Source)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		o.add(path, 1, 1, "%v", err)
		return o.findings
	case hash != m.Hash:
		o.add(path, 1, 1, "the records have the hash %s, and the manifest has %s. Run m2 ingest %s -snapshot %s again", hash, m.Hash, m.Source, m.Snapshot)
	}
	for i, r := range recs {
		want := src.Class // the licence of the source, as the catalogue classifies it
		if r.Licence != "" {
			want = ingest.ClassOf(r.Licence)
		}
		if r.Class != want {
			o.add(path, i+1, 1, "the record %s has the class %q, and its licence gives %s", r.ID, r.Class, want)
		}
		cls := r.Class
		if cls.StoresText() {
			cls = want
		}
		if r.Text != "" && !cls.StoresText() {
			o.add(path, i+1, 1, "the record %s stores text, and the class %s does not let math2 store it. Only the record and the fingerprints can be stored", r.ID, cls)
		}
	}
	return o.findings
}

// prints returns the fingerprints of the body of a node.
func prints(n *node.Node) []uint32 {
	return ingest.Fingerprints(n.Body)
}

// l02 checks that no node body copies text from a source whose class does not let the text go into node prose (rule L02).
func l02(a *auditor) outcome {
	var o outcome
	im := a.importsOf()
	if im.noRecords != "" {
		o.reason = im.noRecords
		return o
	}
	for _, n := range a.scopeNodes() {
		if retired(n) {
			continue
		}
		o.checked++
		recs, shared := im.copies(prints(n))
		for _, r := range recs {
			if !r.Class.Prose() {
				o.add(n.Path, n.BodyLine, 1, "the body shares %d fingerprints with %s %s, which has the class %s. Text of that class must not go into node prose. Write the text in your own words", shared[r], im.source[r], r.ID, r.Class)
			}
		}
	}
	return o
}

// l03 checks the attribution of share-alike text (rule L03). Each share-alike source in the catalogue has an attribution line for the attribution file of the release. Each node body that uses text of a share-alike item names the item in sources, same_as or close_to.
func l03(a *auditor) outcome {
	var o outcome
	im := a.importsOf()
	if im.cat != nil {
		for _, s := range im.cat.Sources {
			if ingest.ShareAlike(s.Licence) && s.Class.Prose() {
				o.checked++
				if strings.TrimSpace(s.Attribution) == "" {
					o.add(ingest.CatalogueFile, im.catLine(s.ID), 1, "%s has the share-alike licence %s and no attribution line. Add attribution", s.ID, s.Licence)
				}
			}
		}
	}
	if im.noRecords != "" {
		if o.checked == 0 {
			o.reason = im.noRecords
		}
		return o
	}
	g := a.graphOf()
	for _, n := range a.scopeNodes() {
		if retired(n) {
			continue
		}
		recs, _ := im.copies(prints(n))
		cited := im.cited(g, n)
		for _, r := range recs {
			licence := im.licenceOf(r)
			if !r.Class.Prose() || !ingest.ShareAlike(licence) {
				continue
			}
			o.checked++
			if !slices.Contains(cited, r) {
				line, col := keyPos(n, "sources", "same_as", "kind")
				o.add(n.Path, line, col, "the body uses text of %s %s under %s. Name the item in sources or same_as to give attribution", im.source[r], r.ID, licence)
			}
		}
	}
	return o
}

// l04 reports a node body that is similar to the text of an item that the node cites, for a person to check that it is not copied (rule L04).
func l04(a *auditor) outcome {
	var o outcome
	im := a.importsOf()
	if im.noRecords != "" {
		o.reason = im.noRecords
		return o
	}
	g := a.graphOf()
	for _, n := range a.scopeNodes() {
		cited := im.cited(g, n)
		if retired(n) || len(cited) == 0 {
			continue
		}
		o.checked++
		p := prints(n)
		for _, r := range cited {
			k := ingest.Shared(p, r.Fingerprints)
			if k >= similarShared && float64(k) >= similarShare*float64(len(p)) {
				o.add(n.Path, n.BodyLine, 1, "%d of the %d fingerprints of the body are in %s %s. Check that the body does not copy the text", k, len(p), im.source[r], r.ID)
			}
		}
	}
	return o
}
