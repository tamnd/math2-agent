package site

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Budget is the most bytes that one page can take on the network before math fonts: the page and each style sheet, script and image that it loads, with gzip. Spec 13.7 sets it.
const Budget = 50_000

// The targets of spec 13.7 that Check tests.
const (
	TargetWeight  = "weight"  // a page takes less than Budget on the network
	TargetMathML  = "mathml"  // each formula has MathML
	TargetNoJS    = "nojs"    // a reader can read each page with JavaScript off
	TargetLists   = "lists"   // each figure has a list with the same data
	TargetOffline = "offline" // a page loads no file from another host
)

// Targets returns the targets that Check tests, in the order of a report.
func Targets() []string {
	return []string{TargetWeight, TargetMathML, TargetNoJS, TargetLists, TargetOffline}
}

// Problem is one page that misses one target.
type Problem struct {
	Page    string // the path of the page in the site
	Target  string // one of the Target constants
	Message string
}

func (p Problem) String() string { return fmt.Sprintf("%s: %s: %s", p.Page, p.Target, p.Message) }

// Weight is the size of one page on the network.
type Weight struct {
	Page  string
	Bytes int // the page and the files that it loads, before math fonts, with gzip
}

// Report is the result of Check.
type Report struct {
	Pages    int       // the number of HTML pages
	Formulas int       // the number of formulas
	Figures  int       // the number of graph figures
	Heaviest []Weight  // the five pages with the most bytes, the heaviest first
	Problems []Problem // sorted by page, then by target
}

// OK reports whether each page meets each target.
func (r *Report) OK() bool { return len(r.Problems) == 0 }

// Passed reports whether each page meets the target.
func (r *Report) Passed(target string) bool {
	return !slices.ContainsFunc(r.Problems, func(p Problem) bool { return p.Target == target })
}

var (
	tagRe   = regexp.MustCompile(`<(link|script|img|svg|span|math|figcaption|ol|code)\b([^>]*)>`)
	attrRe  = regexp.MustCompile(`([a-z-]+)(?:="([^"]*)")?`)
	idRe    = regexp.MustCompile(`\bid="([^"]+)"`)
	fontExt = []string{".woff2", ".woff", ".ttf", ".otf"}
)

type element struct {
	name  string
	attrs map[string]string
	at    int // the offset of the tag in the page
}

func elements(page string) []element {
	var out []element
	for _, m := range tagRe.FindAllStringSubmatchIndex(page, -1) {
		t := element{name: page[m[2]:m[3]], attrs: map[string]string{}, at: m[0]}
		for _, a := range attrRe.FindAllStringSubmatch(page[m[4]:m[5]], -1) {
			t.attrs[a[1]] = a[2]
		}
		out = append(out, t)
	}
	return out
}

