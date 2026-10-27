// Package planetmath is the ingest adapter of PlanetMath, the free encyclopedia of mathematics (spec 09.2, corpus K5.3). It gives one record for each entry in a git archive of one of the PlanetMath repositories.
//
// PlanetMath keeps its entries in one git repository for each class of the MSC, such as 20_Group_theory_and_generalizations, in the organisation https://github.com/planetmath. An import reads one repository. The Dump is the repository of MSC 20, and a step of the import plan names another repository with its URL. Each entry is one LaTeX file at the top of the repository, such as 20-00-Centralizer.tex. The file starts with the metadata of the entry, one \pm command on each line, and the line \endmetadata ends the metadata.
//
// The entries are under CC BY-SA 3.0, so the source has the class open (spec 09.3). The licence is share-alike, so the inventory keeps no text of an entry (inventory.FromRecords), and rule L03 asks that a node that uses the text names the entry. The text of a record is the LaTeX between \begin{document} and \end{document}, or the LaTeX after the metadata when the entry has no \begin{document}, and the licence gate keeps it with its fingerprints.
//
// The id of a record is the canonical name of the entry, for example Centralizer. The kind is the type of the entry in lower case, for example definition, theorem or proof, and entry when the entry has no type. The title is the title of the entry. The URL is the page of the entry, which is the canonical name in lower case, and the locator is the file at the commit of the snapshot. The meta of a record has these keys, each one only when the entry has a value:
//
//	msc       the MSC codes of the entry, with a space between them, for example 20-00 20A05
//	authors   the authors of the entry, with | between them. The attribution of CC BY-SA names them.
//	defines   the terms that the entry defines, with | between them
//	synonyms  the other names of the entry, with | between them
//	related   the canonical names of the related entries, with | between them
//	modified  the date of the last change, for example 2013-03-22
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/planetmath"
package planetmath

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
)

// Home is the home of PlanetMath. The page of an entry is Home/<name>, with the canonical name in lower case.
const Home = "https://planetmath.org"

// Organisation is the organisation on GitHub that has the repositories of PlanetMath, one for each class of the MSC.
const Organisation = "https://github.com/planetmath"

// Pilot is the repository of MSC 20, group theory and generalizations.
const Pilot = "20_Group_theory_and_generalizations"

// Archive returns the git archive of the master branch of a repository of PlanetMath, for example of Pilot.
func Archive(repository string) string {
	return Organisation + "/" + repository + "/archive/refs/heads/master.tar.gz"
}

// Adapter is the adapter of PlanetMath.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of PlanetMath.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "planetmath",
		Name:        "PlanetMath",
		Home:        Home,
		Licence:     "CC-BY-SA-3.0",
		Class:       ingest.Open,
		Attribution: "The PlanetMath authors, PlanetMath, " + Home + ", CC BY-SA 3.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "planetmath/1" }

// Dump returns the git archive of the repository of MSC 20.
func (Adapter) Dump() string { return Archive(Pilot) }

// Fetch gets the git archive at req.URL and sets req.Commit to its commit.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	return ingest.FetchArchive(ctx, req, w)
}

// canonical is the form of a canonical name: letters and digits.
var canonical = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// Read reads a git archive of a repository and emits one record for each entry. A LaTeX file at the top of the repository with no \pmcanonicalname is not an entry, and Read does not read it. Read fails on a canonical name that is not letters and digits, on a canonical name that two entries have, on an entry with no title or with no \endmetadata, and on an archive with no entries.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	var recs []ingest.Record
	files := map[string]string{}
	_, err := ingest.WalkArchive(r, func(name string, f io.Reader) error {
		if path.Dir(name) != "." || path.Ext(name) != ".tex" {
			return nil
		}
		rec, ok, err := entry(name, f)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !ok {
			return nil
		}
		if first, dup := files[rec.ID]; dup {
			return fmt.Errorf("%s and %s both have the canonical name %s", first, name, rec.ID)
		}
		files[rec.ID] = name
		recs = append(recs, rec)
		return nil
	})
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		return errors.New("the archive has no PlanetMath entries")
	}
	for _, rec := range recs {
		if err := emit(rec); err != nil {
			return err
		}
	}
	return nil
}

