package fetch

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census"
)

// The bases of the APIs of the catalogues of phases R2 and R3.
const (
	DataCiteBase  = "https://api.datacite.org"
	DOAJBase      = "https://doaj.org/api"
	GutenbergBase = "https://www.gutenberg.org"
	ArchiveBase   = "https://archive.org"
	WikidataBase  = "https://query.wikidata.org/sparql"
	NumdamOAIBase = "https://www.numdam.org/oai/"
)

// DOAJFirstDay is the first day of the field created_date of the articles of DOAJ.
const DOAJFirstDay = "2004-01-01"

// subsample emits a random part of about n records of all, with the weight len(all) / n. When n is len(all) or more, it emits each record with the weight 1.
func subsample(all []census.Record, n int, seed uint64, emit func(census.Record) error) error {
	picks := census.Offsets(len(all), n, seed)
	w := weight(len(all), len(picks))
	for _, i := range picks {
		r := all[i]
		r.Weight = w
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// DataCite samples the DOIs of DataCite with the subject mathematics. Its API gives random records, so the sample takes pages of random records and drops the repeats.
type DataCite struct{ Base string }

// Name gives datacite.
func (DataCite) Name() string { return "datacite" }

type dcItem struct {
	Attributes struct {
		DOI     string                   `json:"doi"`
		Titles  []struct{ Title string } `json:"titles"`
		Year    int                      `json:"publicationYear"`
		Lang    string                   `json:"language"`
		Creator []struct {
			Name string `json:"name"`
		} `json:"creators"`
		Subjects []struct {
			Subject string `json:"subject"`
			Scheme  string `json:"subjectScheme"`
		} `json:"subjects"`
		Types struct {
			General string `json:"resourceTypeGeneral"`
		} `json:"types"`
		Related []struct {
			ID   string `json:"relatedIdentifier"`
			Type string `json:"relatedIdentifierType"`
			Rel  string `json:"relationType"`
		} `json:"relatedIdentifiers"`
	} `json:"attributes"`
}

type dcList struct {
	Data []dcItem `json:"data"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

var dcKinds = map[string]string{
	"JournalArticle": "article", "Preprint": "preprint", "Book": "monograph", "BookChapter": "chapter", "Dissertation": "thesis",
	"Report": "report", "ConferencePaper": "conference-paper", "ConferenceProceeding": "proceedings", "Software": "software",
	"Dataset": "dataset", "Text": "other",
}

const dcQuery = "subjects.subject:Mathematics"

// Sample takes pages of 1000 random records until it has n distinct DOIs, or until a page adds none.
func (d DataCite) Sample(ctx context.Context, c *Client, n int, _ uint64, emit func(census.Record) error) (Info, error) {
	base := d.Base + "/dois?query=" + url.QueryEscape(dcQuery)
	var first dcList
	if err := c.JSON(ctx, base+"&page%5Bsize%5D=0", &first); err != nil {
		return Info{}, wrap("datacite", err)
	}
	info := Info{Total: first.Meta.Total, Filter: dcQuery}
	seen := map[string]bool{}
	var all []census.Record
	for len(all) < min(n, info.Total) {
		var list dcList
		if err := c.JSON(ctx, fmt.Sprintf("%s&random=true&page%%5Bsize%%5D=%d", base, min(1000, n)), &list); err != nil {
			return info, wrap("datacite", err)
		}
		added := 0
		for _, it := range list.Data {
			if doi := strings.ToLower(it.Attributes.DOI); doi != "" && !seen[doi] && len(all) < n {
				seen[doi] = true
				all = append(all, dcRecord(it))
				added++
			}
		}
		if added == 0 {
			break
		}
	}
	w := weight(info.Total, len(all))
	for _, r := range all {
		r.Weight = w
		if err := emit(r); err != nil {
			return info, err
		}
	}
	return info, nil
}

func dcRecord(it dcItem) census.Record {
	a := it.Attributes
	r := census.Record{NativeID: strings.ToLower(a.DOI), Kind: dcKinds[a.Types.General], Language: Lang(a.Lang), IDs: resource.IDs{"doi": {strings.ToLower(a.DOI)}}}
	if r.Kind == "" {
		r.Kind = "other"
	}
	if len(a.Titles) > 0 {
		r.Title = a.Titles[0].Title
	}
	if a.Year > 0 {
		r.Date = strconv.Itoa(a.Year)
	}
	for _, cr := range a.Creator {
		r.Authors = append(r.Authors, cr.Name)
	}
	for _, s := range a.Subjects {
		switch strings.ToLower(s.Scheme) {
		case "msc", "msc2020", "msc2010":
			r.Signals.MSC = addUnique(r.Signals.MSC, s.Subject)
		case "ddc":
			r.Signals.DDC = addUnique(r.Signals.DDC, s.Subject)
		}
		if strings.EqualFold(strings.TrimPrefix(s.Subject, "FOS: "), "mathematics") {
			r.Signals.MathHeading = true
		}
	}
	for _, rel := range a.Related {
		if strings.EqualFold(rel.Type, "arXiv") {
			r.IDs["arxiv"] = addUnique(r.IDs["arxiv"], strings.TrimPrefix(strings.ToLower(rel.ID), "arxiv:"))
		}
	}
	return r
}

// DOAJ samples the articles of DOAJ in the LCC class Mathematics. The API gives at most 1000 records of a query, so the sample is a cluster sample: it takes each article of random days of the field created_date.
type DOAJ struct {
	Base  string
	Today string // YYYY-MM-DD, the last day of the frame. Empty means today.
}

// Name gives doaj.
func (DOAJ) Name() string { return "doaj" }

type doajDoc struct {
	ID      string `json:"id"`
	Bibjson struct {
		Title      string `json:"title"`
		Year       string `json:"year"`
		Identifier []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"identifier"`
		Author []struct {
			Name string `json:"name"`
		} `json:"author"`
		Subject []struct {
			Code   string `json:"code"`
			Scheme string `json:"scheme"`
		} `json:"subject"`
		Journal struct {
			Language []string `json:"language"`
		} `json:"journal"`
	} `json:"bibjson"`
}

type doajList struct {
	Total   int       `json:"total"`
	Results []doajDoc `json:"results"`
}

const doajQuery = `index.classification.exact:"Mathematics"`

// doajPages is the most pages of 100 records of one query.
const doajPages = 10

// Sample takes random days and each article of those days, until it has about n articles.
func (d DOAJ) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	get := func(q string, page, size int) (doajList, error) {
		var list doajList
		err := c.JSON(ctx, fmt.Sprintf("%s/search/articles/%s?page=%d&pageSize=%d", d.Base, url.PathEscape(q), page, size), &list)
		return list, err
	}
	first, err := get(doajQuery, 1, 1)
	if err != nil {
		return Info{}, wrap("doaj", err)
	}
	start, _ := time.Parse(time.DateOnly, DOAJFirstDay)
	end := time.Now().UTC()
	if d.Today != "" {
		if end, err = time.Parse(time.DateOnly, d.Today); err != nil {
			return Info{}, fmt.Errorf("doaj: the day %q is not YYYY-MM-DD", d.Today)
		}
	}
	days := int(end.Sub(start).Hours()/24) + 1
	info := Info{Total: first.Total, Filter: doajQuery + ", in clusters of a day of created_date"}
	var all []census.Record
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) // #nosec G404 -- a sample needs a seeded generator, not a secure one.
	for _, k := range rng.Perm(days) {
		if len(all) >= n {
			break
		}
		day := start.AddDate(0, 0, k).Format(time.DateOnly)
		q := fmt.Sprintf("%s AND created_date:[%s TO %s]", doajQuery, day, day)
		for p := 1; p <= doajPages; p++ {
			list, err := get(q, p, 100)
			if err != nil {
				return info, wrap("doaj", err)
			}
			for _, x := range list.Results {
				all = append(all, doajRecord(x))
			}
			if p*100 >= list.Total {
				break
			}
		}
	}
	w := weight(info.Total, len(all))
	for _, r := range all {
		r.Weight = w
		if err := emit(r); err != nil {
			return info, err
		}
	}
	return info, nil
}

