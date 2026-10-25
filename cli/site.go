package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/site"
)

const siteUsage = `Usage:
  m2 site build [-content DIR] [-spec DIR] [-out DIR] [-url URL] [-build DIR] [-commit C] [-date now]
  m2 site check [-url URL] [-json] [DIR]
  m2 site serve [-url URL] [-addr HOST:PORT] [DIR]
  m2 site archive [-url URL] [-o FILE] [-name NAME] [DIR]`

func runSite(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, siteUsage)
		return ExitUsage
	}
	switch args[0] {
	case "build":
		return runSiteBuild(args[1:], stdout, stderr)
	case "check":
		return runSiteCheck(args[1:], stdout, stderr)
	case "serve":
		return runSiteServe(args[1:], stdout, stderr)
	case "archive":
		return runSiteArchive(args[1:], stdout, stderr)
	}
	fmt.Fprintln(stderr, siteUsage)
	return ExitUsage
}

// sitePath returns the path of the site in its public URL, for example /math2.
func sitePath(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	return u.Path, nil
}

func runSiteCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 site check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "https://tamnd.github.io/math2", "the public URL that the site was built with")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return ExitUsage
	}
	dir := "_site"
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}
	base, err := sitePath(*rawURL)
	if err != nil {
		fmt.Fprintf(stderr, "m2 site check: -url: %v\n", err)
		return ExitUsage
	}
	r, err := site.Check(dir, base)
	if err != nil {
		fmt.Fprintf(stderr, "m2 site check: %v\n", err)
		return ExitFailure
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			return ExitFailure
		}
	} else {
		printSiteReport(stdout, r)
	}
	if !r.OK() {
		return ExitFailure
	}
	return ExitOK
}

func printSiteReport(w io.Writer, r *site.Report) {
	fmt.Fprintf(w, "%d pages, %d formulas, %d figures\n", r.Pages, r.Formulas, r.Figures)
	if len(r.Heaviest) > 0 {
		h := r.Heaviest[0]
		fmt.Fprintf(w, "heaviest page: %s, %d bytes with gzip before math fonts (budget %d)\n", h.Page, h.Bytes, site.Budget)
	}
	for _, t := range site.Targets() {
		state := "pass"
		if !r.Passed(t) {
			state = "FAIL"
		}
		fmt.Fprintf(w, "%-8s %s\n", t, state)
	}
	for _, p := range r.Problems {
		fmt.Fprintln(w, p)
	}
}

func runSiteServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 site serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "http://localhost:8000/math2", "the public URL that the site was built with. Its path is the path of the site on the server.")
	addr := fs.String("addr", "127.0.0.1:8000", "the address to listen on")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return ExitUsage
	}
	dir := "_site"
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}
	base, err := sitePath(*rawURL)
	if err != nil {
		fmt.Fprintf(stderr, "m2 site serve: -url: %v\n", err)
		return ExitUsage
	}
	if _, err := os.Stat(dir); err != nil {
		fmt.Fprintf(stderr, "m2 site serve: %v\n", err)
		return ExitFailure
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(stderr, "m2 site serve: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(stdout, "m2 site serve: http://%s%s/\n", ln.Addr(), base)
	srv := &http.Server{Handler: site.Handler(dir, base), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "m2 site serve: %v\n", err)
		return ExitFailure
	}
	return ExitOK
}

func runSiteArchive(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 site archive", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "https://tamnd.github.io/math2", "the public URL that the site was built with")
	out := fs.String("o", "math2-site.zip", "the zip file to write")
	name := fs.String("name", "", "the folder in the zip file. Leave it empty for the name of the file without .zip.")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return ExitUsage
	}
	dir := "_site"
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}
	base, err := sitePath(*rawURL)
	if err != nil {
		fmt.Fprintf(stderr, "m2 site archive: -url: %v\n", err)
		return ExitUsage
	}
	if *name == "" {
		*name = strings.TrimSuffix(filepath.Base(*out), ".zip")
	}
	// Write to a temporary file, so that an error leaves no part of an archive.
	f, err := os.CreateTemp(filepath.Dir(*out), ".m2-archive-*")
	if err != nil {
		fmt.Fprintf(stderr, "m2 site archive: %v\n", err)
		return ExitFailure
	}
	defer func() { _ = os.Remove(f.Name()) }()
	h := sha256.New()
	n := &countWriter{}
	err = site.Archive(io.MultiWriter(f, h, n), dir, base, *name)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), *out)
	}
	if err != nil {
		fmt.Fprintf(stderr, "m2 site archive: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(stdout, "m2 site archive wrote %s: %d bytes, sha256 %x\n", *out, n.n, h.Sum(nil))
	return ExitOK
}

// countWriter counts the bytes written to it.
type countWriter struct{ n int64 }

func (c *countWriter) Write(b []byte) (int, error) {
	c.n += int64(len(b))
	return len(b), nil
}

func runSiteBuild(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 site build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	opt := site.Options{Version: version, Current: current}
	fs.StringVar(&opt.Content, "content", ".", "the math2 working tree")
	fs.StringVar(&opt.Spec, "spec", "", "the spec directory of math2-agent. Leave it empty for no spec pages.")
	fs.StringVar(&opt.Out, "out", "_site", "the output directory. The build removes it first.")
	fs.StringVar(&opt.URL, "url", "https://tamnd.github.io/math2", "the public URL of the site")
	fs.StringVar(&opt.Repo, "repo", "tamnd/math2", "the content repository on GitHub")
	fs.StringVar(&opt.AgentRepo, "agent-repo", "tamnd/math2-agent", "the toolchain repository on GitHub")
	fs.StringVar(&opt.Build, "build", "", "the output directory of m2 build for the tree. The pages then show the status that the build computed. Leave it empty for no status.")
	fs.StringVar(&opt.Commit, "commit", "", "the content commit")
	date := fs.String("date", "", "the build date: now, or empty for no date. SOURCE_DATE_EPOCH also sets it.")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	switch {
	case *date == "now":
		opt.Date = time.Now()
	case *date != "":
		fmt.Fprintf(stderr, "m2 site build: -date must be now or empty, not %q\n", *date)
		return ExitUsage
	case os.Getenv("SOURCE_DATE_EPOCH") != "":
		sec, err := strconv.ParseInt(os.Getenv("SOURCE_DATE_EPOCH"), 10, 64)
		if err != nil {
			fmt.Fprintf(stderr, "m2 site build: SOURCE_DATE_EPOCH: %v\n", err)
			return ExitUsage
		}
		opt.Date = time.Unix(sec, 0)
	}
	for _, m := range plan {
		opt.Milestones = append(opt.Milestones, site.Milestone{
			ID:    m.ID,
			Title: m.Title,
			Dates: "due " + m.Due,
			URL:   fmt.Sprintf("https://github.com/%s/milestone/%d", opt.AgentRepo, m.Number),
		})
	}
	if err := site.Build(opt); err != nil {
		fmt.Fprintf(stderr, "m2 site build: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(stdout, "m2 site build: wrote %s\n", opt.Out)
	return ExitOK
}
