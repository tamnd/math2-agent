package release

import (
	"cmp"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// name matches a release name: a monthly release YYYY.MM, or a patch release YYYY.MM.N (spec 06.2).
var name = regexp.MustCompile(`^([0-9]{4})\.(0[1-9]|1[0-2])(?:\.([1-9][0-9]*))?$`)

// IsName reports whether s is the name of a release: YYYY.MM for a monthly release, or YYYY.MM.N for a patch release.
func IsName(s string) bool { return name.MatchString(s) }

// CompareNames compares two release names in the order of time. A patch release comes after its monthly release, and 2027.04.10 comes after 2027.04.9. A name that is not a release name comes first.
func CompareNames(a, b string) int {
	return slices.Compare(parts(a), parts(b))
}

// parts returns the year, the month and the patch number of a release name.
func parts(s string) []int {
	m := name.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	out := make([]int, 3)
	for i, p := range m[1:] {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

// NextName returns the name of the next release in the month of t. It is YYYY.MM when the month has no release in names. Otherwise it is the next patch release of the month.
func NextName(names []string, t time.Time) string {
	month := t.UTC().Format("2006.01")
	patch := -1
	for _, n := range names {
		p := parts(n)
		if p == nil || fmt.Sprintf("%04d.%02d", p[0], p[1]) != month {
			continue
		}
		patch = max(patch, p[2])
	}
	if patch < 0 {
		return month
	}
	return fmt.Sprintf("%s.%d", month, patch+1)
}

// List returns the names of the releases that have a manifest in the tree root, in the order of time. The file next.yaml, which pins the m2 version of CI, is not a release.
func List(root string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(Dir), "*.yaml"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range paths {
		if n := strings.TrimSuffix(filepath.Base(p), ".yaml"); IsName(n) {
			out = append(out, n)
		}
	}
	slices.SortFunc(out, func(a, b string) int { return cmp.Or(CompareNames(a, b), strings.Compare(a, b)) })
	return out, nil
}

// ManifestFile returns the path of the manifest of the release from the tree root, with forward slashes.
func ManifestFile(release string) string { return path.Join(Dir, release+".yaml") }

// LevelsFile returns the path of the levels of the nodes at the release from the tree root, with forward slashes.
func LevelsFile(release string) string { return path.Join(Dir, release+".levels") }

// NotesFile returns the path of the release notes from the tree root, with forward slashes.
func NotesFile(release string) string { return path.Join(Dir, release+".md") }
