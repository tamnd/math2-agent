package site

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// archiveTime is the time of each file in an archive. A fixed time makes two archives of the same site the same bytes.
var archiveTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// archiveReadme is the first file of an archive.
const archiveReadme = `This is the Math 2.0 wiki, for reading with no network.

Open index.html in a web browser. Each page works from disk with no server. Search works in the browser, and the math needs no script.

The prose is under CC BY-SA 4.0. The metadata is under CC0 1.0. The code is under Apache-2.0. Each page names its source.
`

// linkRe matches each attribute of a page that can hold a link into the site.
var linkRe = regexp.MustCompile(`(\s(?:href|src|action|data-base)=")([^"]*)(")`)

// Archive writes the site in dir, the output of Build, to w as a zip file that a reader can open from disk, with no network and no server. base is the path of the site on its host, as for Check. Each file of the archive is in the folder name.
//
// The archive has each file of the site and a README.txt. In each HTML page, each link into the site becomes a relative link, and a link to a directory names its index.html, because a browser shows a directory on disk as a list of files. A browser does not let a page on disk read api/search.json, so the archive also has assets/search.js, the same index as a script, and the search page loads it. Two archives of the same site are the same bytes.
func Archive(w io.Writer, dir, base, name string) error {
	base = "/" + strings.Trim(base, "/")
	if base == "/" {
		base = ""
	}
	name = strings.Trim(path.Clean("/"+name), "/")
	if name == "" {
		return fmt.Errorf("site: the archive has no folder name")
	}
	zw := zip.NewWriter(w)
	add := func(rel string, data []byte) error {
		h := &zip.FileHeader{Name: name + "/" + rel, Method: zip.Deflate, Modified: archiveTime}
		h.SetMode(0o644)
		f, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	}
	if err := add("README.txt", []byte(archiveReadme)); err != nil {
		return err
	}
	index, err := searchScript(dir, base)
	if err != nil {
		return err
	}
	if err := add("assets/search.js", index); err != nil {
		return err
	}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if rel == "README.txt" || rel == "assets/search.js" {
			return fmt.Errorf("site: the site has the file %s, and the archive needs that name", rel)
		}
		data, err := os.ReadFile(p) // #nosec G304,G122 -- the path comes from the walk of the output directory.
		if err != nil {
			return err
		}
		if path.Ext(rel) == ".html" {
			data = relink(rel, base, data)
		}
		return add(rel, data)
	})
	if err != nil {
		return err
	}
	return zw.Close()
}

// searchScript returns the search index of the site as a script that sets window.m2Search. Each URL in it is relative to the root of the site.
func searchScript(dir, base string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(dir, "api", "search.json")) // #nosec G304 -- the path is in the output directory.
	if err != nil {
		return nil, err
	}
	var entries []map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("site: api/search.json: %w", err)
	}
	for _, e := range entries {
		if u, ok := e["u"].(string); ok {
			if p, ok := sitePath(base, u); ok {
				e["u"] = relative(".", p, false)
			}
		}
	}
	var buf bytes.Buffer
	buf.WriteString("// The search index of the archive. A page on disk cannot read api/search.json.\nwindow.m2Search = ")
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(entries); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// relink changes each link into the site in the page at rel to a link relative to the page. The search page also loads assets/search.js.
func relink(rel, base string, page []byte) []byte {
	if bytes.Contains(page, []byte(`id="q"`)) {
		script := []byte(`<script defer src="` + base + `/assets/site.js"></script>`)
		page = bytes.Replace(page, script, append([]byte(`<script defer src="`+base+`/assets/search.js"></script>`+"\n"), script...), 1)
	}
	dir := path.Dir(rel)
	return linkRe.ReplaceAllFunc(page, func(m []byte) []byte {
		sub := linkRe.FindSubmatch(m)
		p, ok := sitePath(base, string(sub[2]))
		if !ok {
			return m
		}
		r := relative(dir, p, bytes.Contains(sub[1], []byte("data-base")))
		return bytes.Join([][]byte{sub[1], []byte(r), sub[3]}, nil)
	})
}

// sitePath returns the path in the site that the URL u names, with its query and fragment, when u is a path into the site at base.
func sitePath(base, u string) (string, bool) {
	if !strings.HasPrefix(u, "/") || strings.HasPrefix(u, "//") {
		return "", false
	}
	if base != "" {
		if u != base && !strings.HasPrefix(u, base+"/") && !strings.HasPrefix(u, base+"#") && !strings.HasPrefix(u, base+"?") {
			return "", false
		}
		u = strings.TrimPrefix(u, base)
	}
	return strings.TrimPrefix(u, "/"), true
}

// relative returns the link from a page in the directory dir to the path p in the site. A link to a directory names its index.html. For the base of a script, prefix gives the directory itself, with a slash at the end.
func relative(dir, p string, prefix bool) string {
	i := strings.IndexAny(p, "?#")
	if i < 0 {
		i = len(p)
	}
	file, rest := p[:i], p[i:]
	if !prefix && (file == "" || strings.HasSuffix(file, "/")) {
		file += "index.html"
	}
	var from, to []string
	if dir != "." && dir != "" {
		from = strings.Split(dir, "/")
	}
	if f := strings.TrimSuffix(file, "/"); f != "" {
		to = strings.Split(f, "/")
	}
	n := 0
	for n < len(from) && n < len(to) && from[n] == to[n] {
		n++
	}
	out := strings.Repeat("../", len(from)-n) + strings.Join(to[n:], "/")
	if prefix && out == "" {
		out = "./"
	} else if prefix && !strings.HasSuffix(out, "/") {
		out += "/"
	}
	return out + rest
}
