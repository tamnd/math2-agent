package wikipedia

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/site"
	"github.com/tamnd/math2-agent/wpcorpus"
	"github.com/tamnd/math2-agent/wpsource"
)

// AdoptPath is the report of m2 corpus adopt -source wikipedia, relative to the root of the tree.
const AdoptPath = "reports/adopt/wikipedia.md"

// AdoptBy is the name of the run in the outcomes and the history of the rows.
const AdoptBy = "m2 corpus adopt"

// The rules of schema/outcomes.v1.yaml that m2 corpus adopt names in the outcomes that it gives (corpus wikipedia WP9.2). Rule I04 checks that each rule gives one outcome.
const (
	NodeRule  = "wikipedia-node"     // a minted node has the text of the item of the row
	DeferRule = "wikipedia-deferred" // the item of the row waits for the write role or for a person
)

// The states of an item in the adopt report.
const (
	Adopted = "adopted" // the run wrote a node with the text of the item
	Drafted = "drafted" // a node with a provisional id has the text, and waits for m2 mint
	Linked  = "linked"  // the run set the outcome of the row to the minted node
	Waiting = "waiting" // the item goes to the write role or to a person, with the reason
)

// AdoptNode is what the adopt job reads from a node of the tree.
type AdoptNode struct {
	ID        string
	Name      string
	Aliases   []string
	MSC       []string
	Inventory []string
	Proves    string
	Mentions  []string          // the mention of each wikipedia source of the node
	SameAs    map[string]string // the ids of same_as, as wikidata: Q1 and wikipedia: Title
	Retired   bool
}

// AdoptOptions are the inputs of Adopt that are not the inventory and the nodes.
type AdoptOptions struct {
	Corpus string                 // the checkout of tamnd/wikipedia-math
	Pages  map[int]bool           // the pages to adopt from. Nil means each page.
	Hold   map[string]string      // the mentions that a person holds, with the reason
	Draw   func() (string, error) // gives a new provisional id
	Day    string                 // the date of the run, YYYY-MM-DD
	Until  string                 // when not empty, the date until which a row whose item waits is deferred
}

// AdoptItem is one line of the adopt report.
type AdoptItem struct {
	Mention string
	Row     string
	Page    int
	Title   string
	Name    string
	Kind    string
	Cue     string
	State   string
	Reason  string
	Node    string
}

// AdoptDraft is a node file that the run writes.
type AdoptDraft struct {
	ID   string
	Path string // nodes/~/<id>.md
	Data []byte
}

// AdoptResult is the result of Adopt.
type AdoptResult struct {
	Drafts   []AdoptDraft
	Rows     []inventory.Row // each row, with the outcomes that the run set
	Linked   int             // the rows that the run set to the outcome node
	Deferred int             // the rows that the run deferred
	Items    []AdoptItem
}

// adoptFlags are the flags of WP9.2 that stop the adoption of an item.
var adoptFlags = []string{"citation-needed", "dubious", "disputed", "informal"}

// genericName matches a name that is a label and not the name of the item, as Theorem (2), Theorem A (groups), Examples or The Criterion.
var genericName = regexp.MustCompile(`(?i)^(the |main |the main )?(theorem|lemma|corollary|proposition|criterion|result|formula|conjecture|identity|inequality|claim|fact|definition|definitions|example|examples|remark|proof|statement|statements|properties|facts|notation|applications?|generali[sz]ations?|extensions?|construction|history|overview|motivation|algorithms?|representations?)( \(\d+\))?$|^(theorem|lemma|corollary|proposition) [A-Z]( \([^)]*\))?$`)

// labelName matches a numbered label, such as "Axiom 1" or "Claim 2". The label has a meaning only in its article.
var labelName = regexp.MustCompile(`(?i)^(axiom|theorem|lemma|corollary|proposition|claim|conjecture|definition|fact|step|case|rule|property|observation|remark|example|exercise|problem|condition|assumption|hypothesis)s? ([0-9]+[a-z]?|[A-Z]|[IVX]+)(\.[0-9]+)*$`)

// headingPrefix matches a heading that names a part of the text about an item and not the item, such as "Precise statement of the Adyan–Rabin theorem" or "History: the lemma that is not Burnside's".
var headingPrefix = regexp.MustCompile(`(?i)^(characteri[sz]ations?|equivalent formulations|proof of|disproof of|disproved|variants? of|interpretations? (of|via)|relations? (with|to)|extension to include|incompatibility with|modern approach|general formulas|general statement|particular values|analytic criteria|geometry of|conjectures? (on|of)|criterion for|degree of|classical integral|extended|weaker|stronger|original|history|accuracy|an?|theorems?|importance|sharpness|via|equivalence|connections|explicit|formulas?|identity group|another|alternative|connection|derivation|equivalent forms|from|comparison|relationship|relation to|validity|an introduction|theorems proved|precise statement|formal statement|statements?|definitions?|examples?|applications?|consequences|corollary|corollaries|involving|proofs?|outline|sketch|overview|generali[sz]ations?|properties|results|facts|motivation|background|introduction|some|other|further|basic|main|key)\b`)

// pluralTail matches a heading that names a group of items, such as "Freudenthal-Hopf theorems".
var pluralTail = regexp.MustCompile(`(?i)\b(theorems|results|facts|properties|lemmas|formulas|formulae|conjectures|identities|axioms|inequalities|relations|criteria|propositions|principles|primes|graphs)$`)

// kindWord matches a heading that is only the name of a kind of item, such as "Principle".
var kindWord = regexp.MustCompile(`(?i)^(principle|axiom|theorem|lemma|proposition|statement|definition|formula|identity)$`)

