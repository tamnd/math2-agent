package contests

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
)

// PutnamAdapter reads the William Lowell Putnam Mathematical Competition from the Putnam archive of Kiran Kedlaya. The archive is one page with a row for each year and the links to the TeX and PDF files of that year. The MAA holds the copyright of the problems, so the adapter keeps the links only. It reads the index page and no linked file, because the page asks robots not to follow its links.
type PutnamAdapter struct{ base }

// Putnam is the adapter of the Putnam archive.
var Putnam = PutnamAdapter{base{
	id: "putnam", name: "The William Lowell Putnam Mathematical Competition archive", home: "https://kskedlaya.org/putnam-archive/", dump: "https://kskedlaya.org/putnam-archive/",
	competition: "William Lowell Putnam Mathematical Competition", names: []string{"Putnam Competition", "Putnam Mathematical Competition", "Putnam"},
	cadence: "yearly", gap: Gap, language: "en", creator: resource.Creator{Name: "Mathematical Association of America", Role: "author"},
	terms: ingest.Terms{Robots: "disallows /papers/, /photos/ and *.jpg; the index has the robots meta nofollow, so only the index is read", Rate: "one page for each fetch", AI: "none stated"},
}}

var (
	putnamRowRE = regexp.MustCompile(`(?is)<tr[^>]*>\s*<th[^>]*>\s*(\d{4})\b(.*?)</tr>`)
	twelveRE    = regexp.MustCompile(`(?i)\b12\s+problems\b`)
)

// Fetch reads the index at req.URL and writes one set for each year. When the page states that a paper has 12 problems, each year has the problems A1 to A6 and B1 to B6, each linked to the PDF of the year.
func (a PutnamAdapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	p := &pager{get: req.Get, gap: a.gap}
	data, err := p.page(ctx, req.URL)
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("%s: the archive is not found", req.URL)
	}
	page := string(data)
	twelve := twelveRE.MatchString(text(page))
	var sets []Set
	byYear := map[string]int{}
	for _, m := range putnamRowRE.FindAllStringSubmatch(page, -1) {
		y := m[1]
		var pdf string
		var links []string
		for _, h := range hrefs(req.URL, m[2]) {
			if strings.HasSuffix(h, "/"+y+".pdf") && pdf == "" {
				pdf = h
			}
			if !slices.Contains(links, h) {
				links = append(links, h)
			}
		}
		if i, ok := byYear[y]; ok {
			for _, h := range links {
				if !slices.Contains(sets[i].Links, h) {
					sets[i].Links = append(sets[i].Links, h)
				}
			}
			continue
		}
		s := Set{ID: y, Title: "Putnam " + y, Date: y, Links: links}
		if twelve && pdf != "" {
			for _, part := range []string{"A", "B"} {
				for n := 1; n <= 6; n++ {
					k := fmt.Sprintf("%s%d", part, n)
					s.Problems = append(s.Problems, Problem{Key: k, Number: k, Links: []string{pdf}})
				}
			}
		}
		byYear[y] = len(sets)
		sets = append(sets, s)
	}
	if len(sets) == 0 {
		return fmt.Errorf("%s: the page has no row of a year", req.URL)
	}
	out, err := a.start(req, w)
	if err != nil {
		return err
	}
	for _, s := range sets {
		if err := out.set(s); err != nil {
			return err
		}
	}
	return out.close()
}
