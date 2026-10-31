package copycheck

import (
	"strings"
	"testing"
)

// TestFingerprint checks stage 3 with the fingerprint of a closed item, which the corpus has with no text. The test wrote the item text itself.
func TestFingerprint(t *testing.T) {
	item := `Let $R$ be a commutative ring with unit, and let $M$ be a finitely generated module over $R$. If $IM = M$ for an ideal $I$ contained in the Jacobson radical of $R$, then $M = 0$, see \cite{AM}.`
	fp := itemMinHash(itemSignature(itemNormalize(item)))
	c := New([]Text{{Source: "arxiv", URI: "ax://paper/2401.00001#NAK1", Licence: "arxiv-1.0", Class: "record", MinHash: fp}}, nil)

	// The same item with other spacing, another citation and a label is the same fingerprint.
	copied := "Some words first.\n\n" + strings.ReplaceAll(item, `\cite{AM}`, `\cite[Cor. 2.5]{Mat}`) + `\label{x}`
	r := c.Check(Section{Field: "body.statement", Text: copied})
	if !r.Blocks() || len(r.Findings) != 1 || r.Findings[0].Stage != 3 || r.Findings[0].URI != "ax://paper/2401.00001#NAK1" || r.Findings[0].Similarity < 0.9 {
		t.Fatalf("the report of a copy is %+v", r.Findings)
	}
	if f := r.Findings[0]; copied[f.Start:f.End] != copied[strings.Index(copied, "Let"):] {
		t.Errorf("the match is the part %q", copied[f.Start:f.End])
	}

	own := `For a module of finite type over a local ring, the module is zero when its image modulo the maximal ideal is zero; this follows by induction on the number of generators.`
	if r := c.Check(Section{Field: "body.statement", Text: own}); len(r.Findings) != 0 {
		t.Errorf("the report of a new text is %+v", r.Findings)
	}
	if r := c.Check(Section{Field: "body.statement", Text: "Then $M = 0$."}); len(r.Findings) != 0 {
		t.Errorf("a short text gives %+v", r.Findings)
	}
}

func TestParseItem(t *testing.T) {
	sig := itemSignature("a text")
	got, ok := parseItem(itemMinHash(sig))
	if !ok || got != sig {
		t.Errorf("parseItem gives %v, %v", got, ok)
	}
	if _, ok := parseItem("not base64!"); ok {
		t.Error("parseItem reads a string that is not a fingerprint")
	}
}

// TestArxivReader pins the fingerprint that the fingerprint package of arxiv-reader gives for one text, thus stage 3 reads the fingerprints of the object records.
func TestArxivReader(t *testing.T) {
	const want = "uY6lVCZT/i17pmExIj+h7nlg9muZr9G5kubbsSgeuBovow8dFOuI9UyIcDPuUxeMLpQZ7opfoUT6qSRR3AgScDil0qSRlTxvwNcbLVcJ7WBkfRFhWHBd8wx9U5s90aLrKgFTLusf58lEX6UIudB2bJdj8NRZd1Bvaf4DrkRQwmeIZeLbEI6+/Va1hIXpOHfL2VaM8apAbLvn4sruGUy6iVSgqWQptXO46AcWJ24cErKOYqC1gmzCpJYURonsfcLWJvfSYGlF+i/oE9ExaLL7U/gJ1V409+HA2kq55xqPoWzclQWG/Uza+FgxiT3ZJgbShPk3v8SvtbQzgXtO2LZmXA=="
	if got := itemMinHash(itemSignature(itemNormalize(`Every compact metric space is complete, see \cite{Ke}.`))); got != want {
		t.Errorf("the fingerprint is %s", got)
	}
}