// theTail matches a heading that names an item of its article with "the", such as "Derivation of the formula".
var theTail = regexp.MustCompile(`(?i)\bthe (formula|theorem|lemma|identity|equation|method|result|proof|series|inequality)$`)

// sourceLabel matches a name that is a kind of item and its source, such as "Theorem (Brahmagupta)".
var sourceLabel = regexp.MustCompile(`^(Lemma|Theorem|Proposition|Definition|Axiom|Corollary) \(`)

// plainTail matches a short heading in lower case that needs its article to have a meaning, such as "Structure theorem" or "Volume formula".
var plainTail = regexp.MustCompile(`^[A-Z][a-z-]* (theorem|formula|lemma|conjecture|identity|inequality|criterion|principle)$`)

// lostPart matches a name that lost a part, such as math, so that it ends with a small word, has two spaces, or has an HTML entity, as "Approximations of" or "Proof that  is irrational".
var lostPart = regexp.MustCompile(`(?i)\b(of|to|for|with|and|or|the|a|an|by|from|that|is|in|on)$|  |&[a-z]+;`)

// numberedName matches a name with a number of its article, such as "1. Axiom of extensionality", "Axiom of pairing - 4", "Theorem 1. Every valid formula is provable" or "2 theorem".
var numberedName = regexp.MustCompile(`(?i)^[0-9]+\.\s|\s-\s[0-9]+$|^(theorem|lemma|proposition|propositions|corollary|axiom|problem|question|conjecture|exercise)\s[0-9IVX]+(\.[0-9A-Z]+)*\b|^[0-9]+ (theorem|lemma)$|\btheorem (one|two|three|four|five)$|^(theorem|lemma|proposition|corollary) [A-Z]\.?$`)

// actionHead matches a heading that names a step or a part of the text, such as "Using the Chinese remainder theorem", "As a theorem of arithmetic" or "The second theorem".
var actionHead = regexp.MustCompile(`(?i)^([a-z]+ing|as|by|in|on|for|with|without|cases|role|versions?|similarities|resolutions?|analogs|appeals|implications|objections?|criticism|chronology|software|determination|uniqueness|create|produce|solutions?)\b|^the (first|second|third|fourth|basic|main|general|extra|following|original)\b|\bexamples?\b`)

// partPhrase matches a heading that names a part of the text about an item inside the heading, such as "Graphon version of counting lemma" or "Bandwidth formulas for some graphs", or a heading with "The" and a word in lower case, such as "The stacking theorem".
var partPhrase = regexp.MustCompile(`(?i:\b(version of|analogues|for some|in terms of|laws for|rules for|approach to|formulation of|in other bases|as functors|for other groups)\b)|^The [a-z]`)

// oddName matches a name that a person must give again, because it has a question mark, a quote, a year, an open prefix or two names, as "Dirac's Theorem (1952)" or "Co-".
// otherName matches a heading that gives a second name of the item in brackets, as "Dual of Dilworth's theorem (Mirsky's theorem)".
var otherName = regexp.MustCompile(`(?i)\([^)]*\b(theorem|lemma|formula|identity|inequality|conjecture)\)$`)

// pluralWord matches a list entry that is one word in the plural, as "Lattices". The entry names a group of items.
var pluralWord = regexp.MustCompile(`^[A-Z][a-z]+s$`)

var oddName = regexp.MustCompile(`\(or |[?"#]|^'|'$|\s\([0-9]{4}\)$|\s\([A-Z]{2,6}\)$|-$| / | or `)

// questionHead matches a name that is a question, as "How many unit distances".
var questionHead = regexp.MustCompile(`^(How|Can|Are|Is|Does|Do|What|Which|Why|When)\b`)

// greekName matches a glossary term that is the name of a Greek letter, as "Alpha". The entry tells the notation and not one item.
var greekName = regexp.MustCompile(`(?i)^(alpha|beta|gamma|delta|epsilon|zeta|eta|theta|iota|kappa|lambda|mu|nu|xi|omicron|pi|rho|sigma|tau|upsilon|phi|chi|psi|omega)$`)

// seeEntry matches a glossary entry that points to other entries and has no definition, as "For strong connectivity, see connected.", "See transpose." or "Synonym for induced.".
var seeEntry = regexp.MustCompile(`(?i)^(For [^.]*, see |See |Synonyms? (for|of) |Same as |An? (synonym|alternative (name|spelling)) for )`)

// personEntry matches a glossary entry about a person, as "Julius Petersen (1839–1910), Danish graph theorist.".
var personEntry = regexp.MustCompile(`\([0-9]{4}[–-][0-9]{4}\)|\(c\. ?[0-9]{1,4}( BCE| BC)?[–-]|\([0-9]{1,4} (BCE|BC)[–-]|^[A-Z][^ ,]+( [A-Z][^ ,]+)+, [^.]*\b(mathematician|theorist|logician|scientist|statistician)\b`)

// abbreviation matches a glossary term that is an abbreviation, such as "GCH". The node takes the full name.
var abbreviation = regexp.MustCompile(`^[A-Z][A-Z0-9-]{0,5}$`)

