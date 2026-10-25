package audit

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/bump"
	"github.com/tamnd/math2-agent/release"
)

// manifest is one release manifest of the tree, with the positions of its keys.
type manifest struct {
	*release.Manifest
	path string
	pos  map[string][2]int // the line and the column of each key at the top level
}

// at returns the position of the key in the manifest, or of the start of the file when the key is not there.
func (m manifest) at(key string) (line, col int) {
	if p, ok := m.pos[key]; ok {
		return p[0], p[1]
	}
	return 1, 1
}

// manifests reads the release manifests in manifests/releases/, in the order of time. An audit of a pull request checks only the last one, as a sample, and then the slice has the last manifest and the manifest before it, which rule R03 needs. A manifest that does not parse gives a finding.
func (a *auditor) manifests(o *outcome) []manifest {
	names, _ := release.List(a.root)
	if a.pr && len(names) > 2 {
		names = names[len(names)-2:]
	}
	var out []manifest
	for _, n := range names {
		rel := release.ManifestFile(n)
		p := filepath.Join(a.root, filepath.FromSlash(rel))
		m, err := release.Read(p)
		if err != nil {
			o.checked++
			o.add(rel, 1, 1, "the manifest does not parse: %v", err)
			continue
		}
		mf := manifest{Manifest: m, path: rel, pos: map[string][2]int{}}
		var doc yaml.Node
		if data, err := os.ReadFile(p); err == nil && yaml.Unmarshal(data, &doc) == nil && len(doc.Content) > 0 { // #nosec G304 -- the path comes from a glob below the tree.
			c := doc.Content[0].Content
			for i := 0; i+1 < len(c); i += 2 {
				mf.pos[c[i].Value] = [2]int{c[i].Line, c[i].Column}
			}
		}
		out = append(out, mf)
	}
	return out
}

// sample returns the manifests that a rule checks: all of them, or the last one in an audit of a pull request.
func (a *auditor) sample(ms []manifest) []manifest {
	if a.pr && len(ms) > 1 {
		return ms[len(ms)-1:]
	}
	return ms
}

// r01 rebuilds the derived graph of each release manifest from its commit and compares the hash with the manifest (rule R01). The rebuild check of the manifest must also say that the second machine reproduced the hash.
func r01(a *auditor) outcome {
	var o outcome
	if !isRepo(a.root) {
		o.reason = "the tree is not a git repository"
		return o
	}
	ms := a.manifests(&o)
	if len(ms) == 0 && o.checked == 0 {
		o.reason = "the tree has no release manifest in " + release.Dir
		return o
	}
	for _, m := range a.sample(ms) {
		o.checked++
		line, col := m.at("artefacts")
		want, ok := m.Artefacts[release.Graph]
		if !ok {
			o.add(m.path, line, col, "the manifest of %s has no graph artefact", m.Release)
			continue
		}
		got, err := release.RebuildGraph(context.Background(), a.root, m.Commit)
		switch {
		case err != nil:
			l, c := m.at("commit")
			o.add(m.path, l, c, "the graph of %s does not rebuild from the commit %s: %s. A shallow clone does not have the commit. Fetch the full history", m.Release, m.Commit, firstLine(err.Error()))
		case got.Hash != want.Hash:
			o.add(m.path, line, col, "the graph of %s rebuilds to %s, and the manifest has %s", m.Release, got.Hash, want.Hash)
		}
		if rc := m.RebuildCheck; rc != nil && !rc.GraphHashReproduced {
			l, c := m.at("rebuild_check")
			o.add(m.path, l, c, "the rebuild of %s on %s did not reproduce the graph hash", m.Release, rc.Runner)
		}
	}
	return o
}

