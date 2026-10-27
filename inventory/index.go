package inventory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/align"
)

// IndexDir has the index terms of the books, one file records/index/<book>.jsonl for each book, relative to the root of the tree (corpus spec K4.7 and K5.5). The terms are records: one term and one locator on each line, not a copy of the index.
const IndexDir = "records/index"

// Term is one term of the index of a book.
type Term struct {
	Term    string `json:"term"`              // the term as the index prints it, for example "group, abelian" or "group!abelian"
	Locator string `json:"locator,omitempty"` // where the book defines the term, for example p. 41
}

// Index is the index terms of one book.
type Index struct {
	Book  string // the name of the file without .jsonl, for example rudin_pma
	Terms []Term
}

// IndexMatch is the match rate of the index terms of one book with the rows (corpus spec K4.7).
type IndexMatch struct {
	Book    string   `json:"book"`
	Terms   int      `json:"terms"`             // the terms of the index, each one time
	Matched int      `json:"matched"`           // the terms with a row
	Rate    float64  `json:"rate"`              // Matched divided by Terms
	Missing []string `json:"missing,omitempty"` // the terms with no row, in the order of the index. Each one is a candidate gap.
}

var bookName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ReadIndexes reads the index terms of each book from records/index/ in the tree at root, in the order of the books. A tree with no index gives no error. A line with no term, a field that Term does not have, and a file name that is not a source identifier are errors.
func ReadIndexes(root string) ([]Index, error) {
	var out []Index
	err := walkFiles(filepath.Join(root, IndexDir), ".jsonl", func(path string, data []byte) error {
		book := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if !bookName.MatchString(book) {
			return fmt.Errorf("%s: the name of the book must match %s", path, bookName)
		}
		terms, err := ParseIndex(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, Index{Book: book, Terms: terms})
		return nil
	})
	slices.SortFunc(out, func(a, b Index) int { return strings.Compare(a.Book, b.Book) })
	return out, err
}

// ParseIndex reads the index terms of one book, one JSON object on each line. It skips empty lines.
func ParseIndex(data []byte) ([]Term, error) {
	var out []Term
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		dec.DisallowUnknownFields()
		var t Term
		if err := dec.Decode(&t); err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		if strings.TrimSpace(t.Term) == "" {
			return nil, fmt.Errorf("line %d: the term is empty", n)
		}
		out = append(out, t)
	}
	return out, sc.Err()
}

// parenthesis matches a part of a term in parentheses, as in "order (of an element)".
var parenthesis = regexp.MustCompile(`\s*\([^()]*\)`)

// termKeys gives the keys that a term can match. The first key is of the term in the order of speech, so that "group, abelian" and "group!abelian" are "abelian group", and two terms with the same first key are one term. The next keys are of the term as the index prints it, and of each of these forms without the parts in parentheses.
func termKeys(term string) []string {
	term = strings.ReplaceAll(term, "!", ", ")
	forms := []string{term}
	if parts := strings.Split(term, ","); len(parts) > 1 {
		slices.Reverse(parts)
		forms = []string{strings.Join(parts, " "), term}
	}
	for _, f := range slices.Clone(forms) {
		forms = append(forms, parenthesis.ReplaceAllString(f, ""))
	}
	var keys []string
	for _, f := range forms {
		if k := align.Key(f); k != "" && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// MatchIndexes gives the match rate of each book with the rows, in the order of the books (corpus spec K4.7). A term matches a row when one of its keys is the key of the name of the row, or of the name or an alias of a mention of the row. A row that the clustering merged or split, or that is out of scope, is not an item, and no term matches it. Two terms of a book with the same first key of termKeys, such as "subgroup, normal" and "normal subgroups", count as one term.
func MatchIndexes(rows []Row, ms []Mention, idx []Index) []IndexMatch {
	byID := map[string]*Mention{}
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	names := map[string]bool{}
	for i := range rows {
		r := &rows[i]
		if r.Outcome != nil && slices.Contains([]string{OutcomeMerged, OutcomeSplit, OutcomeOutOfScope}, r.Outcome.Value) {
			continue
		}
		list := []string{r.Name}
		for _, id := range r.Mentions {
			if m, ok := byID[id]; ok {
				list = append(list, m.Name)
				list = append(list, m.Aliases...)
			}
		}
		for _, n := range list {
			if k := align.Key(n); k != "" {
				names[k] = true
			}
		}
	}
	var out []IndexMatch
	for _, b := range idx {
		res := IndexMatch{Book: b.Book}
		seen := map[string]bool{}
		for _, t := range b.Terms {
			keys := termKeys(t.Term)
			if len(keys) == 0 || seen[keys[0]] {
				continue
			}
			seen[keys[0]] = true
			res.Terms++
			if slices.ContainsFunc(keys, func(k string) bool { return names[k] }) {
				res.Matched++
			} else {
				res.Missing = append(res.Missing, t.Term)
			}
		}
		if res.Terms > 0 {
			res.Rate = float64(res.Matched) / float64(res.Terms)
		}
		out = append(out, res)
	}
	return out
}