// whyName gives the reason why an item waits when its name is not a good name for a node, or "". A heading of a section can name a part of the text, a group of items, or an item that has a meaning only in its article. A person gives the name of such an item.
func whyName(m *inventory.Mention) string {
	if r, _ := utf8.DecodeRuneInString(m.Name); !unicode.IsLetter(r) && !unicode.IsDigit(r) {
		return "the name does not start with a letter or a digit, so the name lost a part, such as math"
	}
	if lostPart.MatchString(m.Name) {
		return "the name ends with a small word or has a gap, so the name lost a part, such as math"
	}
	if numberedName.MatchString(m.Name) {
		return "the name has a number of its article, and a person gives the name"
	}
	if m.Cue != "lead" && (oddName.MatchString(m.Name) || questionHead.MatchString(m.Name)) {
		return "the name has a question, a quote, a year, or an open prefix, and a person gives the name"
	}
	if m.Cue == "term" && greekName.MatchString(m.Name) {
		return "the term is the name of a Greek letter, so the entry tells the notation and not one item"
	}
	if (m.Cue == "term" || m.Cue == "list-entry") && abbreviation.MatchString(m.Name) {
		return "the term is an abbreviation, and the node takes the full name"
	}
	if r, _ := utf8.DecodeRuneInString(m.Name); m.Cue != "lead" && m.Cue != "term" && r >= 'a' && r <= 'z' {
		return "the name starts with a lower case letter, so the name is a part of a sentence"
	}
	if m.Cue != "lead" && utf8.RuneCountInString(m.Name) == 1 {
		return "the name is one character, so a person gives the name"
	}
	if sourceLabel.MatchString(m.Name) {
		return "the name is a kind of item and its source, and not the name of the item"
	}
	if m.Cue == "lead" || m.Cue == "term" || m.Cue == "list-entry" {
		if strings.HasPrefix(m.Name, "the ") || strings.HasPrefix(m.Name, "a ") || strings.HasPrefix(m.Name, "an ") {
			return "the name starts with an article, and a person gives the name"
		}
		if m.Cue == "list-entry" && strings.Contains(m.Name, ": ") {
			return "the entry names more than one item"
		}
		if m.Cue == "list-entry" && (pluralTail.MatchString(m.Name) || pluralWord.MatchString(m.Name)) {
			return "the entry names a group of items"
		}
		return ""
	}
	switch {
	case !strings.Contains(m.Name, " "):
		return "the heading is one word, so it names a part of the text and not one item"
	case actionHead.MatchString(m.Name):
		return "the heading names a step or a part of the text and not one item"
	case theTail.MatchString(m.Name):
		return "the heading names an item of its article with \"the\""
	case kindWord.MatchString(m.Name):
		return "the heading names a kind of item and not one item"
	case otherName.MatchString(m.Name):
		return "the heading gives a second name of the item in brackets, and a person gives the name"
	case headingPrefix.MatchString(m.Name), partPhrase.MatchString(m.Name):
		return "the heading names a part of the text and not one item"
	case strings.Contains(m.Name, ": "), strings.Contains(m.Name, " and "), strings.Contains(m.Name, ", "):
		return "the heading names more than one item"
	case pluralTail.MatchString(m.Name):
		return "the heading names a group of items"
	case plainTail.MatchString(m.Name):
		return "the heading has a meaning only in its article"
	}
	return ""
}

// fieldName matches the name of a field of study, as "Classical differential geometry" or "Single operator theory".
var fieldName = regexp.MustCompile(`(?i)\b(geometry|theory|mathematics|trigonometry|topology|calculus)$`)

// caseFor matches the word that joins the name of an item and a case of it, as in "Local Langlands conjectures for GL2".
var caseFor = regexp.MustCompile(` (for|over) `)

// caseOf reports if a name is the name of the subject of the article on the page and a case of it, as "Local Langlands conjectures for GL2" in the article "Local Langlands conjectures".
func caseOf(name string, page int, subjectOf map[string]int) bool {
	loc := caseFor.FindStringIndex(name)
	if loc == nil {
		return false
	}
	head := strings.TrimPrefix(name[:loc[0]], "The ")
	p, ok := subjectOf[inventory.NameKey(head)]
	return ok && p == page
}

// proofName gives the name of the proof of a statement. A leading "The" of the name of the statement is in lower case, as in "Proof of the ping-pong lemma".
func proofName(name string) string {
	if rest, ok := strings.CutPrefix(name, "The "); ok {
		name = "the " + rest
	}
	return "Proof of " + name
}

// adoptCues are the cues that give the item a name of its own (WP9.1 reason 2). A plain heading names a section, not an item.
var adoptCues = []string{"lead", "math-theorem", "named-heading", "term", "inline-head", "list-entry"}

// cardSection gives the kind of the node and the section of its card for the kind of a mention (K8.4).
func cardSection(kind string) (string, string) {
	switch kind {
	case "definition":
		return "concept", "Summary"
	case "statement":
		return "statement", "Statement"
	case "conjecture":
		return "conjecture", "Statement"
	case "problem":
		return "problem", "Problem"
	case "example":
		return "example", "Example"
	case "counterexample":
		return "counterexample", "Counterexample"
	case "proof":
		return "proof", "Proof"
	}
	return "", ""
}

