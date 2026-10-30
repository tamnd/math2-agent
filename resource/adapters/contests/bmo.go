package contests

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
)

// BMOAdapter reads the British Mathematical Olympiad from the page of past papers of the BMO site of the UKMT. The page links the paper of each round of each year and gives the date of the paper. The site states no licence, so the adapter keeps the links only.
type BMOAdapter struct{ base }

// BMO is the adapter of bmos.ukmt.org.uk.
var BMO = BMOAdapter{base{
	id: "bmo", name: "British Mathematical Olympiad", home: "https://bmos.ukmt.org.uk/", dump: "https://bmos.ukmt.org.uk/home/bmo.shtml",
	competition: "British Mathematical Olympiad", names: []string{"BMO"},
	cadence: "yearly", gap: Gap,
	terms: ingest.Terms{Robots: "n/a (robots.txt gives 404)", Rate: "one page for each fetch", AI: "none stated"},
}}

var (
	paperLinkRE = regexp.MustCompile(`(?is)<a[^>]*href="([^"]*/(bmo[12]?)-(\d{4})\.pdf)"[^>]*>(.*?)</a>`)
	dayRE       = regexp.MustCompile(`\b(\d{1,2})\s+([A-Z][a-z]+)\s+(\d{4})\b`)
	fourRE      = regexp.MustCompile(`\b(1[89]\d\d|20\d\d)\b`)
)

// bmoRound gives the title and the number of problems of a paper. The page states that BMO1 had five questions up to 2005 and six from 2006, and that BMO2 has four. The number of problems of a paper before 1992 is not known.
func bmoRound(round string, year int) (string, int) {
	y := strconv.Itoa(year)
	switch {
	case round == "bmo1" && year >= 2006:
		return "BMO Round 1 " + y, 6
	case round == "bmo1":
		return "BMO Round 1 " + y, 5
	case round == "bmo2":
		return "BMO Round 2 " + y, 4
	}
	return "BMO " + y, 0
}

// bmoDate gives the date of a paper from the text of its link, as 2025-11-19 for "19 November 2025", or the year only.
func bmoDate(s, year string) string {
	s = text(s)
	if m := dayRE.FindStringSubmatch(s); m != nil {
		if t, err := time.Parse("2 January 2006", m[1]+" "+m[2]+" "+m[3]); err == nil {
			return t.Format(time.DateOnly)
		}
	}
	if m := fourRE.FindString(s); m != "" {
		return m
	}
	return year
}

// Fetch reads the page at req.URL and writes one set for each paper of BMO, BMO1 and BMO2. The papers of the Further International Selection Test are not a part of the BMO, so they give no set.
func (a BMOAdapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	p := &pager{get: req.Get, gap: a.gap}
	data, err := p.page(ctx, req.URL)
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("%s: the page is not found", req.URL)
	}
	var sets []Set
	seen := map[string]bool{}
	for _, m := range paperLinkRE.FindAllStringSubmatch(string(data), -1) {
		round, y := strings.ToLower(m[2]), m[3]
		id := round + "-" + y
		if seen[id] {
			continue
		}
		seen[id] = true
		year, _ := strconv.Atoi(y)
		title, n := bmoRound(round, year)
		pdf := resolve(a.home, strings.TrimSpace(m[1]))
		s := Set{ID: id, Title: title, Date: bmoDate(m[4], y), Links: []string{pdf}}
		if n > 0 {
			s.Problems = numbered(n, pdf)
		}
		sets = append(sets, s)
	}
	if len(sets) == 0 {
		return fmt.Errorf("%s: the page links no paper", req.URL)
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
