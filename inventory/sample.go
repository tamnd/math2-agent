package inventory

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
)

// The strata of the monthly sample audit of arXiv (corpus arxiv AX11.5).
const (
	StratumRender   = "extraction render"
	StratumSource   = "extraction source"
	StratumNative   = "extraction native"
	StratumVision   = "extraction vision"
	StratumProse    = "prose items"
	StratumUnfold   = "unfolding"
	StratumEdges    = "model and outside edges"
	StratumSame     = "automatic same"
	StratumResearch = "research nodes"
	StratumNewArea  = "research nodes of a new area"
)

// Stratum is one stratum of a sample: its population and the units that the draw took.
type Stratum struct {
	Name       string   `json:"name"`
	Unit       string   `json:"unit"`       // paper, item, statement, edge, pair or node
	Size       int      `json:"size"`       // the number of units that the draw takes each month
	Population int      `json:"population"` // the number of units that the draw can take
	Note       string   `json:"note,omitempty"`
	Packets    []Packet `json:"packets"`
}

// Packet is one unit of a sample for an auditor. It has no field that tells who or what wrote the unit, because the audit is blind (corpus K9.7).
type Packet struct {
	Stratum string `json:"stratum"`
	ID      string `json:"id"`                // the paper, the mention, the edge as FROM>TO, the pair as A=B, or the tag of the node
	Paper   string `json:"paper,omitempty"`   // the arXiv id of the paper
	Kind    string `json:"kind,omitempty"`    // the kind of the mention
	Locator string `json:"locator,omitempty"` // where the unit is in the paper
	TextRef string `json:"text_ref,omitempty"`
	Area    string `json:"area,omitempty"` // the MSC code at two digits, for a node of a new area
	Check   string `json:"check"`          // what the auditor checks
}

// SampleSizes is the sample of each stratum each month (AX11.5).
var SampleSizes = map[string]int{
	StratumRender: 50, StratumSource: 50, StratumNative: 50, StratumVision: 50,
	StratumProse: 200, StratumUnfold: 200, StratumEdges: 200, StratumSame: 200,
	StratumResearch: 100, StratumNewArea: 200,
}

var checks = map[string]string{
	StratumRender:   "each item of the paper is a mention, with the right kind, number and text, and no mention is not an item",
	StratumSource:   "each item of the paper is a mention, with the right kind, number and text, and no mention is not an item",
	StratumNative:   "each item of the paper is a mention, with the right kind, number and text, and no mention is not an item",
	StratumVision:   "each item of the paper is a mention, with the right kind, number and text, and no mention is not an item",
	StratumProse:    "the text is an item of the paper, and the kind and the extent are right",
	StratumUnfold:   "the unfolded statement says the same as the statement in the paper, with no hypothesis lost",
	StratumEdges:    "the item states or uses the result at the other end of the edge",
	StratumSame:     "the two mentions are the same item",
	StratumResearch: "the node states the item of the paper faithfully, and its sources and its kind are right",
	StratumNewArea:  "the node states the item of the paper faithfully, and its sources and its kind are right",
}