// Adopt makes the nodes of WP9.2 from the Wikipedia rows of the pages. First it sets the outcome node on each open row that a minted node names in its inventory field. Then it takes each open row with a mention of the pages, and it writes a node with the text of the item, or it reports why the item waits. A proof attached to a statement with a node, or to a statement that the run adopts, becomes a proof node.
func Adopt(rows []inventory.Row, ms []inventory.Mention, nodes []AdoptNode, opt AdoptOptions) (*AdoptResult, error) {
	res := &AdoptResult{Rows: rows}
	byRow := map[string]*inventory.Row{}
	rowOf := map[string]*inventory.Row{}
	for i := range res.Rows {
		r := &res.Rows[i]
		byRow[r.ID] = r
		for _, m := range r.Mentions {
			rowOf[m] = r
		}
	}

	// The nodes that the tree has.
	byName := map[string]string{}
	bySame := map[string]string{}
	nameOf := map[string]string{}
	proved := map[string]bool{}
	done := map[string]string{}
	msc := map[string][]string{}
	for _, n := range nodes {
		if n.Retired {
			continue
		}
		for _, s := range append([]string{n.Name}, n.Aliases...) {
			if k := inventory.NameKey(s); k != "" {
				if _, ok := byName[k]; !ok {
					byName[k] = n.ID
				}
			}
		}
		for k, v := range n.SameAs {
			if _, ok := bySame[k+":"+v]; !ok && v != "" {
				bySame[k+":"+v] = n.ID
			}
		}
		if n.Proves != "" {
			proved[n.Proves] = true
		}
		for _, m := range n.Mentions {
			done[m] = n.ID
		}
		msc[n.ID] = n.MSC
		nameOf[n.ID] = n.Name
		provisional := strings.HasPrefix(n.ID, "~")
		for _, id := range n.Inventory {
			r := byRow[id]
			if r == nil || r.Outcome != nil || provisional || len(n.Mentions) == 0 {
				continue
			}
			r.Outcome = &inventory.Outcome{Value: inventory.OutcomeNode, Target: []string{n.ID}, Rule: NodeRule, By: AdoptBy}
			r.History = append(r.History, inventory.Event{Op: "outcome", At: opt.Day, By: AdoptBy, Note: "node " + n.ID + " has the adopted text"})
			res.Linked++
		}
	}

	// The subjects of the articles.
	byMention := map[string]*inventory.Mention{}
	subjectOf := map[string]int{}
	title := map[int]string{}
	for i := range ms {
		m := &ms[i]
		if m.Source != Source {
			continue
		}
		byMention[m.ID] = m
		if strings.Contains(m.ID, "#") {
			continue
		}
		title[m.Page] = m.NativeID
		for _, s := range append([]string{m.Name, Name(m.NativeID)}, m.Aliases...) {
			if k := inventory.NameKey(s); k != "" {
				if _, ok := subjectOf[k]; !ok {
					subjectOf[k] = m.Page
				}
			}
		}
	}
	nodeOf := func(mention string) string {
		r := rowOf[mention]
		if r == nil || r.Outcome == nil || r.Outcome.Value != inventory.OutcomeNode || len(r.Outcome.Target) == 0 {
			return ""
		}
		return r.Outcome.Target[0]
	}
	pageNode := func(page int) string { return nodeOf(wpcorpus.MentionID(page, "")) }

	pages := &pageCache{corpus: opt.Corpus, files: map[int]*pageFile{}}
	adopted := map[string]string{}
	var proofs []*inventory.Mention
	for i := range ms {
		m := &ms[i]
		if m.Source != Source || opt.Pages != nil && !opt.Pages[m.Page] {
			continue
		}
		if m.Kind == "proof" {
			proofs = append(proofs, m)
			continue
		}
		r := rowOf[m.ID]
		if r == nil || !r.IsOpen(dayTime(opt.Day)) {
			continue
		}
		it := AdoptItem{Mention: m.ID, Row: r.ID, Page: m.Page, Title: title[m.Page], Name: m.Name, Kind: m.Kind, Cue: m.Cue}
		if id, ok := done[m.ID]; ok {
			it.State, it.Node = Drafted, id
			res.Items = append(res.Items, it)
			continue
		}
		it.State = Waiting
		it.Reason = whyWait(m, opt.Hold, subjectOf, byName)
		if it.Reason == "" {
			it.Reason = whySame(m, title[m.Page], bySame)
		}
		if it.Reason == "" {
			d, why, err := adoptNode(m, displayName(m), freeAliases(m, byName), pages, pageNode, opt, msc[pageNode(m.Page)], "", r.ID)
			if err != nil {
				return nil, err
			}
			if d == nil {
				it.Reason = why
			} else {
				res.Drafts = append(res.Drafts, *d)
				adopted[m.ID] = d.ID
				nameOf[d.ID] = m.Name
				if k := inventory.NameKey(m.Name); byName[k] == "" {
					byName[k] = d.ID
				}
				it.State, it.Node = Adopted, d.ID
			}
		}
		if it.State == Waiting && opt.Until != "" {
			r.Outcome = &inventory.Outcome{Value: inventory.OutcomeDeferred, Reason: it.Reason, Rule: DeferRule, Until: opt.Until, By: AdoptBy}
			r.History = append(r.History, inventory.Event{Op: "outcome", At: opt.Day, By: AdoptBy, Note: "deferred until " + opt.Until})
			res.Deferred++
		}
		res.Items = append(res.Items, it)
	}

	// The proofs of the statements with a node.
	for _, m := range proofs {
		it := AdoptItem{Mention: m.ID, Page: m.Page, Title: title[m.Page], Name: m.Name, Kind: m.Kind, Cue: m.Cue, State: Waiting}
		target := adopted[m.AttachedTo]
		if target == "" {
			target = nodeOf(m.AttachedTo)
		}
		t := byMention[m.AttachedTo]
		switch {
		case strings.HasPrefix(done[m.ID], "~"):
			it.State, it.Node = Drafted, done[m.ID]
		case done[m.ID] != "":
			continue // a minted node has the proof, and the proof has no row to link
		case target == "":
			it.Reason = "the statement that the proof proves has no node"
		case t != nil && t.Kind != "statement":
			it.Reason = "the proof is attached to an item that is not a statement"
		case nameOf[target] == "":
			it.Reason = "node " + target + " has no name"
		case strings.HasPrefix(nameOf[target], "Proof "):
			it.Reason = "node " + target + " has the name of a proof, and a person gives the name of the statement"
		case proved[target]:
			it.Reason = "node " + target + " has a proof node"
		default:
			it.Reason = whyFlag(m, opt.Hold)
		}
		if it.State == Waiting && it.Reason == "" {
			d, why, err := adoptOne(m, proofName(nameOf[target]), pages, pageNode, opt, msc[pageNode(m.Page)], target, "")
			if err != nil {
				return nil, err
			}
			if d == nil {
				it.Reason = why
			} else {
				res.Drafts = append(res.Drafts, *d)
				proved[target] = true
				it.State, it.Node = Adopted, d.ID
			}
		}
		res.Items = append(res.Items, it)
	}
	slices.SortStableFunc(res.Items, func(a, b AdoptItem) int {
		if a.Page != b.Page {
			return a.Page - b.Page
		}
		return strings.Compare(a.Mention, b.Mention)
	})
	return res, nil
}

