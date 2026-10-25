// Package compute runs the computations of math2 (spec 08).
//
// A computation is a node of kind computation. Its front matter is the [Record]: the image by digest, the entry point next to the node in compute/TAG/, the inputs, the seed, the budgets, the hash of the output and, for a computation that can reach E4, a certificate. [Runner.Run] runs a record in a [sandbox.Sandbox] and writes what it saw to build/compute/TAG/ARCH/run.json. [Runner.Verify] also compares the output with the hash of the record, and runs the fixed checker of the certificate kind on the certificate.
//
// The images are in manifests/images.yaml ([Images]). A record names an image by the digest of one architecture, and the runner runs the digest of the same image for the architecture of the machine. Thus two runs on two architectures run the same image, and rule C02 compares their outputs.
package compute

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/node"
)

// Record is the record of one computation (spec 08.2).
type Record struct {
	Tag         string         // the tag or the provisional id of the node
	Path        string         // the path of the node file from the tree root
	Claim       string         // what the computation shows
	Role        string         // "experiment" for an experiment (spec 08.4), or ""
	Image       string         // the image as name@sha256:HEX
	Entry       string         // the entry point, as a path in compute/TAG/
	Inputs      map[string]any // the inputs, or nil
	Seed        *int64         // the seed, or nil for a deterministic computation
	Resources   Resources      // the budgets
	Output      *Output        // the recorded output, or nil before the first run
	Certificate *Certificate   // the certificate, or nil for a computation that stays E1
}

// Resources are the budgets of a computation. A zero value means the default of the sandbox.
type Resources struct {
	CPU    int
	Memory int64 // bytes
	Wall   time.Duration
}

// Output is the recorded output of a computation.
type Output struct {
	Hash    string // blake3:HEX of the standard output
	Summary string
}

// Certificate is the certificate of a computation (spec 08.3).
type Certificate struct {
	Kind         string // one of the kinds of Checkers
	Artefact     string // blake3:HEX of the certificate file
	Checker      string // the checker image as name@sha256:HEX
	CheckerEntry string // the checker program, as a path in compute/TAG/
}

// Dir returns the directory of the code of the computation, compute/TAG.
func (r *Record) Dir() string { return "compute/" + r.Tag }

// EntryPath returns the path of the entry point from the tree root.
func (r *Record) EntryPath() string { return r.Dir() + "/" + r.Entry }

// FromNode reads the record of a computation node. It returns an error when the node is not a computation or its record is not valid. The schema checks the form of the values, so the error names only the first problem.
func FromNode(n *node.Node) (*Record, error) {
	if n.Kind() != "computation" {
		return nil, fmt.Errorf("compute: %s is a %s, not a computation", n.ID, n.Kind())
	}
	r := &Record{Tag: n.ID, Path: n.Path, Claim: n.Get("claim"), Role: n.Get("role")}
	engine, ok := n.Meta["engine"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("compute: %s has no engine", n.ID)
	}
	r.Image, _ = engine["image"].(string)
	if _, _, ok := SplitRef(r.Image); !ok {
		return nil, fmt.Errorf("compute: %s names the image %q, and an image must be name@sha256:HEX", n.ID, r.Image)
	}
	entry, _ := engine["entry"].(string)
	var err error
	if r.Entry, err = codePath(r.Dir(), entry); err != nil {
		return nil, fmt.Errorf("compute: %s: the entry %v", n.ID, err)
	}
	if v, ok := engine["inputs"]; ok && v != nil {
		if r.Inputs, ok = v.(map[string]any); !ok {
			return nil, fmt.Errorf("compute: %s: the inputs are not a map", n.ID)
		}
	}
	if v, ok := engine["seed"].(int); ok {
		s := int64(v)
		r.Seed = &s
	}
	if res, ok := engine["resources"].(map[string]any); ok {
		if v, ok := res["cpu"].(int); ok {
			r.Resources.CPU = v
		}
		if v, ok := res["mem"]; ok {
			if r.Resources.Memory, err = ParseBytes(fmt.Sprint(v)); err != nil {
				return nil, fmt.Errorf("compute: %s: mem: %w", n.ID, err)
			}
		}
		if v, ok := res["wall"]; ok {
			s, _ := v.(string)
			if r.Resources.Wall, err = time.ParseDuration(s); err != nil || r.Resources.Wall <= 0 {
				return nil, fmt.Errorf("compute: %s: the wall time %v is not a duration such as 120s or 10m", n.ID, v)
			}
		}
	}
	if out, ok := n.Meta["output"].(map[string]any); ok {
		r.Output = &Output{}
		r.Output.Hash, _ = out["hash"].(string)
		r.Output.Summary, _ = out["summary"].(string)
	}
	if c, ok := n.Meta["certificate"].(map[string]any); ok {
		r.Certificate = &Certificate{}
		r.Certificate.Kind, _ = c["kind"].(string)
		r.Certificate.Artefact, _ = c["artefact"].(string)
		r.Certificate.Checker, _ = c["checker"].(string)
		entry, _ := c["checker_entry"].(string)
		if entry != "" {
			if r.Certificate.CheckerEntry, err = codePath(r.Dir(), entry); err != nil {
				return nil, fmt.Errorf("compute: %s: the checker entry %v", n.ID, err)
			}
		}
	}
	return r, nil
}

// codePath returns p as a path in dir. p is a path in dir, or a path from the tree root that starts with dir.
func codePath(dir, p string) (string, error) {
	p = strings.TrimPrefix(p, dir+"/")
	if p == "" || path.IsAbs(p) || path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "\\") {
		return "", fmt.Errorf("%q is not a path in %s/", p, dir)
	}
	return p, nil
}

var refRe = regexp.MustCompile(`^([a-z0-9][a-z0-9._-]*)@(sha256:[0-9a-f]{64})$`)

// SplitRef splits an image reference name@sha256:HEX into the name and the digest sha256:HEX.
func SplitRef(ref string) (name, digest string, ok bool) {
	m := refRe.FindStringSubmatch(ref)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

var bytesRe = regexp.MustCompile(`^([0-9]+)\s*([KMGT]?)(i?B?)$`)

// ParseBytes parses a size such as 512M, 4G or 1GiB. The suffixes K, M, G and T are powers of 1024.
func ParseBytes(s string) (int64, error) {
	m := bytesRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("%q is not a size such as 512M or 4G", s)
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, err
	}
	shift := strings.Index("KMGT", m[2])*10 + 10
	if m[2] == "" {
		shift = 0
	}
	if n <= 0 || n > (1<<62)>>shift {
		return 0, errors.New("the size is out of range")
	}
	return n << shift, nil
}
