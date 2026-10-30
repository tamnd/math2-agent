package contests

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
)

// KvantAdapter reads the problem column "Задачник «Кванта»" of the journal Kvant from kvant.digital. The old site kvant.mccme.ru now sends each request to kvant.digital. The front page links each issue, and the table of contents of an issue has an item, as "Задачи М2902 to М2905; Ф2909 to Ф2912" with dashes, for the new problems of the column. The adapter keeps the mathematics problems (the М numbers) and the link to the item. The editors hold the copyright of the journal, so the adapter keeps no text.
type KvantAdapter struct{ base }

// Kvant is the adapter of kvant.digital.
var Kvant = KvantAdapter{base{
	id: "kvant", name: "Kvant", home: "https://www.kvant.digital/", dump: "https://www.kvant.digital/",
	competition: "Kvant problem column", names: []string{"Kvant problems", "Kvant", "Задачник «Кванта»"},
	cadence: "monthly", gap: Gap,
	terms: ingest.Terms{Robots: "disallows /search, /view and the query parameters; allows /issues/ and /problems/", Rate: "one page each 5 s", AI: "none stated"},
}}

// MaxRange is the largest range of numbers that one item of the table of contents can give.
const MaxRange = 50

var (
	issueRE = regexp.MustCompile(`href="((?:https?:)?//[^"/]*kvant\.digital)?(/issues/(\d{4})/([0-9]+(?:-[0-9]+)?)/)"`)
	tocRE   = regexp.MustCompile(`(?is)<div class="toc--title">\s*<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	// The letter М is Cyrillic in the journal, but a Latin M is read too. The dash of a range is an em dash, an en dash or a hyphen.
	kvantRE = regexp.MustCompile(`[\x{041C}M]\s*(\d+)(?:\s*[\x{2014}\x{2013}-]\s*[\x{041C}M]?\s*(\d+))?`)
)

// kvantProblems gives the mathematics problems of the items of a table of contents whose title starts with "Задачи".
func kvantProblems(page string) []Problem {
	var out []Problem
	seen := map[int]bool{}
	for _, m := range tocRE.FindAllStringSubmatch(page, -1) {
		title := text(m[2])
		if !strings.HasPrefix(title, "Задачи") {
			continue
		}
		link := resolve(Kvant.home, m[1])
		for _, r := range kvantRE.FindAllStringSubmatch(title, -1) {
			lo, _ := strconv.Atoi(r[1])
			hi := lo
			if r[2] != "" {
				hi, _ = strconv.Atoi(r[2])
			}
			if hi < lo || hi-lo > MaxRange {
				hi = lo
			}
			for n := lo; n <= hi; n++ {
				if seen[n] {
					continue
				}
				seen[n] = true
				k := strconv.Itoa(n)
				out = append(out, Problem{Key: "M" + k, Number: "\u041c" + k, Links: []string{link}})
			}
		}
	}
	return out
}

// Fetch reads the front page at req.URL for the links to the issues, then the table of contents of each issue. An issue with no problem of the column gives no set. Fetch fails when no issue gives a problem, because then the pages have changed.
func (a KvantAdapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	p := &pager{get: req.Get, gap: a.gap}
	data, err := p.page(ctx, req.URL)
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("%s: the front page is not found", req.URL)
	}
	type issue struct{ path, year, number string }
	var issues []issue
	seen := map[string]bool{}
	for _, m := range issueRE.FindAllStringSubmatch(string(data), -1) {
		if !seen[m[2]] {
			seen[m[2]] = true
			issues = append(issues, issue{m[2], m[3], m[4]})
		}
	}
	if len(issues) == 0 {
		return fmt.Errorf("%s: the page links no issue", req.URL)
	}
	out, err := a.start(req, w)
	if err != nil {
		return err
	}
	n := 0
	for _, is := range issues {
		// The request goes to the host of req.URL. The link of the record goes to kvant.digital.
		data, err := p.page(ctx, resolve(req.URL, is.path))
		if err != nil {
			return err
		}
		ps := kvantProblems(string(data))
		if len(ps) == 0 {
			continue
		}
		n += len(ps)
		id := is.year + "-" + is.number
		s := Set{ID: id, Title: "Kvant " + is.year + ", no. " + is.number, Date: is.year, Links: []string{resolve(a.home, is.path)}, Problems: ps}
		if err := out.set(s); err != nil {
			return err
		}
	}
	if n == 0 {
		return fmt.Errorf("%s: no issue has a problem of the column", req.URL)
	}
	return out.close()
}
