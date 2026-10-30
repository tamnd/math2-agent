package fetch

import (
	"context"
	"errors"
	"fmt"
	"html"
	"math"
	"math/rand/v2"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census"
)

// The bases of the search APIs of the catalogues of phases R2 and R3.
const (
	LoCSRUBase          = "http://lx2.loc.gov:210/LCDB"
	SemanticScholarBase = "https://api.semanticscholar.org/graph/v1"
	MathNetRuBase       = "https://www.mathnet.ru"
)

// LoC samples the catalogue of the Library of Congress through its SRU server, with the query dc.subject=mathematics. The records are MARC records, as in the MARC files of MDSConnect.
type LoC struct {
	Base     string
	LastYear int // the last year of the sample, or 0 for this year
}

// Name gives loc.
func (LoC) Name() string { return "loc" }

// LoCWindow is the most records that a search of the SRU server gives, over all its pages.
const LoCWindow = 10000

// LoCFirstYear is the first year of the sample.
const LoCFirstYear = 1450

const locQuery = "dc.subject=mathematics"

type marcField struct {
	Tag  string `xml:"tag,attr"`
	Text string `xml:",chardata"`
	Sub  []struct {
		Code string `xml:"code,attr"`
		Text string `xml:",chardata"`
	} `xml:"subfield"`
}

type marcRecord struct {
	Leader  string      `xml:"leader"`
	Control []marcField `xml:"controlfield"`
	Data    []marcField `xml:"datafield"`
}

type sruPage struct {
	Total   int          `xml:"numberOfRecords"`
	Records []marcRecord `xml:"records>record>recordData>record"`
	Diag    string       `xml:"diagnostics>diagnostic>details"`
}

// Sample takes a sample in strata of the date of the record, because the server pages only inside a window of 10000 records. It reads the count of each decade with the truncated date, as dc.date=195*, and the count of each year of a decade with more than 10000 records. It gives each stratum a part of n by its count, and takes random pages of the stratum. The records with no date are not in the sample.
func (l LoC) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	get := func(date string, start, size int) (sruPage, error) {
		v := url.Values{"operation": {"searchRetrieve"}, "version": {"1.1"}, "recordSchema": {"marcxml"},
			"query": {locQuery + " and dc.date=" + date}, "startRecord": {strconv.Itoa(start)}, "maximumRecords": {strconv.Itoa(size)}}
		var p sruPage
		body, err := c.Get(ctx, l.Base+"?"+v.Encode())
		if err != nil {
			return p, err
		}
		if err := laxUnmarshal(body, &p); err != nil {
			return p, err
		}
		if p.Diag != "" {
			return p, errors.New(strings.TrimSpace(p.Diag))
		}
		return p, nil
	}
	last := l.LastYear
	if last == 0 {
		last = time.Now().Year()
	}
	type stratum struct {
		date  string
		count int
	}
	var strata []stratum
	total := 0
	for d := LoCFirstYear / 10 * 10; d <= last; d += 10 {
		p, err := get(strconv.Itoa(d/10)+"*", 1, 0)
		if err != nil {
			return Info{}, wrap("loc", err)
		}
		if p.Total <= LoCWindow {
			if p.Total > 0 {
				strata = append(strata, stratum{strconv.Itoa(d/10) + "*", p.Total})
				total += p.Total
			}
			continue
		}
		for y := d; y < d+10 && y <= last; y++ {
			py, err := get(strconv.Itoa(y), 1, 0)
			if err != nil {
				return Info{}, wrap("loc", err)
			}
			if py.Total > 0 {
				strata = append(strata, stratum{strconv.Itoa(y), py.Total})
				total += py.Total
			}
		}
	}
	info := Info{Total: total, Filter: fmt.Sprintf("%s, with a date of %d to %d, in strata of the decade or the year", locQuery, LoCFirstYear/10*10, last)}
	for i, s := range strata {
		k := max(1, int(math.Round(float64(n)*float64(s.count)/float64(max(total, 1)))))
		size := min(100, max(10, k))
		window := min(s.count, LoCWindow)
		var recs []census.Record
		for _, pg := range pages(window, size, (k+size-1)/size, seed+uint64(i)) {
			p, err := get(s.date, pg*size+1, size)
			if err != nil {
				return info, wrap("loc", err)
			}
			for _, m := range p.Records {
				recs = append(recs, marcCensus(m))
			}
		}
		for _, r := range recs {
			r.Weight = float64(s.count) / float64(len(recs))
			if err := emit(r); err != nil {
				return info, err
			}
		}
	}
	return info, nil
}