func doajRecord(x doajDoc) census.Record {
	b := x.Bibjson
	r := census.Record{NativeID: x.ID, Kind: "article", Title: b.Title, Date: b.Year, IDs: resource.IDs{}}
	if len(b.Journal.Language) == 1 {
		r.Language = Lang(b.Journal.Language[0])
	}
	for _, a := range b.Author {
		r.Authors = append(r.Authors, a.Name)
	}
	for _, id := range b.Identifier {
		if id.Type == "doi" {
			r.IDs["doi"] = resource.Values{strings.ToLower(id.ID)}
		}
	}
	for _, s := range b.Subject {
		if s.Scheme == "LCC" {
			r.Signals.LCC = addUnique(r.Signals.LCC, LCCRange(s.Code))
		}
	}
	return r
}

// LCCRange gives the first class of an LCC range of DOAJ, as QA1 for QA1-939 or QA75.5 for QA75.5-76.95.
func LCCRange(s string) string {
	s = strings.TrimSpace(s)
	i := strings.IndexFunc(s, func(r rune) bool { return r >= '0' && r <= '9' })
	if i < 0 {
		return s
	}
	if j := strings.IndexByte(s[i:], '-'); j >= 0 {
		return s[:i+j]
	}
	return s
}

// Gutenberg reads the catalogue file of Project Gutenberg and keeps the texts of mathematics: the texts with the LCC class QA, or with a subject or a bookshelf of mathematics.
type Gutenberg struct{ Base string }

