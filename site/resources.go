package site

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/depth"
)

// resPerPage is the number of resources on one page of a list. A list of more resources has more pages.
const resPerPage = 500

// resView is one resource on its page and in the lists.
type resView struct {
	*resource.Resource
	URL, URI     string
	Creators     string // the names of the creators, in one line
	Year         string
	Tier         string // the tier of the build, or ""
	Depth        string // the depth of the build as D<n>, or ""
	Target       string // the target depth as D<n>, or ""
	Duplicate    *link  // the live resource of a duplicate
	IDs          []link
	Subjects     []string
	Expressions  []resExpr
	Relations    []resRel
	RelatedBy    []resRel // the edges from other resources to this one
	Cites, Cited []resCite
	Mentions     []resMention
}

// resExpr is one expression on the page of its resource.
type resExpr struct {
	*resource.Expression
	Contributors   string
	IDs            []link
	Manifestations []resManif
	Structure      []resNode // the table of contents, as a flat list with a depth for each node
	Source         string    // the source of the structure
}

// resManif is one manifestation of an expression.
type resManif struct {
	*resource.Manifestation
	IDs    []link
	Access string
}

// resNode is one node of a structure, with its depth in the tree from 0.
type resNode struct {
	resource.Node
	Depth int
}

// resRel is one edge between two resources.
type resRel struct {
	Kind, From, Volume string
	To                 link
}

// resCite is one reference of a citing expression.
type resCite struct {
	N    int
	From string
	Text string
	To   *link
}

// resMention is one mention whose field resource names the resource.
type resMention struct {
	Name, Kind, Source, At string
	Card                   string // the card of the inventory row of the mention, or ""
}

// resKind is the list of the resources of one kind.
type resKind struct {
	Kind, Dir, URL string
	Rows           []*resView
	Pages          []resPageLink
}

type resPageLink struct {
	N        int
	URL      string
	Current  bool
	From, To int
}

type resListPage struct {
	Kind  *resKind
	Rows  []*resView
	Pages []resPageLink
	Kinds []*resKind
}

type resIndex struct {
	Total, Live int
	Kinds       []*resKind
	Tiers       []invCount
	Decades     []invCount
	Access      []invCount
	Languages   []invCount
	SpecURL     string
}

// resURL returns the URL path of the page of a resource. The expression and the manifestation of a target, as in <id>/e<n>, give an anchor on the page.
func (b *builder) resURL(target string) string {
	id, key, _ := strings.Cut(target, "/")
	u := b.url("res/" + id + "/")
	if key != "" {
		u += "#" + strings.ReplaceAll(key, "/", "-")
	}
	return u
}

// resLink returns the link to a resource target, with the title of the resource when the catalogue has it.
func (b *builder) resLink(target string) link {
	id, _, _ := strings.Cut(target, "/")
	l := link{URL: b.resURL(target), Text: target}
	if v := b.res[id]; v != nil {
		l.Text = v.Title
		if target != id {
			l.Note = strings.TrimPrefix(target, id+"/")
		}
	} else {
		l.URL, l.Missing = "", true
	}
	return l
}

