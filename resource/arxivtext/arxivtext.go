// Package arxivtext builds the source records of the language of the arXiv papers (spec corpus resources RS2.3 and RS8.3). arXiv gives no language for a paper, so a preprint of the corpus has no expression with a language and stays below depth D1.
//
// A run reads the objects file of each paper in a local checkout of tamnd/arxiv (analysis/objects/<YYMM>/<id>.md), removes the formulas from the text of the objects, and counts the function words of each language of Words. The language with the most function words is the language of the paper when it has at least MinWords of them and Margin times the count of the next language. A paper with too little text has no record. Each record has the arXiv id of the paper, so the clustering puts the record on that resource. The source is a filler of package cluster: it gives the language only when no catalogue gives one.
package arxivtext

import (
	"cmp"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// Source is the id of the source in manifests/sources.yaml.
const Source = "arxiv_text"

// Version is the version of the adapter.
const Version = "arxiv_text/1"

// MinWords is the fewest function words of the language of a paper.
const MinWords = 20

// Margin is how many times the count of the language of a paper must be the count of the next language.
const Margin = 3

// Words are the function words of each language, as BCP 47 tags. A word of one letter is not in the lists, because a letter is often a variable of a formula.
var Words = map[string][]string{
	"en": strings.Fields("the of and is that we for with this let then be are by on it which there"),
	"fr": strings.Fields("les des est que une pour dans sur nous avec soit alors sont qui"),
	"de": strings.Fields("der die das und ist eine mit für von wir sei dann den nicht auf sind"),
	"es": strings.Fields("los las que una para con por sea entonces del como son"),
	"it": strings.Fields("il che una per con sia allora della sono come"),
	"pt": strings.Fields("os que uma para com seja então não são como"),
	"ru": strings.Fields("и в не что на это с по для пусть если то как из"),
}

var (
	formula = regexp.MustCompile(`(?s)\\\[.*?\\\]|\\\(.*?\\\)|\$\$.*?\$\$|\$[^$]*\$`)
	word    = regexp.MustCompile(`\pL+`)
)

// Detect gives the language of the text, or "" when the text has too few function words or no language wins by Margin.
func Detect(text string) string {
	counts := map[string]int{}
	index := map[string][]string{}
	for l, ws := range Words {
		for _, w := range ws {
			index[w] = append(index[w], l)
		}
	}
	for _, w := range word.FindAllString(strings.ToLower(formula.ReplaceAllString(text, " ")), -1) {
		for _, l := range index[w] {
			counts[l]++
		}
	}
	langs := make([]string, 0, len(Words))
	for l := range Words {
		langs = append(langs, l)
	}
	slices.SortFunc(langs, func(a, b string) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b)) })
	best, next := langs[0], counts[langs[1]]
	if counts[best] < MinWords || counts[best] < Margin*next {
		return ""
	}
	return best
}

// Counts are the counts of one run.
type Counts struct {
	Asked int // the resources that the run read the text of
	Found int // the resources with a language
}

// needs reports whether the resource is live, in scope, has an arXiv id and has no expression with a language. A resource that a record of the source is on also needs it, because the records of a run replace all the records of the last run.
func needs(r *resource.Resource) bool {
	if !r.Live() || r.Scope == nil || r.Scope.Value != resource.In {
		return false
	}
	return arxivID(r) != "" && (r.Holds(Source) || !slices.ContainsFunc(r.Expressions, func(e resource.Expression) bool { return e.Language != "" }))
}

// arxivID gives the first arXiv id of the resource or of one of its expressions, or "".
func arxivID(r *resource.Resource) string {
	if v := r.IDs["arxiv"]; len(v) > 0 {
		return v[0]
	}
	for _, e := range r.Expressions {
		if v := e.IDs["arxiv"]; len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// Build reads the objects file of each resource that needs a language in the checkout at root, and gives one record for each paper with a language. A paper with no objects file has no record.
func Build(root string, rs []resource.Resource) ([]cluster.Record, Counts, error) {
	var c Counts
	var out []cluster.Record
	seen := map[string]bool{}
	for i := range rs {
		r := &rs[i]
		id := arxivID(r)
		if !needs(r) || seen[id] {
			continue
		}
		seen[id] = true
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(arxiv.ObjectsFile(id)))) // #nosec G304 -- the path is the objects file of an arXiv id in the checkout that the user gives.
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, c, err
		}
		c.Asked++
		l := Detect(body(string(b)))
		if l == "" {
			continue
		}
		c.Found++
		out = append(out, cluster.Record{Source: Source, NativeID: id, Level: "work", Kind: r.Kind, Title: r.Title, Language: l, IDs: resource.IDs{"arxiv": {id}}})
	}
	return out, c, nil
}

// body gives the text of an objects file after its header of records.
func body(s string) string {
	if !strings.HasPrefix(s, "---\n") {
		return s
	}
	if _, rest, ok := strings.Cut(s[4:], "\n---\n"); ok {
		return rest
	}
	return ""
}

// Entry gives the entry of the source in manifests/sources.yaml.
func Entry() ingest.Source {
	return ingest.Source{
		ID:          Source,
		Name:        "The language of the arXiv papers, from the text of the objects in tamnd/arxiv",
		Kind:        "catalogue",
		Home:        "https://github.com/tamnd/arxiv",
		Licence:     "LicenseRef-facts",
		Class:       ingest.RecordOnly,
		Channel:     "git",
		Adapter:     Version,
		Attribution: "The language that m2 detects in the text of the analysis plane of tamnd/arxiv",
	}
}

// Snapshot gives the snapshot of the records of one run: the hash of the records and the day.
func Snapshot(recs []cluster.Record, day string) (ingest.Snapshot, error) {
	h := blake3.New()
	enc := json.NewEncoder(h)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			return ingest.Snapshot{}, err
		}
	}
	return ingest.Snapshot{URL: "m2 res import " + Source, Hash: "blake3:" + hex.EncodeToString(h.Sum(nil)), Date: day}, nil
}
