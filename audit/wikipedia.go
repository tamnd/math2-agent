package audit

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/wpcorpus"
)

// wpTree is the Wikipedia corpus that the Y rules read: the manifests, and
// for each page of the content plane its census and its objects.
type wpTree struct {
	m     *wpcorpus.Manifests
	pages []wpPage
	err   error
}

type wpPage struct {
	page    *wpcorpus.Page
	census  *wpcorpus.Census // nil when the census file does not exist or does not parse
	cerr    error
	objects []wpcorpus.Object
	oerr    error
}

// wp reads the corpus that Options.Wikipedia names, the first time that a
// rule needs it.
func (a *auditor) wp() *wpTree {
	if a.wpt != nil {
		return a.wpt
	}
	t := &wpTree{}
	a.wpt = t
	if t.m, t.err = wpcorpus.Load(a.wikipedia); t.err != nil {
		return t
	}
	ids, err := wpcorpus.Pages(a.wikipedia)
	if err != nil {
		t.err = err
		return t
	}
	for _, id := range ids {
		p, err := wpcorpus.ReadContent(a.wikipedia, id)
		if err != nil {
			t.err = err
			return t
		}
		wp := wpPage{page: p}
		wp.census, wp.cerr = wpcorpus.ReadCensus(a.wikipedia, p.ID, p.Revision)
		wp.objects, wp.oerr = wpcorpus.ReadObjects(a.wikipedia, p.ID, p.Revision)
		t.pages = append(t.pages, wp)
	}
	return t
}

// wpStart gives the corpus to a Y rule, or sets the reason why the rule
// skips, or adds the error that stopped the read.
func (a *auditor) wpStart(o *outcome) *wpTree {
	if a.wikipedia == "" {
		o.reason = "no checkout of tamnd/wikipedia-math is given, so set -wikipedia to run the rule"
		return nil
	}
	t := a.wp()
	if t.err != nil {
		o.add(a.wikipedia, 0, 0, "%v", t.err)
		return nil
	}
	return t
}

// y02 checks that each revision of the content plane has a census record and
// an objects file, and that the census names the extractor and the revision
// (WP4.5).
func y02(a *auditor) outcome {
	var o outcome
	t := a.wpStart(&o)
	if t == nil {
		return o
	}
	for _, p := range t.pages {
		o.checked++
		id, rev := p.page.ID, p.page.Revision
		cpath := wpcorpus.CensusPath(id, rev)
		switch c := p.census; {
		case errors.Is(p.cerr, os.ErrNotExist):
			o.add(cpath, 0, 0, "the revision %d of %q has no census record. Run m2 wp extract -page %d", rev, p.page.Title, id)
		case p.cerr != nil:
			o.add(cpath, 0, 0, "the census record does not parse: %v", p.cerr)
		case c.Page != id || c.Revision != rev:
			o.add(cpath, 0, 0, "the census names the page %d and the revision %d, and the file is of the page %d and the revision %d", c.Page, c.Revision, id, rev)
		case c.Extractor == "":
			o.add(cpath, 0, 0, "the census does not name the extractor version")
		}
		switch opath := wpcorpus.ObjectsPath(id, rev); {
		case errors.Is(p.oerr, os.ErrNotExist):
			o.add(opath, 0, 0, "the revision %d of %q has no objects file", rev, p.page.Title)
		case p.oerr != nil:
			o.add(opath, 0, 0, "the objects file does not parse: %v", p.oerr)
		}
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "the content plane of the checkout has no page"
	}
	return o
}

// pointerFindings are the findings that a pointer of W-in can have.
var pointerFindings = map[string]bool{wpcorpus.FindingBrokenAnchor: true, wpcorpus.FindingTemplateAnchor: true}

// y03 checks that each counted cue of each census has an object of the
// objects file, or a waiver of waivers.yaml, and that each pointer of W-in
// has an object or a finding (WP4.4, WP4.8).
func y03(a *auditor) outcome {
	var o outcome
	t := a.wpStart(&o)
	if t == nil {
		return o
	}
	waivers := map[string]bool{}
	for _, w := range t.m.Waivers {
		waivers[w.ID] = true
	}
	for _, p := range t.pages {
		if p.census == nil || p.oerr != nil {
			continue // Y02 reports it
		}
		refs := map[string]bool{}
		for _, ob := range p.objects {
			refs[ob.Ref] = true
		}
		path := wpcorpus.CensusPath(p.page.ID, p.page.Revision)
		for _, q := range p.census.Cues {
			o.checked++
			switch {
			case q.Object == "" && q.Waiver == "":
				o.add(path, 0, 0, "the cue %s (%s) at the bytes %d to %d of %q has no object and no waiver", q.Cue, q.Kind, q.Span[0], q.Span[1], p.page.Title)
			case q.Object != "" && !refs[q.Object]:
				o.add(path, 0, 0, "the cue %s at the bytes %d to %d names the object %s, and the objects file has no such object", q.Cue, q.Span[0], q.Span[1], q.Object)
			case q.Waiver != "" && !waivers[q.Waiver]:
				o.add(path, 0, 0, "the cue %s at the bytes %d to %d names the waiver %s, and waivers.yaml has no such waiver", q.Cue, q.Span[0], q.Span[1], q.Waiver)
			case q.Waiver != "" && q.Reason == "":
				o.add(path, 0, 0, "the cue %s at the bytes %d to %d has the waiver %s with no reason", q.Cue, q.Span[0], q.Span[1], q.Waiver)
			}
		}
		for _, q := range p.census.Pointers {
			o.checked++
			switch {
			case q.Object == "" && q.Finding == "":
				o.add(path, 0, 0, "the %s %q to the anchor %q of %q has no object and no finding", q.Via, q.From, q.Anchor, p.page.Title)
			case q.Object != "" && !refs[q.Object]:
				o.add(path, 0, 0, "the %s %q to the anchor %q names the object %s, and the objects file has no such object", q.Via, q.From, q.Anchor, q.Object)
			case q.Finding != "" && !pointerFindings[q.Finding]:
				o.add(path, 0, 0, "the %s %q to the anchor %q has the finding %s, which is not a finding of WP4.4", q.Via, q.From, q.Anchor, q.Finding)
			}
		}
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "no census of the checkout has a counted cue"
	}
	return o
}

