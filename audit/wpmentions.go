package audit

import (
	"errors"
	"fmt"
	"os"
	"regexp"

	wpImport "github.com/tamnd/math2-agent/importer/wikipedia"
	"github.com/tamnd/math2-agent/inventory"
)

// y04 checks that each article in scope has an article mention, and that each object of an item kind has a mention (WP5.8). A proof must be attached, and a member must name a mention that exists.
func y04(a *auditor) outcome {
	var o outcome
	if a.wikipedia == "" {
		o.reason = "no checkout of tamnd/wikipedia-math is given, so set -wikipedia to run the rule"
		return o
	}
	t := a.inventory()
	if t.err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", t.err)
		return o
	}
	wants, err := wpImport.Expect(a.wikipedia)
	if errors.Is(err, os.ErrNotExist) {
		o.reason = "the checkout has no scope list yet. Run m2 wp scope to make it"
		return o
	}
	if err != nil {
		o.add(a.wikipedia, 0, 0, "%v", err)
		return o
	}
	for _, w := range wants {
		o.checked++
		switch {
		case w.Mention == "":
			o.add(w.File, 0, 0, "the object %s has no tag, so it has no mention. Run m2 wp extract on the checkout", w.Ref)
		case t.ms[w.Mention] == nil && w.Ref == "":
			o.add(w.File, 0, 0, "the article has no mention: %s is not in %s. Run m2 corpus import -source wikipedia", w.Mention, inventory.MentionsDir)
		case t.ms[w.Mention] == nil:
			o.add(w.File, 0, 0, "the object %s has no mention: %s is not in %s. Run m2 corpus import -source wikipedia", w.Ref, w.Mention, inventory.MentionsDir)
		}
	}
	err = inventory.WalkMentions(a.root, func(file string, line int, m inventory.Mention) error {
		if m.Source != wpImport.Source || m.Retired != 0 {
			return nil
		}
		switch {
		case m.Kind == "proof" && m.AttachedTo == "":
			o.add(file, line, 1, "the proof %s has no attached_to", m.ID)
		case m.AttachedTo != "" && t.ms[m.AttachedTo] == nil:
			o.add(file, line, 1, "the mention %s is attached to %s, and that mention does not exist", m.ID, m.AttachedTo)
		case m.MemberOf != "" && t.ms[m.MemberOf] == nil:
			o.add(file, line, 1, "the mention %s is a member of %s, and that mention does not exist", m.ID, m.MemberOf)
		}
		return nil
	})
	if err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", err)
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "the checkout has no article with an objects file"
	}
	return o
}

var (
	wpTextRef   = regexp.MustCompile(`^content/wikitext/\d{4}/\d+\.wiki@r\d+(#\d+-\d+)?$`)
	wpLocator   = regexp.MustCompile(`, https://en\.wikipedia\.org/w/index\.php\?oldid=\d+(#\S*)?$`)
	wpMentionID = regexp.MustCompile(`^wikipedia://en/(\d+)(#\S+)?$`)
)

// y05 checks that no Wikipedia mention or edge of the record plane holds article text (WP5.3). The text of the articles is CC BY-SA, and the record plane is CC0. A mention can have its name, its aliases and the title and section in its locator. The other text fields must be empty, and text_ref and hash must have their forms.
func y05(a *auditor) outcome {
	var o outcome
	err := inventory.WalkMentions(a.root, func(file string, line int, m inventory.Mention) error {
		if m.Source != wpImport.Source {
			return nil
		}
		o.checked++
		bad := func(field, why string) {
			o.add(file, line, 1, "the Wikipedia mention %s has a %s that %s", m.ID, field, why)
		}
		for _, f := range [][2]string{{"text", m.Text}, {"head", m.Head}, {"label", m.Label}, {"env", m.Env}, {"number", m.Number}} {
			if f[1] != "" {
				bad(f[0], "is not empty")
			}
		}
		if len(m.Uses) > 0 {
			bad("uses", "is not empty")
		}
		if len(m.Cites) > 0 {
			bad("cites", "is not empty")
		}
		if m.TextRef != "" && !wpTextRef.MatchString(m.TextRef) {
			bad("text_ref", "is not of the form content/wikitext/<shard>/<pageid>.wiki@r<revision>#<start>-<end>")
		}
		if m.Hash != "" && !contentHash.MatchString(m.Hash) {
			bad("hash", "is not a sha256 hash")
		}
		if m.Locator != "" && !wpLocator.MatchString(m.Locator) {
			bad("locator", "does not end with the permanent link of the revision")
		}
		return nil
	})
	if err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", err)
	}
	err = inventory.WalkEdges(a.root, wpImport.Source, func(file string, line int, e inventory.Edge) error {
		o.checked++
		for _, f := range [][2]string{{"predicate", e.Predicate}, {"method", e.Method}, {"confidence", e.Confidence}, {"finding", e.Finding}} {
			if !token.MatchString(f[1]) {
				o.add(file, line, 1, "the edge from %s to %s has a %s that is not a short token", e.From, e.To, f[0])
			}
		}
		return nil
	})
	if err != nil {
		o.add(inventory.EdgesDir, 0, 0, "%v", err)
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "the record plane has no Wikipedia mention"
	}
	return o
}

// y06 checks that each Wikipedia mention and edge names a page id and a revision id, not only a title (WP5.2).
func y06(a *auditor) outcome {
	var o outcome
	err := inventory.WalkMentions(a.root, func(file string, line int, m inventory.Mention) error {
		if m.Source != wpImport.Source {
			return nil
		}
		o.checked++
		id := wpMentionID.FindStringSubmatch(m.ID)
		switch {
		case id == nil:
			o.add(file, line, 1, "the Wikipedia mention %s does not have an id of the form wikipedia://en/<pageid>", m.ID)
		case m.Page == 0:
			o.add(file, line, 1, "the Wikipedia mention %s has no page id", m.ID)
		case id[1] != fmt.Sprint(m.Page):
			o.add(file, line, 1, "the Wikipedia mention %s has the page id %d, and its id names another page", m.ID, m.Page)
		}
		if m.Revision == 0 {
			o.add(file, line, 1, "the Wikipedia mention %s has no revision id", m.ID)
		}
		return nil
	})
	if err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", err)
	}
	err = inventory.WalkEdges(a.root, wpImport.Source, func(file string, line int, e inventory.Edge) error {
		o.checked++
		if !wpMentionID.MatchString(e.From) {
			o.add(file, line, 1, "the edge to %s comes from %s, which is not a mention id with a page id", e.To, e.From)
		}
		if e.Revision == 0 {
			o.add(file, line, 1, "the edge from %s to %s has no revision id", e.From, e.To)
		}
		return nil
	})
	if err != nil {
		o.add(inventory.EdgesDir, 0, 0, "%v", err)
	}
	if o.checked == 0 && len(o.findings) == 0 {
		o.reason = "the record plane has no Wikipedia mention"
	}
	return o
}
