package exercise_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/exercise"
	"github.com/tamnd/math2-agent/node"
)

const (
	v1 = "---\ntag: EX6H3W\nkind: example\nname: A large subgroup\nuses: [3C53TD]\n---\n\n## Example\n\nFind the order of $H$.\n\n## Remarks\n\nRole: exercise. About: 3C53TD. Difficulty: 2. Group: B. Check: exact.\n\n### Hint\n\nUse Lagrange's theorem.\n\n### Answer\n\n12\n\n### Solution\n\n1. The order of $H$ divides 12.\n2. Thus it is 12.\n"
	v2 = "---\ntag: EX6H3W\nkind: exercise\nname: A large subgroup\nuses: [3C53TD]\nabout: [3C53TD]\ndifficulty: 2\nmath_group: B\ncheck: exact\n---\n\n## Exercise\n\nFind the order of $H$.\n\n## Hint\n\nUse Lagrange's theorem.\n\n## Answer\n\n12\n\n## Solution\n\n1. The order of $H$ divides 12.\n2. Thus it is 12.\n"
)

func parse(t *testing.T, text string) *node.Node {
	t.Helper()
	n, err := node.Parse("nodes/EX6/EX6H3W.md", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		old        bool
	}{{"node.v1", v1, true}, {"node.v2", v2, false}} {
		e, err := exercise.Parse(parse(t, tc.text))
		if err != nil || e == nil {
			t.Fatalf("%s: %v, %v", tc.name, e, err)
		}
		want := exercise.Exercise{ID: "EX6H3W", About: []string{"3C53TD"}, Difficulty: 2, Group: "B", Check: "exact", Task: "Find the order of $H$.", Hint: "Use Lagrange's theorem.", Answer: "12", Solution: "1. The order of $H$ divides 12.\n2. Thus it is 12.", Old: tc.old}
		if e.ID != want.ID || !slices.Equal(e.About, want.About) || e.Difficulty != want.Difficulty || e.Group != want.Group || e.Check != want.Check || e.Task != want.Task || e.Hint != want.Hint || e.Answer != want.Answer || e.Solution != want.Solution || e.Old != want.Old {
			t.Errorf("%s: got %+v, want %+v", tc.name, *e, want)
		}
	}
}

func TestParseExternal(t *testing.T) {
	text := strings.Replace(v2, "check: exact\n", "check: exact\nexternal:\n  - system: webwork\n    id: Library/a.pg\n    url: https://example.org/a\n", 1)
	e, err := exercise.Parse(parse(t, text))
	if err != nil {
		t.Fatal(err)
	}
	if want := []exercise.External{{System: "webwork", ID: "Library/a.pg", URL: "https://example.org/a"}}; !slices.Equal(e.External, want) {
		t.Errorf("external is %v, want %v", e.External, want)
	}
}

func TestParseOther(t *testing.T) {
	text := strings.Replace(v1, "Role: exercise. ", "", 1)
	if e, err := exercise.Parse(parse(t, text)); e != nil || err != nil {
		t.Errorf("an example that is not an exercise gives %v, %v", e, err)
	}
}

func TestMigrate(t *testing.T) {
	got, err := exercise.Migrate(parse(t, v1))
	if err != nil || got == nil {
		t.Fatalf("%v, %v", got, err)
	}
	want := parse(t, v2)
	if got.Kind() != exercise.Kind || got.Body != want.Body {
		t.Errorf("the kind is %s and the body is\n%s\nwant\n%s", got.Kind(), got.Body, want.Body)
	}
	for _, k := range []string{"about", "difficulty", "math_group", "check", "uses"} {
		if g, w := got.Meta[k], want.Meta[k]; !equal(g, w) {
			t.Errorf("%s is %v, want %v", k, g, w)
		}
	}
	if again, err := exercise.Migrate(got); again != nil || err != nil {
		t.Errorf("a node.v2 exercise gives %v, %v", again, err)
	}
}

func equal(a, b any) bool {
	x, ok1 := a.([]any)
	y, ok2 := b.([]any)
	if ok1 && ok2 {
		return slices.Equal(x, y)
	}
	return a == b
}

func TestMigrateNoPlace(t *testing.T) {
	text := strings.Replace(v1, "## Remarks", "## Notation\n\n$H$ is a subgroup.\n\n## Remarks", 1)
	_, err := exercise.Migrate(parse(t, text))
	if err == nil || !strings.Contains(err.Error(), "body.notation") {
		t.Errorf("a section with no place gives %v", err)
	}
}
