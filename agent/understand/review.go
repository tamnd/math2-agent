package understand

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The review queues of the passes (AX7.4 to AX7.6, rule D11) and the gold sets U and C (AX11.2). A record that a check did not accept carries its queue: prose-review, unfold-review or proof-target. A record whose result was not valid twice carries Failed. People work the queues. They also mark the gold sets, which hold marks and ids, never text of a paper (AX11.1).

// FailedQueue is the queue of the records whose result was not valid twice.
const FailedQueue = "model-failed"

// ReadAll reads the records of the pass from records/understand/arxiv/PASS/ in the tree at root, sorted by id. A tree with no records of the pass gives none and no error.
func ReadAll(root, pass string) ([]Record, error) {
	dir := filepath.Join(root, filepath.FromSlash(Dir), pass)
	files, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".jsonl" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f.Name())) // #nosec G304 -- the caller owns the tree
		if err != nil {
			return nil, err
		}
		rs, err := ParseRecords(data)
		if err != nil {
			return nil, fmt.Errorf("%s/%s/%s: %w", Dir, pass, f.Name(), err)
		}
		out = append(out, rs...)
	}
	slices.SortFunc(out, func(a, b Record) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// Queue is one review queue of one pass.
type Queue struct {
	Pass  string   `json:"pass"`
	Name  string   `json:"queue"`
	Items []string `json:"items"` // the ids of the records
}

// Reviews returns the review queues of each pass in the tree at root, in the order of Passes and then of the queue names. A queue with no record is not in the result.
func Reviews(root string) ([]Queue, error) {
	var out []Queue
	for _, pass := range Passes {
		rs, err := ReadAll(root, pass)
		if err != nil {
			return nil, err
		}
		by := map[string][]string{}
		for _, r := range rs {
			switch {
			case r.Failed != "":
				by[FailedQueue] = append(by[FailedQueue], r.ID)
			case r.Queue != "":
				by[r.Queue] = append(by[r.Queue], r.ID)
			}
		}
		for _, q := range slices.Sorted(maps.Keys(by)) {
			out = append(out, Queue{Pass: pass, Name: q, Items: by[q]})
		}
	}
	return out, nil
}

// The files of the gold sets U and C, relative to the root of the tree.
const (
	GoldUnfoldFile   = "records/gold/unfolding.jsonl"
	GoldClassifyFile = "records/gold/classify.jsonl"
)

// The targets of AX11.3.
const (
	TargetEquivalent = 0.95 // of the unfoldings that passed the check, the share that people mark equivalent
	TargetRejected   = 0.90 // of the unfoldings that people mark wrong, the share that the check rejected
	TargetClassify   = 0.95 // the accuracy of classify
)

// GoldUnfold is one mark of gold set U: what a person says of one unfolded form of a statement of an open paper.
type GoldUnfold struct {
	Item    string   `json:"item"`    // the statement, ax://paper/ID#TAG
	Hash    string   `json:"hash"`    // the hash of the unfolded form that the person read, from the record
	Verdict string   `json:"verdict"` // equivalent, stronger, weaker or different
	Missing []string `json:"missing,omitempty"`
	Extra   []string `json:"extra,omitempty"`
	By      string   `json:"by"`
	On      string   `json:"on"`
}

// GoldClassify is one mark of gold set C: the kind of one item, as a person decides it.
type GoldClassify struct {
	Item string `json:"item"`
	Kind string `json:"kind"`
	By   string `json:"by"`
	On   string `json:"on"`
}

// ReadGoldUnfold reads gold set U from the tree at root. A tree with no set gives none and no error. A mark with a missing field, a verdict that is not one of equivalent, stronger, weaker and different, or a second mark of one form is an error.
func ReadGoldUnfold(root string) ([]GoldUnfold, error) {
	out, err := readGold[GoldUnfold](root, GoldUnfoldFile)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i, g := range out {
		switch {
		case g.Item == "" || g.Hash == "" || g.By == "" || g.On == "":
			return nil, fmt.Errorf("%s:%d: a mark needs item, hash, by and on", GoldUnfoldFile, i+1)
		case g.Verdict == "unsure" || !slices.Contains(Verdicts, g.Verdict):
			return nil, fmt.Errorf("%s:%d: the verdict %q is not equivalent, stronger, weaker or different", GoldUnfoldFile, i+1, g.Verdict)
		case seen[g.Item+" "+g.Hash]:
			return nil, fmt.Errorf("%s:%d: the form %s of %s has a mark already", GoldUnfoldFile, i+1, g.Hash, g.Item)
		}
		seen[g.Item+" "+g.Hash] = true
	}
	return out, nil
}

// ReadGoldClassify reads gold set C from the tree at root. A tree with no set gives none and no error. A mark with a missing field, a kind that classify cannot give, or a second mark of one item is an error.
func ReadGoldClassify(root string) ([]GoldClassify, error) {
	out, err := readGold[GoldClassify](root, GoldClassifyFile)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i, g := range out {
		switch {
		case g.Item == "" || g.Kind == "" || g.By == "" || g.On == "":
			return nil, fmt.Errorf("%s:%d: a mark needs item, kind, by and on", GoldClassifyFile, i+1)
		case !slices.Contains(Kinds, g.Kind):
			return nil, fmt.Errorf("%s:%d: the kind %q is not one of the kinds of classify", GoldClassifyFile, i+1, g.Kind)
		case seen[g.Item]:
			return nil, fmt.Errorf("%s:%d: the item %s has a mark already", GoldClassifyFile, i+1, g.Item)
		}
		seen[g.Item] = true
	}
	return out, nil
}

func readGold[T any](root, file string) ([]T, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file))) // #nosec G304 -- the caller owns the tree
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []T
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 1<<20)
	for n := 1; sc.Scan(); n++ {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		dec.DisallowUnknownFields()
		var g T
		if err := dec.Decode(&g); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", file, n, err)
		}
		out = append(out, g)
	}
	return out, sc.Err()
}

