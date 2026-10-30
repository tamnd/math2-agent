package audit

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
)

// The longest values of the fields of free text in the records of resources (RS14.2). A longer value holds text of the work, not a fact about it.
const (
	maxTitle = 500 // a title, an English title, a label and a title of a node of a structure
	maxNote  = 500 // a note of the history and a reason of a scope
	maxRef   = 2000
)

// h09 checks that no record of a resource holds text of a work beyond the facts of RS14.2 (rule H09). A record has no field for an abstract, a body or the content, so a record with one does not parse and rule H06 reports it. This rule checks the lengths of the fields of free text, the titles of the nodes of each structure, and that a cites edge keeps the text of a reference only when its citing expression has a manifestation of the class open.
func h09(a *auditor) outcome {
	t := a.resources()
	if o, done := t.skip(); done {
		return o
	}
	var o outcome
	long := func(file string, line int, what, v string, limit int) {
		if n := utf8.RuneCountInString(v); n > limit {
			o.add(file, line, min(line, 1), "the %s has %d characters, more than %d. A record holds facts, not the text of a work", what, n, limit)
		}
	}
	open := map[string]bool{} // the expressions with a manifestation of the class open, as <id>/<key>
	for _, r := range t.recs {
		o.checked++
		long(r.file, r.line, "title of "+r.ID, r.Title, maxTitle)
		long(r.file, r.line, "English title of "+r.ID, r.TitleEn, maxTitle)
		for _, e := range r.Expressions {
			long(r.file, r.line, "label of "+r.ID+"/"+e.Key, e.Label, maxTitle)
			if slices.ContainsFunc(e.Manifestations, func(m resource.Manifestation) bool { return m.Access != nil && m.Access.Class == "open" }) {
				open[r.ID+"/"+e.Key] = true
			}
		}
		for _, ev := range r.History {
			long(r.file, r.line, "note of the history of "+r.ID, ev.Note, maxNote)
		}
		if r.Scope != nil {
			long(r.file, r.line, "reason of the scope of "+r.ID, r.Scope.Reason, maxNote)
		}
	}
	files, err := mdrec.Glob(filepath.Join(a.root, resource.CitesDir, "*"+mdrec.Ext))
	if err != nil {
		o.add(resource.CitesDir, 0, 0, "%v", err)
		return o
	}
	for _, path := range files {
		rel := filepath.ToSlash(filepath.Join(resource.CitesDir, filepath.Base(mdrec.Which(path))))
		recs, err := mdrec.Read(path)
		if err != nil {
			o.add(rel, 0, 0, "%v", err)
			continue
		}
		cs, err := mdrec.Unmarshal[resource.Cite](recs)
		if err != nil {
			o.add(rel, 0, 0, "%v", err)
			continue
		}
		for n, c := range cs {
			if c.Text == "" {
				continue
			}
			o.checked++
			if !open[c.From] {
				o.add(rel, n+1, 1, "the reference %d of %s keeps its text, and %s has no manifestation of the class open. Keep the parsed fields only", c.N, c.From, c.From)
			}
			long(rel, n+1, "text of the reference "+c.From+" "+strconv.Itoa(c.N), c.Text, maxRef)
		}
	}
	err = filepath.WalkDir(filepath.Join(a.root, resource.StructureDir), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		id := filepath.Base(filepath.Dir(path))
		key := strings.TrimSuffix(filepath.Base(path), ".json")
		rel := resource.StructureDir + "/" + id + "/" + key + ".json"
		s, err := resource.ReadStructure(a.root, id, key)
		if err != nil {
			o.add(rel, 0, 0, "%v", err)
			return nil
		}
		o.checked++
		var walk func([]resource.Node)
		walk = func(ns []resource.Node) {
			for _, nd := range ns {
				long(rel, 0, "title of the node "+nd.Key, nd.Title, maxTitle)
				walk(nd.Children)
			}
		}
		walk(s.Nodes)
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		o.add(resource.StructureDir, 0, 0, "%v", err)
	}
	return o
}
