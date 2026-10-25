package coverage

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/parquet-go/parquet-go"

	"github.com/tamnd/math2-agent/msc"
)

// Report is the coverage of one state of the tree: a release, or next for the work after the last release.
type Report struct {
	Name     string    // the name of the report, for example 2027.04 or next
	Rows     []Row     // the rows of Compute
	Expected *Expected // the count of the documents, or nil
}

// WriteParquet writes the rows to w in Parquet. The same rows give the same bytes.
func (r *Report) WriteParquet(w io.Writer) error {
	if err := parquet.Write(w, r.Rows, parquet.CreatedBy("m2", "coverage/1", "")); err != nil {
		return fmt.Errorf("coverage: write parquet: %w", err)
	}
	return nil
}

// Markdown returns the summary of the code and of the codes one level below it, as a Markdown table. The empty code gives the summary of the top-level codes.
func (r *Report) Markdown(code string) []byte {
	var b bytes.Buffer
	byCode := make(map[string]*Row, len(r.Rows))
	for i := range r.Rows {
		byCode[r.Rows[i].Code] = &r.Rows[i]
	}
	var rows []*Row
	if code == "" {
		fmt.Fprintf(&b, "# Coverage of %s\n\n", r.Name)
		b.WriteString("This is the coverage of math2 for each MSC2020 class, at the levels of spec 18.1. Each class has a page with its areas.\n\n")
		for i := range r.Rows {
			if r.Rows[i].Level == 1 {
				rows = append(rows, &r.Rows[i])
			}
		}
	} else if top, ok := byCode[code]; ok {
		fmt.Fprintf(&b, "# Coverage of %s, %s, in %s\n\n", code, top.Label, r.Name)
		fmt.Fprintf(&b, "This is the coverage of math2 for the class and for its areas, at the levels of spec 18.1. The summary of all classes is in [%s.md](../%s.md).\n\n", r.Name, r.Name)
		rows = append(rows, top)
		for i := range r.Rows {
			if msc.Parent(r.Rows[i].Code) == code {
				rows = append(rows, &r.Rows[i])
			}
		}
	}
	if r.Expected != nil {
		fmt.Fprintf(&b, "Documents is the count of the documents of the source %s on %s, and Counted is the records of the same source. C0 is Counted divided by Documents. Records is the records of all sources. ", r.Expected.Source, r.Expected.Date)
	} else {
		b.WriteString("No count of the documents is pinned. ")
	}
	b.WriteString("C0 and C1 have a value only for a class with a count. Skeleton is the items of the skeleton of a class in topics/skeletons/, and Mapped is the items with a live node that has a source. C2 is Mapped divided by Skeleton, thus C2 has a value only for a class with a skeleton. C3 and C4 need the cores of spec 18.3, which are not written yet. A dash means that the level has no value.\n\n")
	b.WriteString("| Code | Area | Records | Counted | Documents | C0 | C1 | Skeleton | Mapped | C2 | C3 | C4 | Nodes | E2 | E5 |\n")
	b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, row := range rows {
		name := row.Code
		if code == "" {
			name = fmt.Sprintf("[%s](%s/%s.md)", row.Code, r.Name, row.Code[:2])
		}
		docs := "-"
		if row.Expected > 0 {
			docs = fmt.Sprint(row.Expected)
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %s | %s | %s | %d | %d | %s | %s | %s | %d | %d | %d |\n", name, labelCell.Replace(row.Label), row.Records, row.Recorded, docs, percent(row.C0), percent(row.C1), row.Skeleton, row.Mapped, percent(row.C2), percent(row.C3), percent(row.C4), row.Nodes, row.E2, row.E5)
	}
	return b.Bytes()
}

// percent formats a level as a percentage with one decimal, or a dash.
func percent(f *float64) string {
	if f == nil {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", *f*100)
}

// Write writes the report to the math2 tree at root: reports/coverage/<name>.parquet with all the rows, reports/coverage/<name>.md with the summary of the classes, and reports/coverage/<name>/<NN>.md with the summary of each class. It removes the other Markdown files of the report. Write returns the paths of the files from root, in the order that it writes them.
func (r *Report) Write(root string) ([]string, error) {
	base := filepath.Join(root, filepath.FromSlash(Dir))
	classes := filepath.Join(base, r.Name)
	if err := os.MkdirAll(classes, 0o750); err != nil {
		return nil, err
	}
	old, err := filepath.Glob(filepath.Join(classes, "*.md"))
	if err != nil {
		return nil, err
	}
	for _, p := range old {
		if err := os.Remove(p); err != nil {
			return nil, err
		}
	}
	var pq bytes.Buffer
	if err := r.WriteParquet(&pq); err != nil {
		return nil, err
	}
	files := []struct {
		rel  string
		data []byte
	}{{r.Name + ".parquet", pq.Bytes()}, {r.Name + ".md", r.Markdown("")}}
	for _, row := range r.Rows {
		if row.Level == 1 {
			files = append(files, struct {
				rel  string
				data []byte
			}{r.Name + "/" + row.Code[:2] + ".md", r.Markdown(row.Code)})
		}
	}
	var paths []string
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(base, filepath.FromSlash(f.rel)), f.data, 0o600); err != nil {
			return paths, err
		}
		paths = append(paths, Dir+"/"+f.rel)
	}
	return paths, nil
}

// labelCell escapes a label for a cell of a Markdown table.
var labelCell = strings.NewReplacer("|", `\|`)