// loadResources reads the catalogue of the tree, the tiers and the depths of the build, the structures and the cites. A tree with no catalogue gives no resource pages but the index.
func (b *builder) loadResources() error {
	rs, err := resource.ReadResources(b.opt.Content)
	if err != nil {
		return fmt.Errorf("site: read the resources: %w", err)
	}
	rows, err := depth.Read(b.opt.Content)
	if err != nil {
		return fmt.Errorf("site: read the depths of the resources: %w", err)
	}
	b.res = make(map[string]*resView, len(rs))
	for i := range rs {
		r := &rs[i]
		v := &resView{Resource: r, URL: b.resURL(r.ID), URI: resource.URI(r.ID), Creators: creators(r.Creators), Year: year(r.Date)}
		if row, ok := rows[r.ID]; ok {
			v.Tier, v.Depth = row.Tier, "D"+strconv.Itoa(row.Depth)
			if row.Target > 0 {
				v.Target = "D" + strconv.Itoa(row.Target)
			}
		}
		v.IDs = idLinks(r.IDs)
		for _, s := range r.Subjects {
			v.Subjects = append(v.Subjects, s.System+" "+s.Code)
		}
		b.res[r.ID] = v
		b.resOrder = append(b.resOrder, v)
	}
	for _, v := range b.resOrder {
		r := v.Resource
		if r.Scope != nil && r.Scope.Value == resource.Duplicate && r.Scope.Target != "" {
			l := b.resLink(r.Scope.Target)
			v.Duplicate = &l
		}
		for i := range r.Expressions {
			e := &r.Expressions[i]
			x := resExpr{Expression: e, Contributors: creators(e.Contributors), IDs: idLinks(e.IDs)}
			for j := range e.Manifestations {
				m := &e.Manifestations[j]
				x.Manifestations = append(x.Manifestations, resManif{Manifestation: m, IDs: idLinks(m.IDs), Access: access(cmp.Or(m.Access, e.Access))})
			}
			if e.Structure != "" {
				s, err := resource.ReadStructure(b.opt.Content, r.ID, e.Key)
				if err != nil {
					return fmt.Errorf("site: %w", err)
				}
				if s != nil {
					x.Structure, x.Source = flatten(s.Nodes, 0, nil), s.Source
				}
			}
			v.Expressions = append(v.Expressions, x)
		}
		for _, rel := range r.Relations {
			v.Relations = append(v.Relations, resRel{Kind: rel.Kind, From: rel.From, Volume: rel.Volume, To: b.resLink(rel.To)})
			id, _, _ := strings.Cut(rel.To, "/")
			if t := b.res[id]; t != nil {
				t.RelatedBy = append(t.RelatedBy, resRel{Kind: rel.Kind, Volume: rel.Volume, To: link{URL: v.URL, Text: r.Title}})
			}
		}
	}
	cs, err := resource.ReadCites(b.opt.Content)
	if err != nil {
		return fmt.Errorf("site: read the cites of the resources: %w", err)
	}
	for _, c := range cs {
		id, _, _ := strings.Cut(c.From, "/")
		v := b.res[id]
		if v == nil {
			continue
		}
		rc := resCite{N: c.N, From: strings.TrimPrefix(c.From, id+"/"), Text: c.Text}
		if c.To != "" {
			l := b.resLink(c.To)
			rc.To = &l
			to, _, _ := strings.Cut(c.To, "/")
			if t := b.res[to]; t != nil {
				t.Cited = append(t.Cited, resCite{N: c.N, From: c.From, To: &link{URL: v.URL, Text: v.Title}})
			}
		}
		v.Cites = append(v.Cites, rc)
	}
	for id, ms := range b.resMentions {
		if v := b.res[id]; v != nil {
			v.Mentions = ms
		}
	}
	return nil
}

// flatten gives the nodes of a structure in the order of the tree, each with its depth.
func flatten(nodes []resource.Node, d int, out []resNode) []resNode {
	for _, n := range nodes {
		kids := n.Children
		n.Children = nil
		out = append(out, resNode{Node: n, Depth: d})
		out = flatten(kids, d+1, out)
	}
	return out
}

// creators gives the names of the creators in one line, with the role of each creator that is not an author.
func creators(cs []resource.Creator) string {
	var names []string
	for _, c := range cs {
		n := c.Name
		if c.Role != "" && c.Role != "author" {
			n += " (" + c.Role + ")"
		}
		names = append(names, n)
	}
	return strings.Join(names, ", ")
}

// year gives the year of an ISO 8601 date, or "".
func year(date string) string {
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}

// access gives the access of a manifestation in a few words, or "".
func access(a *resource.Access) string {
	if a == nil {
		return ""
	}
	s := a.Class
	if a.Licence != "" {
		s += ", " + a.Licence
	}
	if a.Text != "" {
		s += ", " + a.Text
	}
	return s
}

