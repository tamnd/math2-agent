package fetch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/oai"
	"github.com/tamnd/math2-agent/resource/census"
)

// The bases of the OAI-PMH endpoints of the catalogues of phases R2 and R3.
const (
	DOABOAIBase         = "https://directory.doabooks.org/oai/request"
	OAPENOAIBase        = "https://library.oapen.org/oai/request"
	EuDMLOAIBase        = "https://eudml.org/oai/OAIHandler"
	CyberLeninkaOAIBase = "https://cyberleninka.ru/oai"
)

// DOABBooks is the OAI-PMH set of the books of DOAB. The other sets of DOAB hold the chapters, the publishers and the funders.
const DOABBooks = "com_20.500.12854_5"

// getter makes an ingest.Getter from the client, for the package oai.
func getter(c *Client) ingest.Getter {
	return func(ctx context.Context, url string) (io.ReadCloser, error) {
		body, err := c.Get(ctx, url)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(body)), nil
	}
}

type dcValue struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

// dcMeta is the metadata of a record in the format oai_dc. DSpace adds the element resourceType of OpenAIRE and the element alternateIdentifier.
type dcMeta struct {
	Title      []string  `xml:"title"`
	Creator    []string  `xml:"creator"`
	Subject    []dcValue `xml:"subject"`
	Date       []dcValue `xml:"date"`
	Type       []string  `xml:"type"`
	Resource   string    `xml:"resourceType"`
	Identifier []dcValue `xml:"identifier"`
	AltID      []dcValue `xml:"alternateIdentifier"`
	Language   []string  `xml:"language"`
}

// harvestDC harvests a set of an OAI-PMH endpoint in the format oai_dc, and calls fn for each record that is not deleted.
func harvestDC(ctx context.Context, c *Client, base, set string, fn func(id string, m dcMeta)) error {
	return oai.Harvest(ctx, getter(c), base, "oai_dc", set, func(r oai.Record) error {
		if r.Deleted {
			return nil
		}
		var m dcMeta
		if err := oai.Unmarshal(r.Metadata, &m); err != nil {
			return fmt.Errorf("%s: %w", r.ID, err)
		}
		fn(r.ID, m)
		return nil
	})
}

var (
	dcYear = regexp.MustCompile(`^[0-9]{4}`)
	dcDOI  = regexp.MustCompile(`(?i)(?:^|doi\.org/|^doi:\s*)(10\.[0-9]{4,9}/\S+)`)
	dcISBN = regexp.MustCompile(`^97[89][0-9]{10}$`)
)

// oaiKinds maps the types of oai_dc and of OpenAIRE to the kinds of resources.
var oaiKinds = map[string]string{
	"book": "monograph", "chapter": "chapter", "article": "article", "journal article": "article", "research article": "article",
	"text": "article", "thesis": "thesis", "doctoral thesis": "thesis", "conference object": "conference-paper", "conference paper": "conference-paper",
}

// oaiRecord makes a census record from the fields of oai_dc that each catalogue shares. The caller sets the native id and the signals. The kind is def when the record gives no known type.
func oaiRecord(m dcMeta, def string) census.Record {
	r := census.Record{Kind: def, Authors: m.Creator, IDs: resource.IDs{}}
	if len(m.Title) > 0 {
		r.Title = strings.Join(strings.Fields(m.Title[0]), " ")
	}
	for _, t := range append([]string{m.Resource}, m.Type...) {
		if k, ok := oaiKinds[strings.ToLower(strings.TrimSpace(t))]; ok {
			r.Kind = k
			break
		}
	}
	date := ""
	for _, d := range m.Date {
		v := strings.TrimSpace(d.Value)
		if !dcYear.MatchString(v) {
			continue
		}
		if strings.EqualFold(d.Type, "issued") {
			date = v
			break
		}
		if date == "" {
			date = v
		}
	}
	if i := strings.IndexByte(date, 'T'); i >= 0 {
		date = date[:i]
	}
	r.Date = date
	if len(m.Language) > 0 {
		r.Language = Lang(m.Language[0])
	}
	for _, id := range append(m.Identifier, m.AltID...) {
		v := strings.TrimSpace(id.Value)
		if x := dcDOI.FindStringSubmatch(v); x != nil {
			r.IDs["doi"] = addUnique(r.IDs["doi"], strings.ToLower(x[1]))
		}
		if isbn := strings.ReplaceAll(strings.TrimPrefix(strings.ToUpper(v), "ISBN "), "-", ""); dcISBN.MatchString(isbn) {
			r.IDs["isbn"] = addUnique(r.IDs["isbn"], isbn)
		}
	}
	return r
}

// localID gives the part of an OAI identifier after oai:host:, as 20.500.12854/141887 for oai:directory.doabooks.org:20.500.12854/141887.
func localID(id string) string {
	rest := strings.TrimPrefix(id, "oai:")
	if _, after, ok := strings.Cut(rest, ":"); ok {
		return after
	}
	return id
}

