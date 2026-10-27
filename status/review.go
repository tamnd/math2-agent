package status

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Review is one fidelity review (spec 10.6). The file reviews/<shard>/<tag>.yaml holds the reviews of the node with the tag, in a list under the key reviews.
type Review struct {
	ID            string   `yaml:"id" json:"id"`                         // for example r-7K3QX2-01. A formal binding lists it in fidelity_reviews.
	Reviewer      string   `yaml:"reviewer" json:"reviewer"`             // orcid:ID or github:LOGIN, as in registers/reviewers.reg
	Date          string   `yaml:"date" json:"date"`                     // the date of the review, YYYY-MM-DD
	Decl          string   `yaml:"decl" json:"decl"`                     // the declaration that the reviewer read: the adapter when the binding has one
	StatementHash string   `yaml:"statement_hash" json:"statement_hash"` // the hash of the statement section, as node.HashText gives it
	TypeHash      string   `yaml:"type_hash" json:"type_hash"`           // the elaborated-type hash of Decl
	Verdict       string   `yaml:"verdict" json:"verdict"`               // faithful, faithful-with-conventions or not-faithful
	Conventions   []string `yaml:"conventions,omitempty" json:"conventions,omitempty"`
	Class         string   `yaml:"class,omitempty" json:"class,omitempty"` // the class of the fault for a not-faithful verdict
	Notes         string   `yaml:"notes,omitempty" json:"notes,omitempty"`
	Minutes       float64  `yaml:"minutes,omitempty" json:"minutes,omitempty"` // the time of the review as the reviewer gives it, or 0 when the review does not record it (corpus K12.5)
}

// The verdicts of a review.
const (
	Faithful                = "faithful"
	FaithfulWithConventions = "faithful-with-conventions"
	NotFaithful             = "not-faithful"
)

// LoadReviews reads each file below root/reviews and returns the reviews by id. A tree without reviews/ has no reviews.
func LoadReviews(root string) (map[string]Review, error) {
	out := map[string]Review{}
	dir := filepath.Join(root, "reviews")
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == dir {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 G122 -- the path comes from a walk of the working tree, which the caller owns.
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		var file struct {
			Reviews []Review `yaml:"reviews"`
		}
		dec := yaml.NewDecoder(strings.NewReader(string(data)))
		dec.KnownFields(true)
		if err := dec.Decode(&file); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		for _, r := range file.Reviews {
			if r.ID == "" {
				return fmt.Errorf("%s: a review has no id", rel)
			}
			if _, ok := out[r.ID]; ok {
				return fmt.Errorf("%s: the review %s is in the tree two times", rel, r.ID)
			}
			if !slices.Contains([]string{Faithful, FaithfulWithConventions, NotFaithful}, r.Verdict) {
				return fmt.Errorf("%s: the review %s has the verdict %q. The verdicts are %s, %s and %s", rel, r.ID, r.Verdict, Faithful, FaithfulWithConventions, NotFaithful)
			}
			out[r.ID] = r
		}
		return nil
	})
	return out, err
}
