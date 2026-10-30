package fetch

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/charmap"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census"
)

// openDump opens a local dump file, and reads it through gzip when its name ends in .gz.
func openDump(path string) (io.Reader, func() error, error) {
	f, err := os.Open(path) // #nosec G304 -- the user names the dump file.
	if err != nil {
		return nil, nil, err
	}
	if !strings.HasSuffix(path, ".gz") {
		return bufio.NewReaderSize(f, 1<<20), f.Close, nil
	}
	z, err := gzip.NewReader(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return z, func() error { _ = z.Close(); return f.Close() }, nil
}

// reservoir keeps a uniform random part of k items of a stream, by the algorithm R.
type reservoir struct {
	k    int
	seen int
	keep []census.Record
	rng  *rand.Rand
}

func newReservoir(k int, seed uint64) *reservoir {
	return &reservoir{k: k, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))} // #nosec G404 -- a sample needs a seeded generator, not a secure one.
}

// add offers one item. The function mk makes the item only when the reservoir keeps it.
func (r *reservoir) add(mk func() census.Record) {
	r.seen++
	if len(r.keep) < r.k {
		r.keep = append(r.keep, mk())
		return
	}
	if j := r.rng.IntN(r.seen); j < r.k {
		r.keep[j] = mk()
	}
}

// emit emits the kept items with the weight seen / kept.
func (r *reservoir) emit(emit func(census.Record) error) error {
	w := weight(r.seen, len(r.keep))
	for _, x := range r.keep {
		x.Weight = w
		if err := emit(x); err != nil {
			return err
		}
	}
	return nil
}

// DBLP reads the XML dump of dblp (dblp.xml or dblp.xml.gz) in a local file and keeps a random part of its publications. The search API of dblp sends a page that tests for a bot, so the sample reads the dump, as RS8.3 says. The records of dblp have no subject, so they have no signal of the scope tests.
type DBLP struct{ File string }

// Name gives dblp.
func (DBLP) Name() string { return "dblp" }

// dblpKinds maps the elements of the publications of dblp to the kinds of resources. The element www holds the persons, and the sample skips it.
var dblpKinds = map[string]string{
	"article": "article", "inproceedings": "conference-paper", "proceedings": "proceedings", "book": "monograph",
	"incollection": "chapter", "phdthesis": "thesis", "mastersthesis": "thesis", "data": "dataset",
}

type dblpPubl struct {
	Key     string   `xml:"key,attr"`
	Type    string   `xml:"publtype,attr"`
	Authors []string `xml:"author"`
	Editors []string `xml:"editor"`
	Title   struct {
		Inner string `xml:",innerxml"` // the title can hold markup, as <i> or <sub>
	} `xml:"title"`
	Year string   `xml:"year"`
	EE   []string `xml:"ee"`
	ISBN []string `xml:"isbn"`
}

// Sample reads each publication of the dump and keeps about n of them.
func (d DBLP) Sample(ctx context.Context, _ *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	if d.File == "" {
		return Info{}, errors.New("dblp: give the file of the XML dump")
	}
	in, closeFn, err := openDump(d.File)
	if err != nil {
		return Info{}, wrap("dblp", err)
	}
	defer func() { _ = closeFn() }()
	info := Info{Filter: "each publication of the XML dump"}
	res := newReservoir(n, seed)
	if err := dblpRead(ctx, in, res); err != nil {
		return info, wrap("dblp", err)
	}
	info.Total = res.seen
	return info, res.emit(emit)
}

// dblpRead decodes the dump. The dump is in ISO-8859-1 and uses the entities of its DTD, which are the entities of HTML.
func dblpRead(ctx context.Context, in io.Reader, res *reservoir) error {
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
		kind, ok := dblpKinds[se.Name.Local]
		if !ok {
			continue
		}
		var p dblpPubl
		if err := dec.DecodeElement(&p, &se); err != nil {
			return err
		}
		res.add(func() census.Record { return dblpRecord(p, kind) })
	}
}