// r02 checks the rebuild of the .olean files of each release manifest (rule R02). The rebuild check of the manifest must say that the second machine reproduced the hash, or list each file that is different with a reason.
func r02(a *auditor) outcome {
	var o outcome
	ms := a.manifests(&o)
	if len(ms) == 0 && o.checked == 0 {
		o.reason = "the tree has no release manifest in " + release.Dir
		return o
	}
	for _, m := range a.sample(ms) {
		o.checked++
		if _, ok := m.Artefacts[release.Oleans]; !ok {
			line, col := m.at("artefacts")
			o.add(m.path, line, col, "the manifest of %s has no oleans artefact", m.Release)
			continue
		}
		line, col := m.at("rebuild_check")
		rc := m.RebuildCheck
		switch {
		case rc == nil:
			o.add(m.path, line, col, "the manifest of %s has no rebuild check. Rebuild the release on a second machine and record the result", m.Release)
		case rc.OleansHashReproduced:
		case len(rc.OleanDifferences) == 0:
			o.add(m.path, line, col, "the rebuild of %s on %s did not reproduce the .olean files, and the manifest lists no difference", m.Release, rc.Runner)
		default:
			for _, d := range rc.OleanDifferences {
				if strings.TrimSpace(d.Reason) == "" {
					o.add(m.path, line, col, "the rebuild of %s gives a different %s, and the manifest gives no reason", m.Release, d.Path)
				}
			}
		}
	}
	return o
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

// r03 checks that the notes of each release give the cause of each change in a count of its manifest (rule R03). The counts table of the notes must have a row for each count that changed since the release before, with the same numbers and a cause, and no other row.
func r03(a *auditor) outcome {
	var o outcome
	ms := a.manifests(&o)
	if len(ms) == 0 && o.checked == 0 {
		o.reason = "the tree has no release manifest in " + release.Dir
		return o
	}
	prev := map[string]*release.Manifest{}
	for i := 1; i < len(ms); i++ {
		prev[ms[i].Release] = ms[i-1].Manifest
	}
	for _, m := range a.sample(ms) {
		o.checked++
		path := release.NotesFile(m.Release)
		data, err := os.ReadFile(filepath.Join(a.root, filepath.FromSlash(path))) // #nosec G304 -- the path is below the tree.
		if err != nil {
			l, c := m.at("counts")
			o.add(m.path, l, c, "the release %s has no notes in %s. Run m2 release notes %s", m.Release, path, m.Release)
			continue
		}
		n, err := release.ParseNotes(data)
		if err != nil {
			o.add(path, 1, 1, "%v", err)
			continue
		}
		rows := map[string]release.Change{}
		for _, c := range n.Counts {
			rows[c.Count] = c
		}
		line := countsLine(data)
		for _, want := range release.Changes(prev[m.Release], m.Manifest) {
			got, ok := rows[want.Count]
			delete(rows, want.Count)
			switch {
			case !ok:
				o.add(path, line, 1, "the notes do not give the change of %s from %d to %d", want.Count, want.Before, want.After)
			case got.Before != want.Before || got.After != want.After:
				o.add(path, line, 1, "the notes give the change of %s from %d to %d, and the manifests give %d to %d. Run m2 release notes %s again", want.Count, got.Before, got.After, want.Before, want.After, m.Release)
			case strings.TrimSpace(got.Cause) == "":
				o.add(path, line, 1, "the notes give no cause for the change of %s from %d to %d", want.Count, want.Before, want.After)
			}
		}
		for _, k := range slices.Sorted(maps.Keys(rows)) {
			o.add(path, line, 1, "the notes give a change of %s, and the manifests give no change of it", k)
		}
	}
	return o
}

// countsLine returns the line of the counts heading in notes, or 1.
func countsLine(data []byte) int {
	for i, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == "## Counts" {
			return i + 1
		}
	}
	return 1
}

// restoreDir is the directory of the records of the restore drills in the tree.
const restoreDir = "reports/restore"

// drill is the record of one restore drill, reports/restore/YYYY-MM-DD.yaml (spec 06.6).
type drill struct {
	Date    string `yaml:"date"`
	Release string `yaml:"release"`
	Runner  string `yaml:"runner"`
	Passed  bool   `yaml:"passed"`
	Note    string `yaml:"note"`
}

// quarter is the longest time between two restore drills.
const quarter = 92 * 24 * time.Hour

// r04 checks that the last restore drill passed, and that it is not older than one quarter at the last release (rule R04). A tree without a record of a drill gives n/a, because the first drill comes after the first release is archived.
func r04(a *auditor) outcome {
	var o outcome
	paths, _ := filepath.Glob(filepath.Join(a.root, filepath.FromSlash(restoreDir), "*.yaml"))
	slices.Sort(paths)
	var last *drill
	var lastPath string
	for _, p := range paths {
		rel, _ := filepath.Rel(a.root, p)
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(p) // #nosec G304 -- the path comes from a glob below the tree.
		d := &drill{}
		if err == nil {
			err = yaml.Unmarshal(data, d)
		}
		if _, perr := time.Parse(time.DateOnly, d.Date); err == nil && perr != nil {
			err = fmt.Errorf("the date %q is not YYYY-MM-DD", d.Date)
		}
		if err != nil {
			o.checked++
			o.add(rel, 1, 1, "the record of the drill does not parse: %v", err)
			continue
		}
		if last == nil || d.Date >= last.Date {
			last, lastPath = d, rel
		}
	}
	if last == nil {
		if o.checked == 0 {
			o.reason = "the tree has no record of a restore drill in " + restoreDir
		}
		return o
	}
	o.checked++
	if !last.Passed {
		o.add(lastPath, 1, 1, "the restore drill of %s on the release %s failed. A failed drill blocks the next release", last.Date, last.Release)
	}
	names, _ := release.List(a.root)
	if len(names) == 0 {
		return o
	}
	newest := names[len(names)-1]
	cut, _ := time.Parse("2006.01", newest[:7])
	done, _ := time.Parse(time.DateOnly, last.Date)
	if cut.Sub(done) > quarter {
		o.add(release.ManifestFile(newest), 1, 1, "the last restore drill is of %s, more than one quarter before the release %s. Run a drill before the release", last.Date, newest)
	}
	return o
}

