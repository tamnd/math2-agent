package contests

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
)

// KoMaLAdapter reads the problems in mathematics of KöMaL, the Hungarian journal for secondary schools. The journal gives new problems each month from September to May. The page of a month links each problem to its own page. The site states no licence, so the adapter keeps the ids and the links only.
type KoMaLAdapter struct {
	base
	Start int              // the first school year to read, as 2000 for September 2000 to May 2001
	Now   func() time.Time // the clock, which tests set
}

// KoMaL is the adapter of komal.hu.
var KoMaL = KoMaLAdapter{base: base{
	id: "komal", name: "KöMaL, Mathematical and Physical Journal for Secondary Schools", home: "https://www.komal.hu/", dump: "https://www.komal.hu/feladat?a=honap&t=mat&l=en",
	competition: "KöMaL Problem Solving Competition", names: []string{"KöMaL", "KöMaL Competition", "KöMaL Points Competition"},
	cadence: "monthly", gap: Gap, language: "en", creator: resource.Creator{Name: "KöMaL editorial board", Role: "editor"},
	terms: ingest.Terms{Robots: "disallows /dir1 and /dir2/ only", Rate: "one page each 5 s", AI: "none stated"},
}, Start: 2000, Now: time.Now}

var (
	komalIDRE = regexp.MustCompile(`^([A-Z]+(?:/[A-Z]+)?)(\d+)$`)
	h1RE      = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
)

// komalProblems gives the problems that the page of a month links to, in the order of the page.
func komalProblems(u, page string) []Problem {
	var out []Problem
	for _, h := range hrefs(u, page) {
		p, err := url.Parse(h)
		if err != nil {
			continue
		}
		q := p.Query()
		m := komalIDRE.FindStringSubmatch(q.Get("f"))
		if q.Get("a") != "feladat" || m == nil {
			continue
		}
		key := strings.ReplaceAll(m[0], "/", "")
		if slices.ContainsFunc(out, func(p Problem) bool { return p.Key == key }) {
			continue
		}
		link := "https://www.komal.hu/feladat?" + url.Values{"a": {"feladat"}, "f": {m[0]}, "l": {"en"}}.Encode()
		out = append(out, Problem{Key: key, Number: m[1] + ". " + m[2], Links: []string{link}})
	}
	return out
}

// Fetch reads the page of each month from September of the Start year up to the current month, with the parameter h set to the month on the URL req.URL. A month with no problem gives no set.
func (a KoMaLAdapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	dump, err := url.Parse(req.URL)
	if err != nil {
		return err
	}
	out, err := a.start(req, w)
	if err != nil {
		return err
	}
	now := a.Now()
	last := now.Year()*100 + int(now.Month())
	p := &pager{get: req.Get, gap: a.gap}
	for y := a.Start; y <= now.Year(); y++ {
		for _, mo := range []int{9, 10, 11, 12, 101, 102, 103, 104, 105} {
			year, month := y+mo/100, mo%100
			ym := year*100 + month
			if ym > last {
				continue
			}
			u := *dump
			q := u.Query()
			q.Set("a", "honap")
			q.Set("h", fmt.Sprint(ym))
			u.RawQuery = q.Encode()
			data, err := p.page(ctx, u.String())
			if err != nil {
				return err
			}
			ps := komalProblems(u.String(), string(data))
			if len(ps) == 0 {
				continue
			}
			id := fmt.Sprintf("%d-%02d", year, month)
			title := "KöMaL " + time.Month(month).String() + " " + fmt.Sprint(year)
			if m := h1RE.FindStringSubmatch(string(data)); m != nil && text(m[1]) != "" {
				title = text(m[1])
			}
			if err := out.set(Set{ID: id, Title: title, Date: id, Links: []string{"https://www.komal.hu/feladat?a=honap&h=" + fmt.Sprint(ym) + "&t=mat&l=en"}, Problems: ps}); err != nil {
				return err
			}
		}
	}
	return out.close()
}