func dblpRecord(p dblpPubl, kind string) census.Record {
	r := census.Record{NativeID: p.Key, Kind: kind, Date: strings.TrimSpace(p.Year), Title: mnText(p.Title.Inner), IDs: resource.IDs{}}
	r.Title = strings.TrimSuffix(r.Title, ".")
	r.Authors = p.Authors
	if len(r.Authors) == 0 {
		r.Authors = p.Editors
	}
	if strings.HasPrefix(p.Key, "journals/corr/") {
		r.Kind = "preprint"
	}
	if p.Type == "informal" && r.Kind == "article" {
		r.Kind = "preprint"
	}
	for _, ee := range p.EE {
		ee = strings.TrimSpace(ee)
		if x := dcDOI.FindStringSubmatch(ee); x != nil {
			r.IDs["doi"] = addUnique(r.IDs["doi"], strings.ToLower(x[1]))
		}
		if id, ok := strings.CutPrefix(ee, "https://arxiv.org/abs/"); ok {
			r.IDs["arxiv"] = addUnique(r.IDs["arxiv"], id)
		}
	}
	if len(p.ISBN) > 0 {
		r.IDs["isbn"] = p.ISBN[:min(len(p.ISBN), 5)]
	}
	return r
}

// Hathi reads a Hathifile (hathi_full_YYYYMMDD.txt.gz) in a local file and keeps a random part of its catalogue records. A Hathifile has one line for each volume, so the sample keeps the first volume of each catalogue record. The Hathifiles have no subject and no class, so the records have no signal of the scope tests. The ids LCCN, OCLC and ISBN let the census match them to the other catalogues.
type Hathi struct{ File string }

// Name gives hathitrust.
func (Hathi) Name() string { return "hathitrust" }

// The columns of a Hathifile, from 0.
const (
	hfHTID    = 0
	hfBibKey  = 3
	hfOCLC    = 7
	hfISBN    = 8
	hfLCCN    = 10
	hfTitle   = 11
	hfDate    = 16
	hfLang    = 18
	hfFormat  = 19
	hfAuthor  = 25
	hfColumns = 26
)

var hfKinds = map[string]string{"BK": "monograph", "SE": "journal"}

// bitset is a set of small numbers.
type bitset []uint64

// add adds k and reports whether the set did not have it.
func (b *bitset) add(k int) bool {
	i := k / 64
	if i >= len(*b) {
		*b = append(*b, make([]uint64, i-len(*b)+1+len(*b)/2)...)
	}
	m := uint64(1) << (k % 64)
	if (*b)[i]&m != 0 {
		return false
	}
	(*b)[i] |= m
	return true
}

// Sample reads each line of the file and keeps about n catalogue records.
func (h Hathi) Sample(ctx context.Context, _ *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	if h.File == "" {
		return Info{}, errors.New("hathitrust: give the Hathifile")
	}
	in, closeFn, err := openDump(h.File)
	if err != nil {
		return Info{}, wrap("hathitrust", err)
	}
	defer func() { _ = closeFn() }()
	info := Info{Filter: "each catalogue record of the Hathifile, with no subject filter"}
	res := newReservoir(n, seed)
	var seen bitset
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for line := 0; sc.Scan(); line++ {
		if line%100000 == 0 {
			if err := ctx.Err(); err != nil {
				return info, err
			}
		}
		row := strings.Split(sc.Text(), "\t")
		if len(row) < hfColumns {
			continue
		}
		key, err := strconv.Atoi(row[hfBibKey])
		if err != nil || key < 0 || key > 1<<31 || !seen.add(key) {
			continue
		}
		res.add(func() census.Record { return hathiRecord(row, key) })
	}
	if err := sc.Err(); err != nil {
		return info, wrap("hathitrust", err)
	}
	info.Total = res.seen
	return info, res.emit(emit)
}

func hathiRecord(row []string, key int) census.Record {
	id := strconv.Itoa(key)
	r := census.Record{NativeID: id, Kind: "other", Title: strings.TrimRight(strings.TrimSpace(row[hfTitle]), " /.:"), Language: Lang(row[hfLang]),
		IDs: resource.IDs{"hathi_record": {id}, "hathi": {row[hfHTID]}}}
	if k, ok := hfKinds[row[hfFormat]]; ok {
		r.Kind = k
	}
	if d := row[hfDate]; dcYear.MatchString(d) && d != "9999" {
		r.Date = d[:4]
	}
	if a := strings.TrimRight(strings.TrimSpace(row[hfAuthor]), ",. "); a != "" {
		r.Authors = []string{a}
	}
	list := func(col int) []string {
		var out []string
		for _, v := range strings.Split(row[col], ",") {
			if v = strings.TrimSpace(v); v != "" && len(out) < 5 {
				out = append(out, v)
			}
		}
		return out
	}
	if v := list(hfOCLC); len(v) > 0 {
		r.IDs["oclc"] = v
	}
	if v := list(hfISBN); len(v) > 0 {
		r.IDs["isbn"] = v
	}
	if v := list(hfLCCN); len(v) > 0 {
		r.IDs["lccn"] = v
	}
	return r
}
