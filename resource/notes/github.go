// Package notes finds candidate lecture notes for the catalogue (spec corpus resources RS7.1). The repository scan of step 4 reads the search API of GitHub and gives repositories with LaTeX or PreTeXt and a licence whose name, description or topics name a mathematical subject and notes. A hit is a candidate and not a resource: a person or the judge confirms it.
//
// The scan uses the public search API with no key and no token. That API allows about 10 search requests a minute, so the scan waits Gap between two requests. It sends no email. It never clones a repository: it reads only the fields of the search results.
//
// GitLab has no fetcher here, because its search API needs a token.
package notes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/mdrec"
)

// Rules is the version of the queries and the filters of the scan. A change to Queries, Subjects or Keep gives a new version.
const Rules = "github-notes/1"

// SearchAPI is the repository search of the GitHub REST API.
const SearchAPI = "https://api.github.com/search/repositories"

// Gap is the shortest time between two requests. The search API allows 10 requests a minute with no token, and a gap of 7 seconds gives at most 9.
const Gap = 7 * time.Second

// MinGap is the shortest gap that the scan accepts: 6 seconds is 10 requests a minute.
const MinGap = 6 * time.Second

// PerPage is the number of repositories in one page of results, the most that the API gives.
const PerPage = 100

// MaxPages is the most pages of one query. The API gives the first 1000 results of a query and no more.
const MaxPages = 10

// File is the file of the candidates in the tree, a file of records (package mdrec).
const File = "build/resources/candidates/github-notes.md"

// Queries are the search queries of the scan. The first two find LaTeX repositories, which GitHub names TeX. GitHub has no language for PreTeXt, so the last two find PreTeXt repositories by their topic and by their README.
var Queries = []string{
	`"lecture notes" in:name,description language:TeX`,
	`"course notes" in:name,description language:TeX`,
	`notes in:name,description topic:pretext`,
	`"lecture notes" pretext in:readme`,
}

// Subjects are the words and phrases that name a mathematical subject. Keep looks for them in the name, the description and the topics of a repository.
var Subjects = []string{
	"mathematics", "mathematical", "math", "maths",
	"algebra", "linear algebra", "abstract algebra", "commutative algebra", "homological algebra",
	"analysis", "real analysis", "complex analysis", "functional analysis", "harmonic analysis", "numerical analysis",
	"calculus", "geometry", "topology", "number theory", "combinatorics", "graph theory",
	"probability", "statistics", "stochastic", "measure theory", "set theory", "category theory", "model theory",
	"differential equations", "pde", "pdes", "ode", "odes", "dynamical systems", "ergodic theory",
	"group theory", "representation theory", "lie groups", "lie algebras", "galois theory", "galois",
	"manifolds", "homology", "cohomology", "homotopy", "sheaves", "schemes", "riemannian", "symplectic",
	"logic", "proof", "proofs", "optimization", "operator algebras", "knot theory",
}

// Candidate is one repository that the scan found. Status is candidate when the scan writes it. A person or the judge sets it to confirmed or rejected, and a later scan keeps that value.
type Candidate struct {
	Repo        string `json:"repo"`                  // owner/name
	URL         string `json:"url"`                   // the page of the repository
	Licence     string `json:"licence"`               // the SPDX id that GitHub gives, or NOASSERTION for a licence file that GitHub does not know
	Language    string `json:"language,omitempty"`    // the main language that GitHub gives
	Description string `json:"description,omitempty"` // the description of the repository
	Topics      string `json:"topics,omitempty"`      // the topics, with a space between two
	Stars       int    `json:"stars"`                 // the stars when the scan read the repository
	Date        string `json:"date"`                  // the day of the last push, YYYY-MM-DD
	Subject     string `json:"subject"`               // the first word of Subjects that the repository names
	Query       string `json:"query"`                 // the query that found the repository first
	Found       string `json:"found"`                 // the day of the scan that found the repository first
	Rules       string `json:"rules"`                 // the version of the scan that found the repository first
	Status      string `json:"status"`                // candidate, confirmed or rejected
}