// resIDURL gives the URL for the external ids of a resource that the links of the nodes do not cover.
var resIDURL = map[string]func(id string) string{
	"zbl":                 func(id string) string { return "https://zbmath.org/?q=an:" + id },
	"isbn":                func(id string) string { return "https://openlibrary.org/isbn/" + id },
	"oclc":                func(id string) string { return "https://www.worldcat.org/oclc/" + id },
	"lccn":                func(id string) string { return "https://lccn.loc.gov/" + id },
	"openlibrary_work":    func(id string) string { return "https://openlibrary.org/works/" + id },
	"openlibrary_edition": func(id string) string { return "https://openlibrary.org/books/" + id },
	"openalex":            func(id string) string { return "https://openalex.org/" + id },
	"hal":                 func(id string) string { return "https://hal.science/" + id },
	"numdam":              func(id string) string { return "http://www.numdam.org/item/" + id },
	"eudml":               func(id string) string { return "https://eudml.org/doc/" + id },
	"hathi":               func(id string) string { return "https://hdl.handle.net/2027/" + id },
	"hathi_record":        func(id string) string { return "https://catalog.hathitrust.org/record/" + id },
	"ia":                  func(id string) string { return "https://archive.org/details/" + id },
	"gutenberg":           func(id string) string { return "https://www.gutenberg.org/ebooks/" + id },
	"handle":              func(id string) string { return "https://hdl.handle.net/" + id },
	"ocw":                 func(id string) string { return "https://ocw.mit.edu/courses/" + id + "/" },
	"oyc":                 func(id string) string { return "https://oyc.yale.edu/" + id },
	"nptel":               func(id string) string { return "https://nptel.ac.in/courses/" + id },
	"youtube_playlist":    func(id string) string { return "https://www.youtube.com/playlist?list=" + id },
	"youtube_video":       func(id string) string { return "https://www.youtube.com/watch?v=" + id },
	"swmath":              func(id string) string { return "https://zbmath.org/software/" + id },
	"zenodo":              func(id string) string { return "https://zenodo.org/records/" + id },
	"swh":                 func(id string) string { return "https://archive.softwareheritage.org/" + id },
	"ror":                 func(id string) string { return "https://ror.org/" + id },
	"orcid":               func(id string) string { return "https://orcid.org/" + id },
	"mgp":                 func(id string) string { return "https://www.mathgenealogy.org/id.php?id=" + id },
}

// idLinks gives a link for each external id, in the order of the systems.
func idLinks(ids resource.IDs) []link {
	var out []link
	for _, sys := range slices.Sorted(maps.Keys(ids)) {
		for _, id := range ids[sys] {
			l := idLink(sys, id)
			if f, ok := resIDURL[sys]; ok && l.URL == "" {
				l.URL = f(id)
			}
			out = append(out, l)
		}
	}
	return out
}

// resKinds groups the resources by kind, in the order of resource.Kinds, and gives each list its pages.
func (b *builder) resKinds() []*resKind {
	by := map[string]*resKind{}
	for _, v := range b.resOrder {
		k := by[v.Kind]
		if k == nil {
			k = &resKind{Kind: v.Kind, Dir: "res/kind/" + v.Kind, URL: b.url("res/kind/" + v.Kind + "/")}
			by[v.Kind] = k
		}
		k.Rows = append(k.Rows, v)
	}
	var out []*resKind
	for _, name := range slices.Sorted(maps.Keys(by)) {
		k := by[name]
		slices.SortFunc(k.Rows, func(a, c *resView) int {
			return cmp.Or(cmp.Compare(strings.ToLower(a.Title), strings.ToLower(c.Title)), cmp.Compare(a.ID, c.ID))
		})
		for i := 0; i*resPerPage < len(k.Rows); i++ {
			u := k.URL
			if i > 0 {
				u += strconv.Itoa(i+1) + "/"
			}
			k.Pages = append(k.Pages, resPageLink{N: i + 1, URL: u, From: i*resPerPage + 1, To: min((i+1)*resPerPage, len(k.Rows))})
		}
		out = append(out, k)
	}
	slices.SortStableFunc(out, func(a, c *resKind) int { return cmp.Compare(kindRank(a.Kind), kindRank(c.Kind)) })
	return out
}

// kindRank gives the place of a kind in resource.Kinds. An unknown kind comes last.
func kindRank(k string) int {
	if i := slices.Index(resource.Kinds, k); i >= 0 {
		return i
	}
	return len(resource.Kinds)
}