// Name gives gutenberg.
func (Gutenberg) Name() string { return "gutenberg" }

// GutenbergCatalogue is the path of the catalogue file under the base.
const GutenbergCatalogue = "/cache/epub/feeds/pg_catalog.csv"

// Sample reads the catalogue file and emits about n texts of mathematics.
func (g Gutenberg) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	body, err := c.Get(ctx, g.Base+GutenbergCatalogue)
	if err != nil {
		return Info{}, wrap("gutenberg", err)
	}
	all, err := gutenbergRecords(body)
	if err != nil {
		return Info{}, wrap("gutenberg", err)
	}
	info := Info{Total: len(all), Filter: "the texts with the LCC class QA, or a subject or a bookshelf of mathematics"}
	return info, subsample(all, n, seed, emit)
}

func gutenbergRecords(body []byte) ([]census.Record, error) {
	rd := csv.NewReader(bytes.NewReader(body))
	rd.FieldsPerRecord = -1
	head, err := rd.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	for _, h := range []string{"Text#", "Type", "Title", "Language", "Authors", "Subjects", "LoCC", "Bookshelves"} {
		if _, ok := col[h]; !ok {
			return nil, fmt.Errorf("the catalogue has no column %s", h)
		}
	}
	var out []census.Record
	for {
		row, err := rd.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		get := func(h string) string {
			if i := col[h]; i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		if get("Type") != "Text" {
			continue
		}
		split := func(h string) []string {
			var vs []string
			for _, v := range strings.Split(get(h), ";") {
				if v = strings.TrimSpace(v); v != "" {
					vs = append(vs, v)
				}
			}
			return vs
		}
		classes, headings := split("LoCC"), append(split("Subjects"), split("Bookshelves")...)
		qa := slices.ContainsFunc(classes, func(c string) bool { return strings.HasPrefix(c, "QA") })
		heading := mathHeading(headings)
		if !qa && !heading {
			continue
		}
		id := get("Text#")
		r := census.Record{NativeID: id, Kind: "monograph", Title: get("Title"), Authors: split("Authors"), IDs: resource.IDs{"gutenberg": {id}}}
		if langs := split("Language"); len(langs) > 0 {
			r.Language = Lang(langs[0])
		}
		r.Signals.MathHeading = heading
		out = append(out, r)
	}
}

// mathHeading reports whether a subject heading starts with Mathematics or names a branch of mathematics.
func mathHeading(subjects []string) bool {
	for _, s := range subjects {
		head, _, _ := strings.Cut(strings.ToLower(strings.TrimPrefix(s, "Category: ")), " -- ")
		switch strings.TrimSpace(head) {
		case "mathematics", "algebra", "geometry", "arithmetic", "calculus", "trigonometry", "number theory", "probabilities", "mathematical analysis", "differential equations", "functions":
			return true
		}
	}
	return false
}

// Archive reads the texts of the Internet Archive with the subject mathematics through the scrape API, which pages by a cursor, and emits a random part of them.
type Archive struct{ Base string }

// Name gives internetarchive.
func (Archive) Name() string { return "internetarchive" }

// archiveQuery is the query of the texts with the subject mathematics.
const archiveQuery = "subject:mathematics AND mediatype:texts"

type archiveItem struct {
	ID       string `json:"identifier"`
	Title    any    `json:"title"`
	Year     any    `json:"year"`
	Language any    `json:"language"`
	Creator  any    `json:"creator"`
	Subject  any    `json:"subject"`
	Call     any    `json:"call_number"`
	ISBN     any    `json:"isbn"`
}

type archivePage struct {
	Items  []archiveItem `json:"items"`
	Cursor string        `json:"cursor"`
	Total  int           `json:"total"`
}

// strs reads a field of the Internet Archive, which is a string or a list of strings.
func strs(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case float64:
		return []string{strconv.Itoa(int(x))}
	case []any:
		var out []string
		for _, e := range x {
			out = append(out, strs(e)...)
		}
		return out
	}
	return nil
}

