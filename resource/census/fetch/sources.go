package fetch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census"
	"github.com/tamnd/math2-agent/resource/ids"
)

// The bases of the APIs.
const (
	ZBMathBase      = "https://api.zbmath.org/v1"
	OpenAlexBase    = "https://api.openalex.org"
	CrossrefBase    = "https://api.crossref.org"
	HALBase         = "https://api.archives-ouvertes.fr"
	OpenLibraryBase = "https://openlibrary.org"
)

// OpenAlexMath is the OpenAlex filter of the works whose primary topic is in the field Mathematics.
const OpenAlexMath = "primary_topic.field.id:26"

// ZBMath samples zbMATH Open and looks up DOIs in it.
type ZBMath struct{ Base string }

// Name gives zbmath.
func (ZBMath) Name() string { return "zbmath" }

type zbDoc struct {
	ID           int    `json:"id"`
	Identifier   string `json:"identifier"`
	Year         string `json:"year"`
	DocumentType struct {
		Code string `json:"code"`
	} `json:"document_type"`
	Title struct {
		Title string `json:"title"`
	} `json:"title"`
	Language struct {
		Languages []string `json:"languages"`
	} `json:"language"`
	Contributors struct {
		Authors []struct {
			Name string `json:"name"`
		} `json:"authors"`
	} `json:"contributors"`
	Links []struct {
		Identifier string `json:"identifier"`
		Type       string `json:"type"`
	} `json:"links"`
	MSC []struct {
		Code string `json:"code"`
	} `json:"msc"`
	Source struct {
		Series []struct {
			ISSN []struct {
				Number string `json:"number"`
			} `json:"issn"`
		} `json:"series"`
	} `json:"source"`
}

type zbPage struct {
	Result []zbDoc `json:"result"`
	Status struct {
		Total int `json:"nr_total_results"`
	} `json:"status"`
}

var zbKinds = map[string]string{"j": "article", "a": "chapter", "b": "monograph", "p": "preprint", "s": "proceedings"}

// Sample takes random pages of 100 documents.
func (z ZBMath) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	var first zbPage
	if err := c.JSON(ctx, z.Base+"/document/_all?page=0&results_per_page=1", &first); err != nil {
		return Info{}, wrap("zbmath", err)
	}
	info := Info{Total: first.Status.Total, Filter: "all documents"}
	w := weight(info.Total, n)
	for _, p := range pages(info.Total, 100, (n+99)/100, seed) {
		var page zbPage
		if err := c.JSON(ctx, fmt.Sprintf("%s/document/_all?page=%d&results_per_page=100", z.Base, p), &page); err != nil {
			return info, wrap("zbmath", err)
		}
		for _, d := range page.Result {
			r := zbRecord(d)
			r.Weight = w
			if err := emit(r); err != nil {
				return info, err
			}
		}
	}
	return info, nil
}

func zbRecord(d zbDoc) census.Record {
	r := census.Record{
		NativeID: d.Identifier,
		Kind:     zbKinds[d.DocumentType.Code],
		Date:     d.Year,
		Title:    d.Title.Title,
		IDs:      resource.IDs{},
	}
	if r.NativeID == "" {
		r.NativeID = strconv.Itoa(d.ID)
	}
	if r.Kind == "" {
		r.Kind = "article"
	}
	if len(d.Language.Languages) > 0 {
		r.Language = Lang(d.Language.Languages[0])
	}
	for _, a := range d.Contributors.Authors {
		r.Authors = append(r.Authors, a.Name)
	}
	if d.Identifier != "" {
		r.IDs["zbl"] = resource.Values{d.Identifier}
	}
	r.IDs["zbl_id"] = resource.Values{strconv.Itoa(d.ID)}
	for _, l := range d.Links {
		switch l.Type {
		case "doi":
			r.IDs["doi"] = append(r.IDs["doi"], l.Identifier)
		case "arxiv":
			r.IDs["arxiv"] = append(r.IDs["arxiv"], l.Identifier)
		}
	}
	for _, m := range d.MSC {
		r.Signals.MSC = addUnique(r.Signals.MSC, m.Code)
	}
	for _, s := range d.Source.Series {
		for _, i := range s.ISSN {
			r.Signals.Venues = addUnique(r.Signals.Venues, i.Number)
		}
	}
	r.Signals.ZBMath = true
	return r
}