// Check tests each HTML page in dir, the output of Build, against the static targets of spec 13.7. base is the path of the site on its host, as in the URL of Build, for example /math2. Check cannot measure the time to the first paint, because that needs a browser.
func Check(dir, base string) (*Report, error) {
	base = "/" + strings.Trim(base, "/")
	if base == "/" {
		base = ""
	}
	r := &Report{}
	sizes := map[string]int{} // the gzip size of each file that a page loads
	var weights []Weight
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".html" {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(p) // #nosec G304,G122 -- the path comes from the walk of the output directory.
		if err != nil {
			return err
		}
		r.Pages++
		w, probs, err := checkPage(dir, base, rel, string(data), sizes, r)
		if err != nil {
			return err
		}
		weights = append(weights, Weight{rel, w})
		r.Problems = append(r.Problems, probs...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(weights, func(a, b Weight) int {
		if a.Bytes != b.Bytes {
			return b.Bytes - a.Bytes
		}
		return strings.Compare(a.Page, b.Page)
	})
	r.Heaviest = weights[:min(5, len(weights))]
	order := Targets()
	slices.SortStableFunc(r.Problems, func(a, b Problem) int {
		if c := strings.Compare(a.Page, b.Page); c != 0 {
			return c
		}
		return slices.Index(order, a.Target) - slices.Index(order, b.Target)
	})
	return r, nil
}

func checkPage(dir, base, rel, page string, sizes map[string]int, r *Report) (int, []Problem, error) {
	var probs []Problem
	add := func(target, format string, args ...any) {
		probs = append(probs, Problem{rel, target, fmt.Sprintf(format, args...)})
	}
	weight := gzipSize([]byte(page))
	ids := map[string]bool{}
	for _, m := range idRe.FindAllStringSubmatch(page, -1) {
		ids[m[1]] = true
	}
	ts := elements(page)
	katex, mathml := 0, 0
	for i, t := range ts {
		var ref string
		switch {
		case t.name == "link" && t.attrs["rel"] == "stylesheet":
			ref = t.attrs["href"]
		case t.name == "script" && t.attrs["src"] != "":
			ref = t.attrs["src"]
			_, deferred := t.attrs["defer"]
			_, async := t.attrs["async"]
			if !deferred && !async && t.attrs["type"] != "module" {
				add(TargetNoJS, "the script %s blocks the page", ref)
			}
		case t.name == "img":
			ref = t.attrs["src"]
		case t.name == "span" && t.attrs["class"] == "katex":
			katex++
		case t.name == "math":
			mathml++
		case t.name == "span" && t.attrs["class"] == "math":
			r.Formulas++
			// The build puts KaTeX markup or a tex-error code element in each math span. Raw TeX here needs a script to read.
			next := ""
			if i+1 < len(ts) {
				next = ts[i+1].name + "." + ts[i+1].attrs["class"]
			}
			if next != "span.katex" && next != "code.tex-error" {
				add(TargetNoJS, "a formula at byte %d has no markup", t.at)
			}
		case t.name == "svg" && strings.Contains(" "+t.attrs["class"]+" ", " dag "):
			r.Figures++
			if t.attrs["role"] != "img" || !ids[t.attrs["aria-labelledby"]] {
				add(TargetLists, "a figure has no role img or no caption")
			}
			if !strings.Contains(page[t.at:], `<ol class="reading">`) {
				add(TargetLists, "a figure has no list of its nodes")
			}
		}
		if ref == "" {
			continue
		}
		if strings.HasPrefix(ref, "data:") {
			continue
		}
		if strings.Contains(ref, "//") {
			add(TargetOffline, "the page loads %s", ref)
			continue
		}
		file, ok := resolve(base, rel, ref)
		if !ok {
			add(TargetOffline, "the page loads %s, which is not in the site", ref)
			continue
		}
		if slices.Contains(fontExt, path.Ext(file)) {
			continue
		}
		n, ok := sizes[file]
		if !ok {
			data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file))) // #nosec G304 -- resolve keeps the path in the output directory.
			if err != nil {
				add(TargetOffline, "the page loads %s, which is not in the site", ref)
				continue
			}
			n = gzipSize(data)
			sizes[file] = n
		}
		weight += n
	}
	if katex != mathml {
		add(TargetMathML, "%d formulas have KaTeX HTML, and %d have MathML", katex, mathml)
	}
	if weight >= Budget {
		add(TargetWeight, "the page takes %d bytes, and the budget is %d", weight, Budget)
	}
	return weight, probs, nil
}

// resolve returns the path in the site of the file that ref names, from the page at rel.
func resolve(base, rel, ref string) (string, bool) {
	ref, _, _ = strings.Cut(ref, "?")
	ref, _, _ = strings.Cut(ref, "#")
	var p string
	if strings.HasPrefix(ref, "/") {
		if base != "" && ref != base && !strings.HasPrefix(ref, base+"/") {
			return "", false
		}
		p = path.Clean(strings.TrimPrefix(ref, base))
	} else {
		p = path.Join("/", path.Dir(rel), ref)
	}
	p = strings.TrimPrefix(p, "/")
	if p == "" || strings.HasPrefix(p, "..") {
		return "", false
	}
	return p, true
}

func gzipSize(data []byte) int {
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	_, _ = zw.Write(data)
	_ = zw.Close()
	return buf.Len()
}
