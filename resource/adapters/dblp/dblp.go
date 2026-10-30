// Package dblp is the adapter of dblp, the computer science bibliography (spec corpus resources RS6.1 and RS8.3, phase R3). It reads the publications of the XML dump of dblp in theoretical computer science, discrete mathematics and logic, and it gives an ingest record and a source record for each publication.
//
// Fetch reads the XML dump dblp.xml.gz as a stream, from a local file or from a URL. The dump is in ISO-8859-1 and uses the entities of its DTD, which are the entities of HTML. Fetch keeps the articles, the papers in proceedings, the proceedings, the books, the chapters and the doctoral theses whose stream is in Venues, or whose DOI, ISBN or arXiv id is an id of a resource in the tree. It writes one JSON object for each publication and each line, compressed with gzip. The records of dblp are under CC0, and the adapter keeps the class record only. The adapter sends no email and no key.
//
// Give the path of the local dump with the flag -url of m2 ingest, as m2 ingest -url /data/dblp.xml.gz dblp.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/dblp"
package dblp

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/encoding/charmap"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Dump is the URL of the XML dump of dblp.
const Dump = "https://dblp.org/xml/dblp.xml.gz"

// Kinds maps the elements of the publications of dblp to the kinds of resources. Fetch skips the other elements, as www, which holds the persons.
var Kinds = map[string]string{
	"article": "article", "inproceedings": "conference-paper", "proceedings": "proceedings", "book": "monograph", "incollection": "chapter", "phdthesis": "thesis",
}

// Venues are the streams of dblp that Fetch keeps: the journals and the conferences of theoretical computer science, discrete mathematics and logic. A stream is the first two parts of the key of a publication, as journals/tcs in journals/tcs/Smith01.
var Venues = []string{
	"journals/tcs", "journals/jacm", "journals/siamcomp", "journals/algorithmica", "journals/talg", "journals/toct", "journals/toc", "journals/cc", "journals/jcss",
	"journals/iandc", "journals/mst", "journals/ijfcs", "journals/fuin", "journals/mscs", "journals/jsc", "journals/tit", "journals/dm", "journals/dam",
	"journals/combinatorica", "journals/ejc", "journals/jct", "journals/gc", "journals/combinatorics", "journals/dmtcs", "journals/siamdm", "journals/jgt",
	"journals/rsa", "journals/cpc", "journals/dcg", "journals/comgeo", "journals/lmcs", "journals/tocl", "journals/jsyml", "journals/apal", "journals/mlq",
	"journals/sLogica", "journals/jar", "conf/stoc", "conf/focs", "conf/soda", "conf/icalp", "conf/stacs", "conf/mfcs", "conf/coco", "conf/lics", "conf/csl",
	"conf/fsttcs", "conf/isaac", "conf/esa", "conf/approx", "conf/compgeom", "conf/cade", "conf/itp", "conf/types", "conf/rta", "conf/fscd",
}

// Keys are the id systems that join a publication to a resource in the tree.
var Keys = []string{"doi", "isbn", "arxiv"}

// Adapter is the adapter of dblp.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of dblp.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "dblp",
		Name:        "dblp, the computer science bibliography",
		Home:        "https://dblp.org",
		Licence:     "CC0-1.0",
		Class:       ingest.RecordOnly,
		Attribution: "dblp computer science bibliography, https://dblp.org, CC0 1.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "dblp/1" }

// Dump returns the URL of the XML dump.
func (Adapter) Dump() string { return Dump }

// Publication is one publication, as one line of the snapshot.
type Publication struct {
	Key       string   `json:"key"`
	Element   string   `json:"element"`            // article, inproceedings and the other elements of Kinds
	Type      string   `json:"publtype,omitempty"` // as informal or survey
	Title     string   `json:"title"`
	Authors   []string `json:"authors,omitempty"`
	Editors   []string `json:"editors,omitempty"`
	Year      string   `json:"year,omitempty"`
	Venue     string   `json:"venue,omitempty"` // the journal or the book title
	Volume    string   `json:"volume,omitempty"`
	Number    string   `json:"number,omitempty"`
	Pages     string   `json:"pages,omitempty"`
	Publisher string   `json:"publisher,omitempty"`
	School    string   `json:"school,omitempty"`
	DOI       []string `json:"doi,omitempty"`
	ArXiv     []string `json:"arxiv,omitempty"`
	ISBN      []string `json:"isbn,omitempty"`
	Crossref  string   `json:"crossref,omitempty"` // the key of the proceedings or the book that holds the publication
}