// Batch gives 20.
func (ZBMath) Batch() int { return 20 }

// Lookup asks for the DOIs with one query doi:(a | b | ...). A DOI with a character of the query syntax is not asked.
func (z ZBMath) Lookup(ctx context.Context, c *Client, dois []string) (map[string]bool, error) {
	var ask []string
	for _, d := range dois {
		if !strings.ContainsAny(d, "()|\"& ") {
			ask = append(ask, d)
		}
	}
	found := map[string]bool{}
	if len(ask) == 0 {
		return found, nil
	}
	q := url.Values{"search_string": {"doi:(" + strings.Join(ask, " | ") + ")"}, "results_per_page": {"100"}}
	var page zbPage
	if err := c.JSON(ctx, z.Base+"/document/_search?"+q.Encode(), &page); err != nil {
		return nil, wrap("zbmath", err)
	}
	for _, d := range page.Result {
		for _, l := range d.Links {
			if l.Type == "doi" {
				if n, err := ids.Normalize("doi", l.Identifier); err == nil {
					found[n] = true
				}
			}
		}
	}
	return found, nil
}

// OpenAlex samples OpenAlex with a filter and looks up DOIs in it.
type OpenAlex struct {
	Base   string
	Filter string
}

// Name gives openalex.
func (OpenAlex) Name() string { return "openalex" }

type oaWork struct {
	ID              string `json:"id"`
	DOI             string `json:"doi"`
	Title           string `json:"title"`
	PublicationYear int    `json:"publication_year"`
	Language        string `json:"language"`
	Type            string `json:"type"`
	Authorships     []struct {
		Author struct {
			DisplayName string `json:"display_name"`
		} `json:"author"`
	} `json:"authorships"`
	Topics []struct {
		Score float64 `json:"score"`
		Field struct {
			DisplayName string `json:"display_name"`
		} `json:"field"`
	} `json:"topics"`
	PrimaryLocation *struct {
		Source *struct {
			ISSNL string `json:"issn_l"`
		} `json:"source"`
	} `json:"primary_location"`
}

type oaPage struct {
	Meta struct {
		Count int `json:"count"`
	} `json:"meta"`
	Results []oaWork `json:"results"`
}

var oaKinds = map[string]string{
	"article": "article", "book": "monograph", "book-chapter": "chapter", "dissertation": "thesis", "preprint": "preprint",
	"review": "survey", "report": "report", "dataset": "dataset", "editorial": "article", "letter": "article",
}

const oaSelect = "id,doi,title,publication_year,language,type,authorships,topics,primary_location"

// Sample uses the sample parameter of OpenAlex, with pages of 200.
func (o OpenAlex) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	var first oaPage
	if err := c.JSON(ctx, o.Base+"/works?per-page=1&select=id&filter="+url.QueryEscape(o.Filter), &first); err != nil {
		return Info{}, wrap("openalex", err)
	}
	info := Info{Total: first.Meta.Count, Filter: o.Filter}
	n = min(n, 10000, info.Total)
	w := weight(info.Total, n)
	for p := 1; (p-1)*200 < n; p++ {
		q := url.Values{"filter": {o.Filter}, "sample": {strconv.Itoa(n)}, "seed": {strconv.FormatUint(seed%1000000, 10)}, "per-page": {"200"}, "page": {strconv.Itoa(p)}, "select": {oaSelect}}
		var page oaPage
		if err := c.JSON(ctx, o.Base+"/works?"+q.Encode(), &page); err != nil {
			return info, wrap("openalex", err)
		}
		for _, x := range page.Results {
			r := oaRecord(x)
			r.Weight = w
			if err := emit(r); err != nil {
				return info, err
			}
		}
	}
	return info, nil
}

func oaRecord(x oaWork) census.Record {
	r := census.Record{
		NativeID: strings.TrimPrefix(x.ID, "https://openalex.org/"),
		Kind:     oaKinds[x.Type],
		Language: x.Language,
		Title:    x.Title,
		IDs:      resource.IDs{"openalex": {strings.TrimPrefix(x.ID, "https://openalex.org/")}},
	}
	if r.Kind == "" {
		r.Kind = "other"
	}
	if x.PublicationYear > 0 {
		r.Date = strconv.Itoa(x.PublicationYear)
	}
	if x.DOI != "" {
		r.IDs["doi"] = resource.Values{x.DOI}
	}
	for _, a := range x.Authorships {
		r.Authors = append(r.Authors, a.Author.DisplayName)
	}
	for _, t := range x.Topics {
		r.Signals.Topics = append(r.Signals.Topics, scopeTopic(t.Field.DisplayName, t.Score))
	}
	if x.PrimaryLocation != nil && x.PrimaryLocation.Source != nil && x.PrimaryLocation.Source.ISSNL != "" {
		r.Signals.Venues = []string{x.PrimaryLocation.Source.ISSNL}
	}
	return r
}

