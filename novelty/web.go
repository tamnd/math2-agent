package novelty

import (
	"cmp"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// UserAgent is the User-Agent of a search on the web when the UserAgent of its [Web] is empty.
const UserAgent = "m2 (+https://github.com/tamnd/math2-agent)"

// MaxAnswer is the largest answer of a service that a search reads.
const MaxAnswer = 16 << 20

// Web is the client of a search of a service on the web. Each search sends one request.
type Web struct {
	Client    *http.Client // nil means http.DefaultClient
	URL       string       // the base URL of the service. Empty means the public service.
	UserAgent string       // the User-Agent of each request. Empty means [UserAgent].
	K         int          // the number of matches. Zero means [K].
}

func (w *Web) k() int { return cmp.Or(w.K, K) }

// get sends a GET request for the URL and decodes the answer with decode.
func (w *Web) get(ctx context.Context, u string, decode func(io.Reader) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", cmp.Or(w.UserAgent, UserAgent))
	resp, err := cmp.Or(w.Client, http.DefaultClient).Do(req) // #nosec G107 G704 -- the URL is the URL of the service that the caller chose.
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s gives %s", req.URL.Host, resp.Status)
	}
	return decode(io.LimitReader(resp.Body, MaxAnswer))
}

// ZbMATH searches the documents of zbMATH Open with its REST interface.
type ZbMATH struct{ Web }

// ZbMATHURL is the REST interface of zbMATH Open.
const ZbMATHURL = "https://api.zbmath.org/v1/"

// Name returns zbmath.
func (z *ZbMATH) Name() string { return "zbmath" }

// Search returns the documents that the words of the claim find, with their titles for the judge.
func (z *ZbMATH) Search(ctx context.Context, c Claim) (string, []Match, error) {
	q := c.Query()
	u := cmp.Or(z.URL, ZbMATHURL) + "document/_search?" + url.Values{"search_string": {q}, "page": {"0"}, "results_per_page": {strconv.Itoa(z.k())}}.Encode()
	var a struct {
		Result []struct {
			ID    json.Number `json:"id"`
			Title struct {
				Title string `json:"title"`
			} `json:"title"`
			URL string `json:"zbmath_url"`
		} `json:"result"`
	}
	if err := z.get(ctx, u, func(r io.Reader) error { return json.NewDecoder(r).Decode(&a) }); err != nil {
		return q, nil, fmt.Errorf("novelty: zbmath: %w", err)
	}
	var out []Match
	for _, d := range a.Result {
		if d.ID == "" {
			continue
		}
		out = append(out, Match{Source: z.Name(), ID: d.ID.String(), URL: d.URL, Title: d.Title.Title})
	}
	return q, out, nil
}

// ArXiv searches the papers of arXiv with its API. The judge gets the title and the abstract of each paper.
type ArXiv struct{ Web }

// ArXivURL is the API of arXiv.
const ArXivURL = "https://export.arxiv.org/api/query"

// Name returns arxiv.
func (a *ArXiv) Name() string { return "arxiv" }

var arxivVersion = regexp.MustCompile(`v[0-9]+$`)

// Search returns the papers that have each word of the query of the claim.
func (a *ArXiv) Search(ctx context.Context, c Claim) (string, []Match, error) {
	q := c.Query()
	var terms []string
	for _, w := range strings.Fields(q) {
		terms = append(terms, "all:"+strings.Trim(w, `"`))
	}
	u := cmp.Or(a.URL, ArXivURL) + "?" + url.Values{"search_query": {strings.Join(terms, " AND ")}, "start": {"0"}, "max_results": {strconv.Itoa(a.k())}}.Encode()
	var feed struct {
		Entries []struct {
			ID      string `xml:"id"`
			Title   string `xml:"title"`
			Summary string `xml:"summary"`
		} `xml:"http://www.w3.org/2005/Atom entry"`
	}
	if err := a.get(ctx, u, func(r io.Reader) error { return xml.NewDecoder(r).Decode(&feed) }); err != nil {
		return q, nil, fmt.Errorf("novelty: arxiv: %w", err)
	}
	var out []Match
	for _, e := range feed.Entries {
		_, id, ok := strings.Cut(strings.TrimSpace(e.ID), "arxiv.org/abs/")
		if !ok {
			continue
		}
		id = arxivVersion.ReplaceAllString(id, "")
		out = append(out, Match{Source: a.Name(), ID: id, URL: "https://arxiv.org/abs/" + id, Title: strings.Join(strings.Fields(e.Title), " "), Text: strings.Join(strings.Fields(e.Summary), " ")})
	}
	return q, out, nil
}