type publ struct {
	Key     string   `xml:"key,attr"`
	Type    string   `xml:"publtype,attr"`
	Authors []string `xml:"author"`
	Editors []string `xml:"editor"`
	Title   struct {
		Inner string `xml:",innerxml"` // the title can hold markup, as <i> or <sub>
	} `xml:"title"`
	Year      string   `xml:"year"`
	Journal   string   `xml:"journal"`
	Booktitle string   `xml:"booktitle"`
	Volume    string   `xml:"volume"`
	Number    string   `xml:"number"`
	Pages     string   `xml:"pages"`
	Publisher string   `xml:"publisher"`
	School    string   `xml:"school"`
	EE        []string `xml:"ee"`
	ISBN      []string `xml:"isbn"`
	Crossref  string   `xml:"crossref"`
}

var (
	tag     = regexp.MustCompile(`<[^>]*>`)
	homonym = regexp.MustCompile(`\s+[0-9]{4}$`) // the number that dblp adds to a name that two persons share, as 0001 in John Smith 0001
	doi     = regexp.MustCompile(`(?i)doi\.org/(10\.[0-9]{4,9}/\S+)$`)
	arxiv   = regexp.MustCompile(`arxiv\.org/abs/(\S+)$`)
)

// Fetch reads the dump at req.URL, a local path or an HTTP URL, and writes the publications that Keep keeps as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	want, err := Wanted(req.Root)
	if err != nil {
		return err
	}
	var body io.ReadCloser
	if strings.HasPrefix(req.URL, "https://") || strings.HasPrefix(req.URL, "http://") {
		body, err = req.Get(ctx, req.URL)
	} else {
		body, err = os.Open(req.URL) // #nosec G304 -- the user names the dump file.
	}
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	err = Each(ctx, body, func(p Publication) error {
		if !Keep(p, want) {
			return nil
		}
		return enc.Encode(p)
	})
	if err != nil {
		return fmt.Errorf("dblp: %s: %w", req.URL, err)
	}
	return zw.Close()
}

// Wanted gives the ids of the live resources in the tree at root, in the systems of Keys. An empty root gives no ids.
func Wanted(root string) (map[string]bool, error) {
	want := map[string]bool{}
	if root == "" {
		return want, nil
	}
	rs, err := resource.ReadResources(root)
	if err != nil {
		return nil, err
	}
	add := func(m resource.IDs) {
		for _, k := range Keys {
			for _, v := range m[k] {
				want[k+":"+v] = true
			}
		}
	}
	for i := range rs {
		if !rs[i].Live() {
			continue
		}
		add(rs[i].IDs)
		for _, e := range rs[i].Expressions {
			add(e.IDs)
			for _, m := range e.Manifestations {
				add(m.IDs)
			}
		}
	}
	return want, nil
}

// Stream gives the stream of a key, as journals/tcs in journals/tcs/Smith01.
func Stream(key string) string {
	parts := strings.SplitN(key, "/", 3)
	if len(parts) < 2 {
		return key
	}
	return parts[0] + "/" + parts[1]
}

// Keep reports whether Fetch keeps a publication: its stream is in Venues, or one of its ids is in want.
func Keep(p Publication, want map[string]bool) bool {
	if slices.Contains(Venues, Stream(p.Key)) {
		return true
	}
	match := func(key string, vs []string) bool {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil && want[key+":"+n] {
				return true
			}
		}
		return false
	}
	return match("doi", p.DOI) || match("isbn", p.ISBN) || match("arxiv", p.ArXiv)
}

// Each reads the dump, plain or compressed with gzip, and calls fn for each publication of an element of Kinds.
func Each(ctx context.Context, r io.Reader, fn func(Publication) error) error {
	br := bufio.NewReaderSize(r, 1<<20)
	var in io.Reader = br
	if magic, err := br.Peek(2); err == nil && bytes.Equal(magic, []byte{0x1f, 0x8b}) {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return err
		}
		defer func() { _ = zr.Close() }()
		in = zr
	}
	dec := xml.NewDecoder(in)
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	dec.CharsetReader = func(label string, r io.Reader) (io.Reader, error) {
		if strings.EqualFold(label, "ISO-8859-1") || strings.EqualFold(label, "latin1") {
			return charmap.ISO8859_1.NewDecoder().Reader(r), nil
		}
		return nil, fmt.Errorf("the charset %s is not known", label)
	}
	for count := 0; ; count++ {
		if count%100000 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if _, ok := Kinds[se.Name.Local]; !ok {
			continue
		}
		var x publ
		if err := dec.DecodeElement(&x, &se); err != nil {
			return err
		}
		if x.Key == "" {
			continue
		}
		if err := fn(publication(se.Name.Local, x)); err != nil {
			return err
		}
	}
}

