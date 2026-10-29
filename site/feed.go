package site

import (
	"encoding/xml"
	"fmt"
	"hash/fnv"
	"html/template"
	"slices"
	"strings"
	"time"
)

// The feeds show the nodes as cards, one idea on each card. Latest is a timeline of the nodes by their last change, the newest first. Discover is a deck of cards that fill the screen, one at a time, in an order that changes each day.
const (
	latestPer    = 40  // the cards on a page of Latest
	latestMax    = 400 // the cards of Latest in all
	discoverPer  = 20  // the cards on a page of Discover
	discoverMax  = 300 // the cards of Discover in all
	atomEntries  = 50  // the entries of the Atom feed
	feedDateForm = "2 January 2006"
)

// Label returns the kind of the node in words for a reader: its role, for example Theorem, or else its kind.
func (v *nodeView) Label() string {
	s := v.Role
	if s == "" {
		s = v.Kind
	}
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Subject returns the name of the top MSC class of the node, or "".
func (v *nodeView) Subject() string {
	for _, c := range v.MSC {
		if len(c) >= 2 && mscTop[c[:2]] != "" {
			return mscTop[c[:2]]
		}
	}
	return ""
}

// SubjectCode returns the top MSC class of the node, or "".
func (v *nodeView) SubjectCode() string {
	for _, c := range v.MSC {
		if len(c) >= 2 && mscTop[c[:2]] != "" {
			return c[:2]
		}
	}
	return ""
}

// Uses returns the nodes that this node uses, for the line "Builds on" of a page and a card.
func (v *nodeView) Uses() []link {
	for _, g := range v.Refs {
		if g.Field == "uses" {
			return g.Links
		}
	}
	return nil
}

type feedCard struct {
	*nodeView
	Text template.HTML // the text on the card
	New  bool          // the node is new: its first and its last change are on the same day
	Date string
	When string // the time of the last change, for the datetime attribute
}

type feedDay struct {
	Day   string
	Cards []feedCard
}

type feedData struct {
	Days        []feedDay  // Latest, by day
	Cards       []feedCard // Discover
	Page, Pages int
	Prev, Next  string
	History     bool
	Total       int
}

// feedNodes returns the nodes that make good cards: each node with a text for its card, but not a definition of a concept that is also there, because the card of the concept leads to its definitions.
func (b *builder) feedNodes() []*nodeView {
	var out []*nodeView
	for _, v := range b.order {
		if v.Retired != nil || (v.Excerpt == "" && v.PlainExcerpt == "") {
			continue
		}
		if v.Kind == "definition" {
			if c, ok := v.Meta["defines"].(string); ok && b.nodes[c] != nil && b.nodes[c].Excerpt != "" {
				continue
			}
		}
		out = append(out, v)
	}
	return out
}

func (b *builder) card(v *nodeView, plainFirst bool) feedCard {
	c := feedCard{nodeView: v, Text: v.Excerpt}
	if plainFirst && v.PlainExcerpt != "" || c.Text == "" {
		c.Text = v.PlainExcerpt
	}
	if !v.Updated.IsZero() {
		c.Date = v.Updated.UTC().Format(feedDateForm)
		c.When = v.Updated.UTC().Format(time.RFC3339)
		c.New = v.Added.UTC().Format(time.DateOnly) == v.Updated.UTC().Format(time.DateOnly)
	}
	return c
}

// pageURL returns the URL of page i of a feed at dir: dir/ for the first page and dir/i/ for the others.
func (b *builder) pageURL(dir string, i int) string {
	if i == 1 {
		return b.url(dir + "/")
	}
	return b.url(fmt.Sprintf("%s/%d/", dir, i))
}

func feedDir(dir string, i int) string {
	if i == 1 {
		return dir
	}
	return fmt.Sprintf("%s/%d", dir, i)
}

func (b *builder) writeFeeds() error {
	nodes := b.feedNodes()
	latest := slices.Clone(nodes)
	if b.history {
		slices.SortStableFunc(latest, func(x, y *nodeView) int { return y.Updated.Compare(x.Updated) })
	}
	latest = latest[:min(len(latest), latestMax)]
	pages := max(1, (len(latest)+latestPer-1)/latestPer)
	for i := 1; i <= pages; i++ {
		d := feedData{Page: i, Pages: pages, History: b.history, Total: len(nodes)}
		for _, v := range latest[(i-1)*latestPer : min(len(latest), i*latestPer)] {
			c := b.card(v, false)
			if n := len(d.Days); n == 0 || d.Days[n-1].Day != c.Date {
				d.Days = append(d.Days, feedDay{Day: c.Date})
			}
			d.Days[len(d.Days)-1].Cards = append(d.Days[len(d.Days)-1].Cards, c)
		}
		if i > 1 {
			d.Prev = b.pageURL("latest", i-1)
		}
		if i < pages {
			d.Next = b.pageURL("latest", i+1)
		}
		title := "Latest"
		if i > 1 {
			title = fmt.Sprintf("Latest, page %d", i)
		}
		p := page{Title: title, Description: "The newest and the changed ideas in Math 2.0, the newest first.", Section: "latest", Math: true, FeedURL: b.url("latest/feed.xml")}
		if err := b.writePage(feedDir("latest", i), "latest", d, p); err != nil {
			return err
		}
	}
	if err := b.writeAtom(latest[:min(len(latest), atomEntries)]); err != nil {
		return err
	}

	// Discover shuffles the nodes with the day of the build, so that the deck is new each day and the same for each reader on that day.
	seed := ""
	if !b.opt.Date.IsZero() {
		seed = b.opt.Date.UTC().Format(time.DateOnly)
	}
	deck := slices.Clone(nodes)
	key := func(v *nodeView) uint64 {
		h := fnv.New64a()
		_, _ = h.Write([]byte(seed + v.Tag))
		return h.Sum64()
	}
	slices.SortFunc(deck, func(x, y *nodeView) int {
		kx, ky := key(x), key(y)
		switch {
		case kx < ky:
			return -1
		case kx > ky:
			return 1
		}
		return strings.Compare(x.Tag, y.Tag)
	})
	deck = deck[:min(len(deck), discoverMax)]
	pages = max(1, (len(deck)+discoverPer-1)/discoverPer)
	for i := 1; i <= pages; i++ {
		d := feedData{Page: i, Pages: pages, History: b.history, Total: len(nodes)}
		for _, v := range deck[(i-1)*discoverPer : min(len(deck), i*discoverPer)] {
			d.Cards = append(d.Cards, b.card(v, true))
		}
		if i > 1 {
			d.Prev = b.pageURL("discover", i-1)
		}
		if i < pages {
			d.Next = b.pageURL("discover", i+1)
		}
		p := page{Title: "Discover", Description: "One idea of mathematics at a time. Scroll for the next one.", Section: "discover", Math: true, Bare: true}
		if err := b.writePage(feedDir("discover", i), "discover", d, p); err != nil {
			return err
		}
	}
	return nil
}

// writeAtom writes latest/feed.xml, an Atom feed of the newest nodes, for a feed reader.
func (b *builder) writeAtom(nodes []*nodeView) error {
	type link struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr,omitempty"`
	}
	type content struct {
		Type string `xml:"type,attr"`
		Body string `xml:",chardata"`
	}
	type entry struct {
		Title   string   `xml:"title"`
		ID      string   `xml:"id"`
		Link    link     `xml:"link"`
		Updated string   `xml:"updated"`
		Summary *content `xml:"summary,omitempty"`
		Kind    string   `xml:"category,omitempty"`
	}
	type feed struct {
		XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
		Title   string   `xml:"title"`
		ID      string   `xml:"id"`
		Links   []link   `xml:"link"`
		Updated string   `xml:"updated"`
		Author  string   `xml:"author>name"`
		Entries []entry  `xml:"entry"`
	}
	f := feed{Title: "Math 2.0, the latest ideas", ID: b.abs("latest/"), Author: "Math 2.0", Links: []link{{Href: b.abs("latest/")}, {Href: b.abs("latest/feed.xml"), Rel: "self"}}}
	newest := b.opt.Date
	for _, v := range nodes {
		at := v.Updated
		if at.IsZero() {
			at = b.opt.Date
		}
		if at.After(newest) {
			newest = at
		}
		e := entry{Title: v.Name, ID: b.abs("wiki/" + v.Slug + "/"), Link: link{Href: b.abs("wiki/" + v.Slug + "/")}, Updated: at.UTC().Format(time.RFC3339)}
		if v.Excerpt != "" {
			e.Summary = &content{Type: "html", Body: string(v.Excerpt)}
		}
		f.Entries = append(f.Entries, e)
	}
	f.Updated = newest.UTC().Format(time.RFC3339)
	out, err := xml.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return b.write("latest/feed.xml", append([]byte(xml.Header), append(out, '\n')...))
}
