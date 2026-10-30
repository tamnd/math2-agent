// Package ocw is the adapter of MIT OpenCourseWare (spec corpus resources RS5.1, RS5.2, RS5.4 and RS5.5, phase R3). It reads the course packages, the zip files that OCW gives on the download page of each course, and it keeps the courses of mathematics.
//
// One package gives one course at three levels. The course is the work, with the id <ror>:<code>, as 042nb2s44:18.06. The offering is the expression, with the id <course>:<term>, as 042nb2s44:18.06:2010-spring, the term as its label and the instructors as its contributors. The site is the manifestation, with the slug of the course in the id ocw. A lecture note, a problem set, an exam or a video lecture with its own page becomes a resource of the kind course-material, with a part_of edge to the offering.
//
// The content of OCW is under CC BY-NC-SA 4.0, which is the class share-alike-nc. The adapter keeps the facts: the titles, the numbers, the dates, the topics, the instructors and the readings of each session. It keeps no text of a note, a problem or an exam in a record or a node. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/ocw"
package ocw

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// ROR is the ROR id of the Massachusetts Institute of Technology, the institution of each course of OCW.
const ROR = "042nb2s44"

// Base is the home of OCW.
const Base = "https://ocw.mit.edu"

// Adapter is the adapter of MIT OpenCourseWare.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of OCW.
func (Adapter) Source() ingest.Source {
	return ingest.Source{ID: "ocw", Name: "MIT OpenCourseWare", Kind: "course-platform", Home: Base, Licence: "CC-BY-NC-SA-4.0", Class: ingest.ShareAlikeNC, Channel: "bulk",
		Terms:       &ingest.Terms{AI: "training is allowed with attribution, the models must be non-commercial, and derivative models must be released under BY-NC-SA (https://ocw.mit.edu/pages/privacy-and-terms-of-use/)"},
		Attribution: "MIT OpenCourseWare, https://ocw.mit.edu, CC BY-NC-SA 4.0"}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "ocw/1" }

// Dump returns the URL of the package of one course, 18.06 of spring 2010. A bulk import gives a directory of packages or a list of them with -url.
func (Adapter) Dump() string {
	return Base + "/courses/18-06-linear-algebra-spring-2010/18.06-spring-2010.zip"
}

// MaxMember is the largest member of a package that Fetch reads. The pages are small, and a larger member is a PDF or a video.
const MaxMember = 8 << 20