// Batch gives 50.
func (OpenAlex) Batch() int { return 50 }

// Lookup asks for the DOIs with one filter doi:a|b|... .
func (o OpenAlex) Lookup(ctx context.Context, c *Client, dois []string) (map[string]bool, error) {
	parts := make([]string, len(dois))
	for i, d := range dois {
		parts[i] = "https://doi.org/" + d
	}
	q := url.Values{"filter": {"doi:" + strings.Join(parts, "|")}, "per-page": {"200"}, "select": {"doi"}}
	var page oaPage
	if err := c.JSON(ctx, o.Base+"/works?"+q.Encode(), &page); err != nil {
		return nil, wrap("openalex", err)
	}
	found := map[string]bool{}
	for _, x := range page.Results {
		if n, err := ids.Normalize("doi", x.DOI); err == nil {
			found[n] = true
		}
	}
	return found, nil
}

// Crossref samples Crossref and looks up DOIs in it.
type Crossref struct{ Base string }

// Name gives crossref.
func (Crossref) Name() string { return "crossref" }

type crWork struct {
	DOI    string   `json:"DOI"`
	Title  []string `json:"title"`
	Type   string   `json:"type"`
	ISSN   []string `json:"ISSN"`
	ISBN   []string `json:"ISBN"`
	Author []struct {
		Given  string `json:"given"`
		Family string `json:"family"`
		Name   string `json:"name"`
	} `json:"author"`
	Issued struct {
		DateParts [][]int `json:"date-parts"`
	} `json:"issued"`
}

type crList struct {
	Message struct {
		Total int      `json:"total-results"`
		Items []crWork `json:"items"`
	} `json:"message"`
}

var crKinds = map[string]string{
	"journal-article": "article", "book": "monograph", "monograph": "monograph", "edited-book": "edited-volume", "book-chapter": "chapter",
	"book-part": "chapter", "book-section": "chapter", "reference-entry": "chapter", "proceedings-article": "conference-paper",
	"proceedings": "proceedings", "posted-content": "preprint", "dissertation": "thesis", "report": "report", "dataset": "dataset",
	"journal": "journal", "book-series": "series", "reference-book": "handbook",
}

// Sample takes random sets of 100 works with the sample parameter of Crossref. Crossref has no subject, so the sample is of all works.
func (cr Crossref) Sample(ctx context.Context, c *Client, n int, _ uint64, emit func(census.Record) error) (Info, error) {
	var first crList
	if err := c.JSON(ctx, cr.Base+"/works?rows=0", &first); err != nil {
		return Info{}, wrap("crossref", err)
	}
	info := Info{Total: first.Message.Total, Filter: "all works"}
	w := weight(info.Total, n)
	for got := 0; got < n; got += 100 {
		var list crList
		if err := c.JSON(ctx, cr.Base+"/works?sample=100&select=DOI,title,type,ISSN,ISBN,author,issued", &list); err != nil {
			return info, wrap("crossref", err)
		}
		for _, x := range list.Message.Items {
			r := crRecord(x)
			r.Weight = w
			if err := emit(r); err != nil {
				return info, err
			}
		}
	}
	return info, nil
}

func crRecord(x crWork) census.Record {
	r := census.Record{NativeID: x.DOI, Kind: crKinds[x.Type], IDs: resource.IDs{"doi": {x.DOI}}}
	if r.Kind == "" {
		r.Kind = "other"
	}
	if len(x.Title) > 0 {
		r.Title = x.Title[0]
	}
	if len(x.Issued.DateParts) > 0 && len(x.Issued.DateParts[0]) > 0 && x.Issued.DateParts[0][0] > 0 {
		r.Date = fmt.Sprintf("%04d", x.Issued.DateParts[0][0])
	}
	for _, a := range x.Author {
		switch {
		case a.Family != "" && a.Given != "":
			r.Authors = append(r.Authors, a.Family+", "+a.Given)
		case a.Family != "":
			r.Authors = append(r.Authors, a.Family)
		case a.Name != "":
			r.Authors = append(r.Authors, a.Name)
		}
	}
	if len(x.ISBN) > 0 {
		r.IDs["isbn"] = x.ISBN
	}
	r.Signals.Venues = x.ISSN
	return r
}

