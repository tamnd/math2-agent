package audit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/compute"
	"github.com/tamnd/math2-agent/node"
)

// computations holds what the rules of the compute plane read.
type computations struct {
	records  []computation
	images   *compute.Images                    // an empty manifest when the file does not exist
	noImages string                             // the reason to skip a rule that needs the manifest
	badImage bool                               // the manifest exists and is not valid
	runs     map[string]map[string]*compute.Run // the runs of build/compute, by tag and architecture
	noRuns   string                             // the reason to skip a rule that needs the runs
}

// computation is one computation node in scope.
type computation struct {
	node *node.Node
	rec  *compute.Record // nil when the record is not valid, and then err says why
	err  error
}

// candidate reports whether the computation is an E4 candidate: it has a certificate and it is not an experiment (spec 08.3 and 08.4).
func (c computation) candidate() bool {
	return c.rec != nil && c.rec.Certificate != nil && c.rec.Role != "experiment"
}

// computeDir is the directory of the runs from the tree root.
const computeDir = "build/compute"

// computed reads the computation records, the image manifest and the runs one time.
func (a *auditor) computed() *computations {
	if a.comp != nil {
		return a.comp
	}
	c := &computations{images: &compute.Images{}}
	for _, n := range a.scopeNodes() {
		if n.Kind() != "computation" || retired(n) {
			continue
		}
		rec, err := compute.FromNode(n)
		c.records = append(c.records, computation{node: n, rec: rec, err: err})
	}
	switch im, err := compute.ReadImages(a.root); {
	case errors.Is(err, fs.ErrNotExist):
		c.noImages = compute.ImagesFile + " does not exist"
	case err != nil:
		c.noImages, c.badImage = err.Error(), true
	default:
		c.images = im
	}
	dir := filepath.Join(a.root, computeDir)
	if _, err := os.Stat(dir); err != nil {
		c.noRuns = computeDir + " does not exist. Run m2 compute verify on amd64 and on arm64 first"
	} else if c.runs, err = compute.ReadRuns(dir); err != nil {
		c.noRuns = err.Error()
	}
	a.comp = c
	return c
}

// enginePos returns the position of a key of the engine of a computation, or of the engine when the key is not there.
func enginePos(n *node.Node, key string) (line, col int) {
	return nestedPos(n, "engine", key)
}

// nestedPos returns the position of the key inner in the map at the top-level key outer of the front matter. When inner is not there, it returns the position of outer.
func nestedPos(n *node.Node, outer, inner string) (line, col int) {
	line, col = keyPos(n, outer)
	if n.Front == nil || len(n.Front.Content) == 0 {
		return line, col
	}
	m := n.Front.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != outer {
			continue
		}
		v := m.Content[i+1]
		for j := 0; j+1 < len(v.Content); j += 2 {
			if v.Content[j].Value == inner {
				return v.Content[j].Line, v.Content[j].Column
			}
		}
	}
	return line, col
}

// c01 checks that each computation names an image of the manifest by digest, an entry point that exists in compute/TAG/ and inputs that are a map (rule C01). A computation with no inputs key has no inputs.
func c01(a *auditor) outcome {
	var o outcome
	c := a.computed()
	if c.badImage {
		o.reason = c.noImages
		return o
	}
	for _, x := range c.records {
		o.checked++
		n := x.node
		if x.err != nil {
			line, col := keyPos(n, "engine")
			o.add(n.Path, line, col, "%s", strings.TrimPrefix(x.err.Error(), "compute: "+n.ID+": "))
			continue
		}
		name, _, _ := compute.SplitRef(x.rec.Image)
		if c.images.Named(name) == nil {
			line, col := enginePos(n, "image")
			o.add(n.Path, line, col, "the image %s is not in %s. Use an image of the manifest", name, compute.ImagesFile)
		}
		if fi, err := os.Stat(filepath.Join(a.root, filepath.FromSlash(x.rec.EntryPath()))); err != nil || !fi.Mode().IsRegular() {
			line, col := enginePos(n, "entry")
			o.add(n.Path, line, col, "the entry point %s does not exist. Put the code of the computation in %s/", x.rec.EntryPath(), x.rec.Dir())
		}
	}
	return o
}

