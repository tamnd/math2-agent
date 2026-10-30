// Package inventory holds the inventory of the corpus (corpus spec K04): the mentions of the items that the sources present, and the rows that group the mentions of one item.
//
// A mention is one item as one source presents it. An importer gives the mentions of its source and nothing else. A row is one distinct item. It has an identifier of the format of a tag (corpus spec K3.6), and it gets an outcome when math2 decides what to do with it. The open count is the number of rows that have no outcome.
//
// The files of the inventory are in the record plane of the math2 tree:
//
//	records/mentions/<source>/<shard>.jsonl   the mentions of each source
//	records/inventory/<shard>.jsonl           the rows
//	registers/inventory/<shard>.reg           the register of the row identifiers
//
// The shard of a row is the first three characters of its identifier. The shard of a mention is the first three characters of the Crockford base32 form of the BLAKE3 hash of its identifier. Each file is sorted by identifier, with one JSON object or one register entry on each line. Thus two writes of the same inventory give the same bytes.
package inventory

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/tag"
)

// The directories of the inventory, relative to the root of the tree.
const (
	MentionsDir = "records/mentions"
	RowsDir     = "records/inventory"
	RegisterDir = "registers/inventory"
)

// The layers of the corpus (corpus spec K2.7).
const (
	Textbook = "textbook"
	Formal   = "formal"
	Research = "research"
)

// Unknown is the kind of a mention when the source and the importer do not give one.
const Unknown = "unknown"

// Mention is one item as one source presents it (corpus spec K4.1).
type Mention struct {
	ID       string              `json:"id"`                  // the record identifier, m2://rec/<source>/<id>#<n>
	Source   string              `json:"source"`              // the source, from manifests/sources.yaml
	Snapshot string              `json:"snapshot"`            // the hash of the snapshot that the importer read
	Locator  string              `json:"locator,omitempty"`   // where the item is in the source, for example a URL
	NativeID string              `json:"native_id,omitempty"` // the identifier of the item in the source
	Kind     string              `json:"kind"`                // one of the kinds of spec 04.2, or unknown
	Name     string              `json:"name,omitempty"`      // the name that the source uses
	Aliases  []string            `json:"aliases,omitempty"`   // the other names of the item, from a model list (corpus K7.4)
	Uses     []string            `json:"uses,omitempty"`      // the names of the prerequisites of the item, from a model list (corpus K7.4)
	Cites    []Cite              `json:"cites,omitempty"`     // the sources that a model names for the item, which no one has checked yet (corpus K7.4)
	Text     string              `json:"text,omitempty"`      // the statement, only when the class of the source lets math2 store it
	Licence  string              `json:"licence,omitempty"`   // the licence of the item, for a source with a licence for each item (rule L06)
	MSC      []string            `json:"msc,omitempty"`       // the MSC2020 codes
	Same     map[string][]string `json:"same,omitempty"`      // the identifiers of the item in other systems, by the system names of the crosswalk
	Types    []string            `json:"types,omitempty"`     // the classes of the item in the source, for example the Wikidata items of its property P31
	TypeHash string              `json:"type_hash,omitempty"` // the hash of the canonical form of the type of a statement of a formal library (pass 2 of K4.3)
	Class    string              `json:"class"`               // the licence class (spec 9.3)
	By       string              `json:"by"`                  // the importer and its version, or the run
	Resource string              `json:"resource,omitempty"`  // the resource and the expression of the work that holds the item, as 8F2K1QW7ZB/e3 (corpus resources RS2.8)
	At       *At                 `json:"at,omitempty"`        // the structured locator of the item in the resource (RS2.8). The free-text locator stays for display

	// The fields below are for the mentions of the declarations of formal libraries (corpus K6.3 and K6.5). The other sources leave them empty.
	Pin      string `json:"pin,omitempty"`      // the commit of the library that the importer read
	Intake   string `json:"intake,omitempty"`   // the intake class of the library (corpus K6.2)
	Authored string `json:"authored,omitempty"` // human, ai or mixed, from the source document of the library

	// The fields below are for the mentions of the items of research papers (corpus arxiv AX5.3). The other sources leave them empty.
	Role        string        `json:"role,omitempty"`         // the role of a statement in spec 04.2, for example lemma
	Label       string        `json:"label,omitempty"`        // the label of the item in the paper, for example lem:key
	Hash        string        `json:"hash,omitempty"`         // the content hash of the item (AX3.8)
	TextRef     string        `json:"text_ref,omitempty"`     // where the text of the item is, when math2 does not copy it (spec 9.3)
	Version     int           `json:"version,omitempty"`      // the version of the paper that the mention describes
	Versions    []VersionRef  `json:"versions,omitempty"`     // the other versions that have the item, with their numbers
	Number      string        `json:"number,omitempty"`       // the number that the paper prints
	Env         string        `json:"env,omitempty"`          // the environment of the item
	Head        string        `json:"head,omitempty"`         // how the item starts: environment, command or text
	Also        []string      `json:"also,omitempty"`         // the locators of the other prints of a restated item (AX5.5)
	Origin      string        `json:"origin,omitempty"`       // own, cited or restated (AX3.7)
	CitesResult []CitedResult `json:"cites_result,omitempty"` // the results of other works that the item states (AX5.7)
	ProofStatus string        `json:"proof_status,omitempty"` // of a statement: the state of its proof in the paper
	Census      string        `json:"census,omitempty"`       // the census strength of the version (AX4.6)
	Structure   *Structure    `json:"structure,omitempty"`    // the edges of the item in its paper (AX06)
	AttachedTo  string        `json:"attached_to,omitempty"`  // of an attached mention: what it proves, or the proof that holds it (AX5.2)
	Retired     int           `json:"retired,omitempty"`      // the version in which the item is gone (AX5.6)
	Hints       []string      `json:"hints,omitempty"`        // hints for the passes, for example conjecture-candidate
	Flags       []string      `json:"flags,omitempty"`        // the flags of the object record
	Extractor   string        `json:"extractor,omitempty"`    // the extractor that made the object record

	// The fields below are for the mentions of Wikipedia articles (corpus wikipedia WP5.3). The other sources leave them empty.
	Page        int      `json:"page,omitempty"`         // the page id of the article
	Revision    int      `json:"revision,omitempty"`     // the revision of record of the article
	Section     string   `json:"section,omitempty"`      // the section path of the item
	Anchor      string   `json:"anchor,omitempty"`       // the anchor of the item
	Cue         string   `json:"cue,omitempty"`          // the cue that found the item (WP3.4)
	Strength    string   `json:"strength,omitempty"`     // the strength of the cue (WP3.4)
	ArticleType string   `json:"article_type,omitempty"` // the type of the article (WP1.5)
	Assessment  string   `json:"assessment,omitempty"`   // the class and the importance of the article in the project, as B/High
	AliasesFrom []string `json:"aliases_from,omitempty"` // where each alias came from, in the order of the aliases: lead, redirect or anchor-redirect
	MemberOf    string   `json:"member_of,omitempty"`    // of a member mention: the mention of the article that it points to (WP5.1)
}