// dayTime gives the time of a date YYYY-MM-DD, or the zero time.
func dayTime(day string) time.Time {
	t, _ := time.Parse(time.DateOnly, day)
	return t
}

func whyFlag(m *inventory.Mention, hold map[string]string) string {
	if why, ok := hold[m.ID]; ok {
		return "a person holds the item: " + why
	}
	for _, f := range m.Flags {
		if slices.Contains(adoptFlags, f) {
			return "the item has the flag " + f + " (WP9.2)"
		}
	}
	return ""
}

// whyWait gives the reason why the item of a mention is not adopted, or "".
func whyWait(m *inventory.Mention, hold map[string]string, subjectOf map[string]int, byName map[string]string) string {
	if why := whyFlag(m, hold); why != "" {
		return why
	}
	if m.Kind == "topic" {
		return "a list or a glossary is not a node (WP9.1)"
	}
	if _, sec := cardSection(m.Kind); sec == "" {
		return "the kind of the item is " + m.Kind
	}
	if !slices.Contains(adoptCues, m.Cue) {
		return "the cue " + m.Cue + " names a section and not one item"
	}
	if m.Name == "" || genericName.MatchString(m.Name) || labelName.MatchString(m.Name) {
		return "the item has no name of its own, and the write role names it"
	}
	if why := whyName(m); why != "" {
		return why
	}
	if m.ArticleType == "biography" || m.ArticleType == "field" {
		return "an article of type " + m.ArticleType + " gives a record and not a node (WP9.1)"
	}
	if m.Cue == "term" && fieldName.MatchString(m.Name) {
		return "the term names a field of study, and a field gives a record and not a node (WP9.1)"
	}
	if m.Cue != "lead" && caseOf(m.Name, m.Page, subjectOf) {
		return "the heading names a case of the subject of its article, and the case goes in the node of the subject"
	}
	k := inventory.NameKey(m.Name)
	if p, ok := subjectOf[k]; ok && strings.Contains(m.ID, "#") {
		if p == m.Page {
			return "the item has the name of the subject of its article, and a person decides if they are one item"
		}
		return "the item has its own article, page " + strconv.Itoa(p)
	}
	if id, ok := byName[k]; ok {
		return "node " + id + " has the same name, and a person decides if they are one item"
	}
	return ""
}

// whySame gives the reason why the subject of an article waits when a node has
// the same Wikidata item or the same article in same_as, or "". The names of
// the two can be different, as Out(Fn) and Out(F_n).
func whySame(m *inventory.Mention, title string, bySame map[string]string) string {
	if strings.Contains(m.ID, "#") {
		return ""
	}
	for _, q := range m.Same["wikidata"] {
		if id, ok := bySame["wikidata:"+q]; ok {
			return "node " + id + " has the same Wikidata item " + q + ", and a person decides if they are one item"
		}
	}
	if id, ok := bySame["wikipedia:"+title]; ok && title != "" {
		return "node " + id + " has the same article in same_as, and a person decides if they are one item"
	}
	return ""
}

// adoptOne writes the node of one item. It gives a nil draft and the reason when the text cannot be adopted.
func adoptOne(m *inventory.Mention, name string, pages *pageCache, pageNode func(int) string, opt AdoptOptions, msc []string, proves, row string) (*AdoptDraft, string, error) {
	return adoptNode(m, name, nil, pages, pageNode, opt, msc, proves, row)
}