// c02 checks that each E4 candidate gave the same output and the same certificate on the two runner architectures, and that the runs agree with the record (rule C02).
func c02(a *auditor) outcome {
	var o outcome
	c := a.computed()
	var cands []computation
	for _, x := range c.records {
		if x.candidate() {
			cands = append(cands, x)
		}
	}
	if len(cands) == 0 {
		return o
	}
	if c.noRuns != "" {
		o.reason = c.noRuns
		return o
	}
	for _, x := range cands {
		o.checked++
		n, rec := x.node, x.rec
		line, col := keyPos(n, "engine")
		runs := c.runs[rec.Tag]
		ok := true
		for _, arch := range compute.Arches {
			run := runs[arch]
			switch {
			case run == nil:
				o.add(n.Path, line, col, "the computation has no run on %s. Run m2 compute verify %s on an %s runner", arch, rec.Tag, arch)
			case run.Image != rec.Image || run.Entry != rec.Entry:
				o.add(n.Path, line, col, "the run on %s is of %s %s, and the record has %s %s. Run the computation again", arch, run.Image, run.Entry, rec.Image, rec.Entry)
			case run.TimedOut:
				o.add(n.Path, line, col, "the run on %s took longer than its wall time", arch)
			case run.ExitCode != 0:
				o.add(n.Path, line, col, "the run on %s exited with the code %d", arch, run.ExitCode)
			default:
				continue
			}
			ok = false
		}
		if !ok {
			continue
		}
		amd, arm := runs["amd64"], runs["arm64"]
		switch {
		case amd.Output != arm.Output:
			o.add(n.Path, line, col, "the output is %s on amd64 and %s on arm64. A computation that is not deterministic cannot be E4", amd.Output, arm.Output)
		case amd.Certificate != arm.Certificate:
			o.add(n.Path, line, col, "the certificate is %s on amd64 and %s on arm64. A computation that is not deterministic cannot be E4", amd.Certificate, arm.Certificate)
		case rec.Output == nil || rec.Output.Hash != amd.Output:
			l, cl := nestedPos(n, "output", "hash")
			o.add(n.Path, l, cl, "the two runs give the output %s, and the record has %s", amd.Output, recorded(rec.Output))
		case rec.Certificate.Artefact != amd.Certificate:
			l, cl := nestedPos(n, "certificate", "artefact")
			o.add(n.Path, l, cl, "the two runs give the certificate %s, and the record has %s", cmpNone(amd.Certificate), cmpNone(rec.Certificate.Artefact))
		}
	}
	return o
}

func recorded(out *compute.Output) string {
	if out == nil {
		return "no output"
	}
	return cmpNone(out.Hash)
}

func cmpNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// c03 finds the computations whose image, or whose checker image, is not the current image of its name in the manifest (rule C03). The record stays valid, because it names the digest that ran. The next release runs the computation again with the current image.
func c03(a *auditor) outcome {
	var o outcome
	c := a.computed()
	if c.noImages != "" {
		o.reason = c.noImages
		return o
	}
	for _, x := range c.records {
		if x.rec == nil {
			continue
		}
		refs := [][3]string{{x.rec.Image, "engine", "image"}}
		if x.rec.Certificate != nil {
			refs = append(refs, [3]string{x.rec.Certificate.Checker, "certificate", "checker"})
		}
		checked := false
		for _, r := range refs {
			name, _, _ := compute.SplitRef(r[0])
			img := c.images.Named(name)
			if img == nil || len(img.Digests) == 0 {
				continue
			}
			checked = true
			if _, _, ok := c.images.Find(r[0]); !ok {
				line, col := nestedPos(x.node, r[1], r[2])
				o.add(x.node.Path, line, col, "%s is not a current digest of %s in %s. The next release runs the computation again with the current image", r[0], name, compute.ImagesFile)
			}
		}
		if checked {
			o.checked++
		}
	}
	return o
}