// sub gives the first subfield of a field with the code.
func (f marcField) sub(code string) string {
	for _, s := range f.Sub {
		if s.Code == code {
			return strings.TrimSpace(s.Text)
		}
	}
	return ""
}

// marcKinds maps the type and the level of the leader of a MARC record to the kinds of resources.
var marcKinds = map[string]string{"am": "monograph", "as": "journal", "aa": "chapter", "ab": "article", "tm": "thesis"}

// marcCensus makes a census record from a MARC record.
func marcCensus(m marcRecord) census.Record {
	r := census.Record{Kind: "other", IDs: resource.IDs{}}
	if len(m.Leader) >= 8 {
		if k, ok := marcKinds[m.Leader[6:8]]; ok {
			r.Kind = k
		}
	}
	for _, f := range m.Control {
		v := f.Text
		switch f.Tag {
		case "001":
			r.NativeID = strings.TrimSpace(v)
		case "008":
			if len(v) >= 11 && dcYear.MatchString(v[7:11]) {
				r.Date = v[7:11]
			}
			if len(v) >= 38 {
				r.Language = Lang(v[35:38])
			}
		}
	}
	var heads []string
	for _, f := range m.Data {
		switch f.Tag {
		case "010":
			if v := strings.ReplaceAll(f.sub("a"), " ", ""); v != "" {
				r.IDs["lccn"] = addUnique(r.IDs["lccn"], v)
			}
		case "020":
			if v := strings.Fields(f.sub("a")); len(v) > 0 && len(r.IDs["isbn"]) < 5 {
				r.IDs["isbn"] = addUnique(r.IDs["isbn"], v[0])
			}
		case "035":
			if v, ok := strings.CutPrefix(f.sub("a"), "(OCoLC)"); ok {
				r.IDs["oclc"] = addUnique(r.IDs["oclc"], strings.TrimLeft(strings.TrimSpace(v), "ocmn"))
			}
		case "050":
			if v := strings.Fields(f.sub("a")); len(v) > 0 {
				r.Signals.LCC = addUnique(r.Signals.LCC, v[0])
			}
		case "082":
			if v := strings.Trim(f.sub("a"), "/ "); v != "" {
				r.Signals.DDC = addUnique(r.Signals.DDC, strings.ReplaceAll(v, "/", ""))
			}
		case "084":
			if strings.HasPrefix(strings.ToLower(f.sub("2")), "msc") {
				r.Signals.MSC = addUnique(r.Signals.MSC, f.sub("a"))
			}
		case "100", "110", "700", "710":
			if v := strings.TrimRight(f.sub("a"), ",. "); v != "" {
				r.Authors = append(r.Authors, v)
			}
		case "245":
			t := strings.TrimRight(f.sub("a"), " /:;.=")
			if b := strings.TrimRight(f.sub("b"), " /:;.="); b != "" {
				t += ": " + b
			}
			r.Title = t
		case "650":
			heads = append(heads, strings.TrimRight(f.sub("a"), ". "))
		}
	}
	r.Signals.MathHeading = MathHeading(heads)
	return r
}

// SemanticScholar samples the papers of Semantic Scholar in the field of study Mathematics through the bulk search of the Graph API, with no key. The bulk search gives the papers of a query in the order of the paper id, which is a hash, so the first papers of a stratum are a random part of it. The seed has no effect.
type SemanticScholar struct {
	Base     string
	LastYear int // the last year of the sample, or 0 for this year
}

// Name gives semanticscholar.
func (SemanticScholar) Name() string { return "semanticscholar" }

// S2FirstYear is the first year of the sample.
const S2FirstYear = 1800

const s2Fields = "title,year,externalIds,authors,publicationTypes"

type s2Paper struct {
	PaperID string         `json:"paperId"`
	Title   string         `json:"title"`
	Year    int            `json:"year"`
	IDs     map[string]any `json:"externalIds"`
	Types   []string       `json:"publicationTypes"`
	Authors []struct {
		Name string `json:"name"`
	} `json:"authors"`
}

