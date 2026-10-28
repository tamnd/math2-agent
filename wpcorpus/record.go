package wpcorpus

// The strengths of a cue (WP3.4).
const (
	StrengthStrong = "strong"
	StrengthNormal = "normal"
	StrengthWeak   = "weak"
)

// Object is one record of the objects file of a page revision (WP4.2). In the
// objects file, the header holds the fields and the body holds the text, so
// the header has no page, revision or text.
type Object struct {
	Page          int               `json:"page" yaml:"-"`
	Revision      int               `json:"revision" yaml:"-"`
	Ref           string            `json:"ref" yaml:"ref"`
	Tag           string            `json:"tag,omitempty" yaml:"tag,omitempty"`
	Kind          string            `json:"kind" yaml:"kind"`
	Cue           string            `json:"cue" yaml:"cue"`
	Strength      string            `json:"strength" yaml:"strength"`
	Name          string            `json:"name,omitempty" yaml:"name,omitempty"`
	Section       string            `json:"section,omitempty" yaml:"section,omitempty"`
	SectionAnchor string            `json:"section_anchor,omitempty" yaml:"section_anchor,omitempty"`
	Anchor        string            `json:"anchor,omitempty" yaml:"anchor,omitempty"`
	Span          [2]int            `json:"span" yaml:"span,flow"`
	Text          string            `json:"text,omitempty" yaml:"-"`
	Math          []Math            `json:"math,omitempty" yaml:"math,omitempty"`
	Links         []Link            `json:"links,omitempty" yaml:"links,omitempty"`
	Citations     []string          `json:"citations,omitempty" yaml:"citations,omitempty"`
	AttachedTo    string            `json:"attached_to,omitempty" yaml:"attached_to,omitempty"`
	Fields        map[string]string `json:"fields,omitempty" yaml:"fields,omitempty"`
	Hash          string            `json:"hash" yaml:"hash"`
	Flags         []string          `json:"flags,omitempty" yaml:"flags,omitempty"`
	Extractor     string            `json:"extractor" yaml:"extractor"`
}

// Math is one formula of an object.
type Math struct {
	Form    string   `json:"form" yaml:"form"`
	TeX     string   `json:"tex" yaml:"tex"`
	Source  string   `json:"source" yaml:"source"`
	Display bool     `json:"display,omitempty" yaml:"display,omitempty"`
	Span    [2]int   `json:"span" yaml:"span,flow"`
	Errors  []string `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// Link is one wiki link of an object.
type Link struct {
	Title  string `json:"title" yaml:"title"`
	Anchor string `json:"anchor,omitempty" yaml:"anchor,omitempty"`
}

// Census is the census of a page revision (WP4.3).
type Census struct {
	Page             int            `json:"page" yaml:"page"`
	Revision         int            `json:"revision" yaml:"revision"`
	Title            string         `json:"title" yaml:"title"`
	Type             string         `json:"type" yaml:"type"`
	Extractor        string         `json:"extractor" yaml:"extractor"`
	Manifests        map[string]int `json:"manifests" yaml:"manifests"`
	Strength         string         `json:"strength" yaml:"strength"`
	Counts           Counts         `json:"counts" yaml:"counts"`
	Cues             []Cue          `json:"cues" yaml:"cues"`
	Unmapped         map[string]int `json:"unmapped_templates,omitempty" yaml:"unmapped_templates,omitempty"`
	UnmappedHeadings []string       `json:"unmapped_headings,omitempty" yaml:"unmapped_headings,omitempty"`
	// HTML is the witness W-html, when the corpus has the Parsoid HTML of
	// the revision.
	HTML *Witness `json:"html,omitempty" yaml:"html,omitempty"`
}

// Counts holds the counts of a census.
type Counts struct {
	ItemTemplates   map[string]int `json:"item_templates,omitempty" yaml:"item_templates,omitempty"`
	ItemHeadings    map[string]int `json:"item_headings,omitempty" yaml:"item_headings,omitempty"`
	NamedHeadings   int            `json:"named_headings" yaml:"named_headings"`
	InlineHeads     int            `json:"inline_heads" yaml:"inline_heads"`
	NumberedBlocks  int            `json:"numbered_blocks" yaml:"numbered_blocks"`
	ProofBlocks     int            `json:"proof_blocks" yaml:"proof_blocks"`
	GlossaryEntries int            `json:"glossary_entries" yaml:"glossary_entries"`
	ListEntries     int            `json:"list_entries" yaml:"list_entries"`
	TableRows       int            `json:"table_rows" yaml:"table_rows"`
	Sections        int            `json:"sections,omitempty" yaml:"sections,omitempty"`
	Anchors         int            `json:"anchors" yaml:"anchors"`
	Math            map[string]int `json:"math,omitempty" yaml:"math,omitempty"`
	Citations       int            `json:"citations" yaml:"citations"`
	Maintenance     map[string]int `json:"maintenance,omitempty" yaml:"maintenance,omitempty"`
}

// Cue is one counted cue of a census. Object is the ref of the object that
// the cue gave, and Waiver is the id of the waiver when the cue gave no
// object.
type Cue struct {
	Cue      string `json:"cue" yaml:"cue"`
	Kind     string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Strength string `json:"strength" yaml:"strength"`
	Span     [2]int `json:"span" yaml:"span,flow"`
	Object   string `json:"object,omitempty" yaml:"object,omitempty"`
	Waiver   string `json:"waiver,omitempty" yaml:"waiver,omitempty"`
	Reason   string `json:"reason,omitempty" yaml:"reason,omitempty"`
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
