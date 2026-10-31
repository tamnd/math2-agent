package audit

import (
	"regexp"

	axImport "github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/inventory"
)

// d05 checks that each object of an item kind in the latest version of an extracted arXiv paper has a mention, and that each attached mention names a target that has a mention (AX5.8). It reads the analysis plane of the checkout of tamnd/arxiv that Options.Arxiv names.
func d05(a *auditor) outcome {
	var o outcome
	if a.arxiv == "" {
		o.reason = "no checkout of tamnd/arxiv is given, so set -arxiv to run the rule"
		return o
	}
	t := a.inventory()
	if t.err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", t.err)
		return o
	}
	classified, err := axImport.ReadClassified(a.root)
	if err != nil {
		o.add(axImport.ClassifyDir, 0, 0, "%v", err)
		return o
	}
	wants, err := axImport.Expect(a.arxiv, classified)
	if err != nil {
		o.add(a.arxiv, 0, 0, "%v", err)
		return o
	}
	for _, w := range wants {
		o.checked++
		switch {
		case w.Mention == "":
			o.add(w.File, 0, 0, "the object %s has no tag, so it has no mention. Run ax tags analysis on the checkout", w.Ref)
		case t.ms[w.Mention] == nil:
			o.add(w.File, 0, 0, "the object %s has no mention: %s is not in %s", w.Ref, w.Mention, inventory.MentionsDir)
		}
	}
	err = inventory.WalkMentions(a.root, func(file string, line int, m inventory.Mention) error {
		if m.Source != axImport.Source {
			return nil
		}
		switch {
		case m.Kind == "proof" && m.AttachedTo == "":
			o.add(file, line, 1, "the proof %s has no attached_to", m.ID)
		case m.Attached() && m.AttachedTo != "unknown" && t.ms[m.AttachedTo] == nil:
			o.add(file, line, 1, "the mention %s is attached to %s, and that mention does not exist", m.ID, m.AttachedTo)
		}
		return nil
	})
	if err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", err)
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "the checkout has no object of an item kind"
	}
	return o
}

var (
	recordLocator = regexp.MustCompile(`^\S+v\d+#\S+$`)
	contentHash   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	token         = regexp.MustCompile(`^\S{0,40}$`)
	shortLocator  = regexp.MustCompile(`^[\w.\- ]{0,32}$`)
)

// d06 checks that no arXiv mention of the class record holds text of the paper (AX5.4). Each string field of such a mention must be an id, a label, a number, a hash or a value from a closed list.
func d06(a *auditor) outcome {
	var o outcome
	err := inventory.WalkMentions(a.root, func(file string, line int, m inventory.Mention) error {
		if m.Source != axImport.Source || m.Class != "record" {
			return nil
		}
		o.checked++
		bad := func(field, why string) {
			o.add(file, line, 1, "the mention %s has the class record, and its %s %s", m.ID, field, why)
		}
		if m.Name != "" {
			bad("name", "is not empty")
		}
		if m.Text != "" {
			bad("text", "is not empty")
		}
		if m.TextRef != "" {
			bad("text_ref", "is not empty")
		}
		if len(m.Aliases) > 0 {
			bad("aliases", "are not empty")
		}
		if len(m.Cites) > 0 {
			bad("cites", "are not empty")
		}
		if !recordLocator.MatchString(m.Locator) {
			bad("locator", "is not of the form <id>v<n>#<local>")
		}
		for _, l := range m.Also {
			if !recordLocator.MatchString(l) {
				bad("also", "has a locator that is not of the form <id>v<n>#<local>")
				break
			}
		}
		for _, f := range [][2]string{{"label", m.Label}, {"number", m.Number}, {"env", m.Env}, {"head", m.Head}} {
			if !token.MatchString(f[1]) {
				bad(f[0], "is not a short token")
			}
		}
		if m.Hash != "" && !contentHash.MatchString(m.Hash) {
			bad("hash", "is not a sha256 hash")
		}
		for _, c := range m.CitesResult {
			if !shortLocator.MatchString(c.Locator) {
				bad("cites_result", "has a locator that is not a short reference")
				break
			}
		}
		return nil
	})
	if err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", err)
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "no arXiv mention has the class record"
	}
	return o
}