type s2Page struct {
	Total int       `json:"total"`
	Token string    `json:"token"`
	Data  []s2Paper `json:"data"`
}

var s2Kinds = map[string]string{
	"JournalArticle": "article", "Conference": "conference-paper", "Book": "monograph", "BookSection": "chapter", "Review": "survey",
	"Dataset": "dataset", "Editorial": "other", "LettersAndComments": "other", "Study": "article",
}

// Sample reads the count of each decade, gives each decade a part of n by its count, and reads the first papers of the decade.
func (s SemanticScholar) Sample(ctx context.Context, c *Client, n int, _ uint64, emit func(census.Record) error) (Info, error) {
	get := func(years, fields, token string) (s2Page, error) {
		v := url.Values{"query": {"*"}, "fieldsOfStudy": {"Mathematics"}, "year": {years}, "fields": {fields}}
		if token != "" {
			v.Set("token", token)
		}
		var p s2Page
		err := c.JSON(ctx, s.Base+"/paper/search/bulk?"+v.Encode(), &p)
		return p, err
	}
	last := s.LastYear
	if last == 0 {
		last = time.Now().Year()
	}
	decades := map[int]int{}
	total := 0
	for d := S2FirstYear; d <= last; d += 10 {
		p, err := get(fmt.Sprintf("%d-%d", d, min(d+9, last)), "paperId", "")
		if err != nil {
			return Info{}, wrap("semanticscholar", err)
		}
		decades[d] = p.Total
		total += p.Total
	}
	info := Info{Total: total, Filter: fmt.Sprintf("fieldsOfStudy Mathematics, %d to %d, in strata of the decade", S2FirstYear, last)}
	for d := S2FirstYear; d <= last; d += 10 {
		t := decades[d]
		if t == 0 {
			continue
		}
		k := min(t, max(1, int(math.Round(float64(n)*float64(t)/float64(max(total, 1))))))
		var recs []census.Record
		token := ""
		for len(recs) < k {
			p, err := get(fmt.Sprintf("%d-%d", d, min(d+9, last)), s2Fields, token)
			if err != nil {
				return info, wrap("semanticscholar", err)
			}
			for _, x := range p.Data {
				if len(recs) < k {
					recs = append(recs, s2Record(x))
				}
			}
			if token = p.Token; token == "" || len(p.Data) == 0 {
				break
			}
		}
		for _, r := range recs {
			r.Weight = float64(t) / float64(len(recs))
			if err := emit(r); err != nil {
				return info, err
			}
		}
	}
	return info, nil
}

func s2Record(x s2Paper) census.Record {
	r := census.Record{NativeID: x.PaperID, Kind: "other", Title: x.Title, IDs: resource.IDs{}}
	if x.Year > 0 {
		r.Date = strconv.Itoa(x.Year)
	}
	for _, t := range x.Types {
		if k, ok := s2Kinds[t]; ok {
			r.Kind = k
			break
		}
	}
	for _, a := range x.Authors {
		r.Authors = append(r.Authors, a.Name)
	}
	str := func(k string) string {
		switch v := x.IDs[k].(type) {
		case string:
			return v
		case float64:
			return strconv.FormatInt(int64(v), 10)
		}
		return ""
	}
	if v := str("CorpusId"); v != "" {
		r.IDs["s2"] = resource.Values{v}
	}
	if v := str("DOI"); v != "" {
		r.IDs["doi"] = resource.Values{strings.ToLower(v)}
	}
	if v := str("ArXiv"); v != "" {
		r.IDs["arxiv"] = resource.Values{v}
	}
	return r
}

// MathNetRu samples the journals of Math-Net.Ru from their pages, in clusters of an issue. It reads the list of the journals, the list of the issues of each journal, and the pages of random issues, until it has about n articles. The robots.txt of Math-Net.Ru asks for 2 seconds between two requests, so the sample makes the gap of the client at least the field Gap.
type MathNetRu struct {
	Base string
	Gap  time.Duration // the shortest time between two requests, MathNetRuGap for the site
}

// Name gives mathnetru.
func (MathNetRu) Name() string { return "mathnetru" }

// MathNetRuGap is the crawl delay of the robots.txt of Math-Net.Ru.
const MathNetRuGap = 2 * time.Second

