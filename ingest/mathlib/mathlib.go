// Package mathlib is the ingest adapter of Mathlib, the library of formal mathematics in Lean (spec 09.2). It gives one record for each declaration of Mathlib at the commit that the Lake project of the math2 tree pins.
//
// The adapter does not download Mathlib. It runs M2Extract in the Lake project lean/ of the tree, which must have the .olean files of Mathlib, as m2 build makes them. The snapshot is the library records of all the declarations of the modules under Mathlib, from [lean.Session.Library]. The commit of Mathlib goes in the catalogue with the snapshot. Thus when the tree moves to a new tag of Mathlib, m2 ingest mathlib gives a new snapshot and new records.
//
// The record of a declaration has the hash of the canonical type, the constants that the value uses and the docstring. Mathlib is under the Apache License 2.0, thus the records keep the docstrings as their text.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/mathlib"
package mathlib

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/lean"
)

// Repository is the git repository of Mathlib.
const Repository = "https://github.com/leanprover-community/mathlib4"

// Docs is the home of the documentation of Mathlib. The page of a module is the module name with slashes for dots and .html at the end.
const Docs = "https://leanprover-community.github.io/mathlib4_docs/"

// Page is the number of declarations in each library request to M2Extract.
const Page = 2000

// Adapter is the adapter of Mathlib.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of Mathlib.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:           "mathlib",
		Name:         "Mathlib",
		Home:         Repository,
		Licence:      "Apache-2.0",
		Kind:         ingest.Library,
		Class:        ingest.Open,
		Authored:     "human",
		Intake:       "bind",
		IntakeReason: "a Lean 4 library under Apache-2.0, which allows its text in the prose zone with attribution",
		Modules:      []string{"Mathlib"},
		Attribution:  "Mathlib, the Mathlib Community, " + Repository + ", Apache License 2.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "mathlib/1" }

// Dump returns the repository of Mathlib. The commit comes from the Lake project of the tree.
func (Adapter) Dump() string { return Repository }

// Fetch runs M2Extract in the Lake project lean/ of the tree at req.Root and writes the library records of Mathlib, compressed with gzip. It sets req.Commit to the commit of Mathlib in lake-manifest.json. M2Extract is built in req.Cache.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	project := filepath.Join(req.Root, "lean")
	commit, err := Commit(project)
	if err != nil {
		return err
	}
	if commit == "" {
		return fmt.Errorf("%s/lake-manifest.json has no mathlib package", project)
	}
	req.Commit = commit
	return fetch(ctx, project, req.Cache, "Mathlib", commit, w)
}

// Commit returns the commit of Mathlib in the lake-manifest.json file of the Lake project in dir. It is empty when the project does not depend on Mathlib.
func Commit(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "lake-manifest.json")) // #nosec G304 -- the caller names the project.
	if err != nil {
		return "", err
	}
	var m struct {
		Packages []struct {
			Name string `json:"name"`
			Rev  string `json:"rev"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return "", fmt.Errorf("lake-manifest.json: %w", err)
	}
	for _, p := range m.Packages {
		if p.Name == "mathlib" {
			return p.Rev, nil
		}
	}
	return "", nil
}

// header is the first line of a snapshot.
type header struct {
	Library string `json:"library"` // the root of the modules, for example Mathlib
	Commit  string `json:"commit,omitempty"`
	Lean    string `json:"lean"`  // the version of Lean
	Total   int    `json:"total"` // the number of declarations
}

// fetch writes the library records of the modules under root in the Lake project to w: a header line, and then one line for each declaration.
func fetch(ctx context.Context, project, cache, root, commit string, w io.Writer) (err error) {
	exe, err := lean.Install(ctx, project, cache)
	if err != nil {
		return err
	}
	s, err := lean.Start(ctx, exe, project)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := s.Close(); err == nil {
			err = cerr
		}
	}()
	v, err := s.Version()
	if err != nil {
		return err
	}
	if err := s.Import(root); err != nil {
		return fmt.Errorf("import %s (m2 build makes the .olean files): %w", root, err)
	}
	zw := gzip.NewWriter(w)
	bw := bufio.NewWriter(zw)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	decls, total, err := s.Library(root, 0, Page)
	if err != nil {
		return err
	}
	if err := enc.Encode(header{Library: root, Commit: commit, Lean: v.Lean, Total: total}); err != nil {
		return err
	}
	for n := 0; ; {
		for _, d := range decls {
			if d.Error != "" {
				return fmt.Errorf("%s: %s", d.Decl, d.Error)
			}
			if err := enc.Encode(d); err != nil {
				return err
			}
		}
		if n += len(decls); n >= total || len(decls) == 0 {
			break
		}
		if decls, _, err = s.Library(root, n, Page); err != nil {
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return zw.Close()
}

// Read reads a snapshot and emits one record for each declaration. The number of declarations must be the total in the header.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	dec := json.NewDecoder(r)
	var h header
	if err := dec.Decode(&h); err != nil {
		return fmt.Errorf("the header: %w", err)
	}
	if h.Library == "" {
		return errors.New("the header names no library")
	}
	n := 0
	for {
		var d lean.Decl
		err := dec.Decode(&d)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("declaration %d: %w", n+1, err)
		}
		n++
		if err := emit(Record(d)); err != nil {
			return err
		}
	}
	if n != h.Total {
		return fmt.Errorf("the snapshot has %d declarations, and its header gives %d", n, h.Total)
	}
	return nil
}

// Record makes the ingest record of one declaration. The URL is the place of the declaration in the documentation of Mathlib, and the locator is its module.
func Record(d lean.Decl) ingest.Record {
	meta := map[string]string{"type_hash": d.TypeHash}
	if len(d.Uses) > 0 {
		meta["uses"] = strings.Join(d.Uses, " ")
	}
	return ingest.Record{
		ID:      d.Decl,
		Kind:    d.Kind,
		URL:     Docs + strings.ReplaceAll(d.Module, ".", "/") + ".html#" + d.Decl,
		Locator: d.Module,
		Text:    d.Doc,
		Meta:    meta,
	}
}