// Score is the measure of one pass on its gold set. A rate is nil when the gold set has no mark for it.
type Score struct {
	Set     string   `json:"set"`
	Measure string   `json:"measure"`
	Marks   int      `json:"marks"`   // the marks of the set
	Matched int      `json:"matched"` // the marks with a record of the pass
	N       int      `json:"n"`       // the marks that the measure counts
	Rate    *float64 `json:"rate"`
	Target  float64  `json:"target"`
}

// Met reports whether the rate meets the target.
func (s Score) Met() bool { return s.Rate != nil && *s.Rate >= s.Target }

func rate(k, n int) *float64 {
	if n == 0 {
		return nil
	}
	r := float64(k) / float64(n)
	return &r
}

// MeasureUnfold measures the pass unfold and its check on gold set U (AX11.3). A record passed the check when it has no queue and its verdict is equivalent. The first measure is the share of the passed forms that people mark equivalent; the second is the share of the forms that people mark wrong that the check did not pass.
func MeasureUnfold(rs []Record, gold []GoldUnfold) []Score {
	byForm := map[string]*Record{}
	for i := range rs {
		if u := rs[i].Unfold; u != nil {
			byForm[rs[i].ID+" "+u.Hash] = &rs[i]
		}
	}
	matched, passed, good, wrong, rejected := 0, 0, 0, 0, 0
	for _, g := range gold {
		r := byForm[g.Item+" "+g.Hash]
		if r == nil {
			continue
		}
		matched++
		ok := r.Queue == "" && r.Unfold.Verdict != nil && r.Unfold.Verdict.Verdict == Equivalent
		if ok {
			passed++
			if g.Verdict == Equivalent {
				good++
			}
		}
		if g.Verdict != Equivalent {
			wrong++
			if !ok {
				rejected++
			}
		}
	}
	return []Score{
		{Set: "U", Measure: "passed forms that are equivalent", Marks: len(gold), Matched: matched, N: passed, Rate: rate(good, passed), Target: TargetEquivalent},
		{Set: "U", Measure: "wrong forms that the check rejects", Marks: len(gold), Matched: matched, N: wrong, Rate: rate(rejected, wrong), Target: TargetRejected},
	}
}

// MeasureClassify measures the accuracy of the pass classify on gold set C (AX11.3): the share of the marked items whose record has the kind of the mark.
func MeasureClassify(rs []Record, gold []GoldClassify) Score {
	kind := map[string]string{}
	for _, r := range rs {
		if r.Failed == "" {
			kind[r.ID] = r.Kind
		}
	}
	matched, right := 0, 0
	for _, g := range gold {
		k, ok := kind[g.Item]
		if !ok {
			continue
		}
		matched++
		if k == g.Kind {
			right++
		}
	}
	return Score{Set: "C", Measure: "classify accuracy", Marks: len(gold), Matched: matched, N: matched, Rate: rate(right, matched), Target: TargetClassify}
}