// c04 checks that the certificate of each computation names a known kind, a checker image that is the fixed checker of that kind, and a checker program that exists (rule C04).
func c04(a *auditor) outcome {
	var o outcome
	for _, x := range a.computed().records {
		if x.rec == nil || x.rec.Certificate == nil {
			continue
		}
		o.checked++
		n, cert := x.node, x.rec.Certificate
		ck, ok := compute.CheckerFor(cert.Kind)
		if !ok {
			var kinds []string
			for _, k := range compute.Checkers() {
				kinds = append(kinds, k.Kind)
			}
			line, col := nestedPos(n, "certificate", "kind")
			o.add(n.Path, line, col, "%q is not a certificate kind of spec 08.3. The kinds are %s", cert.Kind, strings.Join(kinds, ", "))
			continue
		}
		if !ck.Allows(cert.Checker, x.rec.Image) {
			line, col := nestedPos(n, "certificate", "checker")
			name, _, _ := compute.SplitRef(cert.Checker)
			if ck.Second && slices.Contains(ck.Images, name) {
				o.add(n.Path, line, col, "a %s certificate needs a checker in a second system, and the checker %s is the image of the computation", ck.Kind, name)
			} else {
				o.add(n.Path, line, col, "the checker %s is not a fixed checker of %s. The fixed checkers of %s are %s", name, ck.Kind, ck.Kind, strings.Join(ck.Images, ", "))
			}
		}
		line, col := nestedPos(n, "certificate", "checker_entry")
		if cert.CheckerEntry == "" {
			o.add(n.Path, line, col, "the certificate has no checker_entry. Name the checker program in %s/", x.rec.Dir())
		} else if fi, err := os.Stat(filepath.Join(a.root, filepath.FromSlash(x.rec.Dir()+"/"+cert.CheckerEntry))); err != nil || !fi.Mode().IsRegular() {
			o.add(n.Path, line, col, "the checker program %s/%s does not exist", x.rec.Dir(), cert.CheckerEntry)
		}
	}
	return o
}

// modelHosts are the hosts of model APIs. A computation log that names one of them shows a model call, or an attempt at one.
var modelHosts = []string{
	"api.anthropic.com",
	"api.openai.com",
	"generativelanguage.googleapis.com",
	"aiplatform.googleapis.com",
	"api.mistral.ai",
	"api.cohere.com",
	"api.cohere.ai",
	"api.together.xyz",
	"api.groq.com",
	"api.deepseek.com",
	"openrouter.ai",
	"api-inference.huggingface.co",
	"bedrock-runtime.",
	"openai.azure.com",
}

// c05 checks that each run in build/compute had no network, and that its logs name no model API (rule C05). The logs are the standard output, the standard error and the log of the checker.
func c05(a *auditor) outcome {
	var o outcome
	files, err := filepath.Glob(filepath.Join(a.root, computeDir, "*", "*", compute.RunFile))
	if err != nil {
		o.reason = err.Error()
		return o
	}
	for _, f := range files {
		rel := filepath.ToSlash(strings.TrimPrefix(f, a.root+string(filepath.Separator)))
		data, err := os.ReadFile(f) // #nosec G304 -- a file of the build output in the working tree.
		if err != nil {
			o.reason = err.Error()
			return o
		}
		o.checked++
		var run compute.Run
		if err := json.Unmarshal(data, &run); err != nil {
			o.add(rel, 1, 1, "the run record is not valid JSON: %v", err)
			continue
		}
		if run.Network != "none" {
			o.add(rel, 1, 1, "the run had the network %q. A computation runs with no network", cmpNone(run.Network))
		}
		dir := filepath.Dir(f)
		for _, log := range []string{compute.StdoutFile, compute.StderrFile, "check.log"} {
			if err := scanModelHosts(&o, filepath.Join(dir, log), filepath.ToSlash(filepath.Join(filepath.Dir(rel), log))); err != nil {
				o.reason = err.Error()
				return o
			}
		}
	}
	return o
}

// scanModelHosts adds a finding for the first line of the log at path that names a model API.
func scanModelHosts(o *outcome, path, rel string) error {
	data, err := os.ReadFile(path) // #nosec G304 -- a file of the build output in the working tree.
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), len(data)+1)
	for line := 1; sc.Scan(); line++ {
		text := strings.ToLower(sc.Text())
		for _, h := range modelHosts {
			if i := strings.Index(text, h); i >= 0 {
				o.add(rel, line, i+1, "the log names the model API %s. A computation must not call a model", strings.TrimSuffix(h, "."))
				return nil
			}
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	return nil
}
