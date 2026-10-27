package status

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeFormal(t *testing.T, text string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "registers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "registers", "formal.reg"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoadFormalKeepsTheLastRecord(t *testing.T) {
	root := writeFormal(t, `# tag outcome date by author reason
AAAAAA pending 2026-10-01 - m2/formalize the-job-is-in-the-queue
AAAAAA out_of_reach 2026-10-02 - github:tamnd needs-the-theory-of-perfectoid-spaces
BBBBBB out_of_reach 2026-10-01 - github:tamnd needs-a-large-library
BBBBBB clear 2026-10-03 - github:tamnd the-library-exists-now
CCCCCC blocked 2026-10-01 DDDDDD m2/formalize no-library-defines-the-object
`)
	d, err := LoadFormal(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := d["AAAAAA"]; got.Outcome != FormalOutOfReach || got.Reason != "needs-the-theory-of-perfectoid-spaces" {
		t.Errorf("AAAAAA has %+v, and the last record must be in effect", got)
	}
	if _, ok := d["BBBBBB"]; ok {
		t.Error("BBBBBB has a decision after a record clear")
	}
	if got := d["CCCCCC"]; got.Outcome != FormalBlocked || got.By != "DDDDDD" {
		t.Errorf("CCCCCC has %+v", got)
	}
}

func TestLoadFormalRefusesABadRecord(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"AAAAAA bound 2026-10-01 - github:tamnd the-build-knows", "is not pending"},
		{"AAAAAA blocked 2026-10-01 - github:tamnd no-library", "needs the tag of the definition"},
		{"AAAAAA pending 2026-10-01 DDDDDD github:tamnd in-the-queue", "for blocked only"},
		{"AAAAAA pending 01-10-2026 - github:tamnd in-the-queue", "is not YYYY-MM-DD"},
		{"AAAAAA pending 2026-10-01 - github:tamnd In_The_Queue", "lowercase"},
		{"AAAAAA pending 2026-10-01 - github:tamnd", "has 5 fields"},
		{"AAAAAA pending 2026-10-02 - m2/formalize in-the-queue\nAAAAAA clear 2026-10-01 - github:tamnd done", "is before the date"},
	} {
		_, err := LoadFormal(writeFormal(t, c.line+"\n"))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q gave the error %v, and it must say %q", c.line, err, c.want)
		}
	}
}

func TestComputeFormal(t *testing.T) {
	sts := []Status{
		{Tag: "PROVED", Kind: "statement", Evidence: Kernel, Bindings: []Binding{{Decl: "a"}}},
		{Tag: "BOUND1", Kind: "statement", Evidence: 3, Bindings: []Binding{{Decl: "b"}}},
		{Tag: "BOUND2", Kind: "definition", Bindings: []Binding{{Decl: "c"}}},
		{Tag: "WAITS1", Kind: "statement"},
		{Tag: "NEEDS1", Kind: "conjecture"},
		{Tag: "DEFIN1", Kind: "definition"},
		{Tag: "FAR001", Kind: "problem"},
		{Tag: "NOTHIN", Kind: "statement"},
		{Tag: "CONCPT", Kind: "concept"},
		{Tag: "GONE01", Kind: "statement", Retired: true},
	}
	decisions := map[string]FormalDecision{
		"BOUND2": {Tag: "BOUND2", Outcome: FormalPending, Date: "2026-10-01", By: "-", Reason: "a-job-ran"},
		"WAITS1": {Tag: "WAITS1", Outcome: FormalPending, Date: "2026-10-01", By: "-", Reason: "the-job-is-in-the-queue"},
		"NEEDS1": {Tag: "NEEDS1", Outcome: FormalBlocked, Date: "2026-10-01", By: "DEFIN1", Reason: "no-library-defines-it"},
		"FAR001": {Tag: "FAR001", Outcome: FormalOutOfReach, Date: "2026-10-01", By: "-", Reason: "needs-a-large-library"},
		"CONCPT": {Tag: "CONCPT", Outcome: FormalOutOfReach, Date: "2026-10-01", By: "-", Reason: "a-concept"},
		"NOSUCH": {Tag: "NOSUCH", Outcome: FormalPending, Date: "2026-10-01", By: "-", Reason: "a-job"},
	}
	f := ComputeFormal(sts, decisions)
	got := map[string]string{}
	var tags []string
	for _, n := range f.Nodes {
		got[n.Tag] = n.Outcome
		tags = append(tags, n.Tag)
	}
	want := map[string]string{
		"PROVED": FormalProved, "BOUND1": FormalBound, "BOUND2": FormalBound, "WAITS1": FormalPending,
		"NEEDS1": FormalBlocked, "DEFIN1": FormalNone, "FAR001": FormalOutOfReach, "NOTHIN": FormalNone,
	}
	for tag, w := range want {
		if got[tag] != w {
			t.Errorf("%s is %q, and it must be %q", tag, got[tag], w)
		}
	}
	if len(got) != len(want) || !slices.IsSorted(tags) {
		t.Errorf("the nodes are %v, and they must be the nodes of a formal kind that are not retired, in tag order", tags)
	}
	if f.Outcomes[FormalNone] != 2 || f.Outcomes[FormalBound] != 2 || f.Outcomes[FormalProved] != 1 {
		t.Errorf("the counts are %v", f.Outcomes)
	}
	problems := strings.Join(f.Problems, "\n")
	for _, w := range []string{
		"BOUND2, and the node is bound, so the record can be cleared",
		"CONCPT, which is a concept",
		"NOSUCH, which is not a node",
		"NEEDS1 is blocked by the definition DEFIN1, whose outcome is none and not pending",
	} {
		if !strings.Contains(problems, w) {
			t.Errorf("the problems do not say %q:\n%s", w, problems)
		}
	}
}