func first(v any) string {
	if s := strs(v); len(s) > 0 {
		return s[0]
	}
	return ""
}

// Sample reads pages of 10000 records until the cursor ends.
func (a Archive) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	v := url.Values{"q": {archiveQuery}, "fields": {"identifier,title,year,language,creator,subject,call_number,isbn"}, "count": {"10000"}}
	info := Info{Filter: archiveQuery}
	var all []census.Record
	for page := 0; page < 1000; page++ {
		var p archivePage
		if err := c.JSON(ctx, a.Base+"/services/search/v1/scrape?"+v.Encode(), &p); err != nil {
			return info, wrap("internetarchive", err)
		}
		if page == 0 {
			info.Total = p.Total // a page after the first gives the records after the cursor
		}
		for _, it := range p.Items {
			all = append(all, archiveRecord(it))
		}
		if p.Cursor == "" || len(p.Items) == 0 {
			break
		}
		v.Set("cursor", p.Cursor)
	}
	info.Total = max(info.Total, len(all))
	return info, subsample(all, n, seed, emit)
}

func archiveRecord(it archiveItem) census.Record {
	r := census.Record{NativeID: it.ID, Kind: "other", Title: first(it.Title), Date: first(it.Year), Language: Lang(first(it.Language)), Authors: strs(it.Creator), IDs: resource.IDs{"ia": {it.ID}}}
	switch {
	case strings.HasPrefix(it.ID, "arxiv-"):
		r.Kind = "preprint"
		r.IDs["arxiv"] = resource.Values{strings.TrimPrefix(it.ID, "arxiv-")}
	case it.ISBN != nil || it.Call != nil:
		r.Kind = "monograph"
	}
	if isbn := strs(it.ISBN); len(isbn) > 0 {
		r.IDs["isbn"] = isbn[:min(len(isbn), 5)]
	}
	for _, cl := range strs(it.Call) {
		if strings.HasPrefix(strings.ToUpper(cl), "Q") {
			r.Signals.LCC = addUnique(r.Signals.LCC, strings.Fields(strings.ToUpper(cl))[0])
		}
	}
	r.Signals.MathHeading = mathHeading(strs(it.Subject))
	return r
}

// Wikidata reads each work of Wikidata whose main subject (P921) is mathematics, a subclass of it, or an area of mathematics, and emits a random part of them.
type Wikidata struct{ Base string }

// Name gives wikidata.
func (Wikidata) Name() string { return "wikidata" }

// WikidataMathItems is the SPARQL pattern of the items of mathematics in ?s.
const WikidataMathItems = `{ ?s wdt:P279* wd:Q395 } UNION { ?s wdt:P31 wd:Q1936384 }`

const wikidataPage = 2000

