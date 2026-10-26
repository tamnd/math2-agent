// Package ingest gets pinned snapshots of external sources and turns them into records (spec 09).
//
// A source is an external system such as zbMATH Open, OEIS or the Stacks Project. An adapter reads a snapshot of one source and gives its items. A snapshot is one download of the bulk channel of the source. It has a URL, a hash, a date and the licence of the source. The catalogue in manifests/sources.yaml records each snapshot that math2 used. At build time, nothing depends on a live external service.
//
// Each source has an access class (spec 09.3). The class decides what math2 can store. The licence gate in Import applies the class to each item:
//
//	open            the text is stored and can go into node prose
//	share-alike-nc  the text is not stored, and it must not go into node prose
//	verbatim        the text is not stored, and it must not go into node prose
//	gfdl            the text is not stored, and it must not go into node prose
//	record          only the record and the locator are stored
//
// For each item, the gate keeps the fingerprints of the text, also when it drops the text. The fingerprints cannot give the text back. The audit rules L02, L03 and L04 compare them with the fingerprints of the node bodies to find copied text.
//
// Pull runs one import. It writes the records to build/records/<source>.jsonl and the import manifest to manifests/ingest/<source>.yaml. Two pulls of the same snapshot give the same bytes.
package ingest

import "strings"

// Class is the access class of a source or an item (spec 09.3).
type Class string

// The access classes, from the most open to the most closed.
const (
	Open         Class = "open"
	ShareAlikeNC Class = "share-alike-nc"
	Verbatim     Class = "verbatim"
	GFDL         Class = "gfdl"
	RecordOnly   Class = "record"
)

// Classes returns the access classes, from the most open to the most closed.
func Classes() []Class {
	return []Class{Open, ShareAlikeNC, Verbatim, GFDL, RecordOnly}
}

// Valid reports whether c is one of the access classes.
func (c Class) Valid() bool {
	switch c {
	case Open, ShareAlikeNC, Verbatim, GFDL, RecordOnly:
		return true
	}
	return false
}

// Prose reports whether text of the class can go into node prose. Only open text can.
func (c Class) Prose() bool { return c == Open }

// StoresText reports whether math2 stores text of the class in its records. Only open text is stored. The text of the other classes stays at its home, and the record refers to it.
func (c Class) StoresText() bool { return c == Open }

// classes maps each known licence to its class. The keys are SPDX identifiers.
var classes = map[string]Class{
	"CC0-1.0":            Open,
	"CC-BY-3.0":          Open,
	"CC-BY-4.0":          Open,
	"CC-BY-SA-2.5":       Open,
	"CC-BY-SA-3.0":       Open,
	"CC-BY-SA-4.0":       Open,
	"Apache-2.0":         Open,
	"MIT":                Open,
	"BSD-2-Clause":       Open,
	"BSD-3-Clause":       Open,
	"CC-BY-NC-3.0":       ShareAlikeNC,
	"CC-BY-NC-4.0":       ShareAlikeNC,
	"CC-BY-NC-SA-3.0":    ShareAlikeNC,
	"CC-BY-NC-SA-4.0":    ShareAlikeNC,
	"CC-BY-ND-4.0":       Verbatim,
	"CC-BY-NC-ND-4.0":    Verbatim,
	"GFDL-1.2-only":      GFDL,
	"GFDL-1.2-or-later":  GFDL,
	"GFDL-1.3-only":      GFDL,
	"GFDL-1.3-or-later":  GFDL,
	PublicDomainLicence:  Open, // only when the work is in the public domain in the United States and in the EU (rule L10)
	"LicenseRef-arXiv":   RecordOnly,
	"LicenseRef-unknown": RecordOnly,
}

// ClassOf returns the access class of a licence, given as an SPDX identifier. A licence that is not known gives RecordOnly, because only the record and the locator are safe to store when the licence is not known.
func ClassOf(licence string) Class {
	if c, ok := classes[licence]; ok {
		return c
	}
	return RecordOnly
}

// ShareAlike reports whether the licence asks that a work that uses the text has the same licence and gives attribution. Rule L03 checks the attribution.
func ShareAlike(licence string) bool {
	return strings.Contains(licence, "-SA-") || strings.HasPrefix(licence, "GFDL-")
}
