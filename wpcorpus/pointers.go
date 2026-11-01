package wpcorpus

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// PointersDir holds the incoming pointers of each article in scope, the
// input of the witness W-in (WP4.2). It is under work, so it is not
// committed. The census record holds the pointers and their matches.
const PointersDir = "work/pointers"

// PointersPath gives the path of the pointers file of a page.
func PointersPath(page int) string {
	return filepath.Join(PointersDir, Shard(page), strconv.Itoa(page)+".md")
}

// The sources of a pointer.
const (
	ViaRedirect = "redirect"
	ViaLink     = "link"
)

// FindingBrokenAnchor is the finding of a pointer to an anchor that the
// article does not have (WP4.4).
const FindingBrokenAnchor = "broken-anchor"

// Pointer is one incoming pointer of the witness W-in: a redirect with a
// fragment, or a link with an anchor from another page (WP4.2).
type Pointer struct {
	Via string `json:"via" yaml:"via"`
	// From is the title of the redirect, or of the page with the link.
	From string `json:"from" yaml:"from"`
	// Anchor is the fragment, with spaces and not underscores.
	Anchor string `json:"anchor" yaml:"anchor"`
	// Object is the ref of the object that the pointer matches (WP4.4).
	Object  string `json:"object,omitempty" yaml:"object,omitempty"`
	Finding string `json:"finding,omitempty" yaml:"finding,omitempty"`
}

// PointerFile is the pointers file of an article.
type PointerFile struct {
	Page     int       `yaml:"page"`
	Title    string    `yaml:"title"`
	Pointers []Pointer `yaml:"pointers,omitempty"`
}

// MarshalPointers gives the pointers file of an article.
func MarshalPointers(f *PointerFile) ([]byte, error) {
	var b bytes.Buffer
	if err := writeFrontMatter(&b, f); err != nil {
		return nil, err
	}
	n := map[string]int{}
	for _, p := range f.Pointers {
		n[p.Via]++
	}
	fmt.Fprintf(&b, "\n# Pointers to %s\n\n%s and %s of other pages point to an anchor of the article.\n", f.Title, count(n[ViaRedirect], "redirect"), count(n[ViaLink], "link"))
	return b.Bytes(), nil
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// WritePointers writes the pointers file of an article.
func WritePointers(root string, f *PointerFile) error {
	data, err := MarshalPointers(f)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(root, PointersPath(f.Page)), data)
}

// ReadPointers reads the pointers file of a page. It gives nil and no error
// when the corpus does not have the file.
func ReadPointers(root string, page int) (*PointerFile, error) {
	data, err := os.ReadFile(filepath.Join(root, PointersPath(page))) // #nosec G304 -- the path is in the corpus that the user names
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	head, _, err := splitFrontMatter(data)
	if err != nil {
		return nil, err
	}
	var f PointerFile
	if err := yaml.Unmarshal(head, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", PointersPath(page), err)
	}
	return &f, nil
}

// normAnchor gives the form of an anchor that the matching compares.
func normAnchor(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "_", " ")), " ")
}

// RedirectPointers reads the redirects with a fragment from the store. It
// gives the pointers by the title of the target, with underscores, for the
// targets in titles.
func (s *Store) RedirectPointers(titles map[string]bool) (map[string][]Pointer, error) {
	out := map[string][]Pointer{}
	ok, err := s.Has("redirect")
	if err != nil || !ok {
		return out, err
	}
	rows, err := s.DB.Query(`SELECT p.page_title, r.rd_title, r.rd_fragment FROM redirect r JOIN page p ON p.page_id = r.rd_from WHERE r.rd_namespace = 0 AND p.page_namespace = 0 AND r.rd_fragment != ''`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var from, to, frag string
		if err := rows.Scan(&from, &to, &frag); err != nil {
			return nil, err
		}
		if a := normAnchor(frag); titles[to] && a != "" {
			out[to] = append(out[to], Pointer{Via: ViaRedirect, From: strings.ReplaceAll(from, "_", " "), Anchor: a})
		}
	}
	return out, rows.Err()
}

// AnchorLink is a link with an anchor in wikitext, as it is written: the
// link title and the fragment. It does not depend on the scope, so m2 wp
// pointers can keep the anchor links of a dump in a cache.
type AnchorLink struct {
	Title string
	Frag  string
}

// AnchorLinks gives the links with an anchor of wikitext, in order.
func AnchorLinks(wikitext string) []AnchorLink {
	var out []AnchorLink
	for rest := wikitext; ; {
		i := strings.Index(rest, "[[")
		if i < 0 {
			break
		}
		rest = rest[i+2:]
		end := strings.IndexAny(rest, "|]\n")
		if end < 0 {
			break
		}
		title, frag, ok := strings.Cut(rest[:end], "#")
		title = strings.TrimPrefix(strings.TrimSpace(title), ":")
		if !ok || title == "" {
			continue
		}
		out = append(out, AnchorLink{title, frag})
	}
	return out
}

// LinkPointers gives the links with an anchor of the wikitext of the page
// from. target gives the title of the article in scope, with underscores,
// that a link title names, or "". A link of a page to itself is not a
// pointer, and two links with the same anchor are one pointer.
func LinkPointers(from, wikitext string, target func(string) string) map[string][]Pointer {
	return AnchorPointers(from, AnchorLinks(wikitext), target)
}

