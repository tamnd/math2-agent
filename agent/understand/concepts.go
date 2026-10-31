package understand

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	axImport "github.com/tamnd/math2-agent/importer/arxiv"
)

// The pass concepts (AX7.3) gives the MSC2020 codes of each item, the concepts that it is about, uses or defines, and the terms of a definition.

// NewConcept is the flag of a concept that has no row and no Wikidata item. It is a candidate for the enumerate queue, and it is not a row (rule I03).
const NewConcept = "new-concept"

// Roles are the roles of a concept in an item (AX7.3).
var Roles = []string{"about", "uses", "defines"}

var (
	mscCode  = regexp.MustCompile(`^[0-9]{2}[A-Z-][0-9xX]{2}$`)
	qid      = regexp.MustCompile(`^Q[1-9][0-9]*$`)
	maxName  = 80
	maxTerms = 8
)

var conceptsPass = pass{
	tool:  "tag_concepts",
	about: "Give the MSC2020 codes and the concepts of each item of the paper. Give all the items in one call.",
	other: "Give the MSC2020 codes and the concepts of each item. Only for the pass concepts.",
	chars: 2000,
	// Concepts reads each item that gets a mention, other than a proof and an object that the extractor could not place, which classify reads first.
	reads: func(item string) bool {
		k, _, _ := strings.Cut(item, "/")
		return axImport.Wanted(item) && k != "proof" && k != "unknown"
	},
	brief: func(b *strings.Builder, p *Paper) {
		fmt.Fprintf(b, "Give the MSC2020 codes and the concepts of each of the %d items below with one call of tag_concepts. Give at most three codes at five characters for each item, the first one first. For each concept that the item is about, uses or defines, give a Wikidata item when you know it exactly, or else the common name of the concept. For a definition, give the terms that it defines and the symbol that it introduces.\n\n", len(p.Items))
	},
	check: checkConcepts,
}

func checkConcepts(r *Record, p *Paper, it Item, raw json.RawMessage) ([]string, string) {
	var in struct {
		MSC      []string
		Concepts []struct{ Wikidata, Name, Role string }
		Terms    []Term
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Sprintf("the result of %s: %v", it.ID, err)
	}
	var left []string
	for _, c := range in.MSC {
		c = strings.TrimSpace(c)
		switch {
		case !mscCode.MatchString(c):
			left = append(left, fmt.Sprintf("the code %q, which is not an MSC2020 code at five characters", c))
		case len(r.MSC) == 3:
			left = append(left, fmt.Sprintf("the code %s, because an item has at most three", c))
		case !slices.Contains(r.MSC, c):
			r.MSC = append(r.MSC, c)
		}
	}
	for _, c := range in.Concepts {
		name := clean(c.Name)
		switch {
		case !slices.Contains(Roles, c.Role):
			left = append(left, fmt.Sprintf("the concept %q, whose role %q is not about, uses or defines", name, c.Role))
			continue
		case name == "" || utf8.RuneCountInString(name) > maxName:
			left = append(left, fmt.Sprintf("a concept with no name or a name longer than %d characters", maxName))
			continue
		}
		x := Concept{Name: name, Role: c.Role}
		if w := strings.TrimSpace(c.Wikidata); w != "" {
			if qid.MatchString(w) {
				x.Wikidata = w
			} else {
				left = append(left, fmt.Sprintf("the Wikidata item %q of %q, which is not a QID", w, name))
			}
		}
		if x.Wikidata == "" {
			x.Flags = []string{NewConcept}
		}
		if !slices.ContainsFunc(r.Concepts, func(y Concept) bool { return y.Name == x.Name && y.Role == x.Role }) {
			r.Concepts = append(r.Concepts, x)
		}
	}
	k, _, _ := strings.Cut(it.Item, "/")
	for _, t := range in.Terms {
		t = Term{Term: clean(t.Term), Symbol: strings.TrimSpace(t.Symbol)}
		switch {
		case k != "definition":
			left = append(left, "the terms, because the item is not a definition")
		case p.Class == "record":
			left = append(left, "the terms, because math2 stores no words of a paper of the class record")
		case t.Term == "" || utf8.RuneCountInString(t.Term) > maxName || utf8.RuneCountInString(t.Symbol) > maxName:
			left = append(left, fmt.Sprintf("a term with no text or longer than %d characters", maxName))
		case len(r.Terms) == maxTerms:
			left = append(left, fmt.Sprintf("the terms after the first %d", maxTerms))
		default:
			r.Terms = append(r.Terms, t)
			continue
		}
		break
	}
	if len(r.MSC) == 0 && len(r.Concepts) == 0 {
		return nil, fmt.Sprintf("%s has no valid code and no valid concept", it.ID)
	}
	return left, ""
}
