package msc_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/msc"
)

func TestCodes(t *testing.T) {
	for _, tc := range []struct {
		code          string
		level         int
		parent, class string
	}{
		{"11-XX", 1, "", "11-XX"},
		{"11-02", 2, "11-XX", "11-XX"},
		{"11Axx", 2, "11-XX", "11-XX"},
		{"11A41", 3, "11Axx", "11-XX"},
		{"11A4", 0, "", ""},
		{"11axx", 0, "", ""},
		{"11-xx", 0, "", ""},
		{"1A41", 0, "", ""},
		{"", 0, "", ""},
	} {
		if msc.Valid(tc.code) != (tc.level > 0) || msc.Level(tc.code) != tc.level || msc.Parent(tc.code) != tc.parent || msc.Class(tc.code) != tc.class {
			t.Errorf("%q: Valid %v, Level %d, Parent %q, Class %q", tc.code, msc.Valid(tc.code), msc.Level(tc.code), msc.Parent(tc.code), msc.Class(tc.code))
		}
	}
	codes := []string{"11Bxx", "11A05", "20-XX", "11-00", "11Axx", "11-XX", "11-02", "11A41", "11B05"}
	slices.SortFunc(codes, msc.Compare)
	if got := strings.Join(codes, " "); got != "11-XX 11-00 11-02 11Axx 11A05 11A41 11Bxx 11B05 20-XX" {
		t.Errorf("the order is %s", got)
	}
}

func TestRead(t *testing.T) {
	if _, err := msc.Read("testdata"); !os.IsNotExist(err) {
		t.Errorf("Read of a tree with no map gives %v", err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "topics"), 0o750); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "msc2020.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(msc.File)), data, 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := msc.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if m.Len() != 10 {
		t.Errorf("Len is %d", m.Len())
	}
	if e, ok := m.Lookup("11A41"); !ok || e.Label != "prime numbers" || e.Topic != "7K3QX2" {
		t.Errorf("Lookup(11A41) gives %+v, %v", e, ok)
	}
	if _, ok := m.Lookup("11A42"); ok {
		t.Error("Lookup finds a code that is not in the map")
	}
	if got := strings.Join(m.Children("11-XX"), " "); got != "11-01 11Axx 11Nxx" {
		t.Errorf("Children(11-XX) is %s", got)
	}
	if got := strings.Join(m.Children(""), " "); got != "11-XX 20-XX" {
		t.Errorf("Children of the root is %s", got)
	}
	if got := m.Codes(); got[0] != "11-XX" || got[len(got)-1] != "20D60" {
		t.Errorf("Codes is %v", got)
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ data, want string }{
		{`"11A41": {label: primes}`, "11A41 has no parent 11Axx"},
		{`"11-XX": {label: number theory, name: x}`, "field name not found"},
		{`"11-YY": {label: number theory}`, `"11-YY" is not an MSC2020 code`},
		{`"11-XX": {label: ""}`, "1 to 8 words"},
		{`"11-XX": {label: one two three four five six seven eight nine}`, "1 to 8 words"},
		{`"11-XX": {label: "\\(K\\)-theory"}`, "1 to 8 words"},
		{"\"11-XX\": {label: number theory}\n\"11-01\": {label: Number Theory}", "11-01 has the label of 11-XX"},
		{`"11-XX": {label: number theory, topic: 7k3qx2}`, `the topic "7k3qx2" is not a tag`},
		{"\"11-XX\": {label: a}\n\"11-XX\": {label: b}", "already defined"},
	} {
		if _, err := msc.Parse([]byte(tc.data)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Parse(%q) gives %v, and the error must have %q", tc.data, err, tc.want)
		}
	}
}