// OAIBooks harvests the books of a DSpace repository of open access books, DOAB or OAPEN, through OAI-PMH in the format oai_dc, and keeps the records with a Thema or BIC class in PB Mathematics. It emits a random part of them. The two repositories share many books, because DOAB harvests OAPEN.
type OAIBooks struct {
	Catalogue string // doab or oapen
	Base      string
	Set       string // the OAI-PMH set, or empty for the whole repository
}

// Name gives the catalogue.
func (b OAIBooks) Name() string { return b.Catalogue }

// ThemaMath reports whether a class of Thema or BIC, as DSpace gives it, is in PB Mathematics. An example is "thema EDItEUR::P Mathematics and Science::PB Mathematics::PBT Probability and statistics".
func ThemaMath(class string) bool {
	for _, part := range strings.Split(class, "::") {
		if strings.HasPrefix(strings.TrimSpace(part), "PB ") {
			return true
		}
	}
	return false
}

// Sample harvests each record of the set and keeps the books of mathematics.
func (b OAIBooks) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	info := Info{Filter: "the records with a Thema or BIC class in PB Mathematics, OAI-PMH format oai_dc"}
	var all []census.Record
	err := harvestDC(ctx, c, b.Base, b.Set, func(id string, m dcMeta) {
		math := false
		for _, s := range m.Subject {
			if s.Type == "classification" && ThemaMath(s.Value) {
				math = true
				break
			}
		}
		if !math {
			return
		}
		r := oaiRecord(m, "monograph")
		r.NativeID = localID(id)
		r.IDs["handle"] = resource.Values{r.NativeID}
		r.Signals.MathHeading = true
		all = append(all, r)
	})
	if err != nil {
		return info, wrap(b.Catalogue, err)
	}
	info.Total = len(all)
	return info, subsample(all, n, seed, emit)
}

// EuDML harvests each record of EuDML through OAI-PMH in the format oai_dc, which each OAI-PMH endpoint must give, and emits a random part of them. Each record of EuDML is a work of mathematics, but the format oai_dc gives no class, so the records have no signal of the scope tests.
type EuDML struct{ Base string }

// Name gives eudml.
func (EuDML) Name() string { return "eudml" }

// eudmlID finds the number of a document of EuDML, as 94429 in urn:eudml:doc:94429 or in https://eudml.org/doc/94429.
var eudmlID = regexp.MustCompile(`(?:urn:eudml:doc:|eudml\.org/doc/|^doc/|^doc:)([1-9][0-9]*)$`)

// Sample harvests each page until the resumption token ends.
func (e EuDML) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	info := Info{Filter: "each record, OAI-PMH format oai_dc"}
	var all []census.Record
	err := harvestDC(ctx, c, e.Base, "", func(id string, m dcMeta) {
		r := oaiRecord(m, "article")
		r.NativeID = localID(id)
		for _, v := range append([]string{r.NativeID}, values(m.Identifier)...) {
			if x := eudmlID.FindStringSubmatch(strings.TrimSpace(v)); x != nil {
				r.IDs["eudml"] = addUnique(r.IDs["eudml"], x[1])
			}
		}
		all = append(all, r)
	})
	if err != nil {
		return info, wrap("eudml", err)
	}
	info.Total = len(all)
	return info, subsample(all, n, seed, emit)
}

// values gives the text of each value.
func values(vs []dcValue) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Value
	}
	return out
}

// CyberLeninka harvests the articles of CyberLeninka through OAI-PMH in the format oai_dc, and keeps the articles with a subject of mathematics, in Russian or in English. It emits a random part of them.
type CyberLeninka struct {
	Base string
	Set  string // the OAI-PMH set, or empty for the whole repository
}

// Name gives cyberleninka.
func (CyberLeninka) Name() string { return "cyberleninka" }

// clArticle finds the slug of an article of CyberLeninka in its URL.
var clArticle = regexp.MustCompile(`cyberleninka\.ru/article/n/([^/?#\s]+)`)

// mathSubject reports whether a subject is a heading of mathematics, in English or in Russian.
func mathSubject(subjects []dcValue) bool {
	var heads []string
	for _, s := range subjects {
		v := strings.ToLower(strings.TrimSpace(s.Value))
		if v == "математика" || strings.HasPrefix(v, "математика ") {
			return true
		}
		heads = append(heads, v)
	}
	return MathHeading(heads)
}

// Sample harvests each page until the resumption token ends.
func (cl CyberLeninka) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	info := Info{Filter: "the articles with the subject mathematics or математика, OAI-PMH format oai_dc"}
	var all []census.Record
	err := harvestDC(ctx, c, cl.Base, cl.Set, func(id string, m dcMeta) {
		if !mathSubject(m.Subject) {
			return
		}
		r := oaiRecord(m, "article")
		r.NativeID = localID(id)
		for _, v := range values(m.Identifier) {
			if x := clArticle.FindStringSubmatch(v); x != nil {
				r.NativeID = x[1]
				break
			}
		}
		r.IDs["cyberleninka"] = resource.Values{r.NativeID}
		r.Signals.MathHeading = true
		all = append(all, r)
	})
	if err != nil {
		return info, wrap("cyberleninka", err)
	}
	info.Total = len(all)
	return info, subsample(all, n, seed, emit)
}