// AnchorPointers gives the pointers of the anchor links of the page from,
// with the rules of LinkPointers.
func AnchorPointers(from string, links []AnchorLink, target func(string) string) map[string][]Pointer {
	out := map[string][]Pointer{}
	self := normTitle(from)
	seen := map[[2]string]bool{}
	for _, l := range links {
		to := target(l.Title)
		a := normAnchor(decodeAnchor(l.Frag))
		if to == "" || to == self || a == "" || seen[[2]string{to, a}] {
			continue
		}
		seen[[2]string{to, a}] = true
		out[to] = append(out[to], Pointer{Via: ViaLink, From: from, Anchor: a})
	}
	return out
}

// decodeAnchor decodes the HTML entities and the percent escapes that a
// link can have in its fragment, such as &#39; and %27.
func decodeAnchor(s string) string {
	s = html.UnescapeString(s)
	if strings.Contains(s, "%") {
		if d, err := url.PathUnescape(s); err == nil {
			s = d
		}
	}
	return s
}

// ScopeTarget gives the function that LinkPointers uses: the title of the
// article in scope, with underscores, that a link title names, directly or
// through a redirect of target, or "".
func ScopeTarget(cs []*Candidate, target map[string]string) func(string) string {
	in := make(map[string]bool, len(cs))
	for _, c := range cs {
		if c.InScope {
			in[normTitle(c.Title)] = true
		}
	}
	return func(title string) string {
		t := normTitle(title)
		if in[t] {
			return t
		}
		if to, ok := target[t]; ok && in[normTitle(to)] {
			return normTitle(to)
		}
		return ""
	}
}

// SortPointers sorts the pointers of an article: the redirects first, then
// by the source and the anchor.
func SortPointers(ps []Pointer) {
	slices.SortFunc(ps, func(a, b Pointer) int {
		if c := strings.Compare(a.Via, b.Via); c != 0 {
			return -c
		}
		if c := strings.Compare(a.From, b.From); c != 0 {
			return c
		}
		return strings.Compare(a.Anchor, b.Anchor)
	})
}

// FindingTemplateAnchor is the finding of a pointer to an anchor that only
// the Parsoid HTML has: a template that W-text does not read makes it.
const FindingTemplateAnchor = "template-anchor"

// The names of the witnesses of a census (WP4.2).
const (
	WitnessText = "text"
	WitnessHTML = "html"
	WitnessIn   = "in"
)

// partKinds are the kinds of the objects that are parts of a section, and
// not items. A section that holds only these objects holds no item.
var partKinds = map[string]bool{"section": true, "citation": true, "infobox": true, "table": true, "subject": true}

// pointers matches each pointer of W-in to an object (WP4.4). A pointer
// matches the object with the anchor, then the section with the anchor. A
// redirect to a section that holds no item makes a pointer-item object with
// the name of the redirect. A pointer that matches no anchor is a finding
// broken-anchor, and the item is searched for by its name.
func (x *extractor) pointers(in []Pointer) {
	made := map[*section]*object{}
	for _, p := range in {
		q := &pointer{Pointer: p}
		x.ptrs = append(x.ptrs, q)
		a := normAnchor(p.Anchor)
		if o := x.anchors[a]; o != nil {
			q.obj = o
		} else if s := x.sectionOf(a); s != nil {
			switch {
			case s.item != nil:
				q.obj = s.item
			case made[s] != nil:
				q.obj = made[s]
			case p.Via == ViaRedirect && !x.holdsItem(s):
				q.obj = x.item("pointer", "", "", Object{Kind: "pointer-item", Cue: "pointer", Strength: StrengthNormal, Name: p.From}, s.node.Start, s.node.End)
				made[s] = q.obj
			default:
				q.obj = s.obj
			}
		} else {
			q.Finding = FindingBrokenAnchor
			q.obj = x.named(a, p.From)
		}
		if o := q.obj; o != nil && p.Via == ViaRedirect && !strings.EqualFold(o.Name, p.From) {
			o.Aliases = appendOnce(o.Aliases, p.From)
		}
	}
}

// sectionOf gives the section with the anchor. An anchor that differs only
// in case matches when no section has the same anchor.
func (x *extractor) sectionOf(a string) *section {
	var fold *section
	for _, s := range x.secs {
		sa := normAnchor(s.anchor)
		if sa == "" {
			continue
		}
		if sa == a {
			return s
		}
		if fold == nil && strings.EqualFold(sa, a) {
			fold = s
		}
	}
	return fold
}

// holdsItem tells if a section, with its subsections, holds an object that
// is an item.
func (x *extractor) holdsItem(s *section) bool {
	for _, o := range x.objs {
		if !partKinds[o.Kind] && s.node.Start <= o.Span[0] && o.Span[1] <= s.node.End {
			return true
		}
	}
	return false
}

// named gives the object whose name is one of the names, the item before
// the section, or nil.
func (x *extractor) named(names ...string) *object {
	var sec *object
	for _, o := range x.objs {
		for _, n := range names {
			if n == "" || !strings.EqualFold(normAnchor(o.Name), normAnchor(n)) {
				continue
			}
			if !partKinds[o.Kind] {
				return o
			}
			if o.Kind == "section" && sec == nil {
				sec = o
			}
		}
	}
	return sec
}
