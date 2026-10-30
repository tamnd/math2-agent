package ids

import (
	"errors"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct{ key, in, want string }{
		{"doi", "https://doi.org/10.1007/978-3-642-13797-4", "10.1007/978-3-642-13797-4"},
		{"doi", "DOI:10.2307/1969265", "10.2307/1969265"},
		{"doi", "10.1215/S0012-7094-49-01622-X", "10.1215/s0012-7094-49-01622-x"},
		{"arxiv", "arXiv:2203.17207v2", "2203.17207"},
		{"arxiv", "https://arxiv.org/abs/math/0601001", "math/0601001"},
		{"arxiv", "math.AG/0601001v1", "math.AG/0601001"},
		{"arxiv", "0704.0001", "0704.0001"},
		{"arxiv_v", "V3", "v3"},
		{"zbl", "Zbl 0346.26002", "0346.26002"},
		{"zbl", "3456789", "3456789"},
		{"jfm", "JFM 27.0011.01", "27.0011.01"},
		{"mr", "MR0385023", "385023"},
		{"isbn", "0-07-054235-X", "9780070542358"},
		{"isbn", "978-0-07-054235-8", "9780070542358"},
		{"issn", "0003486x", "0003-486X"},
		{"issn_l", "0003-486X", "0003-486X"},
		{"oclc", "(OCoLC)ocm01502298", "1502298"},
		{"oclc", "ocm01502298", "1502298"},
		{"lccn", "n78-890351", "n78890351"},
		{"lccn", "85-2 ", "85000002"},
		{"lccn", "2001-000002/AC", "2001000002"},
		{"openlibrary_work", "/works/ol45883w", "OL45883W"},
		{"openlibrary_edition", "https://openlibrary.org/books/OL7353617M", "OL7353617M"},
		{"wikidata", "https://www.wikidata.org/wiki/q1000001", "Q1000001"},
		{"openalex", "https://openalex.org/w2741809807", "W2741809807"},
		{"s2", "CorpusId:0123", "123"},
		{"hal", "hal-01234567v2", "hal-01234567"},
		{"hal", "TEL-00123456", "tel-00123456"},
		{"eudml", "urn:eudml:doc:128467", "128467"},
		{"hathi", "mdp.39015017324221", "mdp.39015017324221"},
		{"hathi_record", "https://catalog.hathitrust.org/record/001234567", "1234567"},
		{"ia", "https://archive.org/details/principlesofmath00rudi", "principlesofmath00rudi"},
		{"gutenberg", "https://www.gutenberg.org/ebooks/33283", "33283"},
		{"gbooks", "4RuWQgAACAAJ", "4RuWQgAACAAJ"},
		{"handle", "hdl:1721.1/12345", "1721.1/12345"},
		{"urn", "URN:NBN:de:101:1-2016", "urn:nbn:de:101:1-2016"},
		{"course", "042nb2s44:18.06", "042nb2s44:18.06"},
		{"offering", "042nb2s44:18.06:2010-spring", "042nb2s44:18.06:2010-spring"},
		{"ocw", "18-06-linear-algebra-spring-2010", "18-06-linear-algebra-spring-2010"},
		{"nptel", "111106051", "111106051"},
		{"youtube_playlist", "PL49CF3715CB9EF31D", "PL49CF3715CB9EF31D"},
		{"youtube_video", "QVKj3LADCnA", "QVKj3LADCnA"},
		{"swmath", "https://zbmath.org/software/825", "825"},
		{"carmin", "https://www.carmin.tv/fr/video/complexity-of-lasso-with-normalized-data", "complexity-of-lasso-with-normalized-data"},
		{"carmin", "oai:carmin.tv:fuzzy-sphere-meets-cft-4-4", "fuzzy-sphere-meets-cft-4-4"},
		{"carmin", "oai:carmin.tv:expression-of-information-from-cell-", "expression-of-information-from-cell-"},
		{"carmin_collection", "videos:collection:the-pentagram-map-and-ramifications", "the-pentagram-map-and-ramifications"},
		{"zenodo", "https://zenodo.org/records/1234567", "1234567"},
		{"swh", "swh:1:rev:309cf2674ee7a0749978cf8265ab91a60aea0f7d", "swh:1:rev:309cf2674ee7a0749978cf8265ab91a60aea0f7d"},
		{"source", "loc-marc", "loc-marc"},
		{"ror", "https://ror.org/042NB2S44", "042nb2s44"},
		{"orcid", "https://orcid.org/0000-0002-1825-0097", "0000-0002-1825-0097"},
		{"orcid", "0000-0002-1694-233x", "0000-0002-1694-233X"},
		{"mgp", "https://www.mathgenealogy.org/id.php?id=18231", "18231"},
		{"nnt", "https://theses.fr/2020upass008", "2020UPASS008"},
		{"nnt", "1965REN0S021", "1965REN0S021"},
		{"idref", "https://www.idref.fr/072229802", "072229802"},
		{"idref", "18447518x", "18447518X"},
	}
	for _, c := range cases {
		got, err := Normalize(c.key, c.in)
		switch {
		case err != nil:
			t.Errorf("%s %q: %v", c.key, c.in, err)
		case got != c.want:
			t.Errorf("%s %q: got %q, want %q", c.key, c.in, got, c.want)
		}
	}
}

