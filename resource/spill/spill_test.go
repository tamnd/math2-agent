package spill

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.gz")
	w, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"a", "line\nbreak", ""} {
		if err := w.Write(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := Read(path, func(b []byte) error {
		var s string
		err := json.Unmarshal(b, &s)
		got = append(got, s)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"a", "line\nbreak", ""}) {
		t.Fatalf("got %q", got)
	}
}