// entry reads one LaTeX file. It reports false when the file has no canonical name.
func entry(name string, f io.Reader) (ingest.Record, bool, error) {
	var (
		rec                                  = ingest.Record{Locator: name, Meta: map[string]string{}}
		msc, authors, defines, syns, related []string
		ended, inBody, begun                 bool
		text, after                          strings.Builder
	)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		t := strings.TrimSpace(line)
		if !ended {
			if t == `\endmetadata` {
				ended = true
				continue
			}
			cmd, args := command(t)
			switch {
			case cmd == "pmcanonicalname" && len(args) > 0:
				rec.ID = args[0]
			case cmd == "pmtitle" && len(args) > 0:
				rec.Title = args[0]
			case cmd == "pmtype" && len(args) > 0:
				rec.Kind = strings.ToLower(args[0])
			case cmd == "pmclassification" && len(args) > 1 && args[0] == "msc":
				msc = append(msc, args[1])
			case (cmd == "pmauthor" || cmd == "pmowner") && len(args) > 0:
				authors = append(authors, args[0])
			case cmd == "pmdefines" && len(args) > 0:
				defines = append(defines, args[0])
			case cmd == "pmsynonym" && len(args) > 0:
				syns = append(syns, args[0])
			case cmd == "pmrelated" && len(args) > 0:
				related = append(related, args[0])
			case cmd == "pmmodified" && len(args) > 0:
				rec.Meta["modified"], _, _ = strings.Cut(args[0], " ")
			}
			continue
		}
		after.WriteString(line + "\n")
		switch {
		case strings.HasPrefix(t, `\begin{document}`):
			inBody, begun = true, true
			text.Reset()
		case strings.HasPrefix(t, `\end{document}`):
			inBody = false
		case inBody:
			text.WriteString(line + "\n")
		}
	}
	if err := sc.Err(); err != nil {
		return rec, false, err
	}
	switch {
	case rec.ID == "":
		return rec, false, nil
	case !canonical.MatchString(rec.ID):
		return rec, false, fmt.Errorf("the canonical name %q is not letters and digits", rec.ID)
	case strings.TrimSpace(rec.Title) == "":
		return rec, false, fmt.Errorf("the entry %s has no title", rec.ID)
	case !ended:
		return rec, false, fmt.Errorf("the entry %s has no \\endmetadata", rec.ID)
	}
	rec.Kind = cmp.Or(rec.Kind, "entry")
	rec.URL = Home + "/" + strings.ToLower(rec.ID)
	body := text.String()
	if !begun {
		body = after.String()
	}
	rec.Text = strings.TrimSpace(body)
	set := func(key, sep string, vs []string) {
		if vs = unique(vs); len(vs) > 0 {
			rec.Meta[key] = strings.Join(vs, sep)
		}
	}
	set("msc", " ", msc)
	set("authors", "|", authors)
	set("defines", "|", defines)
	set("synonyms", "|", syns)
	set("related", "|", related)
	if len(rec.Meta) == 0 {
		rec.Meta = nil
	}
	return rec, true, nil
}

// command reads a line of metadata such as \pmclassification{msc}{20-00}. It returns the name of the command without the backslash and its arguments in braces. An argument can have braces in it, as in \pmtitle{$C_{mn}$}.
func command(t string) (string, []string) {
	rest, ok := strings.CutPrefix(t, `\`)
	if !ok {
		return "", nil
	}
	i := strings.IndexByte(rest, '{')
	if i <= 0 {
		return "", nil
	}
	name, rest := rest[:i], rest[i:]
	var args []string
	for strings.HasPrefix(rest, "{") {
		depth, end := 0, -1
		for j := 0; j < len(rest) && end < 0; j++ {
			switch rest[j] {
			case '\\':
				j++ // an escaped character, such as \{
			case '{':
				depth++
			case '}':
				if depth--; depth == 0 {
					end = j
				}
			}
		}
		if end < 0 {
			break
		}
		args = append(args, strings.TrimSpace(rest[1:end]))
		rest = strings.TrimSpace(rest[end+1:])
	}
	return name, args
}

// unique returns the values that are not empty, in their first order, each one time.
func unique(vs []string) []string {
	var out []string
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
