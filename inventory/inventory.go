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
	Text     string              `json:"text,omitempty"`      // the statement, only when the class of the source lets math2 store it
	MSC      []string            `json:"msc,omitempty"`       // the MSC2020 codes
	Same     map[string][]string `json:"same,omitempty"`      // the identifiers of the item in other systems, by the system names of the crosswalk
	Class    string              `json:"class"`               // the licence class (spec 9.3)
	By       string              `json:"by"`                  // the importer and its version, or the run
}

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
	formalSources   = map[string]bool{"mathlib": true, "formalconjectures": true, "afp": true, "mizar": true, "metamath": true, "coq": true, "rocq": true, "hol-light": true, "agda": true}
	researchSources = map[string]bool{"arxiv": true, "arxivresults": true, "zbmath": true, "palomar": true, "theoremsearch": true}
)

// IsFormal reports whether the source is a formal library.
func IsFormal(source string) bool { return formalSources[source] }

// IsResearch reports whether the source is a source of the research layer.
func IsResearch(source string) bool { return researchSources[source] }

// clean trims s and changes each run of white space to one space.
func clean(s string) string { return strings.Join(strings.Fields(s), " ") }