func adoptNode(m *inventory.Mention, name string, aliases []string, pages *pageCache, pageNode func(int) string, opt AdoptOptions, msc []string, proves, row string) (*AdoptDraft, string, error) {
	pf, err := pages.get(m.Page, m.Revision)
	if err != nil {
		return nil, "", err
	}
	if pf == nil {
		return nil, "the corpus has no content Markdown of the revision", nil
	}
	ref := pf.refs[tagOf(m.ID)]
	if !strings.Contains(m.ID, "#") {
		ref = pf.subject
	}
	span, ok := pf.span(ref)
	if !ok {
		return nil, "the content Markdown has no block for the item", nil
	}
	text := convert(span, pageNode, pf.notes)
	if text == "" {
		return nil, "the block of the item has no text", nil
	}
	if strings.HasPrefix(m.Assessment, "Stub") && sentences(text) < 2 {
		return nil, "the article is a stub with fewer than two sentences about the item (WP9.2)", nil
	}
	if sentences(text) == 0 {
		return nil, "the text of the item has no full sentence, so the write role writes the text (WP9.2)", nil
	}
	if _, sec := cardSection(m.Kind); urlText.MatchString(text) && slices.Contains(check.StatementSections(), sec) {
		return nil, "the text has a URL, and a statement section has no URLs (B02)", nil
	}
	if m.Cue == "term" && seeEntry.MatchString(strings.TrimSpace(text)) {
		return nil, "the entry points to other entries and has no definition", nil
	}
	if (m.Cue == "term" || m.Cue == "list-entry") && personEntry.MatchString(strings.TrimSpace(text)) {
		return nil, "the entry tells about a person, and a person gives a record and not a node (WP9.1)", nil
	}
	if m.Cue == "term" && modifierEntry(m.Name, text) {
		return nil, "the entry name is a word that qualifies a noun in the definition, so the name is not the full term", nil
	}
	if listsItems(m.Cue, text) {
		return nil, "the entry starts with a name and not a sentence, so it lists more than one item", nil
	}
	kind, section := cardSection(m.Kind)
	id, err := opt.Draw()
	if err != nil {
		return nil, "", err
	}
	src := wpsource.Ref{Page: int64(m.Page), Revision: int64(m.Revision)}
	locator := pf.title
	if m.Section != "" {
		locator += ", section " + m.Section
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "tag: %s\nkind: %s\n", id, kind)
	if kind == "statement" {
		role := m.Role
		if role == "" || role == "fact" || role == "axiom" {
			role = "proposition"
		}
		fmt.Fprintf(&b, "role: %s\n", role)
	}
	fmt.Fprintf(&b, "name: %s\n", scalar(name))
	if len(aliases) > 0 && kind != "proof" {
		fmt.Fprintf(&b, "aliases: %s\n", flow(aliases))
	}
	if len(msc) > 0 {
		fmt.Fprintf(&b, "msc: %s\n", flow(msc))
	}
	if proves != "" {
		fmt.Fprintf(&b, "proves: %s\n", proves)
	}
	b.WriteString("sources:\n")
	fmt.Fprintf(&b, "  - %s: %s\n    locator: %s\n    note: adopted\n    mention: %s\n    sections: [%s]\n", wpsource.Key, src, scalar(locator), scalar(m.ID), section)
	if q := m.Same["wikidata"]; len(q) > 0 && !strings.Contains(m.ID, "#") {
		fmt.Fprintf(&b, "same_as:\n  wikidata: %s\n  wikipedia: %s\n", q[0], scalar(pf.title))
	}
	if row != "" {
		fmt.Fprintf(&b, "inventory: [%s]\n", row)
	}
	fmt.Fprintf(&b, "---\n\n## %s\n\n%s\n", section, text)
	return &AdoptDraft{ID: id, Path: "nodes/~/" + id + ".md", Data: b.Bytes()}, "", nil
}

