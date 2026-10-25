package site

import (
	"compress/gzip"
	"net/http"
	"os"
	"path"
	"strings"
)

// Handler serves the site in dir, the output of Build, at the path base, for example /math2. It compresses text with gzip, as GitHub Pages does, so a measure of the pages through it is near to a measure of the public site. Use it to preview a site and to audit it, not as a production server.
func Handler(dir, base string) http.Handler {
	base = "/" + strings.Trim(base, "/")
	files := http.FileServerFS(os.DirFS(dir))
	h := http.StripPrefix(strings.TrimSuffix(base, "/"), files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && base != "/" {
			http.Redirect(w, r, base+"/", http.StatusFound)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || !compressible(r.URL.Path) {
			h.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w}
		defer gw.close()
		h.ServeHTTP(gw, r)
	})
}

// compressible reports whether a file is text. A font or an image is compressed already.
func compressible(p string) bool {
	switch path.Ext(p) {
	case ".woff2", ".woff", ".png", ".jpg", ".gif", ".webp", ".gz", ".zip", ".parquet":
		return false
	}
	return true
}

// gzipWriter compresses the body of a response with status 200. It sends each other response as it is, because http.FileServer sends a short body or none for a redirect, a range or a 304.
type gzipWriter struct {
	http.ResponseWriter
	zw *gzip.Writer
}

func (g *gzipWriter) WriteHeader(code int) {
	if code == http.StatusOK && g.Header().Get("Content-Range") == "" {
		g.Header().Del("Content-Length")
		g.Header().Set("Content-Encoding", "gzip")
		g.zw = gzip.NewWriter(g.ResponseWriter)
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	if g.zw == nil {
		return g.ResponseWriter.Write(b)
	}
	return g.zw.Write(b)
}

func (g *gzipWriter) close() {
	if g.zw != nil {
		_ = g.zw.Close()
	}
}
