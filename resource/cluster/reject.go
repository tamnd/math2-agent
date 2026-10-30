package cluster

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/resource"
)

// RejectedDir holds the records that an import rejected, one file of records for each source (RS11.2). It is rebuilt with the source records and is not in git.
const RejectedDir = "build/resources/rejected"

// Reject is one record that an import rejected, with the reason.
type Reject struct {
	NativeID string `json:"native_id,omitempty"`
	Snapshot string `json:"snapshot"`
	Reason   string `json:"reason"`
}

// Check gives the reason to reject a source record, or "" when the record is good. A record needs a native id, a title and a kind of resource.Kinds.
func Check(r *Record) string {
	switch {
	case r.NativeID == "":
		return "the record has no native id"
	case strings.TrimSpace(r.Title) == "":
		return "the record has no title"
	case !slices.Contains(resource.Kinds, r.Kind):
		return fmt.Sprintf("the kind %q is not a kind of resource", r.Kind)
	}
	return ""
}

// WriteRejects writes the rejected records of one source to RejectedDir/<source>.md in the tree at root. With no rejected record, it removes the file.
func WriteRejects(root, source string, rs []Reject) error {
	if strings.ContainsAny(source, "/\\") || source == "" {
		return fmt.Errorf("cluster: the source %q is not a file name", source)
	}
	path := filepath.Join(root, RejectedDir, source+mdrec.Ext)
	if len(rs) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	recs, err := mdrec.Marshal(rs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return mdrec.Write(path, recs)
}

// ReadRejects reads the rejected records of one source. A source with no file has none.
func ReadRejects(root, source string) ([]Reject, error) {
	path := filepath.Join(root, RejectedDir, source+mdrec.Ext)
	recs, err := mdrec.Read(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return mdrec.Unmarshal[Reject](recs)
}
