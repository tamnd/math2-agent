package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/math2-agent/ingest/arxiv"
	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census"
	"github.com/tamnd/math2-agent/resource/scope"
)

// Arxiv reads each mathematics paper of the metadata plane of tamnd/arxiv in a local checkout. It does not sample: the plane is a full dump, so each record has the weight 1.
type Arxiv struct{ Dir string }

// Name gives arxiv.
func (Arxiv) Name() string { return "arxiv" }

type axPaper struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Authors []struct {
		Surname  string `json:"surname"`
		Forename string `json:"forename"`
	} `json:"authors"`
	Categories []string `json:"categories"`
	DOI        string   `json:"doi"`
	MSCClass   string   `json:"msc_class"`
}

// Sample reads the files of the months of the plane in the order of their names. It ignores n and the seed.
func (a Arxiv) Sample(ctx context.Context, _ *Client, _ int, _ uint64, emit func(census.Record) error) (Info, error) {
	if a.Dir == "" {
		return Info{}, fmt.Errorf("arxiv: give the directory of the metadata plane")
	}
	var files []string
	for _, pat := range []string{"*" + mdrec.Ext, "*" + mdrec.Old} {
		m, err := filepath.Glob(filepath.Join(a.Dir, pat))
		if err != nil {
			return Info{}, err
		}
		files = append(files, m...)
	}
	sort.Strings(files)
	info := Info{Filter: "the mathematics papers of the metadata plane"}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return info, err
		}
		recs, err := mdrec.Read(f)
		if err != nil {
			return info, fmt.Errorf("arxiv: %s: %w", f, err)
		}
		for _, raw := range recs {
			var p axPaper
			if err := json.Unmarshal(raw, &p); err != nil {
				return info, fmt.Errorf("arxiv: %s: %w", f, err)
			}
			if !arxiv.Math(p.Categories) {
				continue
			}
			info.Total++
			if err := emit(axRecord(p)); err != nil {
				return info, err
			}
		}
	}
	return info, nil
}

func axRecord(p axPaper) census.Record {
	r := census.Record{NativeID: p.ID, Kind: "preprint", Date: ArxivYear(p.ID), Title: p.Title, IDs: resource.IDs{"arxiv": {p.ID}}}
	for _, au := range p.Authors {
		if au.Forename != "" {
			r.Authors = append(r.Authors, au.Surname+", "+au.Forename)
		} else {
			r.Authors = append(r.Authors, au.Surname)
		}
	}
	if p.DOI != "" {
		r.IDs["doi"] = resource.Values{p.DOI}
	}
	r.Signals.Arxiv = p.Categories
	r.Signals.MSC = arxiv.MSC(p.MSCClass)
	return r
}

// ArxivYear gives the year of an arXiv id from its YYMM part: 0704.0002 gives 2007 and math/9912001 gives 1999.
func ArxivYear(id string) string {
	if _, num, ok := strings.Cut(id, "/"); ok {
		id = num
	}
	if len(id) < 2 || id[0] < '0' || id[0] > '9' || id[1] < '0' || id[1] > '9' {
		return ""
	}
	if id[0] >= '9' {
		return "19" + id[:2]
	}
	return "20" + id[:2]
}

func scopeTopic(field string, score float64) scope.Topic {
	return scope.Topic{Field: field, Score: score}
}
