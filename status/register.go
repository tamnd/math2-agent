package status

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Reviewer is one record of registers/reviewers.reg. A review counts only when its reviewer has a record (spec 16).
type Reviewer struct {
	ID           string   // orcid: and an ORCID iD, or github: and a GitHub login
	First        string   // the date of the first review, YYYY-MM-DD
	Areas        []string // the MSC2020 codes of the areas
	Affiliations []string // the affiliations, for example ror:03vek6s52. It is empty when the reviewer has none.
}

// Independent reports whether r and s are different reviewers with no shared affiliation (rule V02).
func (r Reviewer) Independent(s Reviewer) bool {
	if r.ID == s.ID {
		return false
	}
	for _, a := range r.Affiliations {
		for _, b := range s.Affiliations {
			if a == b {
				return false
			}
		}
	}
	return true
}

// Key names one formal binding: the tag of the node and the declaration.
type Key struct{ Tag, Decl string }

// LoadReviewers reads root/registers/reviewers.reg and returns the reviewers by id. A tree without the register has no reviewers.
func LoadReviewers(root string) (map[string]Reviewer, error) {
	out := map[string]Reviewer{}
	err := register(root, "reviewers.reg", func(line int, f []string) error {
		if len(f) != 3 && len(f) != 4 {
			return fmt.Errorf("line %d has %d fields, and a record has the fields reviewer, first, areas and affiliations", line, len(f))
		}
		r := Reviewer{ID: f[0], First: f[1], Areas: list(f[2])}
		if len(f) == 4 {
			r.Affiliations = list(f[3])
		}
		if _, ok := out[r.ID]; ok {
			return fmt.Errorf("line %d: the reviewer %s has two records", line, r.ID)
		}
		out[r.ID] = r
		return nil
	})
	return out, err
}

// LoadAuthors reads root/registers/bindings.reg and returns the author of each Lean binding. The author of a binding is the author of the bind or rebind record that made it. A rename or a refactor keeps the author. A record without the author field gives no author.
func LoadAuthors(root string) (map[Key]string, error) {
	out := map[Key]string{}
	err := register(root, "bindings.reg", func(line int, f []string) error {
		if len(f) != 6 && len(f) != 7 {
			return fmt.Errorf("line %d has %d fields, and a record has the fields tag, system, old, new, cause, mathlib and author", line, len(f))
		}
		if f[1] != "lean4" {
			return nil
		}
		tag, old, now, cause := f[0], f[2], f[3], f[4]
		author := ""
		if len(f) == 7 && f[6] != "-" {
			author = f[6]
		}
		if cause == "rename" || cause == "refactor" {
			author = out[Key{tag, old}]
		}
		delete(out, Key{tag, old})
		if now != "-" && author != "" {
			out[Key{tag, now}] = author
		}
		return nil
	})
	return out, err
}

// register calls fn on each record of the register in root/registers.
func register(root, name string, fn func(line int, fields []string) error) error {
	data, err := os.ReadFile(filepath.Join(root, "registers", name)) // #nosec G304 -- the caller names the root of the tree.
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	n := 0
	for line := range strings.Lines(string(data)) {
		n++
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if err := fn(n, f); err != nil {
			return fmt.Errorf("registers/%s: %w", name, err)
		}
	}
	return nil
}

// list splits a field with a comma between the values. The field - is an empty list.
func list(field string) []string {
	if field == "-" {
		return nil
	}
	return strings.Split(field, ",")
}