// Fetch reads the packages of the courses at req.URL and writes the pages of the courses of mathematics to a tar file, compressed with gzip. req.URL is a package (a local zip file or the URL of one), the page of a course on OCW, a local directory of packages, or a text file with one package, page or path on each line. Each member of the tar file is <package>/<path>, in the order of the names, so the snapshot of the same packages has the same bytes. Fetch keeps the pages and the data files and drops the PDFs, the images and the videos.
func (a Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	inputs, err := expand(ctx, req, req.URL)
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return fmt.Errorf("ocw: %s gives no course package", req.URL)
	}
	type pkg struct {
		name  string
		files map[string][]byte
	}
	var pkgs []pkg
	names := map[string]bool{}
	for _, in := range inputs {
		files, err := open(ctx, req, in)
		if err != nil {
			return fmt.Errorf("ocw: %s: %w", in, err)
		}
		c := Parse(files)
		if !Math(c) {
			continue
		}
		name := c.Slug
		if name == "" {
			name = strings.TrimSuffix(path.Base(in), ".zip")
		}
		if names[name] {
			continue
		}
		names[name] = true
		pkgs = append(pkgs, pkg{name, files})
	}
	slices.SortFunc(pkgs, func(x, y pkg) int { return strings.Compare(x.name, y.name) })
	zw := gzip.NewWriter(w)
	tw := tar.NewWriter(zw)
	for _, p := range pkgs {
		for _, f := range slices.Sorted(maps.Keys(p.files)) {
			hdr := &tar.Header{Name: p.name + "/" + f, Mode: 0o644, Size: int64(len(p.files[f])), Typeflag: tar.TypeReg, Format: tar.FormatPAX}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if _, err := tw.Write(p.files[f]); err != nil {
				return err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return zw.Close()
}

func remote(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// expand gives the packages and the pages of courses that one input names. A list file and a directory give their entries.
func expand(ctx context.Context, req *ingest.Request, in string) ([]string, error) {
	in = strings.TrimSpace(in)
	if remote(in) {
		if strings.HasSuffix(strings.ToLower(in), ".txt") {
			body, err := req.Get(ctx, in)
			if err != nil {
				return nil, err
			}
			defer func() { _ = body.Close() }()
			return list(ctx, req, body)
		}
		return []string{in}, nil
	}
	st, err := os.Stat(in)
	if err != nil {
		return nil, fmt.Errorf("ocw: %w", err)
	}
	if st.IsDir() {
		ents, err := os.ReadDir(in)
		if err != nil {
			return nil, err
		}
		var out []string
		for _, e := range ents {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
				out = append(out, filepath.Join(in, e.Name()))
			}
		}
		slices.Sort(out)
		return out, nil
	}
	if strings.HasSuffix(strings.ToLower(in), ".zip") {
		return []string{in}, nil
	}
	f, err := os.Open(in) // #nosec G304 -- the user names the list.
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return list(ctx, req, f)
}

// list reads one input on each line. An empty line and a line that starts with # are skipped.
func list(ctx context.Context, req *ingest.Request, r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !remote(line) && !strings.HasSuffix(strings.ToLower(line), ".zip") {
			return nil, fmt.Errorf("ocw: the list has %q, which is not a package or a page of OCW", line)
		}
		more, err := expand(ctx, req, line)
		if err != nil {
			return nil, err
		}
		out = append(out, more...)
	}
	return out, sc.Err()
}

// open reads one package and gives the files that the adapter keeps. A page of a course gives the package that its download page links.
func open(ctx context.Context, req *ingest.Request, in string) (map[string][]byte, error) {
	if !remote(in) {
		zr, err := zip.OpenReader(in)
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		return Members(&zr.Reader)
	}
	u := in
	if !strings.HasSuffix(strings.ToLower(u), ".zip") {
		var err error
		if u, err = packageURL(ctx, req, u); err != nil {
			return nil, err
		}
	}
	dir := req.Cache
	if dir == "" {
		dir = os.TempDir()
	}
	tmp, err := os.CreateTemp(dir, "ocw-*.zip")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	defer func() { _ = tmp.Close() }()
	body, err := req.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	n, err := io.Copy(tmp, body)
	_ = body.Close()
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(tmp, n)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", u, err)
	}
	return Members(zr)
}

// packageURL reads the download page of a course and gives the URL of its package.
func packageURL(ctx context.Context, req *ingest.Request, page string) (string, error) {
	page = strings.TrimSuffix(page, "/")
	if !strings.HasSuffix(page, "/download") {
		page += "/download"
	}
	body, err := req.Get(ctx, page+"/")
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(io.LimitReader(body, MaxMember))
	if err != nil {
		return "", err
	}
	href := zipLink(data)
	if href == "" {
		return "", fmt.Errorf("the download page %s/ links no package", page)
	}
	if strings.HasPrefix(href, "/") {
		href = Base + href
	} else if !remote(href) {
		href = page + "/" + href
	}
	return href, nil
}

// Members gives the files of a package that the adapter keeps, with their paths from the root of the course. The root is the directory of the index.html nearest to the top. The adapter keeps the home page, the data file of the site, the pages, the download page, the video galleries and the data file of each resource. It drops the page of each resource, because that page can have the text of a note or a problem.
func Members(zr *zip.Reader) (map[string][]byte, error) {
	root := ""
	depth := -1
	for _, f := range zr.File {
		name := clean(f.Name)
		if path.Base(name) != "index.html" {
			continue
		}
		if d := strings.Count(name, "/"); depth < 0 || d < depth {
			root, depth = path.Dir(name), d
		}
	}
	if depth < 0 {
		return nil, errors.New("the package has no index.html")
	}
	if root == "." {
		root = ""
	} else {
		root += "/"
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		name, ok := strings.CutPrefix(clean(f.Name), root)
		if !ok || !keep(name) || f.FileInfo().IsDir() || f.UncompressedSize64 > MaxMember {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, MaxMember))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		out[name] = data
	}
	return out, nil
}

