package contests

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
)

// IMOAdapter reads the International Mathematical Olympiad from imo-official.org. The page of the editions gives the years and the host of each year. The page of the problems of a year gives the numbers of the problems and the links to the paper and to the shortlist.
type IMOAdapter struct {
	base
	Now func() time.Time // the clock, which tests set
}

// IMO is the adapter of imo-official.org.
var IMO = IMOAdapter{base: base{
	id: "imo", name: "International Mathematical Olympiad", home: "https://www.imo-official.org/", dump: "https://www.imo-official.org/editions/",
	competition: "International Mathematical Olympiad", names: []string{"IMO"},
	cadence: "yearly", gap: Gap,
	terms: ingest.Terms{Robots: "no Disallow rule; the file holds only the comment of the Cloudflare content signals", Rate: "one page each 5 s", AI: "none stated"},
}, Now: time.Now}

var (
	rowRE     = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	cellRE    = regexp.MustCompile(`(?is)<t[hd][^>]*>(.*?)</t[hd]>`)
	editionRE = regexp.MustCompile(`/editions/(\d{4})/?`)
	yearRE    = regexp.MustCompile(`^\d{4}$`)
	problemRE = regexp.MustCompile(`(?i)\bProblem\s+(\d{1,2})\b`)
	paperRE   = regexp.MustCompile(`(?i)/problems/\d{4}/eng\.pdf$`)
	shortRE   = regexp.MustCompile(`(?i)IMO\d{4}SL\.pdf$`)
)

// maxProblem is the largest number of a problem of the IMO that the adapter reads.
const maxProblem = 12

// edition is one row of the page of the editions.
type edition struct {
	year int
	host string
}

// editions reads the rows of the table of the editions. The header row names the columns of the year, the country and the city.
func editions(page string) []edition {
	col := map[string]int{}
	var out []edition
	seen := map[int]bool{}
	for _, row := range rowRE.FindAllStringSubmatch(page, -1) {
		var cells []string
		for _, c := range cellRE.FindAllStringSubmatch(row[1], -1) {
			cells = append(cells, text(c[1]))
		}
		if len(col) == 0 {
			for i, c := range cells {
				for _, name := range []string{"year", "country", "city"} {
					if _, ok := col[name]; !ok && strings.EqualFold(c, name) {
						col[name] = i
					}
				}
			}
			continue
		}
		cell := func(name string) string {
			if i, ok := col[name]; ok && i < len(cells) {
				return cells[i]
			}
			return ""
		}
		y := cell("year")
		if m := editionRE.FindStringSubmatch(row[1]); m != nil {
			y = m[1]
		}
		if !yearRE.MatchString(y) {
			continue
		}
		year, _ := strconv.Atoi(y)
		if seen[year] {
			continue
		}
		seen[year] = true
		var host []string
		for _, v := range []string{cell("city"), cell("country")} {
			if v != "" {
				host = append(host, v)
			}
		}
		out = append(out, edition{year: year, host: strings.Join(host, ", ")})
	}
	slices.SortFunc(out, func(a, b edition) int { return a.year - b.year })
	return out
}

// imoProblems gives the numbers of the problems on the page of a year, the link to the English paper and the link to the shortlist.
func imoProblems(u, page string) (nums []int, paper, short string) {
	for _, m := range problemRE.FindAllStringSubmatch(text(page), -1) {
		n, _ := strconv.Atoi(m[1])
		if n >= 1 && n <= maxProblem && !slices.Contains(nums, n) {
			nums = append(nums, n)
		}
	}
	slices.Sort(nums)
	for _, h := range hrefs(u, page) {
		switch {
		case paper == "" && paperRE.MatchString(h):
			paper = h
		case short == "" && shortRE.MatchString(h):
			short = h
		}
	}
	return nums, paper, short
}

// Fetch reads the page of the editions at req.URL, then the page of the problems of each year up to now. A year has the problems that its page names. A year in the future is skipped, and so is the current year when its page has no problem yet. A year that has a shortlist gives a second set for it.
func (a IMOAdapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	p := &pager{get: req.Get, gap: a.gap}
	data, err := p.page(ctx, req.URL)
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("%s: the page of the editions is not found", req.URL)
	}
	eds := editions(string(data))
	if len(eds) == 0 {
		return fmt.Errorf("%s: the page has no table of editions", req.URL)
	}
	out, err := a.start(req, w)
	if err != nil {
		return err
	}
	now := a.Now().Year()
	for _, ed := range eds {
		if ed.year > now {
			continue
		}
		u := resolve(req.URL, fmt.Sprintf("/problems/%d/", ed.year))
		data, err := p.page(ctx, u)
		if err != nil {
			return err
		}
		nums, paper, short := imoProblems(u, string(data))
		if len(nums) == 0 && ed.year == now {
			continue
		}
		y := strconv.Itoa(ed.year)
		links := []string{resolve(req.URL, "/editions/"+y+"/")}
		if data != nil {
			links = append(links, u)
		}
		s := Set{ID: y, Title: "IMO " + y, Date: y, Host: ed.host, Links: links}
		plinks := []string{u}
		if paper != "" {
			plinks = append(plinks, paper)
		}
		for _, n := range nums {
			k := strconv.Itoa(n)
			s.Problems = append(s.Problems, Problem{Key: k, Number: k, Links: plinks})
		}
		if err := out.set(s); err != nil {
			return err
		}
		if short != "" {
			if err := out.set(Set{ID: y + "-sl", Title: "IMO " + y + " Shortlist", Date: y, Links: []string{short}, Shortlist: true}); err != nil {
				return err
			}
		}
	}
	return out.close()
}
