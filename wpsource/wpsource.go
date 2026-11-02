// Package wpsource reads the value of the wikipedia key of a node source (corpus wikipedia WP9.3).
//
// The value is en:<pageid>@r<revision>. It names the page and the revision that the node adopted or adapted text from. The locator of the source gives the title and the section.
package wpsource

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Key is the identifier key of a Wikipedia source in the node schema.
const Key = "wikipedia"

// Pattern is the form of the value. The schema has the same pattern.
const Pattern = `^en:[1-9][0-9]*@r[1-9][0-9]*$`

var form = regexp.MustCompile(Pattern)

// Ref is a page and a revision of the English Wikipedia.
type Ref struct {
	Page     int64
	Revision int64
}

// Parse reads a value of the form en:<pageid>@r<revision>.
func Parse(s string) (Ref, error) {
	if !form.MatchString(s) {
		return Ref{}, fmt.Errorf("wpsource: %q is not of the form en:<pageid>@r<revision>", s)
	}
	var r Ref
	var err error
	page, rev, _ := strings.Cut(strings.TrimPrefix(s, "en:"), "@r")
	if r.Page, err = strconv.ParseInt(page, 10, 64); err != nil {
		return Ref{}, fmt.Errorf("wpsource: page id of %q: %w", s, err)
	}
	if r.Revision, err = strconv.ParseInt(rev, 10, 64); err != nil {
		return Ref{}, fmt.Errorf("wpsource: revision of %q: %w", s, err)
	}
	return r, nil
}

// String gives the value of the source key.
func (r Ref) String() string { return fmt.Sprintf("en:%d@r%d", r.Page, r.Revision) }

// URL gives the permanent link of the revision.
func (r Ref) URL() string {
	return fmt.Sprintf("https://en.wikipedia.org/w/index.php?oldid=%d", r.Revision)
}

// HistoryURL gives the history page of the article, which lists its authors.
func (r Ref) HistoryURL() string {
	return fmt.Sprintf("https://en.wikipedia.org/w/index.php?curid=%d&action=history", r.Page)
}

// Title gives the text of a citation of the revision.
func (r Ref) Title() string {
	return fmt.Sprintf("Wikipedia, page %d, revision %d", r.Page, r.Revision)
}
