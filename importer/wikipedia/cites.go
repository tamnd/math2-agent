package wikipedia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	resresolve "github.com/tamnd/math2-agent/resource/resolve"
	"github.com/tamnd/math2-agent/wpcorpus"
)

// The places of the citation files and the citation report (corpus wikipedia WP10.2), relative to the root of the corpus.
const (
	CitationsDir    = "analysis/citations"
	CitationsReport = "reports/citations.md"
)

// The states of a citation that are not states of the resolver.
const (
	CiteNoFields = "no-fields" // a short citation or a <ref> with free text, which the front end does not read yet
)

// citeIDs maps the parameters of a citation template to the id systems of the resolver (WP10.1). jstor and bibcode have no system, so they stay in the fields of the template.
var citeIDs = []struct{ param, sys string }{
	{"doi", "doi"}, {"arxiv", "arxiv"}, {"eprint", "arxiv"}, {"mr", "mr"}, {"zbl", "zbl"}, {"jfm", "jfm"},
	{"isbn", "isbn"}, {"issn", "issn"}, {"oclc", "oclc"}, {"lccn", "lccn"}, {"s2cid", "s2"}, {"hdl", "handle"},
}

// CiteFile is the citation file of one revision of an article: the result of the resolution of each citation object.
type CiteFile struct {
	Page      int        `yaml:"page"`
	Title     string     `yaml:"title"`
	Revision  int        `yaml:"revision"`
	Resolver  string     `yaml:"resolver"`
	Citations []Citation `yaml:"citations"`
}

// Citation is the resolution of one citation object.
type Citation struct {
	Ref      string   `yaml:"ref"`
	Tag      string   `yaml:"tag,omitempty"`
	Cue      string   `yaml:"cue"`
	Section  string   `yaml:"section,omitempty"`
	Title    string   `yaml:"title,omitempty"`
	Year     int      `yaml:"year,omitempty"`
	IDs      []string `yaml:"ids,omitempty,flow"`      // the ids of the template in normal form
	BadIDs   []string `yaml:"bad_ids,omitempty,flow"`  // the ids of the template that do not have the syntax of their system or fail the check digit, as param:value
	DeadIDs  []string `yaml:"dead_ids,omitempty,flow"` // the ids that a live API does not know
	Status   string   `yaml:"status"`                  // found, candidates, not-found or no-fields
	Pass     string   `yaml:"pass,omitempty"`
	Resource string   `yaml:"resource,omitempty"`
	Score    float64  `yaml:"score,omitempty"`
	Locator  string   `yaml:"locator,omitempty"` // the place inside the work: the page, the chapter or the loc of the template
}

// CitationPath gives the path of the citation file of a page revision.
func CitationPath(page, rev int) string {
	return filepath.Join(CitationsDir, wpcorpus.Shard(page), fmt.Sprintf("%d-r%d.md", page, rev))
}

