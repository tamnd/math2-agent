// Package arxiv is the ingest adapter of arXiv (spec 09.2). It reads the metadata plane of tamnd/arxiv, which has one JSONL file for each month of arXiv, and gives one record for each mathematics paper.
//
// A paper is a mathematics paper when one of its categories is in the math archive or is math-ph. The metadata of arXiv is under CC0. The licence of each record is the licence of the last version of the paper that has one. A paper under the default licence of arXiv, or with no licence in the metadata plane yet, has the class record: math2 keeps its metadata and its locator, and no text. The records keep no text in any case, because the text of a paper stays in the content plane of tamnd/arxiv. The locator of a paper is its ax:// reference.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/arxiv"
package arxiv

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
)

// Contents is the GitHub contents API of the metadata plane of tamnd/arxiv. Add ?ref=<commit> to pin a commit.
const Contents = "https://api.github.com/repos/tamnd/arxiv/contents/metadata"

// Adapter is the adapter of arXiv.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of arXiv.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "arxiv",
		Name:        "arXiv, through the metadata plane of tamnd/arxiv",
		Home:        "https://arxiv.org",
		Licence:     "CC0-1.0",
		Class:       ingest.Open,
		Attribution: "arXiv metadata, Cornell University, https://arxiv.org, CC0 1.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "arxiv/1" }

// Dump returns the URL of the metadata plane on the main branch.
func (Adapter) Dump() string { return Contents + "?ref=main" }

// Fetch gets the metadata plane. When req.URL names one .jsonl file, Fetch gets that file. In other cases req.URL is a GitHub contents API listing, and Fetch gets each .jsonl file of the listing in the order of the names. The snapshot is the files one after the other, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	u, get := req.URL, req.Get
	files := []string{u}
	if path, _, _ := strings.Cut(u, "?"); !strings.HasSuffix(path, ".jsonl") {
		var err error
		if files, err = listing(ctx, get, u); err != nil {
			return err
		}
	}
	zw := gzip.NewWriter(w)
	for _, f := range files {
		if err := copyFile(ctx, get, f, zw); err != nil {
			return err
		}
	}
	return zw.Close()
}

// entry is one entry of a GitHub contents API listing.
type entry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Download string `json:"download_url"`
}

// listing returns the download URLs of the .jsonl files of a GitHub contents API listing, in the order of the names.
func listing(ctx context.Context, get ingest.Getter, u string) ([]string, error) {
	body, err := get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	var entries []entry
	if err := json.NewDecoder(body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("%s: %w", u, err)
	}
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.Name, b.Name) })
	var out []string
	for _, e := range entries {
		if e.Type == "file" && strings.HasSuffix(e.Name, ".jsonl") && e.Download != "" {
			out = append(out, e.Download)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s lists no .jsonl files", u)
	}
	return out, nil
}

// copyFile gets one file and writes it to w, with a newline at the end.
func copyFile(ctx context.Context, get ingest.Getter, u string, w io.Writer) error {
	body, err := get(ctx, u)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("%s: %w", u, err)
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	_, err = w.Write(data)
	return err
}

// paper is one line of the metadata plane.
type paper struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Authors []struct {
		Surname  string `json:"surname"`
		Forename string `json:"forename"`
		Suffix   string `json:"suffix"`
	} `json:"authors"`
	Categories []string `json:"categories"`
	Versions   []struct {
		Version int    `json:"version"`
		Licence string `json:"licence"`
	} `json:"versions"`
	DOI        string `json:"doi"`
	JournalRef string `json:"journal_ref"`
	MSCClass   string `json:"msc_class"`
}

// licences maps the licences of tamnd/arxiv to SPDX ids.
var licences = map[string]string{
	"cc0":         "CC0-1.0",
	"cc-by":       "CC-BY-4.0",
	"cc-by-sa":    "CC-BY-SA-4.0",
	"cc-by-nc-sa": "CC-BY-NC-SA-4.0",
	"cc-by-nc-nd": "CC-BY-NC-ND-4.0",
	"arxiv-1.0":   "LicenseRef-arXiv",
}

// Licence returns the SPDX id of a licence of tamnd/arxiv. An empty licence, unknown, and each licence that tamnd/arxiv does not name give LicenseRef-arXiv, the default licence of arXiv.
func Licence(l string) string {
	if s, ok := licences[l]; ok {
		return s
	}
	return "LicenseRef-arXiv"
}

// mscCode matches an MSC code: 11N05, 11Nxx or 11-XX, and 11-02.
var mscCode = regexp.MustCompile(`\b\d\d(?:[A-Z]\d\d|[A-Z]xx|-XX|-\d\d)\b`)

// MSC returns the MSC codes in the free text of the msc-class field of arXiv, in the order of the text and without repeats. For example, "11N05, 11N36 (Primary) 11P32" gives 11N05, 11N36 and 11P32.
func MSC(text string) []string {
	var out []string
	for _, c := range mscCode.FindAllString(text, -1) {
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// Math reports whether a paper with the categories is a mathematics paper.
func Math(categories []string) bool {
	return slices.ContainsFunc(categories, func(c string) bool {
		return strings.HasPrefix(c, "math.") || c == "math" || c == "math-ph"
	})
}

// Read reads the metadata plane and emits one record for each mathematics paper.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var p paper
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if !Math(p.Categories) {
			continue
		}
		if err := emit(item(p)); err != nil {
			return err
		}
	}
	return sc.Err()
}

// item makes the ingest record of one paper.
func item(p paper) ingest.Record {
	licence, version := "", 0
	for _, v := range p.Versions {
		if v.Licence != "" && v.Version >= version {
			licence, version = v.Licence, v.Version
		}
	}
	var authors []string
	for _, a := range p.Authors {
		if s := strings.Join(strings.Fields(a.Forename+" "+a.Surname+" "+a.Suffix), " "); s != "" {
			authors = append(authors, s)
		}
	}
	meta := map[string]string{}
	set := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			meta[k] = v
		}
	}
	set("categories", strings.Join(p.Categories, " "))
	set("msc", strings.Join(MSC(p.MSCClass), " "))
	set("msc_class", p.MSCClass)
	set("doi", p.DOI)
	set("journal", p.JournalRef)
	set("authors", strings.Join(authors, "; "))
	if version > 0 {
		set("licence_version", fmt.Sprintf("v%d", version))
	}
	return ingest.Record{
		ID:      p.ID,
		Kind:    "paper",
		Title:   strings.Join(strings.Fields(p.Title), " "),
		URL:     "https://arxiv.org/abs/" + p.ID,
		Locator: "ax://paper/" + p.ID,
		Licence: Licence(licence),
		Meta:    meta,
	}
}
