package inventory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// GoldFile is the gold set of the clustering, relative to the root of the tree (corpus spec K4.3). It has one pair on each line.
const GoldFile = "records/gold/clustering.jsonl"

// PassNames are the names of the passes of Cluster, in order. They are also the rules that a merge by each pass names.
var PassNames = []string{"identifiers", "equivalence"}

// Pair is one decision of a curator in the gold set: whether two mentions are one item.
type Pair struct {
	A     string `json:"a"`     // a mention
	B     string `json:"b"`     // another mention
	Same  bool   `json:"same"`  // the two mentions are one item
	Layer string `json:"layer"` // the layer of the pair: textbook, formal or research
	By    string `json:"by"`    // the curator
	On    string `json:"on"`    // the date of the decision, YYYY-MM-DD
}

// ReadGold reads the gold set of the tree at root. A tree with no gold set gives no pairs and no error. ReadGold fails on a pair with a missing field, on a pair of one mention with itself, and on a pair that is in the file twice in either order.
func ReadGold(root string) ([]Pair, error) {
	path := filepath.Join(root, filepath.FromSlash(GoldFile))
	data, err := os.ReadFile(path) // #nosec G304 -- the path is in the tree that the caller names.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Pair
	seen := map[[2]string]int{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var p Pair
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", GoldFile, n, err)
		}
		if err := p.check(); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", GoldFile, n, err)
		}
		k := [2]string{min(p.A, p.B), max(p.A, p.B)}
		if first, dup := seen[k]; dup {
			return nil, fmt.Errorf("%s:%d: the pair %s, %s is also on line %d", GoldFile, n, p.A, p.B, first)
		}
		seen[k] = n
		out = append(out, p)
	}
	return out, sc.Err()
}

func (p *Pair) check() error {
	switch {
	case p.A == "" || p.B == "":
		return errors.New("the pair needs the mentions a and b")
	case p.A == p.B:
		return fmt.Errorf("the pair has the mention %s twice", p.A)
	case !slices.Contains([]string{Textbook, Formal, Research}, p.Layer):
		return fmt.Errorf("the layer %q is not textbook, formal or research", p.Layer)
	case p.By == "":
		return errors.New("the pair names no curator in by")
	}
	if _, err := time.Parse(time.DateOnly, p.On); err != nil {
		return fmt.Errorf("the date on %q is not YYYY-MM-DD", p.On)
	}
	return nil
}

// Score is the measure of the passes of Cluster up to one pass, on the pairs of the gold set of one layer.
type Score struct {
	Pass     string `json:"pass"`  // the last pass: the score is of this pass and the passes before it
	Layer    string `json:"layer"` // a layer, or all
	Pairs    int    `json:"pairs"` // the pairs whose two mentions are in the inventory
	TruePos  int    `json:"true_pos"`
	FalsePos int    `json:"false_pos"` // a false merge: the passes join two items
	FalseNeg int    `json:"false_neg"` // a false split: the passes keep one item in two groups
	TrueNeg  int    `json:"true_neg"`
	Missing  int    `json:"missing"` // the pairs with a mention that is not in the inventory. They are not in the score.
	// Precision is TruePos/(TruePos+FalsePos), and Recall is TruePos/(TruePos+FalseNeg). Each is nil when its divisor is zero.
	Precision *float64 `json:"precision"`
	Recall    *float64 `json:"recall"`
}

// Measure gives the precision and the recall of the passes of Cluster on the gold set: one score for each pass and each layer of the gold set, and one score for each pass over all layers. The passes are cumulative, so the score of equivalence is the score of passes 1 and 2 together. Measure runs the passes on the mentions alone, without the rows, so a merge by a curator does not change it.
func Measure(ms []Mention, gold []Pair) []Score {
	passes := Passes(ms)
	var layers []string
	for _, p := range gold {
		if !slices.Contains(layers, p.Layer) {
			layers = append(layers, p.Layer)
		}
	}
	slices.Sort(layers)
	layers = append(layers, "all")
	var out []Score
	for i, groups := range passes {
		for _, layer := range layers {
			s := Score{Pass: PassNames[i], Layer: layer}
			for _, p := range gold {
				if layer != "all" && p.Layer != layer {
					continue
				}
				ga, okA := groups[p.A]
				gb, okB := groups[p.B]
				if !okA || !okB {
					s.Missing++
					continue
				}
				s.Pairs++
				switch joined := ga == gb; {
				case joined && p.Same:
					s.TruePos++
				case joined:
					s.FalsePos++
				case p.Same:
					s.FalseNeg++
				default:
					s.TrueNeg++
				}
			}
			s.Precision = ratio(s.TruePos, s.TruePos+s.FalsePos)
			s.Recall = ratio(s.TruePos, s.TruePos+s.FalseNeg)
			out = append(out, s)
		}
	}
	return out
}

func ratio(a, b int) *float64 {
	if b == 0 {
		return nil
	}
	r := float64(a) / float64(b)
	return &r
}