func TestInvalid(t *testing.T) {
	cases := []struct{ key, in string }{
		{"doi", "11.1007/x"},
		{"doi", "10.12/x"},
		{"arxiv", "2213.00001"},
		{"arxiv", "1502.0001"},
		{"arxiv", "1412.00001"},
		{"arxiv", "math/060100"},
		{"zbl", "346.26002"},
		{"jfm", "27.11.01"},
		{"mr", "MR"},
		{"isbn", "9780070542359"},
		{"isbn", "ISBN 0070542353x"},
		{"isbn", "0070542354"},
		{"isbn", "9770070542358"},
		{"issn", "0003-4861"},
		{"issn", "0003-48X6"},
		{"lccn", "n7889035"},
		{"openlibrary_work", "OL45883M"},
		{"wikidata", "P31"},
		{"wikidata", "Q0"},
		{"openalex", "A123"},
		{"hal", "hal-123"},
		{"gbooks", "short"},
		{"youtube_video", "QVKj3LADCn"},
		{"carmin", "Two Words"},
		{"carmin_collection", "-a"},
		{"swh", "swh:1:rev:309cf"},
		{"source", "OEIS"},
		{"ror", "042nb2s45"},
		{"ror", "142nb2s44"},
		{"orcid", "0000-0002-1825-0098"},
		{"course", "042nb2s45:18.06"},
		{"course", "042nb2s44"},
		{"offering", "042nb2s44:2010-spring"},
		{"offering", "042nb2s44:18.06:spring"},
		{"numdam", "two words"},
		{"mgp", "abc"},
		{"nnt", "2020UPAS008"},
		{"nnt", "s12345"},
		{"idref", "072229803"},
		{"idref", "07222980"},
	}
	for _, c := range cases {
		if got, err := Normalize(c.key, c.in); err == nil {
			t.Errorf("%s %q: got %q, want an error", c.key, c.in, got)
		}
	}
}

func TestCheck(t *testing.T) {
	if err := Check("isbn", "9780070542358"); err != nil {
		t.Fatal(err)
	}
	if err := Check("isbn", "0-07-054235-X"); err == nil {
		t.Fatal("a valid id that is not in normal form must fail")
	}
	if err := Check("doi", "10.2307/1969265"); err != nil {
		t.Fatal(err)
	}
	if err := Check("doi", "10.1215/S0012"); err == nil {
		t.Fatal("a DOI in upper case is not in normal form")
	}
	if err := Check("isni", "0000000121032683"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("got %v, want ErrUnknown", err)
	}
}

func TestKeys(t *testing.T) {
	ks := Keys()
	if len(ks) != len(systems) || !Known("doi") || Known("isni") {
		t.Fatal("Keys and Known do not agree with the table")
	}
	for i := 1; i < len(ks); i++ {
		if ks[i-1] >= ks[i] {
			t.Fatal("Keys is not sorted")
		}
	}
}

func TestSplitArxiv(t *testing.T) {
	cases := []struct{ in, id, v string }{
		{"arXiv:2203.17207v2", "2203.17207", "v2"},
		{"math/0601001", "math/0601001", ""},
		{"math.AG/0601001v12", "math.AG/0601001", "v12"},
	}
	for _, c := range cases {
		id, v, err := SplitArxiv(c.in)
		if err != nil || id != c.id || v != c.v {
			t.Errorf("%q: got %q %q %v", c.in, id, v, err)
		}
	}
}

func TestISBN10(t *testing.T) {
	for in, want := range map[string]string{
		"9780070542358": "007054235X",
		"9780387953359": "0387953353",
		"9791032305690": "",
	} {
		if got := ISBN10(in); got != want {
			t.Errorf("%s: got %q, want %q", in, got, want)
		}
	}
}

func FuzzNormalize(f *testing.F) {
	f.Add("isbn", "0-07-054235-X")
	f.Add("orcid", "0000-0002-1825-0097")
	f.Add("offering", "042nb2s44:18.06:2010-spring")
	f.Fuzz(func(t *testing.T, key, v string) {
		n, err := Normalize(key, v)
		if err != nil {
			return
		}
		if err := Check(key, n); err != nil {
			t.Fatalf("the normal form %q of %q does not pass Check: %v", n, v, err)
		}
	})
}