const wikidataQuery = `SELECT ?x (SAMPLE(?t) AS ?title) (SAMPLE(?d) AS ?date) (SAMPLE(?doi) AS ?doi) (SAMPLE(?zb) AS ?zbmath) (SAMPLE(?ax) AS ?arxiv) (SAMPLE(?isbn) AS ?isbn)
 (GROUP_CONCAT(DISTINCT STRAFTER(STR(?s), "entity/"); separator=" ") AS ?subjects) (GROUP_CONCAT(DISTINCT STRAFTER(STR(?k), "entity/"); separator=" ") AS ?kinds)
WHERE {
 { SELECT DISTINCT ?x WHERE { %s ?x wdt:P921 ?s } ORDER BY ?x LIMIT %d OFFSET %d }
 ?x wdt:P921 ?s .
 OPTIONAL { ?x wdt:P1476 ?t } OPTIONAL { ?x wdt:P577 ?d } OPTIONAL { ?x wdt:P356 ?doi } OPTIONAL { ?x wdt:P894 ?zb }
 OPTIONAL { ?x wdt:P818 ?ax } OPTIONAL { ?x wdt:P212 ?isbn } OPTIONAL { ?x wdt:P31 ?k }
} GROUP BY ?x`

type sparqlResult struct {
	Results struct {
		Bindings []map[string]struct {
			Value string `json:"value"`
		} `json:"bindings"`
	} `json:"results"`
}

// The kinds of the classes of Wikidata.
var wdKinds = map[string]string{
	"Q13442814": "article", "Q580922": "preprint", "Q571": "monograph", "Q7725634": "monograph", "Q47461344": "monograph",
	"Q83790": "textbook", "Q1266946": "thesis", "Q187685": "thesis", "Q1980247": "chapter", "Q23927052": "conference-paper",
	"Q1143604": "proceedings", "Q5633421": "journal", "Q7397": "software",
}

// Sample reads pages of the works in the order of the item and emits about n of them.
func (wd Wikidata) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	info := Info{Filter: "P921 in " + WikidataMathItems}
	var all []census.Record
	for off := 0; ; off += wikidataPage {
		var res sparqlResult
		q := url.Values{"query": {fmt.Sprintf(wikidataQuery, WikidataMathItems, wikidataPage, off)}, "format": {"json"}}
		if err := c.JSON(ctx, wd.Base+"?"+q.Encode(), &res); err != nil {
			return info, wrap("wikidata", err)
		}
		for _, b := range res.Results.Bindings {
			all = append(all, wdRecord(func(k string) string { return b[k].Value }))
		}
		if len(res.Results.Bindings) < wikidataPage {
			break
		}
	}
	info.Total = len(all)
	return info, subsample(all, n, seed, emit)
}

func wdRecord(get func(string) string) census.Record {
	q := strings.TrimPrefix(get("x"), "http://www.wikidata.org/entity/")
	r := census.Record{NativeID: q, Kind: "other", Title: get("title"), IDs: resource.IDs{"wikidata": {q}}}
	if d := get("date"); len(d) >= 10 {
		r.Date = strings.TrimSuffix(strings.TrimSuffix(d[:10], "-01"), "-01")
	}
	for _, k := range strings.Fields(get("kinds")) {
		if kind, ok := wdKinds[k]; ok {
			r.Kind = kind
			break
		}
	}
	if v := get("doi"); v != "" {
		r.IDs["doi"] = resource.Values{strings.ToLower(v)}
	}
	if v := get("arxiv"); v != "" {
		r.IDs["arxiv"] = resource.Values{v}
	}
	if v := get("isbn"); v != "" {
		r.IDs["isbn"] = resource.Values{v}
	}
	if v := get("zbmath"); v != "" {
		r.IDs["zbl"] = resource.Values{v}
		r.Signals.ZBMath = true
	}
	r.Signals.MainSubjects = strings.Fields(get("subjects"))
	return r
}

// Numdam harvests each article of Numdam through OAI-PMH in the format eudml-article2, and emits a random part of them. Most articles have the id of their review in zbMATH Open.
type Numdam struct{ Base string }

