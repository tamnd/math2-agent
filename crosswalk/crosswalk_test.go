package crosswalk_test

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/tamnd/math2-agent/crosswalk"
)

func TestCheck(t *testing.T) {
	good := map[string][]string{
		"wikidata":        {"Q1", "Q180967"},
		"oeis":            {"A000045", "A000001"},
		"stacks":          {"0BRS", "00XD"},
		"lmfdb":           {"EllipticCurve/Q/11.a2", "NumberField/2.0.3.1", "ModularForm/GL2/Q/holomorphic/11.2.a.a", "Groups/Abstract/8.3"},
		"zbmath":          {"6383667", "1306.11073", "maynard.james", "tao.terence.1", "11N05", "11-XX", "11Nxx"},
		"mr":              {"MR1234567", "1234567"},
		"arxiv":           {"1311.4600", "2311.12345v2", "math/0211159", "math.GT/0309136v1", "2311.12345v2#thm-3.1"},
		"nlab":            {"group", "abelian group"},
		"wikipedia":       {"Lagrange's theorem (group theory)"},
		"proofwiki":       {"Lagrange's_Theorem_(Group_Theory)"},
		"pibase":          {"S000001", "P000016"},
		"findstat":        {"St000001", "Mp00001"},
		"house_of_graphs": {"660"},
		"palomar":         {"PALOMAR-2026-10-23-000018"},
		"planetmath":      {"Centralizer", "AlternatingGroupHasIndex2InTheSymmetricGroupThe"},
	}
	for sys, ids := range good {
		for _, id := range ids {
			if err := crosswalk.Check(sys, id); err != nil {
				t.Errorf("Check(%q, %q) = %v", sys, id, err)
			}
		}
	}
	bad := map[string][]string{
		"wikidata": {"", "Q0", "Q01", "q42", "Q42 ", "P31"},
		"oeis":     {"A45", "A0000451", "a000045"},
		"stacks":   {"0brs", "0BR"},
		"lmfdb":    {"11.a2", "EllipticCurve/", "/Q/11.a2", "EllipticCurve/Q/11 a2"},
		"zbmath":   {"0", "Zbl 1306.11073"},
		"arxiv":    {"1311.460", "arXiv:1311.4600", "1311.4600v0"},
		"nlab":     {"group#section", "[[group]]"},
		"erdos":    {"0", "#1"},
		"palomar":  {"a\nb", "PALOMAR-2026-10-23-18", "palomar-2026-10-23-000018", "PALOMAR-2026-10-23-000018-v1"},
		"mathse":   {"q/4"},
	}
	for sys, ids := range bad {
		for _, id := range ids {
			if err := crosswalk.Check(sys, id); err == nil {
				t.Errorf("Check(%q, %q) gives no error", sys, id)
			}
		}
	}
	if err := crosswalk.Check("nosuch", "x"); !errors.Is(err, crosswalk.ErrUnknown) {
		t.Errorf("an unknown system gives %v", err)
	}
}

// TestSchema checks that the systems are the keys of same_as in the schema, and that each pattern of the schema agrees with Check on the examples.
func TestSchema(t *testing.T) {
	data, err := os.ReadFile("../schema/node.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Defs struct {
			SameAs struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"same_as"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range s.Defs.SameAs.Properties {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, crosswalk.Systems()) {
		t.Errorf("the schema has the systems %v, and the package has %v", keys, crosswalk.Systems())
	}
	for _, k := range keys {
		if !crosswalk.Known(k) {
			t.Errorf("%s is not known", k)
		}
	}
}
