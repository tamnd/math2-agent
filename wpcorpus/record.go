package wpcorpus

// The strengths of a cue (WP3.4).
const (
	StrengthStrong = "strong"
	StrengthNormal = "normal"
	StrengthWeak   = "weak"
)

// Object is one record of the objects file of a page revision (WP4.2).
type Object struct {
	Page          int               `json:"page"`
	Revision      int               `json:"revision"`
	Ref           string            `json:"ref"`
	Tag           string            `json:"tag,omitempty"`
	Kind          string            `json:"kind"`
	Cue           string            `json:"cue"`
	Strength      string            `json:"strength"`
	Name          string            `json:"name,omitempty"`
	Section       string            `json:"section,omitempty"`
	SectionAnchor string            `json:"section_anchor,omitempty"`
	Anchor        string            `json:"anchor,omitempty"`
	Span          [2]int            `json:"span"`
	Text          string            `json:"text,omitempty"`
	Math          []Math            `json:"math,omitempty"`
	Links         []Link            `json:"links,omitempty"`
	Citations     []string          `json:"citations,omitempty"`
	AttachedTo    string            `json:"attached_to,omitempty"`
	Fields        map[string]string `json:"fields,omitempty"`
	Hash          string            `json:"hash"`
	Flags         []string          `json:"flags,omitempty"`
	Extractor     string            `json:"extractor"`
}

// Math is one formula of an object.
type Math struct {
	Form    string   `json:"form"`
	TeX     string   `json:"tex"`
	Source  string   `json:"source"`
	Display bool     `json:"display,omitempty"`
	Span    [2]int   `json:"span"`
	Errors  []string `json:"errors,omitempty"`
}

// Link is one wiki link of an object.
type Link struct {
	Title  string `json:"title"`
	Anchor string `json:"anchor,omitempty"`
}

// Census is the census of a page revision (WP4.3).
type Census struct {
	Page             int            `json:"page"`
	Revision         int            `json:"revision"`
	Title            string         `json:"title"`
	Type             string         `json:"type"`
	Extractor        string         `json:"extractor"`
	Manifests        map[string]int `json:"manifests"`
	Strength         string         `json:"strength"`
	Counts           Counts         `json:"counts"`
	Cues             []Cue          `json:"cues"`
	Unmapped         map[string]int `json:"unmapped_templates,omitempty"`
	UnmappedHeadings []string       `json:"unmapped_headings,omitempty"`
}

// Counts holds the counts of a census.
type Counts struct {
	ItemTemplates   map[string]int `json:"item_templates,omitempty"`
	ItemHeadings    map[string]int `json:"item_headings,omitempty"`
	NamedHeadings   int            `json:"named_headings"`
	InlineHeads     int            `json:"inline_heads"`
	NumberedBlocks  int            `json:"numbered_blocks"`
	ProofBlocks     int            `json:"proof_blocks"`
	GlossaryEntries int            `json:"glossary_entries"`
	ListEntries     int            `json:"list_entries"`
	TableRows       int            `json:"table_rows"`
	Anchors         int            `json:"anchors"`
	Math            map[string]int `json:"math,omitempty"`
	Citations       int            `json:"citations"`
	Maintenance     map[string]int `json:"maintenance,omitempty"`
}

// Cue is one counted cue of a census. Object is the ref of the object that
// the cue gave, and Waiver is the id of the waiver when the cue gave no
// object.
type Cue struct {
	Cue      string `json:"cue"`
	Kind     string `json:"kind,omitempty"`
	Strength string `json:"strength"`
	Span     [2]int `json:"span"`
	Object   string `json:"object,omitempty"`
	Waiver   string `json:"waiver,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Gaps returns the cues that have no object and no waiver. Rule Y03 reports
// each of them.
func (c *Census) Gaps() []Cue {
	var out []Cue
	for _, q := range c.Cues {
		if q.Object == "" && q.Waiver == "" {
			out = append(out, q)
		}
	}
	return out
}