// At is the structured locator of a mention in its resource (corpus resources RS2.8). Each field is optional. Page is a page of the manifestation of the resource when the expression has more than one.
type At struct {
	Part    string `json:"part,omitempty"`
	Section string `json:"section,omitempty"` // the number of the section, as 3.14
	Kind    string `json:"kind,omitempty"`    // the kind of the item in the source, as theorem
	Number  string `json:"number,omitempty"`  // the number that the source prints, as 3.14
	Page    string `json:"page,omitempty"`
}

// Cite is a source that a model names for an item. The clustering does not join mentions by a cite, because a model can give a wrong id. A cite counts only when K9.5 confirms it.
type Cite struct {
	System  string `json:"system"`            // a system of the crosswalk, doi or isbn
	ID      string `json:"id"`                // the id in the system, such as 10.1007/978-3-642-61795-0
	Locator string `json:"locator,omitempty"` // where the item is in the source, such as Theorem 2.3
}

// VersionRef is one other version of a paper that has the item of a mention.
type VersionRef struct {
	Version int    `json:"version"`
	Number  string `json:"number,omitempty"`
}

// CitedResult is a result of another work that an item states (corpus arxiv AX5.7).
type CitedResult struct {
	To         string `json:"to"`                // the mention id, the record id of the work, or bib:<key> while the key is not resolved
	Locator    string `json:"locator,omitempty"` // where the result is in the work, for example Theorem 4.1
	Confidence string `json:"confidence"`
}

// Structure is the edges of an item in its paper, as mention ids.
type Structure struct {
	Uses     []string `json:"uses,omitempty"`      // what the item uses
	ProvedBy []string `json:"proved_by,omitempty"` // the proofs of the item
	In       string   `json:"in,omitempty"`        // the section that the item is in
	Names    []string `json:"names,omitempty"`     // the rows of the named results that the text names, unique in the MSC area of the paper (corpus arxiv AX6.2, the name method)
}

// Attached reports whether the mention is attached to another mention and is not a row by itself (corpus arxiv AX5.2).
func (m *Mention) Attached() bool { return m.AttachedTo != "" }

// Row is one distinct item of the inventory (corpus spec K4.1).
type Row struct {
	ID       string   `json:"id"`
	Mentions []string `json:"mentions"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name,omitempty"`
	MSC      []string `json:"msc,omitempty"`
	Layer    string   `json:"layer"`
	Spread   int      `json:"spread"`            // the number of different sources among the mentions
	Outcome  *Outcome `json:"outcome,omitempty"` // nil while the row is open
	History  []Event  `json:"history,omitempty"`
	Proofs   []string `json:"proofs,omitempty"` // the attached mentions of the proofs of the item (corpus arxiv AX5.2)
	Local    []string `json:"local,omitempty"`  // the attached mentions of the claims inside these proofs
}