// displayName gives the name of the node: the name of the mention, with a capital first letter for a glossary term, as the other node names have. A name that starts with a letter symbol, as p-group, does not change.
func displayName(m *inventory.Mention) string {
	// The subject of an article with a qualifier, such as "Block (permutation group theory)", keeps the qualifier, because the name alone has more than one meaning.
	if m.Cue == "lead" && !strings.Contains(m.ID, "#") && m.NativeID != Name(m.NativeID) && inventory.NameKey(m.Name) == inventory.NameKey(Name(m.NativeID)) {
		return m.NativeID
	}
	// A heading such as "The Routh criterion" names the item with "The", and the node takes "Routh criterion".
	if rest, ok := strings.CutPrefix(m.Name, "The "); ok && m.Cue != "lead" && m.Cue != "term" && rest != "" && unicode.IsUpper([]rune(rest)[0]) {
		return rest
	}
	if m.Cue != "term" || m.Name == "" {
		return m.Name
	}
	r := []rune(m.Name)
	if !unicode.IsLower(r[0]) || len(r) > 1 && !unicode.IsLower(r[1]) {
		return m.Name
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// freeAliases gives the aliases of the mention that are not the name or an alias of a node, so that the new node does not take the name of another node. Schema node.v1 gives an alias as a slug, so each alias is a slug, and a slug of the name is not an alias.
func freeAliases(m *inventory.Mention, byName map[string]string) []string {
	var out []string
	name := site.Slug(m.Name)
	for _, a := range m.Aliases {
		if _, ok := byName[inventory.NameKey(a)]; ok {
			continue
		}
		if s := site.Slug(a); s != "" && s != name && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func tagOf(id string) string {
	_, t, _ := strings.Cut(id, "#")
	return t
}

func scalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimSuffix(string(out), "\n")
}

func flow(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = scalar(v)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

var sentenceEnd = regexp.MustCompile(`[.!?](\s|$)`)

// bareName matches an entry that is only the name of a person, as "Emil Artin.".
var bareName = regexp.MustCompile(`^[A-Z]\pL+( [A-Z][\pL.]*)+\.$`)

// usageEntry matches a glossary entry that tells how a word is used and does not define one item, as `The word "big" when applied to a module` or "Named after Richard P. Stanley".
var usageEntry = regexp.MustCompile(`^(The (word|term|prefix|adjective|adverb) |This (often |usually )?(refers|means)|Named after |(Occasionally|Often|Usually|Sometimes) used|An? (alternative|other|older) (term|name) for |(Usually|Often) means )`)

// nameWord matches the first use of a word in a text, with the word or the "'s" that follows it.
var nameWord = regexp.MustCompile(`^('s\b|\s+[\p{Ll}-]{3,}\b)`)

// joinWords are the words that can follow the name of a term in its definition, as in "a ring is" or "called admissible if".
var joinWords = map[string]bool{"is": true, "are": true, "was": true, "were": true, "if": true, "of": true, "in": true, "on": true, "at": true, "to": true, "for": true, "with": true, "and": true, "or": true, "by": true, "that": true, "which": true, "when": true, "where": true, "has": true, "have": true, "means": true, "consists": true, "can": true, "may": true, "over": true, "under": true, "such": true, "as": true, "be": true, "denotes": true, "refers": true, "also": true, "here": true, "there": true}

// modifierEntry reports if a glossary entry has a one-word name that qualifies a noun in the definition, as "Unitary" with "A unitary representation of a group" or "Brauer" with "Brauer's theorem". The full term is the name and the noun, so the entry name is not the name of the item. It also reports an entry whose first paragraph is only a name, as "Emil Artin." or the entry name again, and an entry that defines the name as a property, as "is called admissible if".
func modifierEntry(name, text string) bool {
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n\n")
	if bareName.MatchString(first) || strings.EqualFold(strings.TrimSuffix(first, "."), name) {
		return true
	}
	if usageEntry.MatchString(first) {
		return true
	}
	q := regexp.QuoteMeta(name)
	if regexp.MustCompile(`(?i)\b(called|said to( be| hold)?) [*\[]{0,2}` + q + `\b|\bis \*{0,2}` + q + `\*{0,2} (if|when)\b`).MatchString(first) {
		return true
	}
	if strings.ContainsAny(name, " ") {
		return false
	}
	// The name is the head noun of a longer term, as "Hull" in "An injective hull" or "Regularity" in "Castelnuovo–Mumford regularity".
	if w := strings.Fields(first); len(w) > 2 {
		if w[0] == "A" || w[0] == "An" || w[0] == "The" {
			w = w[1:]
		}
		if !strings.EqualFold(w[0], name) && strings.EqualFold(strings.Trim(w[1], ".,;:*"), name) {
			return true
		}
	}
	if strings.Contains(name, "-") && !strings.Contains(strings.ToLower(first), strings.ToLower(name)) {
		name = strings.ReplaceAll(name, "-", "")
	}
	at := strings.Index(strings.ToLower(first), strings.ToLower(name))
	for at >= 0 {
		if at == 0 || !isLetter(first[at-1]) {
			rest := first[at+len(name):]
			if len(rest) > 1 && rest[0] == '-' && isLetter(rest[1]) || strings.HasPrefix(rest, "–") && len(rest) > 3 && isLetter(rest[3]) {
				return true
			}
			if rest == "" || !isLetter(rest[0]) {
				w := nameWord.FindStringSubmatch(rest)
				if w == nil {
					return false
				}
				word := strings.ToLower(strings.TrimSpace(w[1]))
				return word == "'s" || !joinWords[word]
			}
		}
		next := strings.Index(strings.ToLower(first[at+1:]), strings.ToLower(name))
		if next < 0 {
			break
		}
		at += 1 + next
	}
	return false
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80 }

// listsItems reports if a glossary entry or a list entry starts with a bare name, such as "Oswald Veblen" or "sharp". Such an entry lists the items that have the name, and it is not one item.
func listsItems(cue, text string) bool {
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n\n")
	return (cue == "term" || cue == "list-entry") && sentences(first) == 0
}

// urlText matches a URL in the converted text, such as the link of a citation.
var urlText = regexp.MustCompile(`https?://`)

func sentences(text string) int { return len(sentenceEnd.FindAllStringIndex(text, -1)) }

// pageFile is the content Markdown of one revision, with the objects file that maps the tags to the refs of the blocks.
type pageFile struct {
	title   string
	lines   []string
	refs    map[string]string // tag to ref
	notes   map[string]string // footnote mark to the first line of its text
	subject string            // the ref of the subject block
}

type pageCache struct {
	corpus string
	files  map[int]*pageFile
}

func (c *pageCache) get(page, rev int) (*pageFile, error) {
	if pf, ok := c.files[page]; ok {
		return pf, nil
	}
	c.files[page] = nil
	md, err := os.ReadFile(filepath.Join(c.corpus, wpcorpus.MarkdownPath(page))) // #nosec G304 -- the corpus is a checkout that the user gives.
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var head struct {
		Title    string `yaml:"title"`
		Revision int    `yaml:"revision"`
	}
	if h, _, err := splitHeader(md); err == nil {
		_ = yaml.Unmarshal(h, &head)
	}
	if head.Revision != rev {
		return nil, nil
	}
	od, err := os.ReadFile(filepath.Join(c.corpus, wpcorpus.ObjectsPath(page, rev))) // #nosec G304 -- the corpus is a checkout that the user gives.
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	objs, err := wpcorpus.ParseObjects(od)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", wpcorpus.ObjectsPath(page, rev), err)
	}
	pf := &pageFile{title: head.Title, lines: strings.Split(string(md), "\n"), refs: map[string]string{}, notes: map[string]string{}}
	for _, l := range pf.lines {
		if g := noteLine.FindStringSubmatch(l); g != nil {
			pf.notes[g[1]] = strings.TrimSpace(g[2])
		}
	}
	for _, o := range objs {
		if o.Tag != "" {
			pf.refs[o.Tag] = o.Ref
		}
		if o.Kind == "subject" && pf.subject == "" {
			pf.subject = o.Ref
		}
	}
	c.files[page] = pf
	return pf, nil
}

func splitHeader(data []byte) ([]byte, []byte, error) {
	const fm = "---\n"
	if !bytes.HasPrefix(data, []byte(fm)) {
		return nil, data, fmt.Errorf("no header")
	}
	rest := data[len(fm):]
	i := bytes.Index(rest, []byte("\n"+fm))
	if i < 0 {
		return nil, data, fmt.Errorf("the header does not close")
	}
	return rest[:i+1], rest[i+1+len(fm):], nil
}

// divOpen matches the first line of a fenced div of the content Markdown, as ::: {#o7 .statement ...}.
var divOpen = regexp.MustCompile(`^:{3,} *\{#([A-Za-z0-9_-]+)`)

func isDivClose(l string) bool { return len(l) >= 3 && strings.Trim(l, ":") == "" }

// span gives the lines inside the div of the ref. The divs inside it are other items, so they are not in the result.
func (pf *pageFile) span(ref string) ([]string, bool) {
	if ref == "" {
		return nil, false
	}
	for i, l := range pf.lines {
		g := divOpen.FindStringSubmatch(l)
		if g == nil || g[1] != ref {
			continue
		}
		var out []string
		depth := 0
		for _, l := range pf.lines[i+1:] {
			switch {
			case strings.HasPrefix(l, ":::") && strings.Contains(l, "{"):
				depth++
				continue
			case isDivClose(l):
				if depth == 0 {
					return out, true
				}
				depth--
				continue
			}
			if depth == 0 {
				out = append(out, l)
			}
		}
		return out, true
	}
	return nil, false
}

var (
	mdLink   = regexp.MustCompile(`\[((?:[^\[\]]|\\\[|\\\])*)\]\((wikipedia://en/([0-9]+)(?:#[^)]*)?|https?://[^)\s]+)\)`)
	footnote = regexp.MustCompile(`\[\^[^\]]+\]`)
	noteLine = regexp.MustCompile(`^\[\^([^\]]+)\]: (.+)$`)
	// citeWord matches the end of a text where a footnote mark is a part of the sentence, as in "a proof may be found in [^n3].".
	citeWord  = regexp.MustCompile(`(?i)\b(in|see|by|from|at|of|to|cf\.)\s*$`)
	heading   = regexp.MustCompile(`^#{1,6} `)
	boldLabel = regexp.MustCompile(`^\*\*[^*]+\*\*$`)
)

// convert gives the body text of the item from the lines of its block (WP9.2 step 3). It drops the headings, the images, the label of the item and the footnote marks, and it changes each link to a link to a node, or to its text when the target has no node. The words are not changed.
func convert(lines []string, pageNode func(int) string, notes map[string]string) string {
	var paras []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			paras = append(paras, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			flush()
			continue
		}
		cur = append(cur, l)
	}
	flush()
	var out []string
	for i, p := range paras {
		switch {
		case heading.MatchString(p) && !strings.Contains(p, "\n"):
			continue
		case strings.HasPrefix(p, "*Image:"):
			continue
		case i <= 1 && boldLabel.MatchString(p) && len(out) == 0:
			continue
		}
		p = dropNotes(p, notes)
		p = mdLink.ReplaceAllStringFunc(p, func(s string) string {
			g := mdLink.FindStringSubmatch(s)
			text := g[1]
			if g[3] != "" {
				if page, err := strconv.Atoi(g[3]); err == nil {
					if t := pageNode(page); t != "" && !strings.ContainsAny(text, "|[]") {
						return "[[" + text + "|" + t + "]]"
					}
				}
			}
			return text
		})
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

// dropNotes drops the footnote marks of a paragraph. A mark that is a part of the sentence, after a word such as "in" or "see" and before a punctuation mark or the end, gives the first line of its note in place of the mark, so that the sentence keeps its object.
func dropNotes(p string, notes map[string]string) string {
	var b strings.Builder
	last := 0
	for _, ix := range footnote.FindAllStringIndex(p, -1) {
		before := p[last:ix[0]]
		b.WriteString(before)
		last = ix[1]
		note := strings.TrimSuffix(notes[p[ix[0]+2:ix[1]-1]], ".")
		rest := strings.TrimLeft(p[ix[1]:], " ")
		if note == "" || !citeWord.MatchString(b.String()) || rest != "" && !strings.ContainsAny(rest[:1], ".,;:)") {
			continue
		}
		if !strings.HasSuffix(b.String(), " ") {
			b.WriteString(" ")
		}
		b.WriteString(note)
	}
	b.WriteString(p[last:])
	return b.String()
}

// AdoptReport gives the report of the run: a YAML header with the counts, and a table of the items of each article.
func AdoptReport(res *AdoptResult, day string) []byte {
	count := map[string]int{}
	reasons := map[string]int{}
	for _, it := range res.Items {
		count[it.State]++
		if it.State == Waiting {
			r, _, _ := strings.Cut(it.Reason, ": ")
			r = pageNum.ReplaceAllString(r, "page P")
			r = nodeTag.ReplaceAllString(r, "node T")
			reasons[r]++
		}
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "source: wikipedia\ndate: %s\nitems: %d\nadopted: %d\ndrafted: %d\nlinked: %d\nwaiting: %d\ndeferred: %d\n", day, len(res.Items), count[Adopted], count[Drafted], res.Linked, count[Waiting], res.Deferred)
	b.WriteString("---\n\n# The adoption of Wikipedia text\n\n")
	fmt.Fprintf(&b, "This report gives the items of the Wikipedia articles that m2 corpus adopt read on %s (corpus wikipedia WP9.2). The run adopted %d items, %d items wait for m2 mint, and it set the outcome node on %d rows. %d items wait for the write role or for a person, and the run deferred %d of their rows.\n", day, count[Adopted], count[Drafted], res.Linked, count[Waiting], res.Deferred)
	if len(reasons) > 0 {
		b.WriteString("\n## Why items wait\n\n| Reason | Items |\n|---|---|\n")
		keys := make([]string, 0, len(reasons))
		for k := range reasons {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b string) int {
			if reasons[a] != reasons[b] {
				return reasons[b] - reasons[a]
			}
			return strings.Compare(a, b)
		})
		for _, k := range keys {
			fmt.Fprintf(&b, "| %s | %d |\n", k, reasons[k])
		}
	}
	page := -1
	for _, it := range res.Items {
		if it.Page != page {
			page = it.Page
			fmt.Fprintf(&b, "\n## %s\n\nPage %d.\n\n| Mention | Kind | Cue | Name | State | Node or reason |\n|---|---|---|---|---|---|\n", cell(it.Title), it.Page)
		}
		what := it.Node
		if it.State == Waiting {
			what = it.Reason
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", cell(it.Mention), it.Kind, it.Cue, cell(it.Name), it.State, cell(what))
	}
	return b.Bytes()
}

var (
	pageNum = regexp.MustCompile(`page [0-9]+`)
	nodeTag = regexp.MustCompile(`node [~0-9A-Za-z]+`)
)

func cell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }
