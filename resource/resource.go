// Package resource holds the catalogue of works of the corpus (corpus resources RS02): the books, courses, papers and other works that the items come from.
//
// A resource is one work. It has a list of expressions (editions, translations, versions, offerings), and each expression has a list of manifestations (a print, a PDF, a site). The catalogue never holds the item level of the IFLA Library Reference Model, so the word item keeps its meaning of corpus K2.3.
//
// The files of the catalogue are in the record plane of the math2 tree:
//
//	records/resources/<shard>.md                    the resource records
//	records/resources/cites/<shard>.md              the cites edges, by the shard of the citing resource
//	records/resources/structure/<id>/<expr>.json    the structure of one expression
//	registers/resources/<shard>.reg                 the register of the resource ids
//
// A resource id has the format of a tag (corpus spec K3.3), and it is in the one space of values of tags and inventory rows (K3.6). The shard of a resource is the first three characters of its id. Each file is sorted by id. A file of records is Markdown with a YAML header that holds the records (package mdrec), and a register has one entry on each line, so two writes of the same catalogue give the same bytes.
package resource

import (
	"bytes"
	"encoding/json"
	"slices"
)

// The directories and files of the catalogue, relative to the root of the tree.
const (
	Dir          = "records/resources"
	CitesDir     = "records/resources/cites"
	StructureDir = "records/resources/structure"
	RegisterDir  = "registers/resources"
)

// Kinds are the kinds of a work (RS2.3).
var Kinds = []string{
	"monograph", "textbook", "edited-volume", "proceedings", "problem-book", "handbook", "encyclopedia", "table",
	"chapter", "article", "preprint", "conference-paper", "survey", "thesis", "report", "lecture-notes",
	"course", "course-material", "competition", "problem-set", "journal", "series", "video-series",
	"software", "dataset", "website", "manuscript",
}

// Genres are the facets that cut across kinds (RS2.3).
var Genres = []string{"survey", "expository", "research", "historical", "problems", "solutions", "olympiad", "popular", "teaching"}

// The kinds of an expression and of a manifestation (RS2.4).
var (
	ExpressionKinds    = []string{"edition", "translation", "version", "offering"}
	ManifestationKinds = []string{"print", "pdf", "html", "latex", "pretext", "video", "page"}
)

// ThesisLevels are the values of the field level of a thesis (RS2.3).
var ThesisLevels = []string{"bachelor", "master", "doctoral", "habilitation"}

// Roles are the roles of a creator (RS2.5).
var Roles = []string{"author", "editor", "translator", "instructor", "contributor", "advisor"}

// Relations are the kinds of an edge between resources that the record holds (RS2.7). The edges cites are in their own files, and same_as is in the field ids.
var Relations = []string{"part_of", "translation_of", "version_of", "reprint_of", "based_on", "uses_text", "reviews", "solves", "corrects"}

// Classes are the access classes of a manifestation (spec 9.3 and RS2.6). The default is record.
var Classes = []string{"open", "share-alike-nc", "verbatim", "gfdl", "record"}

// Texts are the values of the field text of an access (RS2.6).
var Texts = []string{"open-text", "free-to-read", "closed", "lost"}

// The outcomes of a scope decision (RS9.6).
const (
	In        = "in"
	Border    = "border"
	Out       = "out"
	Duplicate = "duplicate"
	Deferred  = "deferred"
)

// Outcomes are the values of the field value of a scope.
var Outcomes = []string{In, Border, Out, Duplicate, Deferred}

// Levels are the levels of the record of a catalogue (RS2.2).
var Levels = []string{"work", "expression", "manifestation"}