var (
	citeYear   = regexp.MustCompile(`\b(1[5-9][0-9]{2}|20[0-9]{2})\b`)
	citeNumber = regexp.MustCompile(`^\s*([0-9]+)`)
	citeOrd    = map[string]int{"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5, "sixth": 6, "seventh": 7, "eighth": 8, "ninth": 9, "tenth": 10}
)

// CitationOf gives the citation of the resolver for the fields of a citation template, and the ids that do not normalize, as param:value.
func CitationOf(f map[string]string) (resresolve.Citation, []string) {
	var c resresolve.Citation
	var bad []string
	for _, p := range citeIDs {
		v := strings.TrimSpace(f[p.param])
		if v == "" {
			continue
		}
		// An id in double parentheses is accepted as written: the editor turned off the check of the template on purpose, so it is not a bad id.
		inner, asWritten := strings.CutPrefix(v, "((")
		if asWritten {
			v = strings.TrimSpace(strings.TrimSuffix(inner, "))"))
		}
		k, err := resresolve.Key(p.sys + ":" + v)
		if err != nil {
			if !asWritten {
				bad = append(bad, p.param+":"+v)
			}
			continue
		}
		if !slices.Contains(c.IDs, k) {
			c.IDs = append(c.IDs, k)
		}
	}
	c.Title = strings.TrimSpace(f["title"])
	if c.Title == "" {
		c.Title = strings.TrimSpace(f["chapter"])
	}
	for i := 0; i <= 9; i++ {
		n := ""
		if i > 0 {
			n = strconv.Itoa(i)
		}
		name := strings.TrimSpace(firstOf(f["last"+n], f["author"+n], f["surname"+n]))
		if name == "" {
			if i > 1 {
				break
			}
			continue
		}
		if first := strings.TrimSpace(firstOf(f["first"+n], f["given"+n])); first != "" {
			name = first + " " + name
		}
		c.Authors = append(c.Authors, name)
	}
	if m := citeYear.FindString(firstOf(f["year"], f["date"], f["orig-date"])); m != "" {
		c.Year, _ = strconv.Atoi(m)
	}
	c.Venue = strings.TrimSpace(firstOf(f["journal"], f["work"], f["publisher"]))
	c.Volume = strings.TrimSpace(f["volume"])
	c.Pages = strings.TrimSpace(f["pages"])
	ed := strings.ToLower(strings.TrimSpace(f["edition"]))
	if m := citeNumber.FindStringSubmatch(ed); m != nil {
		c.Edition, _ = strconv.Atoi(m[1])
	} else if w, _, _ := strings.Cut(ed, " "); citeOrd[w] > 0 {
		c.Edition = citeOrd[w]
	}
	parts := []string{strings.Join(c.Authors, ", "), c.Title, c.Venue}
	if c.Year > 0 {
		parts = append(parts, strconv.Itoa(c.Year))
	}
	parts = append(parts, c.IDs...)
	c.Text = strings.Join(slices.DeleteFunc(parts, func(s string) bool { return s == "" }), ", ")
	return c, bad
}

func firstOf(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

// citeLocator gives the place inside the work that the fields name.
func citeLocator(f map[string]string) string {
	for _, k := range []string{"page", "pages", "loc", "at", "chapter"} {
		if v := strings.TrimSpace(f[k]); v != "" {
			return k + " " + v
		}
	}
	return ""
}

// ResolveCitations resolves each citation object of a page revision. dead reports whether a live API does not know an id, or is nil when the resolver does not use the network.
func ResolveCitations(ctx context.Context, r *resresolve.Resolver, page, rev int, title string, objs []wpcorpus.Object, dead func(id string) bool) (*CiteFile, error) {
	out := &CiteFile{Page: page, Title: title, Revision: rev, Resolver: resresolve.Version, Citations: []Citation{}}
	for _, o := range objs {
		if o.Kind != "citation" {
			continue
		}
		ci := Citation{Ref: o.Ref, Tag: o.Tag, Cue: o.Cue, Section: o.Section, Status: CiteNoFields}
		if len(o.Fields) == 0 {
			out.Citations = append(out.Citations, ci)
			continue
		}
		c, bad := CitationOf(o.Fields)
		ci.Title, ci.Year, ci.IDs, ci.BadIDs, ci.Locator = c.Title, c.Year, c.IDs, bad, citeLocator(o.Fields)
		if len(c.IDs) == 0 && c.Title == "" {
			out.Citations = append(out.Citations, ci)
			continue
		}
		res, err := r.Cite(ctx, c)
		if err != nil {
			return nil, err
		}
		ci.Status, ci.Pass, ci.Score = res.Status, res.Pass, res.Score
		if res.Ref != nil {
			ci.Resource = res.Ref.Resource
		}
		if res.Status == resresolve.NotFound && dead != nil {
			for _, id := range c.IDs {
				if dead(id) {
					ci.DeadIDs = append(ci.DeadIDs, id)
				}
			}
		}
		out.Citations = append(out.Citations, ci)
	}
	return out, nil
}

// MarshalCiteFile gives the citation file: the YAML header with the citations, and a summary for people.
func MarshalCiteFile(f *CiteFile) ([]byte, error) {
	var y bytes.Buffer
	enc := yaml.NewEncoder(&y)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	count := map[string]int{}
	for _, c := range f.Citations {
		count[c.Status]++
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(y.Bytes())
	fmt.Fprintf(&b, "---\n\n# Citations of %s\n\nRevision %d has %d citations. %d resolve to a resource, %d have candidates, %d do not resolve, and %d have no fields to resolve.\n", f.Title, f.Revision, len(f.Citations), count[resresolve.Found], count[resresolve.Candidates], count[resresolve.NotFound], count[CiteNoFields])
	return b.Bytes(), nil
}

// ReadCiteFile reads the citation file of a page revision, or gives nil when it does not exist.
func ReadCiteFile(corpus string, page, rev int) (*CiteFile, error) {
	data, err := os.ReadFile(filepath.Join(corpus, CitationPath(page, rev))) // #nosec G304 -- the path is in the corpus that the user names.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ParseCiteFile(data)
}

// ParseCiteFile parses a citation file.
func ParseCiteFile(data []byte) (*CiteFile, error) {
	rest, ok := bytes.CutPrefix(data, []byte("---\n"))
	head, _, ok2 := bytes.Cut(rest, []byte("\n---\n"))
	if !ok || !ok2 {
		return nil, errors.New("the citation file has no YAML header")
	}
	var f CiteFile
	if err := yaml.Unmarshal(head, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
