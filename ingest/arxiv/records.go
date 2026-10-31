package arxiv

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	axcat "github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Records reads the metadata plane and emits one source record for each mathematics paper (RS13.1). The kind is preprint, or lecture-notes when the rule set NotesRules marks the paper (RS7.1). The date is the date of the first version, or the month of the id when the plane has no date. The MSC codes come from the msc-class field, or else from the MSC areas of the arXiv categories (importer/arxiv.CategoryMSC). The DOI of the published version goes into the ids, because the preprint and the publication are expressions of one work (RS10.4).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var p paper
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if !Math(p.Categories) {
			continue
		}
		if err := emit(sourceRecord(p)); err != nil {
			return err
		}
	}
	return sc.Err()
}

// sourceRecord makes the source record of one paper.
func sourceRecord(p paper) cluster.Record {
	rec := cluster.Record{
		NativeID: p.ID,
		Level:    "work",
		Kind:     "preprint",
		Title:    strings.Join(strings.Fields(p.Title), " "),
		Date:     Date(p.ID),
		MSC:      MSC(p.MSCClass),
		IDs:      resource.IDs{},
		Links:    []string{"https://arxiv.org/abs/" + p.ID},
	}
	if len(rec.MSC) == 0 {
		rec.MSC = axcat.CategoryMSC(p.Categories)
	}
	if _, ok := Notes(p.Title, p.Comments, p.Categories); ok {
		rec.Kind = "lecture-notes"
	}
	first := 0
	for _, v := range p.Versions {
		if len(v.Created) >= 10 && (first == 0 || v.Version < first) {
			first, rec.Date = v.Version, v.Created[:10]
		}
	}
	for _, a := range p.Authors {
		if s := strings.Join(strings.Fields(a.Forename+" "+a.Surname+" "+a.Suffix), " "); s != "" {
			rec.Creators = append(rec.Creators, resource.Creator{Name: s, Role: "author"})
		}
	}
	for _, c := range p.Categories {
		rec.Subjects = append(rec.Subjects, resource.Subject{System: "arxiv", Code: c})
	}
	if v, err := ids.Normalize("arxiv", p.ID); err == nil {
		rec.IDs["arxiv"] = resource.Values{v}
	}
	if v, err := ids.Normalize("doi", p.DOI); err == nil {
		rec.IDs["doi"] = resource.Values{v}
	}
	return rec
}

// Date gives the year and the month of an arXiv id, YYYY-MM, or an empty string. The month is the month of the first version. An id of the old form, as math/9901001, has a year from 1991.
func Date(id string) string {
	_, num, old := strings.Cut(id, "/")
	if !old {
		num = id
	}
	if len(num) < 4 {
		return ""
	}
	for _, c := range num[:4] {
		if c < '0' || c > '9' {
			return ""
		}
	}
	century := "20"
	if old && num[:2] >= "91" {
		century = "19"
	}
	if num[2:4] < "01" || num[2:4] > "12" {
		return ""
	}
	return century + num[:2] + "-" + num[2:4]
}
