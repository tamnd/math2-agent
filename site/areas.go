package site

import (
	"regexp"
	"slices"
	"strings"
)

// The areas of the site come from the topic nodes. An area is a topic with no parent topic and at least one child topic. Its sub-areas are its child topics, in the order of the links in the body of the area, and then by name.

// areaView is one area on the Areas page.
type areaView struct {
	Topic *nodeView
	Subs  []*subAreaView
	Count int // the nodes of the area and of its sub-areas, the topics not counted
}

type subAreaView struct {
	Topic *nodeView
	Count int // the nodes in the sub-area, the topics not counted
}

type areasData struct {
	Areas  []*areaView
	Others []*nodeView // the topics that are neither an area nor a sub-area
}

// bodyTag matches the target of a wiki link in a body, [[text|TAG]].
var bodyTag = regexp.MustCompile(`\[\[[^\]|]*\|([0-9A-Z]{6}|[0-9A-Z]{10})\]\]`)

// parentTopics returns the topics that the node is a part of, the topics that exist only.
func (b *builder) parentTopics(v *nodeView) []*nodeView {
	var out []*nodeView
	for _, r := range refsOf(v.Meta["topics"]) {
		if t, ok := b.nodes[r.Text]; ok && t.Kind == "topic" && t != v {
			out = append(out, t)
		}
	}
	return out
}

// areas returns the areas of the site, the areas with the most specific MSC code first, and the other topics.
func (b *builder) topicAreas() areasData {
	children := map[*nodeView][]*nodeView{}
	members := map[*nodeView]int{}
	var roots []*nodeView
	for _, v := range b.order {
		if v.Retired != nil {
			continue
		}
		ps := b.parentTopics(v)
		if v.Kind == "topic" {
			if len(ps) == 0 {
				roots = append(roots, v)
			}
			for _, p := range ps {
				children[p] = append(children[p], v)
			}
			continue
		}
		for _, p := range ps {
			members[p]++
		}
	}
	var d areasData
	for _, r := range roots {
		subs := children[r]
		if len(subs) == 0 {
			d.Others = append(d.Others, r)
			continue
		}
		pos := map[string]int{}
		for i, m := range bodyTag.FindAllStringSubmatch(r.Body, -1) {
			if _, ok := pos[m[1]]; !ok {
				pos[m[1]] = i
			}
		}
		slices.SortStableFunc(subs, func(x, y *nodeView) int {
			px, okx := pos[x.Tag]
			py, oky := pos[y.Tag]
			switch {
			case okx && oky:
				return px - py
			case okx:
				return -1
			case oky:
				return 1
			}
			return strings.Compare(x.Name, y.Name)
		})
		a := &areaView{Topic: r, Count: members[r]}
		for _, s := range subs {
			a.Subs = append(a.Subs, &subAreaView{Topic: s, Count: members[s]})
			a.Count += members[s]
		}
		d.Areas = append(d.Areas, a)
	}
	slices.SortStableFunc(d.Areas, func(x, y *areaView) int { return compareMSC(x.Topic, y.Topic) })
	slices.SortStableFunc(d.Others, compareMSC)
	return d
}

// compareMSC orders two topics by their first MSC code, and then by name.
func compareMSC(x, y *nodeView) int {
	first := func(v *nodeView) string {
		if len(v.MSC) == 0 {
			return "~"
		}
		return v.MSC[0]
	}
	if c := strings.Compare(first(x), first(y)); c != 0 {
		return c
	}
	return strings.Compare(x.Name, y.Name)
}

// TopicPath returns the topics above a node, the area first: the first topic of the node and the first topic of that topic, and so on.
func (b *builder) TopicPath(v *nodeView) []*nodeView {
	var path []*nodeView
	seen := map[*nodeView]bool{v: true}
	for cur := v; ; {
		ps := b.parentTopics(cur)
		if len(ps) == 0 || seen[ps[0]] {
			break
		}
		cur = ps[0]
		seen[cur] = true
		path = append(path, cur)
	}
	slices.Reverse(path)
	return path
}

func (b *builder) writeAreas() error {
	return b.writePage("areas", "areas", b.topicAreas(), page{Title: "Areas", Description: "The areas of mathematics in Math 2.0, each with its sub-areas.", Section: "areas"})
}

// Code returns the top level MSC class of the area, for its colour, or "".
func (a *areaView) Code() string {
	if len(a.Topic.MSC) == 0 || len(a.Topic.MSC[0]) < 2 {
		return ""
	}
	return a.Topic.MSC[0][:2]
}