// y07 reports each census whose readings W-text and W-html differ by more
// than 1 element or 2%, whichever is larger (WP4.3). A census without the
// witness W-html is not checked.
func y07(a *auditor) outcome {
	var o outcome
	t := a.wpStart(&o)
	if t == nil {
		return o
	}
	for _, p := range t.pages {
		if p.census == nil || p.census.HTML == nil {
			continue
		}
		o.checked++
		path := wpcorpus.CensusPath(p.page.ID, p.page.Revision)
		for _, d := range p.census.HTMLDifferences() {
			o.add(path, 0, 0, "the readings of %q differ: %s", p.page.Title, d)
		}
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "no census of the checkout has the witness W-html. Run m2 wp parsoid and then m2 wp extract"
	}
	return o
}

// y08 reports each template that a page uses and that the template table
// does not map (WP3.2). The current table decides, so a template that was
// mapped after the extraction is not reported.
func y08(a *auditor) outcome {
	var o outcome
	t := a.wpStart(&o)
	if t == nil {
		return o
	}
	uses, pages := map[string]int{}, map[string]int{}
	for _, p := range t.pages {
		if p.census == nil {
			continue
		}
		o.checked++
		for k, n := range p.census.Unmapped {
			if t.m.Template(k) == nil {
				uses[k] += n
				pages[k]++
			}
		}
	}
	path := filepath.Join(wpcorpus.ManifestDir, "templates.yaml")
	for _, k := range slices.Sorted(maps.Keys(uses)) {
		o.add(path, 0, 0, "the template {{%s}} has %s in %s, and the table does not map it", k, count(uses[k], "use"), count(pages[k], "article"))
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "the checkout has no census"
	}
	return o
}

// wpHeadingArticles is the number of articles from which rule Y09 reports
// an unmapped heading (WP3.5).
const wpHeadingArticles = 5

// y09 reports each heading that 5 or more articles use and that the heading
// table does not map (WP3.5).
func y09(a *auditor) outcome {
	var o outcome
	t := a.wpStart(&o)
	if t == nil {
		return o
	}
	pages := map[string]int{}
	for _, p := range t.pages {
		if p.census == nil {
			continue
		}
		o.checked++
		seen := map[string]bool{}
		for _, h := range p.census.UnmappedHeadings {
			k := strings.ToLower(strings.Join(strings.Fields(h), " "))
			if !seen[k] && t.m.HeadingRole(h, p.page.Title) == "" {
				seen[k] = true
				pages[k]++
			}
		}
	}
	path := filepath.Join(wpcorpus.ManifestDir, "headings.yaml")
	for _, k := range slices.Sorted(maps.Keys(pages)) {
		if pages[k] >= wpHeadingArticles {
			o.add(path, 0, 0, "the heading %q is in %s, and the table does not map it", k, count(pages[k], "article"))
		}
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "the checkout has no census"
	}
	return o
}

// y10 reports each formula that wiki/texvc did not convert to TeX (WP3.3).
func y10(a *auditor) outcome {
	var o outcome
	t := a.wpStart(&o)
	if t == nil {
		return o
	}
	for _, p := range t.pages {
		path := wpcorpus.ObjectsPath(p.page.ID, p.page.Revision)
		seen := map[[2]int]bool{}
		for _, ob := range p.objects {
			for _, m := range ob.Math {
				if seen[m.Span] {
					continue // an object that contains another object has its formulas too
				}
				seen[m.Span] = true
				o.checked++
				if len(m.Errors) > 0 {
					o.add(path, 0, 0, "the formula %q of the object %s does not convert: %s", m.Source, ob.Ref, strings.Join(m.Errors, "; "))
				}
			}
		}
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "no object of the checkout has a formula"
	}
	return o
}

