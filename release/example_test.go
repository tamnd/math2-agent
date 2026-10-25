package release_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tamnd/math2-agent/release"
)

func ExampleCompare() {
	want, _ := os.MkdirTemp("", "want-")
	got, _ := os.MkdirTemp("", "got-")
	defer func() { _ = os.RemoveAll(want); _ = os.RemoveAll(got) }()
	_ = os.WriteFile(filepath.Join(want, "Math2.olean"), []byte("one"), 0o600)
	_ = os.WriteFile(filepath.Join(got, "Math2.olean"), []byte("two"), 0o600)
	_ = os.WriteFile(filepath.Join(got, "Math2.trace"), []byte("a Lake record"), 0o600)

	ds, err := release.Compare(want, got)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, d := range ds {
		fmt.Println(d.Path)
	}
	// Output: Math2.olean
}

func ExampleNextName() {
	may := time.Date(2027, 5, 3, 0, 0, 0, 0, time.UTC)
	fmt.Println(release.NextName([]string{"2027.04"}, may))
	fmt.Println(release.NextName([]string{"2027.04", "2027.05"}, may))
	// Output:
	// 2027.05
	// 2027.05.1
}

func ExampleChanges() {
	prev := &release.Manifest{Counts: release.Counts{NodesLive: 2, ByKind: map[string]int{"concept": 1, "statement": 1}}}
	cur := &release.Manifest{Counts: release.Counts{NodesLive: 3, ByKind: map[string]int{"concept": 1, "statement": 2}}}
	for _, c := range release.Changes(prev, cur) {
		fmt.Println(c.Count, c.Before, c.After)
	}
	// Output:
	// nodes_live 2 3
	// by_kind.statement 1 2
}