// writeResources writes the index of the catalogue, the lists of each kind, a page for each resource and the JSON summary.
func (b *builder) writeResources() error {
	kinds := b.resKinds()
	d := resIndex{Total: len(b.resOrder), Kinds: kinds}
	tiers, decades, acc, langs := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	for _, v := range b.resOrder {
		if !v.Live() {
			continue
		}
		d.Live++
		tiers[cmp.Or(v.Tier, "none")]++
		dec := "unknown"
		if y, err := strconv.Atoi(v.Year); err == nil {
			dec = strconv.Itoa(y/10*10) + "s"
		}
		decades[dec]++
		seenAcc, seenLang := map[string]bool{}, map[string]bool{}
		for _, e := range v.Expressions {
			if l := cmp.Or(e.Language, "unknown"); !seenLang[l] {
				seenLang[l] = true
				langs[l]++
			}
			for _, m := range e.Manifestations {
				if a := cmp.Or(m.Manifestation.Access, e.Access, &resource.Access{Class: "record"}); !seenAcc[a.Class] {
					seenAcc[a.Class] = true
					acc[a.Class]++
				}
			}
		}
	}
	d.Tiers, d.Access, d.Languages = counts(tiers), counts(acc), counts(langs)
	for _, k := range slices.Sorted(maps.Keys(decades)) {
		d.Decades = append(d.Decades, invCount{k, decades[k]})
	}
	for _, sd := range b.specs {
		if sd.File == "corpus/resources/00-README.md" {
			d.SpecURL = sd.URL
		}
	}
	desc := "The catalogue of resources of Math 2.0: the books, papers, courses and other works that the items of mathematics come from."
	if err := b.writePage("res", "resources", d, page{Title: "Resources", Description: desc, Section: "res", JSONURL: b.url("api/resources.json")}); err != nil {
		return err
	}
	for _, k := range kinds {
		for i, p := range k.Pages {
			dir := k.Dir
			if i > 0 {
				dir += "/" + strconv.Itoa(p.N)
			}
			pages := slices.Clone(k.Pages)
			pages[i].Current = true
			data := resListPage{Kind: k, Rows: k.Rows[p.From-1 : p.To], Pages: pages, Kinds: kinds}
			title := "Resources: " + k.Kind
			if i > 0 {
				title += ", page " + strconv.Itoa(p.N)
			}
			if err := b.writePage(dir, "reslist", data, page{Title: title, Description: "The resources of kind " + k.Kind + " in the catalogue of Math 2.0.", Section: "res"}); err != nil {
				return err
			}
		}
	}
	for _, v := range b.resOrder {
		desc := v.Kind
		if v.Creators != "" {
			desc += " by " + v.Creators
		}
		if v.Year != "" {
			desc += ", " + v.Year
		}
		if err := b.writePage("res/"+v.ID, "resource", v, page{Title: v.Title, Description: desc + ". A resource in the catalogue of Math 2.0.", Section: "res"}); err != nil {
			return err
		}
	}
	type kindJSON struct {
		Kind  string `json:"kind"`
		Count int    `json:"count"`
	}
	sum := struct {
		Total int        `json:"total"`
		Live  int        `json:"live"`
		Kinds []kindJSON `json:"kinds"`
	}{Total: d.Total, Live: d.Live}
	for _, k := range kinds {
		sum.Kinds = append(sum.Kinds, kindJSON{k.Kind, len(k.Rows)})
	}
	return b.writeJSON("api/resources.json", sum)
}

// HasResources reports whether the site has a catalogue of resources.
func (b *builder) HasResources() bool { return len(b.resOrder) > 0 }

// atText gives a structured locator in a few words, as "theorem 3.14, section 3, page 68", or "".
func atText(at *inventory.At) string {
	if at == nil {
		return ""
	}
	var parts []string
	if at.Kind != "" || at.Number != "" {
		parts = append(parts, strings.TrimSpace(at.Kind+" "+at.Number))
	}
	if at.Part != "" {
		parts = append(parts, "part "+at.Part)
	}
	if at.Section != "" {
		parts = append(parts, "section "+at.Section)
	}
	if at.Page != "" {
		parts = append(parts, "page "+at.Page)
	}
	return strings.Join(parts, ", ")
}