// bumpFrom matches the first sentence of a bump report, which gives the new tag and the short commit of Mathlib.
var bumpFrom = regexp.MustCompile(`Mathlib moves from .* to (\S+) \(([0-9a-f]{12})\)\.`)

// bumpGreen is the sentence of a bump report when the build and the audit pass with the new pins.
const bumpGreen = "The build and the audit pass with the new pins."

// r05 checks that each release pins one stable Lean toolchain and one Mathlib commit, that the Lake project of the commit of the release has the same pins, and that the last bump to the Mathlib commit was green (rule R05).
func r05(a *auditor) outcome {
	var o outcome
	ms := a.manifests(&o)
	if len(ms) == 0 && o.checked == 0 {
		o.reason = "the tree has no release manifest in " + release.Dir
		return o
	}
	repo := isRepo(a.root)
	reports, _ := filepath.Glob(filepath.Join(a.root, "reports", "bump", "*.md"))
	slices.Sort(reports)
	for _, m := range a.sample(ms) {
		o.checked++
		line, col := m.at("toolchain")
		if err := m.CheckPins(); err != nil {
			for _, e := range strings.Split(err.Error(), "\n") {
				o.add(m.path, line, col, "%s", e)
			}
		}
		if repo {
			for _, p := range pinsAt(a.root, m.Manifest) {
				o.add(m.path, line, col, "%s", p)
			}
		}
		if len(m.Toolchain.Mathlib) < 12 {
			continue
		}
		var lastReport string
		var green bool
		for _, p := range reports {
			data, err := os.ReadFile(p) // #nosec G304 -- the path comes from a glob below the tree.
			if err != nil {
				continue
			}
			if s := bumpFrom.FindSubmatch(data); s != nil && string(s[2]) == m.Toolchain.Mathlib[:12] {
				lastReport, green = p, strings.Contains(string(data), bumpGreen)
			}
		}
		if lastReport != "" && !green {
			rel, _ := filepath.Rel(a.root, lastReport)
			o.add(m.path, line, col, "the release %s pins the Mathlib commit of %s, and that bump is not green", m.Release, filepath.ToSlash(rel))
		}
	}
	return o
}

// pinsAt compares the pins of the Lake project at the commit of the manifest with the manifest, and returns each problem.
func pinsAt(root string, m *release.Manifest) []string {
	tmp, err := os.MkdirTemp("", "m2-r05-")
	if err != nil {
		return []string{err.Error()}
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	for _, f := range []string{"lean-toolchain", "lakefile.toml", "lake-manifest.json"} {
		if strings.HasPrefix(m.Commit, "-") {
			return []string{fmt.Sprintf("%q is not a commit", m.Commit)}
		}
		data, err := git(root, "show", m.Commit+":lean/"+f)
		if err != nil {
			if f == "lake-manifest.json" {
				continue
			}
			return []string{fmt.Sprintf("the commit %s of the release %s has no lean/%s, or the repository does not have the commit", m.Commit, m.Release, f)}
		}
		if err := os.WriteFile(filepath.Join(tmp, f), []byte(data), 0o600); err != nil {
			return []string{err.Error()}
		}
	}
	p, err := bump.ReadPins(tmp)
	if err != nil {
		return []string{fmt.Sprintf("the pins of the commit %s do not read: %v", m.Commit, err)}
	}
	var out []string
	if p.Toolchain != m.Toolchain.Lean {
		out = append(out, fmt.Sprintf("the commit of the release %s pins the Lean toolchain %s, and the manifest has %s", m.Release, p.Toolchain, m.Toolchain.Lean))
	}
	if p.Commit == "" {
		out = append(out, fmt.Sprintf("the commit of the release %s has no Mathlib commit in lean/lake-manifest.json", m.Release))
	} else if p.Commit != m.Toolchain.Mathlib {
		out = append(out, fmt.Sprintf("the commit of the release %s pins the Mathlib commit %s, and the manifest has %s", m.Release, p.Commit, m.Toolchain.Mathlib))
	}
	if _, ok := bump.Latest([]string{p.Tag}); !ok {
		out = append(out, fmt.Sprintf("the commit of the release %s pins Mathlib at %s, which is not a release tag", m.Release, p.Tag))
	}
	return out
}