// Resource is one work of the catalogue (RS2.4). The build computes the tier, the depth and the signals of a resource and writes them to build/resources/, not to the record.
type Resource struct {
	ID          string            `json:"id"`                    // the resource id (RS3.1)
	Kind        string            `json:"kind"`                  // one of Kinds
	Genre       []string          `json:"genre,omitempty"`       // facets from Genres
	Title       string            `json:"title"`                 // the title in the original language and script
	TitleEn     string            `json:"title_en,omitempty"`    // an English title from a catalogue, never from a model alone
	Creators    []Creator         `json:"creators,omitempty"`    // the authors, editors, instructors and translators
	Date        string            `json:"date,omitempty"`        // the date of the first expression, ISO 8601, as precise as known
	MSC         []Code            `json:"msc,omitempty"`         // the MSC2020 codes, each with its origin
	Subjects    []Subject         `json:"subjects,omitempty"`    // other classifications: LCC, DDC, UDC, arXiv categories
	Level       string            `json:"level,omitempty"`       // of a thesis: one of ThesisLevels
	IDs         IDs               `json:"ids,omitempty"`         // the external ids of the work, in the normal form of RS3.3
	Expressions []Expression      `json:"expressions,omitempty"` // the expressions of the work
	Relations   []Relation        `json:"relations,omitempty"`   // the edges to other resources (RS2.7)
	Scope       *Scope            `json:"scope,omitempty"`       // the scope outcome (RS9.6). Nil means no decision yet
	Catalogue   []CatalogueRecord `json:"catalogue"`             // the records of catalogues that the resource groups
	History     []Event           `json:"history"`               // each merge, split and scope change
}

// Creator is one person or body that made a work (RS2.5). The catalogue is not an author database: it never guesses an id from a name alone.
type Creator struct {
	Name string `json:"name"`          // the name as the catalogue gives it
	Role string `json:"role"`          // one of Roles
	IDs  IDs    `json:"ids,omitempty"` // orcid, zbmath_author, wikidata, mgp, idref, openalex_author, isni, viaf, lcnaf
}

// Code is a code of a classification with the origin of the code: a catalogue, or classifier/<version> (RS9.3).
type Code struct {
	Code string `json:"code"`
	From string `json:"from"`
}

// Subject is a code of a classification other than MSC.
type Subject struct {
	System string `json:"system"` // lcc, ddc, udc, clc, arxiv, acm
	Code   string `json:"code"`
}

// Expression is one realization of a work: an edition, a translation, a version or an offering (RS2.2).
type Expression struct {
	Key            string          `json:"key"`                    // e1, e2, and so on. A key is never used again
	Kind           string          `json:"kind"`                   // one of ExpressionKinds
	Label          string          `json:"label,omitempty"`        // for example 3rd edition, or Spring 2010
	Language       string          `json:"language,omitempty"`     // BCP 47
	Date           string          `json:"date,omitempty"`         // ISO 8601
	Contributors   []Creator       `json:"contributors,omitempty"` // the translators, the instructors and the editors of this expression
	IDs            IDs             `json:"ids,omitempty"`
	Structure      string          `json:"structure,omitempty"` // the path of the structure file, relative to records/resources/structure/
	Access         *Access         `json:"access,omitempty"`    // the access of the expression, when all its manifestations share it
	Manifestations []Manifestation `json:"manifestations,omitempty"`
}

// Manifestation is one embodiment of an expression: a print with its ISBN, a PDF, a site (RS2.2).
type Manifestation struct {
	Key       string   `json:"key"`  // m1, m2, and so on. A key is never used again
	Kind      string   `json:"kind"` // one of ManifestationKinds
	Publisher string   `json:"publisher,omitempty"`
	Place     string   `json:"place,omitempty"`
	Date      string   `json:"date,omitempty"`
	IDs       IDs      `json:"ids,omitempty"`
	Links     []string `json:"links,omitempty"` // the home of the manifestation, never a copy that the rights holder did not authorize (RS14.5)
	Access    *Access  `json:"access,omitempty"`
}

