package uri

import "testing"

func TestParseAndPrint(t *testing.T) {
	good := map[string]URI{
		"m2://7K3QX2":                               {Tag: "7K3QX2"},
		"m2://7K3QX2@2026.11":                       {Tag: "7K3QX2", Release: "2026.11"},
		"m2://7K3QX2@2027.03.2":                     {Tag: "7K3QX2", Release: "2027.03.2"},
		"m2://7K3QX2@c4f1e9a":                       {Tag: "7K3QX2", Commit: "c4f1e9a"},
		"m2://7K3QX2#proof":                         {Tag: "7K3QX2", Section: "proof"},
		"m2://7K3QX2@2026.11#formal":                {Tag: "7K3QX2", Release: "2026.11", Section: "formal"},
		"m2://statement/7K3QX2":                     {Namespace: "statement", Name: "7K3QX2"},
		"m2://concept/compact-space":                {Namespace: "concept", Name: "compact-space"},
		"m2://msc/20D60":                            {Namespace: "msc", Name: "20D60"},
		"m2://lean/Subgroup.card_subgroup_dvd_card": {Namespace: "lean", Name: "Subgroup.card_subgroup_dvd_card"},
		"m2://release/2026.11":                      {Namespace: "release", Name: "2026.11"},
		"m2://ext/wikidata/Q180967":                 {Namespace: "ext", Name: "wikidata/Q180967"},
	}
	for s, want := range good {
		got, err := Parse(s)
		if err != nil {
			t.Errorf("Parse(%q): %v", s, err)
			continue
		}
		if got != want {
			t.Errorf("Parse(%q) = %+v, want %+v", s, got, want)
		}
		if got.String() != s {
			t.Errorf("Parse(%q).String() = %q", s, got.String())
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, s := range []string{
		"ax://7K3QX2",
		"m2://7K3QXI",
		"m2://~3fa94c01",
		"m2://7K3QX2@2026.13",
		"m2://7K3QX2@latest",
		"m2://7K3QX2#",
		"m2://7K3QX2#Proof",
		"m2://concept/Compact Space",
		"m2://release/next",
		"m2://ext/wikidata",
		"m2://wiki/compact-space",
		"m2://concept/",
	} {
		if u, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", s, u)
		}
	}
}