// StackExchange searches the questions of a site of Stack Exchange with its API, for example mathoverflow or math.
type StackExchange struct {
	Web
	Site string // the site, for example mathoverflow. Empty means mathoverflow.
}

// StackExchangeURL is the API of Stack Exchange.
const StackExchangeURL = "https://api.stackexchange.com/2.3/"

// Name returns the site, for example mathoverflow.
func (s *StackExchange) Name() string { return cmp.Or(s.Site, "mathoverflow") }

// Search returns the questions that the words of the claim find, best first, with their titles for the judge.
func (s *StackExchange) Search(ctx context.Context, c Claim) (string, []Match, error) {
	q := c.Query()
	u := cmp.Or(s.URL, StackExchangeURL) + "search/advanced?" + url.Values{"order": {"desc"}, "sort": {"relevance"}, "q": {q}, "site": {s.Name()}, "pagesize": {strconv.Itoa(s.k())}}.Encode()
	var a struct {
		Items []struct {
			ID       int64  `json:"question_id"`
			Title    string `json:"title"`
			Link     string `json:"link"`
			Answered bool   `json:"is_answered"`
		} `json:"items"`
	}
	if err := s.get(ctx, u, func(r io.Reader) error { return json.NewDecoder(r).Decode(&a) }); err != nil {
		return q, nil, fmt.Errorf("novelty: %s: %w", s.Name(), err)
	}
	var out []Match
	for _, it := range a.Items {
		m := Match{Source: s.Name(), ID: strconv.FormatInt(it.ID, 10), URL: it.Link, Title: html.UnescapeString(it.Title)}
		if it.Answered {
			m.Status = "answered"
		}
		out = append(out, m)
	}
	return q, out, nil
}

// OEIS searches the sequences of the OEIS by their terms. It applies only to a claim with terms.
type OEIS struct{ Web }

// OEISURL is the home of the OEIS.
const OEISURL = "https://oeis.org/"

// Name returns oeis.
func (o *OEIS) Name() string { return "oeis" }

// Search returns the sequences that have the terms of the claim, with their names for the judge.
func (o *OEIS) Search(ctx context.Context, c Claim) (string, []Match, error) {
	if len(c.Terms) == 0 {
		return "", nil, nil
	}
	q := strings.Join(c.Terms, ",")
	u := cmp.Or(o.URL, OEISURL) + "search?" + url.Values{"q": {q}, "fmt": {"json"}}.Encode()
	type entry struct {
		Number int    `json:"number"`
		Name   string `json:"name"`
		Data   string `json:"data"`
	}
	var entries []entry
	err := o.get(ctx, u, func(r io.Reader) error {
		var raw json.RawMessage
		if err := json.NewDecoder(r).Decode(&raw); err != nil {
			return err
		}
		// The answer is a list of entries, or null when no entry matches. An older form has the list in results.
		if s := strings.TrimSpace(string(raw)); strings.HasPrefix(s, "{") {
			var old struct {
				Results []entry `json:"results"`
			}
			err := json.Unmarshal(raw, &old)
			entries = old.Results
			return err
		}
		return json.Unmarshal(raw, &entries)
	})
	if err != nil {
		return q, nil, fmt.Errorf("novelty: oeis: %w", err)
	}
	var out []Match
	for _, e := range entries[:min(len(entries), o.k())] {
		id := fmt.Sprintf("A%06d", e.Number)
		out = append(out, Match{Source: o.Name(), ID: id, URL: "https://oeis.org/" + id, Title: e.Name, Text: e.Name + "\n\n" + e.Data})
	}
	return q, out, nil
}