// Access is the licence gate of spec 9.3 at the level of one manifestation (RS2.6).
type Access struct {
	Class        string        `json:"class"`                   // one of Classes
	Licence      string        `json:"licence,omitempty"`       // an SPDX expression, as the manifestation states it
	LicenceRead  *LicenceRead  `json:"licence_read,omitempty"`  // where, when and by whom the licence was read (K5.2.1)
	Text         string        `json:"text,omitempty"`          // one of Texts
	PublicDomain *PublicDomain `json:"public_domain,omitempty"` // the public-domain answers (K5.2.3)
}

// LicenceRead records one reading of a licence at the source itself (K5.2.1).
type LicenceRead struct {
	At string `json:"at"`
	On string `json:"on"`
	By string `json:"by"`
}

// PublicDomain records why a work is in the public domain in the United States and in the EU (K5.2.3).
type PublicDomain struct {
	US    bool   `json:"us"`
	EU    bool   `json:"eu"`
	Basis string `json:"basis"`
}

// Relation is one edge from the resource, or from one of its expressions or manifestations, to another resource (RS2.7).
type Relation struct {
	Kind   string `json:"kind"`             // one of Relations
	From   string `json:"from,omitempty"`   // the local key of the expression or manifestation, as e3 or e3/m1. Empty means the work
	To     string `json:"to"`               // the target, as <id>, <id>/e<n> or <id>/e<n>/m<n>
	Volume string `json:"volume,omitempty"` // of part_of: the volume or the number in the series or the journal
	Source string `json:"source,omitempty"` // the catalogue, the resolver or the person that gave the edge
}

// Scope is the scope decision of a resource (RS9.6).
type Scope struct {
	Value  string `json:"value"`            // one of Outcomes
	Target string `json:"target,omitempty"` // of duplicate: the live resource
	Until  string `json:"until,omitempty"`  // of deferred: the date of the next decision, YYYY-MM-DD
	Reason string `json:"reason,omitempty"` // of deferred and of a decision of a person: why
	Rule   string `json:"rule,omitempty"`   // the test of RS9.2 that gave the decision
	By     string `json:"by"`               // the build and its version, the run, or the person
}

// CatalogueRecord is one record of a catalogue that the resource groups (RS2.4). Rule H10 checks that its source and its snapshot exist.
type CatalogueRecord struct {
	Source   string `json:"source"`    // the source, from manifests/sources.yaml
	Snapshot string `json:"snapshot"`  // the hash of the snapshot that the adapter read
	NativeID string `json:"native_id"` // the id of the record in the source
	Level    string `json:"level"`     // one of Levels
}

// Event is one change in the history of a resource.
type Event struct {
	Op     string `json:"op"`               // add, merge, split, scope or tier
	At     string `json:"at"`               // the date, YYYY-MM-DD
	By     string `json:"by"`               // the build and its version, the run, or the person
	Commit string `json:"commit,omitempty"` // the commit of the change
	Note   string `json:"note,omitempty"`
}

// IDs maps the key of an id system of RS3.2 to its values. A value is a string in JSON when there is one value, and a list when there are more, as a print book with two ISBNs.
type IDs map[string]Values

// Values is the list of values of one id system.
type Values []string

// MarshalJSON writes one value as a string and more values as a list.
func (v Values) MarshalJSON() ([]byte, error) {
	if len(v) == 1 {
		return json.Marshal(v[0])
	}
	return json.Marshal([]string(v))
}

// UnmarshalJSON reads a string or a list of strings.
func (v *Values) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*v = Values{s}
		return nil
	}
	var l []string
	if err := json.Unmarshal(data, &l); err != nil {
		return err
	}
	*v = l
	return nil
}