// DrawArxiv draws the sample of the month for arXiv from the rows and the mentions (AX11.5). Each unit gets a score, the SHA-256 hash of the seed, the month, the stratum and the id, and the draw takes the units with the lowest scores. Thus the draw depends only on the seed, the month and the inventory, and anyone can draw the same sample again. A stratum that has no units yet is in the result with the population 0.
func DrawArxiv(rows []Row, ms []Mention, month, seed string) ([]Stratum, error) {
	if seed == "" || strings.ContainsAny(seed, " \t\n#") {
		return nil, fmt.Errorf("inventory: the seed %q is empty or has a space or a #", seed)
	}
	pop := map[string][]Packet{}
	add := func(p Packet) {
		p.Check = checks[p.Stratum]
		pop[p.Stratum] = append(pop[p.Stratum], p)
	}
	papers := map[string]Packet{}
	byID := map[string]*Mention{}
	for i := range ms {
		m := &ms[i]
		if m.Source != "arxiv" {
			continue
		}
		byID[m.ID] = m
		paper, _, _ := strings.Cut(m.NativeID, "#")
		// The importer takes only the items of the source path (AX5.1), so each paper with a mention has the source path.
		if _, ok := papers[paper]; !ok && paper != "" {
			papers[paper] = Packet{Stratum: StratumSource, ID: paper, Paper: paper, Locator: fmt.Sprintf("%sv%d", paper, m.Version)}
		}
		if m.Head == "text" {
			add(Packet{Stratum: StratumProse, ID: m.ID, Paper: paper, Kind: m.Kind, Locator: m.Locator, TextRef: m.TextRef})
		}
		for _, c := range m.CitesResult {
			if strings.HasPrefix(c.To, "bib:") {
				continue
			}
			add(Packet{Stratum: StratumEdges, ID: m.ID + ">" + c.To, Paper: paper, Kind: m.Kind, Locator: strings.TrimSpace(m.Locator + " cites " + c.Locator), TextRef: m.TextRef})
		}
	}
	for _, p := range papers {
		add(p)
	}
	type dated struct {
		tag, day, area string
	}
	var nodes []dated
	for i := range rows {
		r := &rows[i]
		if r.Layer == Research && len(r.Mentions) > 1 && joinedByRun(r) {
			for j := 1; j < len(r.Mentions); j++ {
				a, b := r.Mentions[0], r.Mentions[j]
				if byID[a] == nil || byID[b] == nil {
					continue
				}
				add(Packet{Stratum: StratumSame, ID: a + "=" + b, Paper: strings.Join([]string{paperOf(byID[a]), paperOf(byID[b])}, " "), Kind: r.Kind, Locator: byID[a].Locator + " = " + byID[b].Locator})
			}
		}
		if r.Layer != Research || r.Outcome == nil || r.Outcome.Value != OutcomeNode {
			continue
		}
		day := ""
		for _, e := range r.History {
			if e.Op == "outcome" {
				day = e.At
			}
		}
		area := ""
		if len(r.MSC) > 0 && len(r.MSC[0]) >= 2 {
			area = r.MSC[0][:2]
		}
		for _, t := range r.Outcome.Target {
			nodes = append(nodes, dated{tag: t, day: day, area: area})
			add(Packet{Stratum: StratumResearch, ID: t, Kind: r.Kind, Locator: strings.Join(r.Mentions, " ")})
		}
	}
	// A new area is an MSC code at two digits whose first research node is in the month. The stratum takes the first nodes of the area, in the order of the days.
	slices.SortFunc(nodes, func(a, b dated) int { return cmp.Or(strings.Compare(a.day, b.day), strings.Compare(a.tag, b.tag)) })
	first := map[string]string{}
	for _, n := range nodes {
		if _, ok := first[n.area]; !ok && n.area != "" {
			first[n.area] = n.day
		}
	}
	taken := map[string]int{}
	var fresh []Packet
	for _, n := range nodes {
		if n.area == "" || !strings.HasPrefix(first[n.area], month) || taken[n.area] >= SampleSizes[StratumNewArea] {
			continue
		}
		taken[n.area]++
		fresh = append(fresh, Packet{Stratum: StratumNewArea, ID: n.tag, Area: n.area, Check: checks[StratumNewArea]})
	}

	out := []Stratum{
		{Name: StratumRender, Unit: "paper", Note: "the importer does not take the items of the render path yet"},
		{Name: StratumSource, Unit: "paper"},
		{Name: StratumNative, Unit: "paper", Note: "the importer does not take the items of the native path yet"},
		{Name: StratumVision, Unit: "paper", Note: "the importer does not take the items of the vision path yet"},
		{Name: StratumProse, Unit: "item"},
		{Name: StratumUnfold, Unit: "statement", Note: "no mention has an unfolded statement yet (AX7.8)"},
		{Name: StratumEdges, Unit: "edge"},
		{Name: StratumSame, Unit: "pair"},
		{Name: StratumResearch, Unit: "node"},
		{Name: StratumNewArea, Unit: "node"},
	}
	for i := range out {
		s := &out[i]
		s.Size = SampleSizes[s.Name]
		ps := pop[s.Name]
		if s.Name == StratumNewArea {
			// Each node of a new area is in the sample, up to the size for each area.
			s.Population, s.Packets = len(fresh), append([]Packet{}, fresh...)
			continue
		}
		s.Population = len(ps)
		type scored struct {
			p     Packet
			score [32]byte
		}
		sc := make([]scored, len(ps))
		for j, p := range ps {
			sc[j] = scored{p, sampleScore(seed, month, s.Name, p.ID)}
		}
		slices.SortFunc(sc, func(a, b scored) int {
			return cmp.Or(bytes.Compare(a.score[:], b.score[:]), strings.Compare(a.p.ID, b.p.ID))
		})
		s.Packets = []Packet{}
		for j := 0; j < len(sc) && j < s.Size; j++ {
			s.Packets = append(s.Packets, sc[j].p)
		}
	}
	return out, nil
}

// joinedByRun reports whether a run, and not a person, put the mentions of the row together.
func joinedByRun(r *Row) bool {
	for _, e := range r.History {
		if (e.Op == "add" || e.Op == "merge") && strings.HasPrefix(e.By, "m2/") {
			return true
		}
	}
	return false
}

func paperOf(m *Mention) string {
	p, _, _ := strings.Cut(m.NativeID, "#")
	return p
}

// sampleScore returns the SHA-256 hash of the parts, with a zero byte after each part.
func sampleScore(parts ...string) [32]byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	var s [32]byte
	h.Sum(s[:0])
	return s
}