// Repo is one repository of the search results, with the fields that the scan reads.
type Repo struct {
	FullName    string   `json:"full_name"`
	HTMLURL     string   `json:"html_url"`
	Description string   `json:"description"`
	Language    string   `json:"language"`
	Topics      []string `json:"topics"`
	Stars       int      `json:"stargazers_count"`
	PushedAt    string   `json:"pushed_at"`
	Fork        bool     `json:"fork"`
	License     *struct {
		Key    string `json:"key"`
		SPDXID string `json:"spdx_id"`
	} `json:"license"`
}

// words gives the text in lower case with one space between two words, and a space at each end, so that a phrase matches only whole words.
func words(s string) string {
	f := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	return " " + strings.Join(f, " ") + " "
}

// text gives the name, the description and the topics of a repository, made by words. The name is without its owner.
func text(r Repo) string {
	_, name, _ := strings.Cut(r.FullName, "/")
	return words(name + " " + r.Description + " " + strings.Join(r.Topics, " "))
}

// Why gives the reason that the scan drops a repository, or an empty string when the scan keeps it.
func Why(r Repo) string {
	text := text(r)
	switch {
	case r.Fork:
		return "fork"
	case r.License == nil || r.License.Key == "":
		return "no licence"
	case !strings.Contains(text, " notes ") && !strings.Contains(text, " lectures "):
		return "no notes"
	case subject(text) == "":
		return "no subject"
	}
	return ""
}

// subject gives the first word of Subjects that the text, made by words, names.
func subject(text string) string {
	for _, s := range Subjects {
		if strings.Contains(text, " "+s+" ") {
			return s
		}
	}
	return ""
}

// Keep makes the candidate of a repository that query found on the day found, and reports whether the scan keeps it (Why).
func Keep(r Repo, query, found string) (Candidate, bool) {
	if Why(r) != "" {
		return Candidate{}, false
	}
	licence := r.License.SPDXID
	if licence == "" {
		licence = "NOASSERTION"
	}
	date := r.PushedAt
	if len(date) >= 10 {
		date = date[:10]
	}
	return Candidate{
		Repo:        r.FullName,
		URL:         r.HTMLURL,
		Licence:     licence,
		Language:    r.Language,
		Description: strings.Join(strings.Fields(r.Description), " "),
		Topics:      strings.Join(r.Topics, " "),
		Stars:       r.Stars,
		Date:        date,
		Subject:     subject(text(r)),
		Query:       query,
		Found:       found,
		Rules:       Rules,
		Status:      "candidate",
	}, true
}

// Scanner reads the search API.
type Scanner struct {
	HTTP      *http.Client
	UserAgent string        // names math2 and its home page, never an email
	Base      string        // the search API, SearchAPI when empty
	Gap       time.Duration // the shortest time between two requests, at least MinGap for the live API
	Pages     int           // the most pages of one query, MaxPages when 0
	Log       io.Writer     // a line for each request, or nil

	last time.Time                                        // the time of the last request
	wait func(ctx context.Context, d time.Duration) error // waits d, a test sets it
}

// Counts are the counts of one scan.
type Counts struct {
	Requests int            // the requests to the API
	Seen     int            // the repositories in the results, with repeats
	Dropped  map[string]int // the repositories that the scan dropped, by reason
	Kept     int            // the candidates, with no repeats
}

// Scan runs each query, page by page, and gives the candidates in the order of the repository name. A repository that two queries find is one candidate, with the first query. The day found goes into each candidate.
func (s *Scanner) Scan(ctx context.Context, queries []string, found string) ([]Candidate, Counts, error) {
	c := Counts{Dropped: map[string]int{}}
	seen := map[string]bool{}
	var out []Candidate
	pages := s.Pages
	if pages <= 0 || pages > MaxPages {
		pages = MaxPages
	}
	for _, q := range queries {
		for page := 1; page <= pages; page++ {
			var res struct {
				Total int    `json:"total_count"`
				Items []Repo `json:"items"`
			}
			if err := s.get(ctx, q, page, &res); err != nil {
				return out, c, err
			}
			c.Requests++
			for _, r := range res.Items {
				c.Seen++
				if seen[r.FullName] {
					continue
				}
				if why := Why(r); why != "" {
					c.Dropped[why]++
					continue
				}
				seen[r.FullName] = true
				cand, _ := Keep(r, q, found)
				out = append(out, cand)
			}
			if len(res.Items) < PerPage || page*PerPage >= min(res.Total, MaxPages*PerPage) {
				break
			}
		}
	}
	slices.SortFunc(out, func(a, b Candidate) int { return strings.Compare(a.Repo, b.Repo) })
	c.Kept = len(out)
	return out, c, nil
}