// Get returns the first value of the system, or the empty string.
func (ids IDs) Get(system string) string {
	if v := ids[system]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// Live reports whether the resource is not merged into another resource.
func (r *Resource) Live() bool { return r.Scope == nil || r.Scope.Value != Duplicate }

// Expression returns the expression with the key, or nil.
func (r *Resource) Expression(key string) *Expression {
	for i := range r.Expressions {
		if r.Expressions[i].Key == key {
			return &r.Expressions[i]
		}
	}
	return nil
}

// URI returns the URI of the resource, m2://res/<id>.
func URI(id string) string { return "m2://res/" + id }

// Shard returns the shard of a resource id: its first three characters.
func Shard(id string) string { return id[:3] }

// Cite is one edge from an expression to the resource that one of its references names (RS2.7 and RS3.7).
type Cite struct {
	From    string  `json:"from"`              // the citing expression, as <id>/e<n>
	N       int     `json:"n"`                 // the number of the reference in the bibliography, from 1
	To      string  `json:"to,omitempty"`      // the cited resource, as <id> or <id>/e<n>. Empty means not resolved
	Text    string  `json:"text,omitempty"`    // the reference as the bibliography prints it, only when the class of the citing manifestation lets math2 store it
	Parsed  *Parsed `json:"parsed,omitempty"`  // the fields of the reference
	Score   float64 `json:"score,omitempty"`   // the score of the match
	Matcher string  `json:"matcher,omitempty"` // the matcher and its version
}

// Parsed holds the fields of a reference that a parser gives. The fields are facts.
type Parsed struct {
	Authors []string `json:"authors,omitempty"`
	Title   string   `json:"title,omitempty"`
	Venue   string   `json:"venue,omitempty"`
	Volume  string   `json:"volume,omitempty"`
	Pages   string   `json:"pages,omitempty"`
	Year    string   `json:"year,omitempty"`
	Edition string   `json:"edition,omitempty"`
	IDs     IDs      `json:"ids,omitempty"`
}

// Structure is the tree of locators of one expression (RS2.9). Titles of chapters and sections are facts, so a structure can exist for a work of any access class.
type Structure struct {
	Resource string `json:"resource"` // the expression, as <id>/e<n>
	Source   string `json:"source"`   // the source of the tree
	Snapshot string `json:"snapshot"` // the hash of the snapshot that the importer read
	Nodes    []Node `json:"nodes"`
}

// Node is one node of a structure: a part, a chapter, a section, a numbered environment, or a session of a course.
type Node struct {
	Key       string    `json:"key"`
	Kind      string    `json:"kind"` // part, chapter, section, subsection, theorem, definition, session, and so on
	Number    string    `json:"number,omitempty"`
	Title     string    `json:"title,omitempty"`
	Page      string    `json:"page,omitempty"`
	Date      string    `json:"date,omitempty"`      // of a session
	Materials []string  `json:"materials,omitempty"` // of a session: the resources of its materials
	Readings  []Reading `json:"readings,omitempty"`  // of a session: the sections of the books that it uses (RS5.4)
	Links     []string  `json:"links,omitempty"`     // of a problem: the pages of the source that show it (RS7.2). The statement is not held
	Children  []Node    `json:"children,omitempty"`
}

// Reading is one section of a book that a session of a course uses. A reading whose book is not in the tree has no resource. It keeps the citation of the course as text for the reference resolver.
type Reading struct {
	Resource string            `json:"resource,omitempty"` // the expression, as <id>/e<n>
	At       map[string]string `json:"at,omitempty"`
	Text     string            `json:"text,omitempty"` // the citation as the course gives it
}

// Find returns the node of the structure with the key, or nil.
func (s *Structure) Find(key string) *Node {
	var find func([]Node) *Node
	find = func(ns []Node) *Node {
		for i := range ns {
			if ns[i].Key == key {
				return &ns[i]
			}
			if n := find(ns[i].Children); n != nil {
				return n
			}
		}
		return nil
	}
	return find(s.Nodes)
}

// encode writes one value as JSON with no HTML escape and no trailing newline.
func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// valid reports whether v is one of the values.
func valid(values []string, v string) bool { return slices.Contains(values, v) }