// count gives "1 use" or "3 uses".
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// y01 checks that each article of the scope list has an article state of
// WP1.8. The state is listed for an article of scope.tsv, fetched when the
// content plane has the article, parsed when it has a census too, and stopped
// when stopped.tsv names it with a reason.
func y01(a *auditor) outcome {
	var o outcome
	t := a.wpStart(&o)
	if t == nil {
		return o
	}
	scope, err := wpcorpus.ReadScopeFile(a.wikipedia, wpcorpus.ScopePath)
	if errors.Is(err, os.ErrNotExist) {
		o.reason = "the checkout has no scope list yet. Run m2 wp scope to make it"
		return o
	}
	if err != nil {
		o.add(wpcorpus.ScopePath, 0, 0, "%v", err)
		return o
	}
	stops, err := wpcorpus.ReadStops(a.wikipedia)
	if err != nil {
		o.add(wpcorpus.StoppedPath, 0, 0, "%v", err)
		return o
	}
	inScope := map[int64]int{}
	for i, c := range scope {
		if prev, ok := inScope[c.PageID]; ok {
			o.add(wpcorpus.ScopePath, i+2, 0, "the page %d is in the scope list two times, on the lines %d and %d", c.PageID, prev, i+2)
		}
		inScope[c.PageID] = i + 2
	}
	stopped := map[int64]bool{}
	for _, s := range stops {
		switch {
		case !wpcorpus.StopReasons[s.Reason]:
			o.add(wpcorpus.StoppedPath, s.Line, 0, "the page %d stops with the reason %q, which is not a reason of WP1.8", s.PageID, s.Reason)
		case s.Reason == "merged" && s.Into == 0:
			o.add(wpcorpus.StoppedPath, s.Line, 0, "the page %d is merged and does not name the page that it goes into", s.PageID)
		case inScope[s.PageID] == 0:
			o.add(wpcorpus.StoppedPath, s.Line, 0, "the page %d stops, and the scope list does not have it", s.PageID)
		default:
			stopped[s.PageID] = true
		}
	}
	fetched := map[int64]bool{}
	for _, p := range t.pages {
		fetched[int64(p.page.ID)] = true
	}
	for _, c := range scope {
		o.checked++
		if !c.InScope && !stopped[c.PageID] && !fetched[c.PageID] {
			o.add(wpcorpus.ScopePath, inScope[c.PageID], 0, "the page %d (%s) is in scope.tsv with in_scope false, so it has no state", c.PageID, c.Title)
		}
	}
	return o
}

// y14 reports each candidate that two or more signals name and that the
// classifier rejects with a confidence below 0.9 (WP1.3). That is where an
// article of mathematics is most likely to be lost.
func y14(a *auditor) outcome {
	var o outcome
	if a.wikipedia == "" {
		o.reason = "no checkout of tamnd/wikipedia-math is given, so set -wikipedia to run the rule"
		return o
	}
	cs, err := wpcorpus.ReadScopeFile(a.wikipedia, wpcorpus.CandidatesPath)
	if errors.Is(err, os.ErrNotExist) {
		o.reason = "the checkout has no candidates yet. Run m2 wp scope to make them"
		return o
	}
	if err != nil {
		o.add(wpcorpus.CandidatesPath, 0, 0, "%v", err)
		return o
	}
	for i, c := range cs {
		o.checked++
		if c.Signals&(c.Signals-1) != 0 && !c.InScope && c.Confidence < 0.9 {
			o.add(wpcorpus.CandidatesPath, i+2, 0, "the signals %s name %s, and the classifier gives %s with the confidence %.2f (rule %s)", c.Signals, c.Title, c.Verdict, c.Confidence, c.Rule)
		}
	}
	return o
}

// y13 reports each article whose revision of record is more than 60 days
// older than its newest settled revision (WP2.6). The metadata file names
// the newest settled revision when m2 wp fetch reads the history.
func y13(a *auditor) outcome {
	var o outcome
	if a.wikipedia == "" {
		o.reason = "no checkout of tamnd/wikipedia-math is given, so set -wikipedia to run the rule"
		return o
	}
	ms, err := wpcorpus.ReadMetadataFiles(a.wikipedia)
	if err != nil {
		o.add(wpcorpus.MetadataDir, 0, 0, "%v", err)
		return o
	}
	if len(ms) == 0 {
		o.reason = "the checkout has no metadata files yet. Run m2 wp fetch to make them"
		return o
	}
	for _, m := range ms {
		o.checked++
		if m.SettledTimestamp == "" {
			continue
		}
		rec, err1 := time.Parse(time.RFC3339, m.Timestamp)
		set, err2 := time.Parse(time.RFC3339, m.SettledTimestamp)
		if err := errors.Join(err1, err2); err != nil {
			o.add(wpcorpus.MetadataPath(m.Page), 0, 0, "%v", err)
			continue
		}
		if d := set.Sub(rec); d > 60*24*time.Hour {
			o.add(wpcorpus.MetadataPath(m.Page), 0, 0, "the revision of record %d of %s is %d days older than the newest settled revision %d", m.Revision, m.Title, int(d.Hours()/24), m.Settled)
		}
	}
	return o
}