// clean gives a member name as a clean relative path, or an empty string for a name that leaves the package.
func clean(name string) string {
	name = path.Clean(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimPrefix(name, "/")
	if name == ".." || strings.HasPrefix(name, "../") {
		return ""
	}
	return name
}

func keep(name string) bool {
	switch {
	case name == "index.html", name == "data.json":
		return true
	case strings.HasSuffix(name, ".html"):
		top, _, _ := strings.Cut(name, "/")
		return top == "pages" || top == "download" || top == "video_galleries"
	case path.Base(name) == "data.json":
		parts := strings.Split(name, "/")
		return len(parts) == 3 && parts[0] == "resources"
	}
	return false
}

// each reads the tar file of a snapshot and calls fn with each course.
func each(r io.Reader, fn func(*Course) error) error {
	tr := tar.NewReader(r)
	name := ""
	files := map[string][]byte{}
	flush := func() error {
		if name == "" {
			return nil
		}
		c := Parse(files)
		if c.Slug == "" {
			c.Slug = name
		}
		name, files = "", map[string][]byte{}
		if !Math(c) {
			return nil
		}
		return fn(c)
	}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("ocw: the snapshot: %w", err)
		}
		pkg, file, ok := strings.Cut(hdr.Name, "/")
		if !ok || hdr.Typeflag != tar.TypeReg {
			continue
		}
		if pkg != name {
			if err := flush(); err != nil {
				return err
			}
			name = pkg
		}
		data, err := io.ReadAll(io.LimitReader(tr, MaxMember))
		if err != nil {
			return err
		}
		files[file] = data
	}
	return flush()
}

// Read reads the courses and emits one ingest record for each course and each material. A record has no text.
func (a Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(c *Course) error {
		meta := map[string]string{"course": c.CourseID(), "offering": c.OfferingID(), "term": c.Label}
		if len(c.Instructors) > 0 {
			meta["instructors"] = strings.Join(c.Instructors, "; ")
		}
		if len(c.Topics) > 0 {
			meta["topics"] = strings.Join(c.Topics, "; ")
		}
		if err := emit(ingest.Record{ID: c.Slug, Kind: "course", Title: c.Title, URL: c.URL(), Meta: meta}); err != nil {
			return err
		}
		for _, m := range c.Materials {
			if !m.Own() {
				continue
			}
			rec := ingest.Record{ID: c.Slug + "/resources/" + m.Slug, Kind: "course-material", Title: m.Title, URL: c.URL() + "resources/" + m.Slug + "/", Locator: c.Slug,
				Meta: map[string]string{"type": m.Type, "offering": c.OfferingID()}}
			if err := emit(rec); err != nil {
				return err
			}
		}
		return nil
	})
}

// Records reads the courses and emits the source records (RS13.1): one record for the site of each offering, and one record for each material with its own page.
func (a Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(c *Course) error {
		for _, rec := range c.SourceRecords() {
			if err := emit(rec); err != nil {
				return err
			}
		}
		return nil
	})
}

// Structures reads the courses and calls emit with the sessions of each offering (RS5.4). The id is offering:<offering id>. A material of a session is ocw:<slug of the material>, and the book of a reading is isbn:<isbn> when the course names one textbook. The caller resolves both to resource ids.
func (a Adapter) Structures(r io.Reader, emit func(id string, nodes []resource.Node) error) error {
	return each(r, func(c *Course) error {
		if c.Code() == "" || c.Term == "" {
			return nil
		}
		nodes := c.Sessions()
		if len(nodes) == 0 {
			return nil
		}
		return emit("offering:"+c.OfferingID(), nodes)
	})
}
