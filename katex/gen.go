//go:build ignore

// gen.go copies the files of a KaTeX release from npm into this package. Run it with go generate after a change to the version or the integrity below.
//
// It verifies the tarball against the integrity that npm publishes. It keeps katex.min.js for the renderer, and katex.min.css, the WOFF2 fonts and the licence for the pages. The style sheet keeps only the WOFF2 source of each font, because each browser that the site supports reads WOFF2.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	version   = "0.19.1"
	integrity = "sha512-jG8uKntjSEcm2EpQOh0XturpPuuC9r3Sq5HLHdsFdWZT27E672SF19QEi+of0/YeOy15lDgRhKvr5614uRrEfA=="
)

// otherFormats matches the WOFF and TrueType sources that follow the WOFF2 source of a font.
var otherFormats = regexp.MustCompile(`,url\(fonts/[A-Za-z0-9_-]+\.(woff|ttf)\) format\("(woff|truetype)"\)`)

func main() {
	log.SetFlags(0)
	c := &http.Client{Timeout: time.Minute}
	resp, err := c.Get("https://registry.npmjs.org/katex/-/katex-" + version + ".tgz")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("npm: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		log.Fatal(err)
	}
	sum := sha512.Sum512(data)
	if got := "sha512-" + base64.StdEncoding.EncodeToString(sum[:]); got != integrity {
		log.Fatalf("the tarball has the integrity %s, not %s", got, integrity)
	}
	if err := os.RemoveAll("assets"); err != nil {
		log.Fatal(err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		log.Fatal(err)
	}
	tr := tar.NewReader(gz)
	n := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		name := strings.TrimPrefix(h.Name, "package/")
		var out string
		switch {
		case name == "dist/katex.min.js":
			out = "katex.min.js"
		case name == "LICENSE":
			out = "assets/LICENSE"
		case name == "dist/katex.min.css":
			out = "assets/katex.min.css"
		case path.Dir(name) == "dist/fonts" && path.Ext(name) == ".woff2":
			out = "assets/fonts/" + path.Base(name)
		default:
			continue
		}
		b, err := io.ReadAll(io.LimitReader(tr, 8<<20))
		if err != nil {
			log.Fatal(err)
		}
		if out == "assets/katex.min.css" {
			b = otherFormats.ReplaceAll(b, nil)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			log.Fatal(err)
		}
		n++
	}
	fmt.Printf("katex %s: %d files\n", version, n)
}
