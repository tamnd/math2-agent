package arxiv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
)

// NotesRules is the version of the rule set that marks a paper as lecture notes (RS7.1, step 1). A change to the rules or to their order gives a new version, and the sample must be measured again.
const NotesRules = "arxiv-notes/1"

// notSeries matches the names of series of books and proceedings that have the words lecture notes in their name, as Lecture Notes in Computer Science. A paper that appears in such a series is not a set of notes because of the name. The rules remove these names from the text before they look for a signal.
var notSeries = regexp.MustCompile(`\blecture notes? in (?:computer science|mathematics|math\b|physics|pure and applied|applied|statistics|control|economics|logic|artificial intelligence|bioinformatics|biomathematics|networks|electrical|mechanical|computational|theoretical|earth|chemistry|informatics)|\blecture notes?[- ](?:series|monograph)|\bproceedings and lecture notes\b|\blecture notes? of (?:the unione matematica italiana|physics|mathematics)\b`)

// notesRule is one rule of the rule set. A rule matches when its pattern matches the text and, if it has them, its other pattern matches too and the paper has its category.
type notesRule struct {
	name     string
	title    bool           // the rule reads the title only
	pattern  *regexp.Regexp // the signal
	also     *regexp.Regexp // a second signal that the text must have too, or nil
	category string         // a category that the paper must have, or empty
}

// notesRules is the rule set of NotesRules, in order. The first rule that matches names the match.
var notesRules = []notesRule{
	{name: "lecture-notes", pattern: regexp.MustCompile(`\blectures? notes?\b`)},
	{name: "title-lectures", title: true, pattern: regexp.MustCompile(`^lectures\b|\blectures (?:on|in)\b`)},
	{name: "course-notes", pattern: regexp.MustCompile(`\b(?:course|class) notes\b`)},
	{name: "notes-for-course", pattern: regexp.MustCompile(`\bnotes (?:for|from|of|to|based on|accompanying)\b[^.;]{0,80}?\b(?:course|minicourse|mini-course|lectures|lecture series|class|school)\b`)},
	{name: "minicourse", pattern: regexp.MustCompile(`\b(?:minicourses?|mini-courses?|mini courses?|lecture course|short course)\b|\bcourse (?:given|taught|delivered|held)\b`)},
	{name: "lectures-given", pattern: regexp.MustCompile(`\blectures (?:given|delivered|presented|held|taught)\b|\bnotes taken by\b|\bbased on (?:a |the |my |his |her |our )?(?:lectures|lecture series|course|minicourse|mini-course)\b`)},
	{name: "school", pattern: regexp.MustCompile(`\b(?:summer|winter|spring|autumn|research) school\b`), also: regexp.MustCompile(`\b(?:lectures?|notes|course|minicourse|tutorial)\b`)},
	{name: "math-ho", category: "math.HO", pattern: regexp.MustCompile(`\b(?:notes|lectures|handouts?)\b`), also: regexp.MustCompile(`\b(?:course|class|students|semester|school|undergraduates?)\b`)},
}

// notesText gives the text that the rules read: lower case, one space between words, and the names of series removed.
func notesText(s string) string {
	s = strings.Join(strings.Fields(strings.ToLower(s)), " ")
	return notSeries.ReplaceAllString(s, " ")
}

// Notes reports whether a paper is a set of lecture notes by the rule set NotesRules, and gives the name of the rule that matched. It reads the title, the comments field and the categories of the paper.
func Notes(title, comments string, categories []string) (string, bool) {
	t := notesText(title)
	text := t + " | " + notesText(comments)
	for _, r := range notesRules {
		in := text
		if r.title {
			in = t
		}
		if r.category != "" && !slices.Contains(categories, r.category) {
			continue
		}
		if r.pattern.MatchString(in) && (r.also == nil || r.also.MatchString(in)) {
			return r.name, true
		}
	}
	return "", false
}

// Labelled is one paper of a labelled sample of the rule set. A person reads the paper and sets Notes. A sample is a file of records (package mdrec), one Labelled in each record.
type Labelled struct {
	ID         string   `json:"id"`                   // the arXiv id
	Title      string   `json:"title"`                // the title, as in the metadata plane
	Comments   string   `json:"comments,omitempty"`   // the comments field, as in the metadata plane
	Categories []string `json:"categories,omitempty"` // the categories
	Notes      bool     `json:"notes"`                // the label: the paper is a set of lecture notes
}

// ReadLabelled reads a labelled sample from the file of records at path.
func ReadLabelled(path string) ([]Labelled, error) {
	recs, err := mdrec.Read(path)
	if err != nil {
		return nil, err
	}
	out := make([]Labelled, 0, len(recs))
	for n, rec := range recs {
		var l Labelled
		dec := json.NewDecoder(bytes.NewReader(rec))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&l); err != nil {
			return nil, fmt.Errorf("%s: record %d: %w", path, n+1, err)
		}
		if l.ID == "" {
			return nil, fmt.Errorf("%s: record %d has no id", path, n+1)
		}
		out = append(out, l)
	}
	return out, nil
}

// Score is the result of the rule set on a labelled sample.
type Score struct {
	Rules     string   // the version of the rule set
	TP, FP    int      // the papers that the rules mark, with the label true and false
	FN, TN    int      // the papers that the rules do not mark, with the label true and false
	Precision float64  // TP / (TP + FP), or 0 when the rules mark no paper
	Recall    float64  // TP / (TP + FN), or 0 when no paper has the label true
	Wrong     []string // the ids of the false positives and the false negatives, in the order of the sample
}

// Measure gives the precision and the recall of the rule set on a labelled sample.
func Measure(sample []Labelled) Score {
	s := Score{Rules: NotesRules}
	for _, l := range sample {
		_, got := Notes(l.Title, l.Comments, l.Categories)
		switch {
		case got && l.Notes:
			s.TP++
		case got:
			s.FP++
			s.Wrong = append(s.Wrong, l.ID)
		case l.Notes:
			s.FN++
			s.Wrong = append(s.Wrong, l.ID)
		default:
			s.TN++
		}
	}
	if s.TP+s.FP > 0 {
		s.Precision = float64(s.TP) / float64(s.TP+s.FP)
	}
	if s.TP+s.FN > 0 {
		s.Recall = float64(s.TP) / float64(s.TP+s.FN)
	}
	return s
}