// Name gives numdam.
func (Numdam) Name() string { return "numdam" }

type numdamArticle struct {
	Lang  string `xml:"lang,attr"`
	Front struct {
		Meta struct {
			Title   string `xml:"title-group>article-title"`
			Contrib []struct {
				Surname string `xml:"name>surname"`
				Given   string `xml:"name>given-names"`
			} `xml:"contrib-group>contrib"`
			Links []struct {
				Type  string `xml:"ext-link-type,attr"`
				Value string `xml:",chardata"`
			} `xml:"ext-link"`
			IDs []struct {
				Type  string `xml:"pub-id-type,attr"`
				Value string `xml:",chardata"`
			} `xml:"article-id"`
			Year string `xml:"pub-date>year"`
		} `xml:"article-meta"`
	} `xml:"front"`
}

type numdamPage struct {
	Error   string `xml:"error"`
	Records []struct {
		Header struct {
			ID     string `xml:"identifier"`
			Status string `xml:"status,attr"`
		} `xml:"header"`
		Article numdamArticle `xml:"metadata>article"`
	} `xml:"ListRecords>record"`
	Token struct {
		Value string `xml:",chardata"`
	} `xml:"ListRecords>resumptionToken"`
}

// numdamYear finds the year in an id of Mathdoc, as 1994 in MSH_1994__126__5_0.
var numdamYear = regexp.MustCompile(`_((?:1[5-9]|20)[0-9]{2})_`)

// Sample harvests each page until the resumption token ends.
func (nd Numdam) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	q := url.Values{"verb": {"ListRecords"}, "metadataPrefix": {"eudml-article2"}}
	info := Info{Filter: "each article, OAI-PMH format eudml-article2"}
	var all []census.Record
	for page := 0; page < 10000; page++ {
		body, err := c.Get(ctx, nd.Base+"?"+q.Encode())
		if err != nil {
			return info, wrap("numdam", err)
		}
		var p numdamPage
		if err := xml.Unmarshal(body, &p); err != nil {
			return info, fmt.Errorf("numdam: page %d: %w", page, err)
		}
		if p.Error != "" {
			return info, fmt.Errorf("numdam: %s", strings.TrimSpace(p.Error))
		}
		for _, rec := range p.Records {
			if rec.Header.Status == "deleted" {
				continue
			}
			all = append(all, numdamRecord(rec.Header.ID, rec.Article))
		}
		if p.Token.Value == "" {
			break
		}
		q = url.Values{"verb": {"ListRecords"}, "resumptionToken": {strings.TrimSpace(p.Token.Value)}}
	}
	info.Total = len(all)
	return info, subsample(all, n, seed, emit)
}

func numdamRecord(oai string, a numdamArticle) census.Record {
	m := a.Front.Meta
	id := strings.TrimPrefix(oai, "oai:numdam.org:")
	r := census.Record{NativeID: id, Kind: "article", Title: strings.Join(strings.Fields(m.Title), " "), Language: Lang(a.Lang), IDs: resource.IDs{"numdam": {id}}}
	r.Date = strings.TrimSpace(m.Year)
	if r.Date == "" {
		if m := numdamYear.FindStringSubmatch(id); m != nil {
			r.Date = m[1]
		}
	}
	for _, c := range m.Contrib {
		name := strings.TrimSpace(c.Surname)
		if g := strings.TrimSpace(c.Given); g != "" {
			name += ", " + g
		}
		if name != "" {
			r.Authors = append(r.Authors, name)
		}
	}
	for _, l := range m.Links {
		v := strings.TrimSpace(l.Value)
		switch l.Type {
		case "zbl-item-id":
			r.IDs["zbl"] = addUnique(r.IDs["zbl"], v)
			r.Signals.ZBMath = true
		case "eudml-item-id":
			r.IDs["eudml"] = addUnique(r.IDs["eudml"], strings.TrimPrefix(v, "urn:eudml:doc:"))
		}
	}
	for _, x := range m.IDs {
		if x.Type == "doi" {
			r.IDs["doi"] = resource.Values{strings.ToLower(strings.TrimSpace(x.Value))}
		}
	}
	return r
}
