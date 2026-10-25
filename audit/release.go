package audit

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

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

// manifests reads the release manifests in manifests/releases/, in order of release. An audit of a pull request reads only the last one, as a sample. A manifest that does not parse gives a finding.
func (a *auditor) manifests(o *outcome) []manifest {
	paths, _ := filepath.Glob(filepath.Join(a.root, filepath.FromSlash(release.Dir), "*.yaml"))
	slices.Sort(paths)
	if a.pr && len(paths) > 1 {
		paths = paths[len(paths)-1:]
	}
	var out []manifest
	for _, p := range paths {
		rel, _ := filepath.Rel(a.root, p)
		rel = filepath.ToSlash(rel)
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
	for _, m := range ms {
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
	for _, m := range ms {
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