// text gives the text of a part of the XML, with no tags and no entities.
func text(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tag.ReplaceAllString(s, ""))), " ")
}

func names(vs []string) []string {
	var out []string
	for _, v := range vs {
		if v = homonym.ReplaceAllString(strings.Join(strings.Fields(v), " "), ""); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func publication(element string, x publ) Publication {
	p := Publication{Key: x.Key, Element: element, Type: x.Type, Title: strings.TrimSuffix(text(x.Title.Inner), "."), Authors: names(x.Authors), Editors: names(x.Editors),
		Year: strings.TrimSpace(x.Year), Venue: strings.TrimSpace(x.Journal), Volume: strings.TrimSpace(x.Volume), Number: strings.TrimSpace(x.Number),
		Pages: strings.TrimSpace(x.Pages), Publisher: strings.TrimSpace(x.Publisher), School: strings.TrimSpace(x.School), Crossref: strings.TrimSpace(x.Crossref)}
	if p.Venue == "" {
		p.Venue = strings.TrimSpace(x.Booktitle)
	}
	for _, ee := range x.EE {
		ee = strings.TrimSpace(ee)
		if m := doi.FindStringSubmatch(ee); m != nil && !slices.Contains(p.DOI, m[1]) {
			p.DOI = append(p.DOI, m[1])
		}
		if m := arxiv.FindStringSubmatch(ee); m != nil && !slices.Contains(p.ArXiv, m[1]) {
			p.ArXiv = append(p.ArXiv, m[1])
		}
	}
	for _, v := range x.ISBN {
		if v = strings.TrimSpace(v); v != "" && !slices.Contains(p.ISBN, v) {
			p.ISBN = append(p.ISBN, v)
		}
	}
	return p
}

func each(r io.Reader, fn func(Publication) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var p Publication
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if p.Key == "" {
			return fmt.Errorf("line %d: the publication has no key", line)
		}
		if err := fn(p); err != nil {
			return err
		}
	}
	return sc.Err()
}

func link(p Publication) string { return "https://dblp.org/rec/" + p.Key }

// Kind gives the kind of a publication. An article of the stream journals/corr, or an article with the type informal, is a preprint. A book with editors and no authors is an edited volume.
func Kind(p Publication) string {
	k := Kinds[p.Element]
	switch {
	case k == "article" && (Stream(p.Key) == "journals/corr" || p.Type == "informal"):
		return "preprint"
	case k == "monograph" && len(p.Authors) == 0 && len(p.Editors) > 0:
		return "edited-volume"
	}
	return k
}

// Read reads the publications and emits one ingest record for each publication.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(p Publication) error {
		return emit(ingest.Record{ID: p.Key, Kind: Kind(p), Title: p.Title, URL: link(p)})
	})
}

// Records reads the publications and emits one source record for each publication (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(p Publication) error { return emit(SourceRecord(p)) })
}

// SourceRecord makes the source record of one publication. A book or a proceedings volume is one edition with a publisher and ISBNs, so its level is manifestation. The level of the other publications is work.
func SourceRecord(p Publication) cluster.Record {
	rec := cluster.Record{NativeID: p.Key, Level: "work", Kind: Kind(p), Title: p.Title, Date: p.Year, Volume: p.Volume, IDs: resource.IDs{}, Links: []string{link(p)}}
	if p.Element == "book" || p.Element == "proceedings" {
		rec.Level, rec.Publisher = "manifestation", p.Publisher
	}
	if first, _, _ := strings.Cut(p.Pages, "-"); first != "" {
		rec.Page = strings.TrimSpace(first)
	}
	put := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil && !slices.Contains(rec.IDs[key], n) {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	put("doi", p.DOI...)
	put("arxiv", p.ArXiv...)
	put("isbn", p.ISBN...)
	for _, name := range p.Authors {
		rec.Creators = append(rec.Creators, resource.Creator{Name: name, Role: "author"})
	}
	for _, name := range p.Editors {
		rec.Creators = append(rec.Creators, resource.Creator{Name: name, Role: "editor"})
	}
	return rec
}