var (
	mnJournal = regexp.MustCompile(`journal\.phtml\?jrnid=([a-z0-9]+)`)
	mnIssue   = regexp.MustCompile(`archive\.phtml\?jrnid=([a-z0-9]+)&(?:amp;)?wshow=issue&[^\s'">]*`)
	mnYear    = regexp.MustCompile(`year=([0-9]{4})`)
	mnPaper   = regexp.MustCompile(`<a class=SLink href="/eng/([a-z]+[0-9]+)">`)
	mnDOI     = regexp.MustCompile(`doi\.org/(10\.[0-9]{4,9}/[^\s'"<>]+)`)
	mnTag     = regexp.MustCompile(`<[^>]*>`)
)

// get gets a page of Math-Net.Ru, which is in the encoding Windows-1251.
func (m MathNetRu) get(ctx context.Context, c *Client, path string) (string, error) {
	body, err := c.Get(ctx, m.Base+path)
	if err != nil {
		return "", err
	}
	text, err := charmap.Windows1251.NewDecoder().Bytes(body)
	if err != nil {
		return "", err
	}
	return string(text), nil
}

// Sample reads the issues of each journal, and the pages of the issues in a random order.
func (m MathNetRu) Sample(ctx context.Context, c *Client, n int, seed uint64, emit func(census.Record) error) (Info, error) {
	c.Gap = max(c.Gap, m.Gap)
	page, err := m.get(ctx, c, "/ej.phtml?option_lang=eng")
	if err != nil {
		return Info{}, wrap("mathnetru", err)
	}
	var journals []string
	for _, x := range mnJournal.FindAllStringSubmatch(page, -1) {
		journals = addUnique(journals, x[1])
	}
	var issues []string
	for _, j := range journals {
		page, err := m.get(ctx, c, "/php/archive.phtml?jrnid="+j+"&wshow=contents&option_lang=eng")
		if err != nil {
			return Info{}, wrap("mathnetru", err)
		}
		for _, x := range mnIssue.FindAllStringSubmatch(page, -1) {
			if x[1] == j {
				issues = addUnique(issues, strings.ReplaceAll(x[0], "&amp;", "&"))
			}
		}
	}
	info := Info{Filter: fmt.Sprintf("each article of %d journals, in clusters of an issue", len(journals))}
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) // #nosec G404 -- a sample needs a seeded generator, not a secure one.
	var all []census.Record
	read := 0
	for _, k := range rng.Perm(len(issues)) {
		if len(all) >= n {
			break
		}
		page, err := m.get(ctx, c, "/php/"+issues[k])
		if err != nil {
			return info, wrap("mathnetru", err)
		}
		read++
		year := ""
		if y := mnYear.FindStringSubmatch(issues[k]); y != nil {
			year = y[1]
		}
		all = append(all, mnRecords(page, year)...)
	}
	if read > 0 {
		info.Total = int(math.Round(float64(len(all)) * float64(len(issues)) / float64(read)))
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

// mnText gives the text of a part of a page, with no tags and no entities.
func mnText(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(mnTag.ReplaceAllString(s, ""))), " ")
}

// mnRecords reads the articles of the page of an issue. Each article is a link to /eng/<id> with the title, the authors after the link, and a block with the DOIs.
func mnRecords(page, year string) []census.Record {
	locs := mnPaper.FindAllStringSubmatchIndex(page, -1)
	var out []census.Record
	for i, loc := range locs {
		end := len(page)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		id := page[loc[2]:loc[3]]
		part := page[loc[1]:end]
		title, rest, _ := strings.Cut(part, "</a>")
		r := census.Record{NativeID: id, Kind: "article", Title: mnText(title), Date: year, IDs: resource.IDs{"mathnetru": {id}}}
		rest = strings.TrimPrefix(strings.TrimSpace(rest), "<br>")
		names, _, _ := strings.Cut(rest, "<div")
		for _, a := range strings.Split(mnText(names), ",") {
			if a = strings.TrimSpace(a); a != "" {
				r.Authors = append(r.Authors, a)
			}
		}
		for _, x := range mnDOI.FindAllStringSubmatch(part, -1) {
			r.IDs["doi"] = addUnique(r.IDs["doi"], strings.ToLower(x[1]))
		}
		out = append(out, r)
	}
	return out
}