// Batch gives 50.
func (Crossref) Batch() int { return 50 }

// Lookup asks for the DOIs with one filter doi:a,doi:b,... .
func (cr Crossref) Lookup(ctx context.Context, c *Client, dois []string) (map[string]bool, error) {
	parts := make([]string, len(dois))
	for i, d := range dois {
		parts[i] = "doi:" + d
	}
	q := url.Values{"filter": {strings.Join(parts, ",")}, "rows": {"100"}, "select": {"DOI"}}
	var list crList
	if err := c.JSON(ctx, cr.Base+"/works?"+q.Encode(), &list); err != nil {
		return nil, wrap("crossref", err)
	}
	found := map[string]bool{}
	for _, x := range list.Message.Items {
		if n, err := ids.Normalize("doi", x.DOI); err == nil {
			found[n] = true
		}
	}
	return found, nil
}

// HAL samples the domain math of HAL and looks up DOIs in it.
type HAL struct{ Base string }

// Name gives hal.
func (HAL) Name() string { return "hal" }

type halDoc struct {
	HalID         string   `json:"halId_s"`
	Title         []string `json:"title_s"`
	Authors       []string `json:"authFullName_s"`
	DOI           string   `json:"doiId_s"`
	Arxiv         string   `json:"arxivId_s"`
	Year          int      `json:"producedDateY_i"`
	Language      []string `json:"language_s"`
	DocType       string   `json:"docType_s"`
	PrimaryDomain string   `json:"primaryDomain_s"`
	Domains       []string `json:"domain_s"`
}

type halList struct {
	Response struct {
		NumFound int      `json:"numFound"`
		Docs     []halDoc `json:"docs"`
	} `json:"response"`
}

var halKinds = map[string]string{
	"ART": "article", "COMM": "conference-paper", "THESE": "thesis", "HDR": "thesis", "OUV": "monograph", "COUV": "chapter",
	"DOUV": "edited-volume", "PREPRINT": "preprint", "UNDEFINED": "preprint", "REPORT": "report", "LECTURE": "lecture-notes",
	"PROCEEDINGS": "proceedings", "SOFTWARE": "software",
}

const halFields = "halId_s,title_s,authFullName_s,doiId_s,arxivId_s,producedDateY_i,language_s,docType_s,primaryDomain_s,domain_s"

// Sample takes random pages of 100 records of the domain math.
func (h HAL) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	base := h.Base + "/search/?q=*:*&fq=level0_domain_s:math&wt=json"
	var first halList
	if err := c.JSON(ctx, base+"&rows=0", &first); err != nil {
		return Info{}, wrap("hal", err)
	}
	info := Info{Total: first.Response.NumFound, Filter: "level0_domain_s:math"}
	w := weight(info.Total, n)
	for _, p := range pages(info.Total, 100, (n+99)/100, seed) {
		var list halList
		if err := c.JSON(ctx, fmt.Sprintf("%s&rows=100&start=%d&sort=docid%%20asc&fl=%s", base, p*100, halFields), &list); err != nil {
			return info, wrap("hal", err)
		}
		for _, d := range list.Response.Docs {
			r := halRecord(d)
			r.Weight = w
			if err := emit(r); err != nil {
				return info, err
			}
		}
	}
	return info, nil
}

func halRecord(d halDoc) census.Record {
	r := census.Record{NativeID: d.HalID, Kind: halKinds[d.DocType], Authors: d.Authors, IDs: resource.IDs{"hal": {d.HalID}}}
	if r.Kind == "" {
		r.Kind = "other"
	}
	if len(d.Title) > 0 {
		r.Title = d.Title[0]
	}
	if d.Year > 0 {
		r.Date = strconv.Itoa(d.Year)
	}
	if len(d.Language) > 0 {
		r.Language = d.Language[0]
	}
	if d.DOI != "" {
		r.IDs["doi"] = resource.Values{d.DOI}
	}
	if d.Arxiv != "" {
		r.IDs["arxiv"] = resource.Values{d.Arxiv}
	}
	r.Signals.Arxiv = addUnique(r.Signals.Arxiv, HALDomain(d.PrimaryDomain))
	for _, dom := range d.Domains {
		if strings.HasPrefix(dom, "1.") {
			r.Signals.Arxiv = addUnique(r.Signals.Arxiv, HALDomain(dom))
		}
	}
	return r
}