// get gets one page of the results of a query, after the gap. On the statuses 403 and 429 of the rate limit it waits for the time that the API gives, at most two minutes, and tries again two times.
func (s *Scanner) get(ctx context.Context, query string, page int, v any) error {
	base := s.Base
	if base == "" {
		base = SearchAPI
	}
	u := base + "?" + url.Values{"q": {query}, "per_page": {strconv.Itoa(PerPage)}, "page": {strconv.Itoa(page)}}.Encode()
	for try := 0; ; try++ {
		if err := s.pace(ctx, s.Gap-time.Since(s.last)); err != nil {
			return err
		}
		s.last = time.Now()
		if s.Log != nil {
			fmt.Fprintf(s.Log, "GET %s\n", u)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", s.UserAgent)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		client := s.HTTP
		if client == nil {
			client = &http.Client{Timeout: time.Minute}
		}
		resp, err := client.Do(req) // #nosec G107 G704 -- the URL is the search API with a query of Queries.
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
		limited := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0"
		switch {
		case resp.StatusCode == http.StatusOK:
			if resp.Header.Get("X-RateLimit-Remaining") == "0" {
				if err := s.pace(ctx, reset(resp.Header)); err != nil {
					return err
				}
			}
			if err := json.Unmarshal(body, v); err != nil {
				return fmt.Errorf("%s: %w", u, err)
			}
			return nil
		case limited && try < 2:
			if err := s.pace(ctx, reset(resp.Header)); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: status %d: %.200s", u, resp.StatusCode, body)
		}
	}
}

// reset gives the time to wait that the headers of a response of the rate limit give: Retry-After, or the time to X-RateLimit-Reset, at most two minutes. With neither header it is one minute.
func reset(h http.Header) time.Duration {
	if n, err := strconv.Atoi(h.Get("Retry-After")); err == nil && n >= 0 {
		return min(time.Duration(n)*time.Second, 2*time.Minute)
	}
	if n, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		return min(max(time.Until(time.Unix(n, 0))+time.Second, 0), 2*time.Minute)
	}
	return time.Minute
}

// pace waits d, or returns at once when d is not positive.
func (s *Scanner) pace(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	if s.wait != nil {
		return s.wait(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Read reads the candidates of the file at path. A file that is not there has no candidates.
func Read(path string) ([]Candidate, error) {
	recs, err := mdrec.Read(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return mdrec.Unmarshal[Candidate](recs)
}

// Merge adds the candidates of a new scan to the candidates of the file. A repository that the file has keeps its query, its day found, its version and its status, and gets the new licence, language, description, topics, stars and date. A repository that the new scan does not find stays. It gives the candidates in the order of the repository name and the number of new candidates.
func Merge(old, scan []Candidate) ([]Candidate, int) {
	at := map[string]int{}
	out := slices.Clone(old)
	for i, c := range out {
		at[c.Repo] = i
	}
	added := 0
	for _, c := range scan {
		i, ok := at[c.Repo]
		if !ok {
			at[c.Repo] = len(out)
			out = append(out, c)
			added++
			continue
		}
		o := &out[i]
		o.URL, o.Licence, o.Language, o.Description, o.Topics, o.Stars, o.Date, o.Subject = c.URL, c.Licence, c.Language, c.Description, c.Topics, c.Stars, c.Date, c.Subject
	}
	slices.SortFunc(out, func(a, b Candidate) int { return strings.Compare(a.Repo, b.Repo) })
	return out, added
}

// Write writes the candidates to the file at path, as a file of records.
func Write(path string, cs []Candidate) error {
	recs, err := mdrec.Marshal(cs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return mdrec.Write(path, recs)
}