// The outcomes of a row (corpus spec K4.4).
const (
	OutcomeNode         = "node"
	OutcomeMember       = "member"
	OutcomeMerged       = "merged"
	OutcomeSplit        = "split"
	OutcomeAligned      = "aligned"
	OutcomeRecord       = "record"
	OutcomeFormalOnly   = "formal_only"
	OutcomeObjectRecord = "object_record"
	OutcomeOutOfScope   = "out_of_scope"
	OutcomeDeferred     = "deferred"
)

// Outcomes returns the outcomes in the order of corpus spec K4.4.
func Outcomes() []string {
	return []string{OutcomeNode, OutcomeMember, OutcomeMerged, OutcomeSplit, OutcomeAligned, OutcomeRecord, OutcomeFormalOnly, OutcomeObjectRecord, OutcomeOutOfScope, OutcomeDeferred}
}

// Outcome is the decision about a row.
type Outcome struct {
	Value  string   `json:"value"`            // one of Outcomes
	Target []string `json:"target,omitempty"` // the tag of the node, or the rows of a merge or a split
	Reason string   `json:"reason,omitempty"` // required for out_of_scope
	Rule   string   `json:"rule,omitempty"`   // the rule of schema/outcomes.v1.yaml that gave the outcome
	Until  string   `json:"until,omitempty"`  // the date, YYYY-MM-DD, of a deferred row
	By     string   `json:"by,omitempty"`     // the person or the run
}

// Event is one change of a row: a merge, a split or a change of the outcome.
type Event struct {
	Op     string `json:"op"`               // add, merge, split or outcome
	At     string `json:"at"`               // the date, YYYY-MM-DD
	By     string `json:"by"`               // the person or the run
	Commit string `json:"commit,omitempty"` // the commit, when it is known
	Note   string `json:"note,omitempty"`
}

// IsOpen reports whether the row counts in the open count on the day: it has no outcome, or it is deferred and its date has passed.
func (r *Row) IsOpen(day time.Time) bool {
	if r.Outcome == nil {
		return true
	}
	if r.Outcome.Value != OutcomeDeferred {
		return false
	}
	until, err := time.Parse(time.DateOnly, r.Outcome.Until)
	return err != nil || !day.Before(until.AddDate(0, 0, 1))
}

// OpenCount returns the number of rows that are open on the day.
func OpenCount(rows []Row, day time.Time) int {
	n := 0
	for i := range rows {
		if rows[i].IsOpen(day) {
			n++
		}
	}
	return n
}

// RowShard returns the shard of a row identifier: its first three characters.
func RowShard(id string) string { return id[:3] }

// MentionShard returns the shard of a mention identifier: the first three characters of the Crockford base32 form of its BLAKE3 hash.
func MentionShard(id string) string {
	sum := blake3.Sum256([]byte(id))
	v := uint(sum[0])<<8 | uint(sum[1])
	return string([]byte{tag.Alphabet[v>>11&31], tag.Alphabet[v>>6&31], tag.Alphabet[v>>1&31]})
}

// MentionID returns the record identifier of the item with the native id in the source, as spec 9.5 gives it. The fragment n tells apart two mentions of one item in one source. It is 1 for the first mention.
func MentionID(source, native string, n int) string {
	return "m2://rec/" + source + "/" + native + "#" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for ; n > 0; n /= 10 {
		i--
		b[i] = byte('0' + n%10)
	}
	return string(b[i:])
}

// sortMentions orders mentions by identifier.
func sortMentions(ms []Mention) {
	slices.SortFunc(ms, func(a, b Mention) int { return cmp.Compare(a.ID, b.ID) })
}

// sortRows orders rows by identifier.
func sortRows(rows []Row) {
	slices.SortFunc(rows, func(a, b Row) int { return cmp.Compare(a.ID, b.ID) })
}

// layerOf returns the layer of a row from the sources of its mentions. A row with a mention from a source of people is in the textbook layer, unless all its human sources are research sources. A row with only formal mentions is in the formal layer.
func layerOf(sources []string) string {
	human, research := false, false
	for _, s := range sources {
		switch {
		case formalSources[s]:
		case researchSources[s]:
			research = true
		default:
			human = true
		}
	}
	switch {
	case human:
		return Textbook
	case research:
		return Research
	}
	return Formal
}

var (
	formalSources   = map[string]bool{"mathlib": true, "formal_conjectures": true, "afp": true, "setmm": true, "mathcomp": true, "mizar": true, "metamath": true, "coq": true, "rocq": true, "hol-light": true, "agda": true}
	researchSources = map[string]bool{"arxiv": true, "arxiv_results": true, "zbmath": true, "palomar": true, "theoremsearch": true}
)

// IsFormal reports whether the source is a formal library.
func IsFormal(source string) bool { return formalSources[source] }

// IsResearch reports whether the source is a source of the research layer.
func IsResearch(source string) bool { return researchSources[source] }

// clean trims s and changes each run of white space to one space.
func clean(s string) string { return strings.Join(strings.Fields(s), " ") }
