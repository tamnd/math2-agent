package release_test

import (
	"fmt"
	"os"
	"path/filepath"

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