// Batch gives 50.
func (HAL) Batch() int { return 50 }

// Lookup asks for the DOIs with one query doiId_s:("a" OR "b" ...).
func (h HAL) Lookup(ctx context.Context, c *Client, dois []string) (map[string]bool, error) {
	parts := make([]string, len(dois))
	for i, d := range dois {
		parts[i] = strconv.Quote(d)
	}
	q := url.Values{"q": {"doiId_s:(" + strings.Join(parts, " OR ") + ")"}, "rows": {"100"}, "fl": {"doiId_s"}, "wt": {"json"}}
	var list halList
	if err := c.JSON(ctx, h.Base+"/search/?"+q.Encode(), &list); err != nil {
		return nil, wrap("hal", err)
	}
	found := map[string]bool{}
	for _, d := range list.Response.Docs {
		if n, err := ids.Normalize("doi", d.DOI); err == nil {
			found[n] = true
		}
	}
	return found, nil
}

// OpenLibrary samples the works of Open Library with a subject.
type OpenLibrary struct {
	Base    string
	Subject string
}

// Name gives openlibrary.
func (OpenLibrary) Name() string { return "openlibrary" }

type olDoc struct {
	Key              string   `json:"key"`
	Title            string   `json:"title"`
	AuthorName       []string `json:"author_name"`
	FirstPublishYear int      `json:"first_publish_year"`
	Language         []string `json:"language"`
	ISBN             []string `json:"isbn"`
	LCC              []string `json:"lcc"`
	DDC              []string `json:"ddc"`
	OCLC             []string `json:"oclc"`
	LCCN             []string `json:"lccn"`
}

type olList struct {
	NumFound int     `json:"numFound"`
	Docs     []olDoc `json:"docs"`
}

const olFields = "key,title,author_name,first_publish_year,language,isbn,lcc,ddc,oclc,lccn"

// Sample takes random pages of 100 works of the subject.
func (o OpenLibrary) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	base := o.Base + "/search.json?subject=" + url.QueryEscape(o.Subject)
	var first olList
	if err := c.JSON(ctx, base+"&limit=1&fields=key", &first); err != nil {
		return Info{}, wrap("openlibrary", err)
	}
	info := Info{Total: first.NumFound, Filter: "subject " + o.Subject}
	w := weight(info.Total, n)
	for _, p := range pages(info.Total, 100, (n+99)/100, seed) {
		var list olList
		if err := c.JSON(ctx, fmt.Sprintf("%s&limit=100&page=%d&fields=%s", base, p+1, olFields), &list); err != nil {
			return info, wrap("openlibrary", err)
		}
		for _, d := range list.Docs {
			r := olRecord(d)
			r.Weight = w
			if err := emit(r); err != nil {
				return info, err
			}
		}
	}
	return info, nil
}

func olRecord(d olDoc) census.Record {
	key := strings.TrimPrefix(d.Key, "/works/")
	r := census.Record{NativeID: key, Kind: "monograph", Title: d.Title, Authors: d.AuthorName, IDs: resource.IDs{"openlibrary_work": {key}}}
	if d.FirstPublishYear > 0 {
		r.Date = strconv.Itoa(d.FirstPublishYear)
	}
	if len(d.Language) > 0 {
		r.Language = Lang(d.Language[0])
	}
	if len(d.ISBN) > 0 {
		r.IDs["isbn"] = d.ISBN[:min(len(d.ISBN), 5)]
	}
	if len(d.OCLC) > 0 {
		r.IDs["oclc"] = d.OCLC[:1]
	}
	if len(d.LCCN) > 0 {
		r.IDs["lccn"] = d.LCCN[:1]
	}
	for _, c := range d.LCC {
		r.Signals.LCC = addUnique(r.Signals.LCC, LCC(c))
	}
	r.Signals.DDC = d.DDC
	return r
}
